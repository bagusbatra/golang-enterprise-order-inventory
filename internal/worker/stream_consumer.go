package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// EventHandler memproses satu Event. Error yang dikembalikan memicu retry
// (spec Section 47); panic di dalam handler di-recover oleh StreamConsumer
// sendiri dan diperlakukan sebagai kegagalan biasa (bukan meng-crash worker).
type EventHandler func(ctx context.Context, event Event) error

// ConsumerConfig mengonfigurasi satu consumer group Redis Streams.
type ConsumerConfig struct {
	Stream       string
	Group        string
	ConsumerName string // di-suffix "-<workerID>" oleh Pool agar unik per worker

	// BackoffSteps sesuai spec Section 47: 1s, 5s, 30s. Attempt ke-N (N>len)
	// memakai elemen terakhir.
	BackoffSteps []time.Duration
	MaxAttempts  int // spec Section 47: 3

	// MinIdleForReclaim: pesan yang masih PENDING (belum di-ACK) lebih lama
	// dari ini dianggap ditinggalkan oleh consumer yang crash, dan boleh
	// di-claim ulang consumer lain lewat XAUTOCLAIM (spec: verifikasi
	// XPENDING/XCLAIM setelah simulasi crash+restart).
	MinIdleForReclaim time.Duration
}

// DefaultBackoffSteps adalah backoff persis sesuai spec Section 47.
func DefaultBackoffSteps() []time.Duration {
	return []time.Duration{1 * time.Second, 5 * time.Second, 30 * time.Second}
}

// StreamConsumer menjalankan satu consumer group: baca pesan baru lewat
// XREADGROUP, retry dengan backoff saat handler gagal, ACK setelah sukses,
// dan pindahkan ke stream "<stream>_dead_letter" setelah MaxAttempts gagal.
// Juga meng-klaim ulang pesan yang tertinggal PENDING (consumer sebelumnya
// crash) lewat XAUTOCLAIM.
type StreamConsumer struct {
	client  *redis.Client
	logger  *zap.Logger
	cfg     ConsumerConfig
	handler EventHandler
}

func NewStreamConsumer(client *redis.Client, logger *zap.Logger, cfg ConsumerConfig, handler EventHandler) *StreamConsumer {
	if len(cfg.BackoffSteps) == 0 {
		cfg.BackoffSteps = DefaultBackoffSteps()
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.MinIdleForReclaim <= 0 {
		cfg.MinIdleForReclaim = 30 * time.Second
	}
	return &StreamConsumer{client: client, logger: logger, cfg: cfg, handler: handler}
}

// Run adalah signature yang cocok dipakai langsung sebagai fungsi `run`
// untuk Pool.Start (lihat pool.go) — setiap worker goroutine memakai nama
// consumer unik "<ConsumerName>-<workerID>" supaya Redis bisa membedakan
// pemilik message pending per worker.
func (c *StreamConsumer) Run(ctx context.Context, workerID int) {
	consumerName := fmt.Sprintf("%s-%d", c.cfg.ConsumerName, workerID)

	if err := EnsureConsumerGroup(ctx, c.client, c.cfg.Stream, c.cfg.Group); err != nil {
		c.logger.Error("failed to ensure consumer group", zap.String("stream", c.cfg.Stream), zap.Error(err))
		return
	}

	reclaimTicker := time.NewTicker(c.cfg.MinIdleForReclaim)
	defer reclaimTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-reclaimTicker.C:
			c.reclaimStale(ctx, consumerName)
		default:
		}

		streams, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.cfg.Group,
			Consumer: consumerName,
			Streams:  []string{c.cfg.Stream, ">"},
			Count:    10,
			Block:    2 * time.Second,
		}).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) || ctx.Err() != nil {
				continue
			}
			c.logger.Error("XREADGROUP failed", zap.String("stream", c.cfg.Stream), zap.Error(err))
			select {
			case <-time.After(time.Second):
			case <-ctx.Done():
				return
			}
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				c.processMessage(ctx, msg)
			}
		}
	}
}

