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

// list godoc
// @Summary List notifikasi milik sendiri
// @Tags Notifications
// @Security BearerAuth
// @Produce json
// @Param page query int false "default 1"
// @Param limit query int false "default 20, max 100"
// @Param is_read query bool false "filter"
// @Success 200 {object} map[string]interface{}
// @Router /notifications [get]
func (h *Handler) list(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	page, limit = response.NormalizePagination(page, limit)

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

// markRead godoc
// @Summary Tandai satu notifikasi sudah dibaca (owner only)
// @Tags Notifications
// @Security BearerAuth
// @Produce json
// @Param id path string true "Notification ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{} "not found"
// @Router /notifications/{id}/read [patch]
func (h *Handler) markRead(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if err := h.service.MarkRead(c.Request.Context(), userID, c.Param("id")); err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// markAllRead godoc
// @Summary Tandai semua notifikasi sudah dibaca
// @Tags Notifications
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /notifications/read-all [patch]
func (h *Handler) markAllRead(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if err := h.service.MarkAllRead(c.Request.Context(), userID); err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
