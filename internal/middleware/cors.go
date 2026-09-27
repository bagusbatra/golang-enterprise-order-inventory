package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS mengizinkan akses lintas origin untuk API ini. Karena project tidak
// memiliki frontend tersendiri (backend-only technical test, dikonsumsi
// lewat Swagger UI/Postman/klien eksternal), origin diizinkan secara
// permisif; batasan keamanan sesungguhnya tetap dijaga oleh JWT+RBAC,
// bukan oleh CORS.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