// processMessage menjalankan handler dengan retry+backoff (spec Section
// 47). ACK hanya terjadi setelah SUKSES atau setelah pesan dipindah ke dead
// letter — jika ctx di-cancel di tengah backoff (graceful shutdown), pesan
// dibiarkan PENDING supaya worker/instance lain bisa meng-klaim & melanjutkan
// (spec: "jika worker mati sebelum ACK, message harus bisa diproses ulang").
func (c *StreamConsumer) processMessage(ctx context.Context, msg redis.XMessage) {
	event, err := parseEvent(msg)
	if err != nil {
		c.logger.Error("failed to parse event, sending straight to dead letter",
			zap.String("message_id", msg.ID), zap.Error(err))
		c.sendToDeadLetter(ctx, msg, "unparseable event: "+err.Error())
		c.ack(ctx, msg.ID)
		return
	}

	var lastErr error
	for attempt := 1; attempt <= c.cfg.MaxAttempts; attempt++ {
		lastErr = c.invokeHandler(ctx, event)
		if lastErr == nil {
			c.ack(ctx, msg.ID)
			return
		}

		c.logger.Warn("event handler failed",
			zap.String("stream", c.cfg.Stream),
			zap.String("event_type", event.EventType),
			zap.Int("attempt", attempt),
			zap.Error(lastErr))

		if attempt < c.cfg.MaxAttempts {
			backoff := c.cfg.BackoffSteps[min(attempt-1, len(c.cfg.BackoffSteps)-1)]
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return // biarkan PENDING, jangan ACK — akan di-reclaim setelah restart.
			}
		}
	}

	c.logger.Error("event exhausted all retry attempts, moving to dead letter",
		zap.String("stream", c.cfg.Stream), zap.String("event_type", event.EventType), zap.Error(lastErr))
	c.sendToDeadLetter(ctx, msg, lastErr.Error())
	c.ack(ctx, msg.ID)
}

func (c *StreamConsumer) invokeHandler(ctx context.Context, event Event) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic in event handler: %v", r)
		}
	}()
	return c.handler(ctx, event)
}

func (c *StreamConsumer) ack(ctx context.Context, id string) {
	if err := c.client.XAck(ctx, c.cfg.Stream, c.cfg.Group, id).Err(); err != nil {
		c.logger.Error("XACK failed", zap.String("message_id", id), zap.Error(err))
	}
}

// sendToDeadLetter menyimpan pesan asli (semua field + alasan gagal) ke
// stream terpisah "<stream>_dead_letter" — pilihan yang didokumentasikan
// (03-BACKEND-CORE.md Iterasi 07 memperbolehkan stream terpisah ATAU tabel;
// stream terpisah dipilih supaya tetap konsisten memakai Redis Streams,
// tanpa migration/tabel baru).
func (c *StreamConsumer) sendToDeadLetter(ctx context.Context, msg redis.XMessage, reason string) {
	deadStream := c.cfg.Stream + "_dead_letter"
	values := make(map[string]any, len(msg.Values)+2)
	for k, v := range msg.Values {
		values[k] = v
	}
	values["dead_letter_reason"] = reason
	values["original_message_id"] = msg.ID

	if err := c.client.XAdd(ctx, &redis.XAddArgs{Stream: deadStream, Values: values}).Err(); err != nil {
		c.logger.Error("failed to write to dead letter stream", zap.String("dead_letter_stream", deadStream), zap.Error(err))
	}
}

// reclaimStale meng-klaim ulang pesan yang PENDING lebih lama dari
// MinIdleForReclaim (ditinggalkan consumer yang crash sebelum ACK) lewat
// XAUTOCLAIM, lalu memprosesnya lewat jalur retry yang sama seperti pesan
// baru.
func (c *StreamConsumer) reclaimStale(ctx context.Context, consumerName string) {
	messages, _, err := c.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   c.cfg.Stream,
		Group:    c.cfg.Group,
		Consumer: consumerName,
		MinIdle:  c.cfg.MinIdleForReclaim,
		Start:    "0-0",
		Count:    10,
	}).Result()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			c.logger.Warn("XAUTOCLAIM failed", zap.String("stream", c.cfg.Stream), zap.Error(err))
		}
		return
	}
	for _, msg := range messages {
		c.logger.Info("reclaimed stale pending message from a crashed consumer",
			zap.String("stream", c.cfg.Stream), zap.String("message_id", msg.ID))
		c.processMessage(ctx, msg)
	}
}
