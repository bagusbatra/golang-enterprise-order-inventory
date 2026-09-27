package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Logger mencatat setiap request selesai dengan format terstruktur sesuai
// spec Section 56: request_id, method, path, status, duration_ms, dan
// user_id jika sudah ada (diisi middleware JWT sebelum handler berjalan).
// TIDAK PERNAH mencatat body request/response di sini — body bisa memuat
// password/token, jadi sengaja tidak dilog secara default.
func Logger(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		duration := time.Since(start)
		fields := []zap.Field{
			zap.String("request_id", GetRequestID(c)),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Int64("duration_ms", duration.Milliseconds()),
		}
		if userID := GetUserID(c); userID != "" {
			fields = append(fields, zap.String("user_id", userID))
		}

		if len(c.Errors) > 0 {
			log.Error("request completed with error", append(fields, zap.String("error", c.Errors.String()))...)
			return
		}
		log.Info("request completed", fields...)
	}
}
