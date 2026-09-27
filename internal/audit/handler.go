package audit

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"order-management/pkg/response"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes — ADMIN only, di-enforce lewat middleware saat wiring di
// main.go (konsisten dengan pola domain lain), bukan di sini.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.list)
	rg.GET("/:id", h.get)
}

func (h *Handler) list(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))

	items, total, err := h.service.List(c.Request.Context(), c.Query("entity"), c.Query("user_id"), page, limit)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.List(c, "Audit logs retrieved successfully", items, response.NewMeta(page, limit, total))
}

func (h *Handler) get(c *gin.Context) {
	item, err := h.service.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Audit log retrieved successfully", item)
}
