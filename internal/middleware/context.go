package middleware

import "github.com/gin-gonic/gin"

// Context key untuk claims JWT yang sudah divalidasi (diisi middleware JWT
// di Iterasi 03). Didefinisikan di sini (bukan di jwt.go) supaya Logger
// middleware bisa memakainya tanpa membuat import cycle.
const (
	UserIDKey = "user_id"
	RoleKey   = "role"
)

// GetUserID mengambil user_id dari context. Mengembalikan "" jika request
// belum lolos middleware JWT (misal endpoint publik).
func GetUserID(c *gin.Context) string {
	if v, ok := c.Get(UserIDKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetRole mengambil role dari context, dipakai middleware RBAC dan service
// layer untuk ownership check (misal CUSTOMER hanya boleh lihat order sendiri).
func GetRole(c *gin.Context) string {
	if v, ok := c.Get(RoleKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
