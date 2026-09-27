package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	apperr "order-management/pkg/errors"
	"order-management/pkg/response"
)

// Recovery menangkap panic di handler mana pun agar satu request yang panic
// tidak menjatuhkan seluruh server (spec Section 54). Panic dicatat lengkap
// dengan request_id untuk investigasi, tapi client hanya menerima response
// 500 generik — detail internal tidak pernah dibocorkan ke client.
func Recovery(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Error("panic recovered",
					zap.String("request_id", GetRequestID(c)),
					zap.String("path", c.Request.URL.Path),
					zap.Any("panic", r),
				)
				response.Error(c, apperr.New(http.StatusInternalServerError, apperr.InternalError, "Internal server error"))
				c.Abort()
			}
		}()
		c.Next()
	}
}
