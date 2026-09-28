package warehouse

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	apperr "order-management/pkg/errors"
	"order-management/pkg/response"
	"order-management/pkg/validator"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, adminOnly gin.HandlerFunc) {
	rg.GET("", h.list)
	rg.GET("/:id", h.get)
	rg.POST("", adminOnly, h.create)
	rg.PUT("/:id", adminOnly, h.update)
	rg.DELETE("/:id", adminOnly, h.delete)
}

// list godoc
// @Summary List warehouse
// @Tags Warehouses
// @Security BearerAuth
// @Produce json
// @Param page query int false "default 1"
// @Param limit query int false "default 20, max 100"
// @Param status query string false "ACTIVE|INACTIVE"
// @Success 200 {object} map[string]interface{}
// @Router /warehouses [get]
func (h *Handler) list(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	page, limit = response.NormalizePagination(page, limit)

	items, total, err := h.service.List(c.Request.Context(), c.Query("status"), page, limit)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.List(c, "Warehouses retrieved successfully", items, response.NewMeta(page, limit, total))
}

// get godoc
// @Summary Detail warehouse
// @Tags Warehouses
// @Security BearerAuth
// @Produce json
// @Param id path string true "Warehouse ID"
// @Success 200 {object} Response
// @Failure 404 {object} map[string]interface{} "not found"
// @Router /warehouses/{id} [get]
func (h *Handler) get(c *gin.Context) {
	item, err := h.service.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Warehouse retrieved successfully", item)
}

// create godoc
// @Summary Buat warehouse baru (ADMIN only)
// @Tags Warehouses
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body CreateRequest true "Data warehouse"
// @Success 201 {object} Response
// @Failure 409 {object} map[string]interface{} "code already exists"
// @Router /warehouses [post]
func (h *Handler) create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.NewValidation("Invalid request body", nil))
		return
	}
	if err := validator.ValidateStruct(req); err != nil {
		response.Error(c, err)
		return
	}

	item, err := h.service.Create(c.Request.Context(), req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusCreated, "Warehouse created successfully", item)
}

// update godoc
// @Summary Update warehouse (ADMIN only)
// @Description Set status=INACTIVE agar tidak bisa dipilih order baru
// @Tags Warehouses
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Warehouse ID"
// @Param request body UpdateRequest true "Field yang diubah"
// @Success 200 {object} Response
// @Failure 404 {object} map[string]interface{} "not found"
// @Router /warehouses/{id} [put]
func (h *Handler) update(c *gin.Context) {
	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.NewValidation("Invalid request body", nil))
		return
	}
	if err := validator.ValidateStruct(req); err != nil {
		response.Error(c, err)
		return
	}

	item, err := h.service.Update(c.Request.Context(), c.Param("id"), req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Warehouse updated successfully", item)
}

// delete godoc
// @Summary Hapus warehouse (ADMIN only)
// @Tags Warehouses
// @Security BearerAuth
// @Param id path string true "Warehouse ID"
// @Success 204 "no content"
// @Failure 404 {object} map[string]interface{} "not found"
// @Router /warehouses/{id} [delete]
func (h *Handler) delete(c *gin.Context) {
	if err := h.service.Delete(c.Request.Context(), c.Param("id")); err != nil {
		response.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
