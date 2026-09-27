package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	apperr "order-management/pkg/errors"
	"order-management/pkg/jwt"
	"order-management/pkg/response"
)

// JWTAuth memvalidasi access token dari header "Authorization: Bearer <token>"
// dan menyuntikkan user_id + role ke context untuk dipakai handler/service
// berikutnya (ownership check, RBAC, logging). Endpoint publik (register,
// login, refresh, health, swagger, payments/callback) TIDAK memakai
// middleware ini sama sekali — bukan "izinkan tanpa token", tapi memang
// tidak didaftarkan di route group yang pakai middleware ini.
func JWTAuth(manager *jwt.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			response.Error(c, apperr.UnauthorizedErr("Missing or invalid Authorization header"))
			c.Abort()
			return
		}

		tokenStr := strings.TrimPrefix(header, "Bearer ")
		claims, err := manager.ParseAccessToken(tokenStr)
		if err != nil {
			if err == jwt.ErrExpiredToken {
				response.Error(c, apperr.New(http.StatusUnauthorized, apperr.AuthTokenExpired, "Access token expired"))
			} else {
				response.Error(c, apperr.UnauthorizedErr("Invalid access token"))
			}
			c.Abort()
			return
		}

		c.Set(UserIDKey, claims.UserID)
		c.Set(RoleKey, claims.Role)
		c.Next()
	}
}
