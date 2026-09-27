package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	apperr "order-management/pkg/errors"
	"order-management/pkg/response"
)

// RateLimit membatasi jumlah request per IP per menit menggunakan fixed
// window counter di Redis (spec Section 57): login 5/menit, API umum
// 100/menit. Jika Redis error/down, middleware FAIL-OPEN (request tetap
// diproses, hanya dicatat sebagai warning) — Redis bukan source of truth
// untuk business data, jadi tidak boleh Redis mati sampai memblokir
// seluruh API (lihat docs/architecture.md Section 5).
func RateLimit(client *redis.Client, log *zap.Logger, keyPrefix string, limit int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		key := fmt.Sprintf("ratelimit:%s:%s", keyPrefix, ip)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 500*time.Millisecond)
		defer cancel()

		count, err := client.Incr(ctx, key).Result()
		if err != nil {
			log.Warn("rate limiter: redis error, fail-open", zap.Error(err))
			c.Next()
			return
		}
		if count == 1 {
			client.Expire(ctx, key, window)
		}

		if count > int64(limit) {
			response.Error(c, apperr.New(http.StatusTooManyRequests, apperr.Code("RATE_LIMIT_EXCEEDED"), "Too many requests, please try again later"))
			c.Abort()
			return
		}
		c.Next()
	}
}
