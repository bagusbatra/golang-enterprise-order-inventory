package worker

import (
	"context"
	"strings"

	"github.com/redis/go-redis/v9"
)

// Produce menambahkan satu event ke Redis Stream. Dipanggil SETELAH transaksi
// database commit (spec Section 46) — kegagalan publish tidak boleh
// membatalkan perubahan data yang sudah commit, jadi caller memutuskan
// sendiri bagaimana menangani error ini (log & lanjut, bukan rollback).
func Produce(ctx context.Context, client *redis.Client, stream string, fields map[string]interface{}) error {
	return client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: fields,
	}).Err()
}

// EnsureConsumerGroup membuat consumer group pada stream jika belum ada.
// Idempotent: error BUSYGROUP (group sudah ada) diabaikan supaya aman
// dipanggil setiap kali worker start (Iterasi 07).
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
