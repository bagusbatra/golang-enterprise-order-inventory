// Package redis adalah stand-in minimal untuk pkg/redis milik Agent 2
// (lihat 02-BACKEND-FOUNDATION.md Iterasi 02). Dibuat oleh Agent 3 agar
// internal/worker (Redis Streams) punya klien untuk dikonsumsi tanpa
// menunggu sesi Agent 2. Agent 2 WAJIB meninjau/mengganti sesuai kebutuhan
// penuh mereka (cache-aside, rate-limit, dll — lihat docs/STATUS.md).
package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type Config struct {
	Host     string
	Port     string
	Password string
}

// Connect membuka koneksi ke Redis dan memverifikasinya dengan PING.
func Connect(ctx context.Context, cfg Config) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Password: cfg.Password,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis: ping gagal: %w", err)
	}
	return client, nil
}
