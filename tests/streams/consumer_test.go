// Package streams membuktikan validasi Worker Test spec Section 74 /
// 03-BACKEND-CORE.md Iterasi 07: job gagal -> tidak ACK -> retry dengan
// backoff -> setelah 3x gagal -> dead letter; DAN skenario crash — worker
// mati sebelum ACK, message tetap PENDING dan bisa di-claim ulang worker
// lain lewat XPENDING/XCLAIM (di sini: XAUTOCLAIM).
//
// WAJIB real Redis (Testcontainers) — dukungan Streams/consumer-group di
// miniredis tidak lengkap untuk XAUTOCLAIM.
package streams

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"order-management/internal/worker"
	"order-management/tests/testutil"
)

func TestStreamConsumer_RetriesThenSucceeds(t *testing.T) {
	client := testutil.SetupRedis(t)
	ctx := context.Background()

	stream := "test_retry_events"
	group := "test-retry-group"
	// Consumer group WAJIB dibuat SEBELUM publish — XGROUP CREATE "$" hanya
	// mengantarkan message yang ditambahkan SETELAH group dibuat (perilaku
	// Redis Streams asli, bukan bug StreamConsumer).
	if err := worker.EnsureConsumerGroup(ctx, client, stream, group); err != nil {
		t.Fatalf("ensure consumer group failed: %v", err)
	}
	if _, err := worker.PublishEvent(ctx, client, stream, "TEST_EVENT", "agg-1", map[string]string{"foo": "bar"}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	var attempts int32
	handlerDone := make(chan struct{})
	handler := func(ctx context.Context, event worker.Event) error {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			return errors.New("simulated failure")
		}
		close(handlerDone)
		return nil
	}

	consumer := worker.NewStreamConsumer(client, zap.NewNop(), worker.ConsumerConfig{
		Stream:            stream,
		Group:             "test-retry-group",
		ConsumerName:      "retry-consumer",
		BackoffSteps:      []time.Duration{50 * time.Millisecond, 50 * time.Millisecond, 50 * time.Millisecond},
		MaxAttempts:       3,
		MinIdleForReclaim: time.Hour, // matikan reclaim untuk test ini
	}, handler)

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go consumer.Run(runCtx, 0)

	select {
	case <-handlerDone:
	case <-time.After(10 * time.Second):
		t.Fatal("handler never succeeded within timeout")
	}

	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Fatalf("expected exactly 3 attempts (fail, fail, succeed), got %d", got)
	}

	// Beri waktu singkat untuk XACK benar-benar tereksekusi setelah handler return.
	time.Sleep(200 * time.Millisecond)
	pending, err := client.XPending(ctx, stream, "test-retry-group").Result()
	if err != nil {
		t.Fatalf("XPENDING failed: %v", err)
	}
	if pending.Count != 0 {
		t.Fatalf("expected 0 pending messages after successful ACK, got %d", pending.Count)
	}
}

func TestStreamConsumer_ExhaustsRetries_MovesToDeadLetter(t *testing.T) {
	client := testutil.SetupRedis(t)
	ctx := context.Background()

	stream := "test_deadletter_events"
	group := "test-deadletter-group"
	if err := worker.EnsureConsumerGroup(ctx, client, stream, group); err != nil {
		t.Fatalf("ensure consumer group failed: %v", err)
	}
	if _, err := worker.PublishEvent(ctx, client, stream, "TEST_EVENT", "agg-1", map[string]string{"foo": "bar"}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	var attempts int32
	allFailed := make(chan struct{})
	handler := func(ctx context.Context, event worker.Event) error {
		n := atomic.AddInt32(&attempts, 1)
		if n == 3 {
			defer close(allFailed)
		}
		return errors.New("always fails")
	}

	consumer := worker.NewStreamConsumer(client, zap.NewNop(), worker.ConsumerConfig{
		Stream:            stream,
		Group:             "test-deadletter-group",
		ConsumerName:      "deadletter-consumer",
		BackoffSteps:      []time.Duration{50 * time.Millisecond, 50 * time.Millisecond, 50 * time.Millisecond},
		MaxAttempts:       3,
		MinIdleForReclaim: time.Hour,
	}, handler)

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go consumer.Run(runCtx, 0)

	select {
	case <-allFailed:
	case <-time.After(10 * time.Second):
		t.Fatal("handler never reached 3rd attempt within timeout")
	}

	// Beri waktu singkat untuk dead-letter write + ACK benar-benar tereksekusi.
	time.Sleep(300 * time.Millisecond)

	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Fatalf("expected EXACTLY 3 attempts (spec: max 3), got %d", got)
	}

	pending, err := client.XPending(ctx, stream, "test-deadletter-group").Result()
	if err != nil {
		t.Fatalf("XPENDING failed: %v", err)
	}
	if pending.Count != 0 {
		t.Fatalf("expected original message to be ACKed (removed from pending) after moving to dead letter, got %d still pending", pending.Count)
	}

	deadLetterStream := stream + "_dead_letter"
	entries, err := client.XRange(ctx, deadLetterStream, "-", "+").Result()
	if err != nil {
		t.Fatalf("failed to read dead letter stream: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 entry in dead letter stream, got %d", len(entries))
	}
	if entries[0].Values["dead_letter_reason"] == nil {
		t.Fatal("expected dead_letter_reason field to be set")
	}
}

