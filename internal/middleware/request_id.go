package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestIDKey adalah context key tempat request ID disimpan agar middleware
// lain (Logger, Recovery) dan handler bisa mengambilnya kembali.
const RequestIDKey = "request_id"

const requestIDHeader = "X-Request-ID"

// RequestID memastikan setiap request punya ID (spec Section 55). Jika
// client mengirim header X-Request-ID, dipakai apa adanya; jika tidak,
// server generate UUID baru. ID ini WAJIB muncul di setiap log request.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(requestIDHeader)
		if id == "" {
			id = uuid.NewString()
		}
		c.Set(RequestIDKey, id)
		c.Header(requestIDHeader, id)
		c.Next()
	}
}

// GetRequestID mengambil request ID dari context, dipakai logger di layer
// lain (service/repository) yang ingin ikut menyertakan request_id di log.
func GetRequestID(c *gin.Context) string {
	if v, ok := c.Get(RequestIDKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
