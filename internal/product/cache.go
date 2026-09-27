package product

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// cache mengimplementasikan cache-aside untuk product detail (spec Section
// 27): GET dulu cek Redis, miss -> ambil dari Postgres -> isi Redis TTL
// 5-15 menit. Update product -> Invalidate (hapus key), BUKAN update cache
// langsung, supaya tidak ada risiko cache menyimpan data stale jika update
// gagal di tengah jalan.
type cache struct {
	client *redis.Client
	ttl    time.Duration
}

func newCache(client *redis.Client, ttl time.Duration) *cache {
	return &cache{client: client, ttl: ttl}
}

func cacheKey(id string) string {
	return fmt.Sprintf("product:detail:%s", id)
}

func (c *cache) Get(ctx context.Context, id string) (*Product, bool) {
	if c.client == nil {
		return nil, false
	}
	val, err := c.client.Get(ctx, cacheKey(id)).Result()
	if err != nil {
		return nil, false
	}
	var p Product
	if err := json.Unmarshal([]byte(val), &p); err != nil {
		return nil, false
	}
	return &p, true
}

func (c *cache) Set(ctx context.Context, p *Product) {
	if c.client == nil {
		return
	}
	data, err := json.Marshal(p)
	if err != nil {
		return
	}
	// Kegagalan set cache tidak boleh menggagalkan request (cache hanya
	// optimisasi, PostgreSQL tetap source of truth) — error diabaikan
	// dengan sengaja di sini.
	_ = c.client.Set(ctx, cacheKey(p.ID), data, c.ttl).Err()
}

func (c *cache) Invalidate(ctx context.Context, id string) {
	if c.client == nil {
		return
	}
	_ = c.client.Del(ctx, cacheKey(id)).Err()
}