// TestStreamConsumer_CrashRecovery_ReclaimsViaXAutoClaim mensimulasikan
// worker yang mati SETELAH menerima message (XREADGROUP) tapi SEBELUM
// memprosesnya/ACK — message harus tetap PENDING dan berhasil di-claim
// ulang & diproses oleh consumer lain lewat XAUTOCLAIM (spec: "verifikasi
// dengan test: simulasikan crash, restart consumer, pastikan pending
// message di-claim ulang via XPENDING/XCLAIM").
func TestStreamConsumer_CrashRecovery_ReclaimsViaXAutoClaim(t *testing.T) {
	client := testutil.SetupRedis(t)
	ctx := context.Background()

	stream := "test_crash_events"
	group := "test-crash-group"

	if err := worker.EnsureConsumerGroup(ctx, client, stream, group); err != nil {
		t.Fatalf("ensure consumer group failed: %v", err)
	}
	if _, err := worker.PublishEvent(ctx, client, stream, "TEST_EVENT", "agg-1", map[string]string{"foo": "bar"}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	// Simulasikan consumer yang crash: baca message (masuk PENDING milik
	// consumer ini) lalu TIDAK PERNAH ACK, TIDAK PERNAH retry — proses
	// mati begitu saja, seolah-olah worker crash tepat setelah menerima job.
	crashedConsumer := "crashed-consumer"
	_, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: crashedConsumer,
		Streams:  []string{stream, ">"},
		Count:    1,
		Block:    2 * time.Second,
	}).Result()
	if err != nil {
		t.Fatalf("simulated crash-read failed: %v", err)
	}

	pendingBefore, err := client.XPending(ctx, stream, group).Result()
	if err != nil {
		t.Fatalf("XPENDING failed: %v", err)
	}
	if pendingBefore.Count != 1 {
		t.Fatalf("expected 1 pending message held by the crashed consumer, got %d", pendingBefore.Count)
	}

	// Consumer "survivor" dengan MinIdleForReclaim sangat kecil supaya
	// reclaim langsung terjadi di test ini tanpa menunggu lama.
	var processed int32
	done := make(chan struct{})
	handler := func(ctx context.Context, event worker.Event) error {
		atomic.AddInt32(&processed, 1)
		close(done)
		return nil
	}

	survivor := worker.NewStreamConsumer(client, zap.NewNop(), worker.ConsumerConfig{
		Stream:            stream,
		Group:             group,
		ConsumerName:      "survivor",
		BackoffSteps:      []time.Duration{50 * time.Millisecond},
		MaxAttempts:       3,
		MinIdleForReclaim: 500 * time.Millisecond,
	}, handler)

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go survivor.Run(runCtx, 0)

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("message left pending by the crashed consumer was never reclaimed and processed")
	}

	if atomic.LoadInt32(&processed) != 1 {
		t.Fatal("expected the reclaimed message to be processed exactly once")
	}

	time.Sleep(200 * time.Millisecond)
	pendingAfter, err := client.XPending(ctx, stream, group).Result()
	if err != nil {
		t.Fatalf("XPENDING failed: %v", err)
	}
	if pendingAfter.Count != 0 {
		t.Fatalf("expected 0 pending messages after successful reclaim+ACK, got %d", pendingAfter.Count)
	}
}
