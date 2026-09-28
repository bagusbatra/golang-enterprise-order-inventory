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

// list godoc
// @Summary List audit log (ADMIN only)
// @Tags Audit
// @Security BearerAuth
// @Produce json
// @Param page query int false "default 1"
// @Param limit query int false "default 20, max 100"
// @Param entity query string false "filter"
// @Param user_id query string false "filter"
// @Success 200 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{} "forbidden"
// @Router /audit-logs [get]
func (h *Handler) list(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	page, limit = response.NormalizePagination(page, limit)

	items, total, err := h.service.List(c.Request.Context(), c.Query("entity"), c.Query("user_id"), page, limit)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.List(c, "Audit logs retrieved successfully", items, response.NewMeta(page, limit, total))
}

// get godoc
// @Summary Detail audit log (ADMIN only)
// @Tags Audit
// @Security BearerAuth
// @Produce json
// @Param id path string true "Audit Log ID"
// @Success 200 {object} Response
// @Failure 404 {object} map[string]interface{} "not found"
// @Router /audit-logs/{id} [get]
func (h *Handler) get(c *gin.Context) {
	item, err := h.service.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Audit log retrieved successfully", item)
}
