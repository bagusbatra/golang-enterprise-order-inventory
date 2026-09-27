package middleware

import (
	"github.com/gin-gonic/gin"

	apperr "order-management/pkg/errors"
	"order-management/pkg/response"
)

// RequireRole membatasi route hanya untuk role yang disebutkan (spec Section
// 4 — role permission table). WAJIB dipasang SETELAH JWTAuth (butuh RoleKey
// sudah terisi di context). Role di luar daftar -> 403 AUTH_FORBIDDEN.
func RequireRole(allowed ...string) gin.HandlerFunc {
	allowedSet := make(map[string]bool, len(allowed))
	for _, r := range allowed {
		allowedSet[r] = true
	}

	return func(c *gin.Context) {
		role := GetRole(c)
		if !allowedSet[role] {
			response.Error(c, apperr.ForbiddenErr("You do not have permission to access this resource"))
			c.Abort()
			return
		}
		c.Next()
	}
}
