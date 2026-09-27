package worker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Event adalah format pesan yang dipublish ke Redis Streams, sesuai spec
// Section 45 persis.
type Event struct {
	EventID     string          `json:"event_id"`
	EventType   string          `json:"event_type"`
	AggregateID string          `json:"aggregate_id"`
	Payload     json.RawMessage `json:"payload"`
	CreatedAt   time.Time       `json:"created_at"`
}

// PublishEvent membungkus payload jadi Event lalu XADD ke stream, disimpan
// sebagai satu field JSON ("data") supaya struktur nested payload tidak
// hilang saat di-flatten jadi field Redis Stream (yang aslinya cuma
// key-value string datar). Dipanggil SETELAH transaksi database commit
// (architecture.md Section 3) — kegagalan publish tidak boleh membatalkan
// perubahan data yang sudah commit, jadi caller memutuskan sendiri
// bagaimana menangani error ini (log & lanjut, bukan rollback).
func PublishEvent(ctx context.Context, client *redis.Client, stream, eventType, aggregateID string, payload any) (string, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	ev := Event{
		EventID:     uuid.NewString(),
		EventType:   eventType,
		AggregateID: aggregateID,
		Payload:     payloadBytes,
		CreatedAt:   time.Now(),
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return "", err
	}

	return client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]any{"data": string(data)},
	}).Result()
}

// parseEvent mengambil field "data" dari satu Redis Stream message dan
// mem-parse-nya balik jadi Event.
func parseEvent(msg redis.XMessage) (Event, error) {
	raw, ok := msg.Values["data"]
	if !ok {
		return Event{}, errors.New("worker: message missing 'data' field")
	}
	str, ok := raw.(string)
	if !ok {
		return Event{}, errors.New("worker: 'data' field is not a string")
	}
	var ev Event
	if err := json.Unmarshal([]byte(str), &ev); err != nil {
		return Event{}, err
	}
	return ev, nil
}

// EnsureConsumerGroup membuat consumer group pada stream jika belum ada.
// Idempotent: error BUSYGROUP (group sudah ada) diabaikan supaya aman
// dipanggil setiap kali worker start.
func EnsureConsumerGroup(ctx context.Context, client *redis.Client, stream, group string) error {
	err := client.XGroupCreateMkStream(ctx, stream, group, "$").Err()
	if err != nil && !isBusyGroupErr(err) {
		return err
	}
	return nil
}

func isBusyGroupErr(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "BUSYGROUP")
}
