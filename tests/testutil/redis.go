package testutil

import (
	"context"
	"fmt"
	"testing"

	"github.com/redis/go-redis/v9"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

// SetupRedis membuat container Redis nyata (bukan miniredis) — dibutuhkan
// untuk memvalidasi Redis Streams consumer group/XAUTOCLAIM sungguhan
// (spec Iterasi 07), yang dukungannya di miniredis tidak lengkap.
func SetupRedis(t *testing.T) *redis.Client {
	t.Helper()
	ctx := context.Background()

	container, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("failed to start redis container (Docker required for this test): %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Logf("failed to terminate redis container: %v", err)
		}
	})

	// Paksa 127.0.0.1 secara eksplisit, JANGAN pakai ConnectionString()
	// bawaan module (memakai hostname "localhost") — root cause yang
	// ditemukan lewat testing nyata: Go men-dial IPv6 (::1) lebih dulu
	// untuk "localhost" (RFC 6555 Happy Eyeballs), dan di environment ini
	// loopback IPv6 blackhole/rusak, sehingga setiap koneksi baru via
	// hostname menunggu penuh sampai timeout. Lihat testutil/postgres.go
	// untuk root cause yang identik.
	port, err := container.MappedPort(ctx, "6379/tcp")
	if err != nil {
		t.Fatalf("failed to get mapped port: %v", err)
	}

	client := redis.NewClient(&redis.Options{Addr: fmt.Sprintf("127.0.0.1:%s", port.Port())})
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("failed to ping redis: %v", err)
	}
	return client
}
