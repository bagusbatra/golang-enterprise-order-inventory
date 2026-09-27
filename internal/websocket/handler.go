package websocket

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"order-management/pkg/jwt"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Technical test ini tidak punya daftar origin frontend resmi untuk
	// di-whitelist — menerima semua origin (bukan endpoint yang membawa
	// cookie/session, otentikasi lewat token di query param).
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Handler menangani GET /ws (spec Section 50). JWT divalidasi manual dari
// query param `?token=` (Locked Decision 03-BACKEND-CORE.md Section 3:
// "bukan middleware Gin biasa karena ini upgrade connection") — BUKAN lewat
// middleware.JWTAuth yang membaca header Authorization.
type Handler struct {
	manager    *Manager
	jwtManager *jwt.Manager
	logger     *zap.Logger
}

func NewHandler(manager *Manager, jwtManager *jwt.Manager, logger *zap.Logger) *Handler {
	return &Handler{manager: manager, jwtManager: jwtManager, logger: logger}
}

func (h *Handler) RegisterRoutes(r gin.IRoutes) {
	r.GET("/ws", h.serveWS)
}

func (h *Handler) serveWS(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	claims, err := h.jwtManager.ParseAccessToken(token)
	if err != nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Warn("websocket upgrade failed", zap.Error(err))
		return
	}

	h.manager.Register(claims.UserID, conn)
	defer func() {
		h.manager.Unregister(claims.UserID, conn)
		_ = conn.Close()
	}()

	// Read loop murni untuk mendeteksi disconnect — server hanya PUSH event
	// (ORDER_STATUS_UPDATED dari Order/Payment/Shipment Service), tidak
	// menerima command apa pun dari client lewat koneksi ini.
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}
