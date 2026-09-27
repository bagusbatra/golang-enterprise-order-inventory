package notification

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"order-management/internal/middleware"
	"order-management/pkg/response"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes — GET/PATCH di sini tidak butuh RBAC tambahan selain JWT
// (semua role boleh melihat/menandai notifikasi MILIK SENDIRI, ownership
// dicek di service, bukan lewat role whitelist).
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.list)
	rg.PATCH("/:id/read", h.markRead)
	rg.PATCH("/read-all", h.markAllRead)
}

func (h *Handler) list(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))

	var isRead *bool
	if v := c.Query("is_read"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			isRead = &b
		}
	}

	userID := middleware.GetUserID(c)
	items, total, err := h.service.List(c.Request.Context(), userID, isRead, page, limit)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.List(c, "Notifications retrieved successfully", items, response.NewMeta(page, limit, total))
}

func (h *Handler) markRead(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if err := h.service.MarkRead(c.Request.Context(), userID, c.Param("id")); err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *Handler) markAllRead(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if err := h.service.MarkAllRead(c.Request.Context(), userID); err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
