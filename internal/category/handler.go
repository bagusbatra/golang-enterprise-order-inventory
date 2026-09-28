package category

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

// RegisterRoutes mendaftarkan GET (any authenticated) dan write endpoint
// (ADMIN only, RBAC-nya dipasang oleh caller per-route, bukan di sini).
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, adminOnly gin.HandlerFunc) {
	rg.GET("", h.list)
	rg.GET("/:id", h.get)
	rg.POST("", adminOnly, h.create)
	rg.PUT("/:id", adminOnly, h.update)
	rg.DELETE("/:id", adminOnly, h.delete)
}

// list godoc
// @Summary List category
// @Tags Categories
// @Security BearerAuth
// @Produce json
// @Param page query int false "default 1"
// @Param limit query int false "default 20, max 100"
// @Param search query string false "filter"
// @Success 200 {object} map[string]interface{}
// @Router /categories [get]
func (h *Handler) list(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	page, limit = response.NormalizePagination(page, limit)

	items, total, err := h.service.List(c.Request.Context(), c.Query("search"), page, limit)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.List(c, "Categories retrieved successfully", items, response.NewMeta(page, limit, total))
}

// get godoc
// @Summary Detail category
// @Tags Categories
// @Security BearerAuth
// @Produce json
// @Param id path string true "Category ID"
// @Success 200 {object} Response
// @Failure 404 {object} map[string]interface{} "not found"
// @Router /categories/{id} [get]
func (h *Handler) get(c *gin.Context) {
	item, err := h.service.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Category retrieved successfully", item)
}

// create godoc
// @Summary Buat category baru (ADMIN only)
// @Tags Categories
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body CreateRequest true "Data category"
// @Success 201 {object} Response
// @Failure 409 {object} map[string]interface{} "name already exists"
// @Router /categories [post]
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
	response.Success(c, http.StatusCreated, "Category created successfully", item)
}

// update godoc
// @Summary Update category (ADMIN only)
// @Tags Categories
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Category ID"
// @Param request body UpdateRequest true "Field yang diubah"
// @Success 200 {object} Response
// @Failure 404 {object} map[string]interface{} "not found"
// @Router /categories/{id} [put]
func (h *Handler) update(c *gin.Context) {
	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.NewValidation("Invalid request body", nil))
		return
	}

	item, err := h.service.Update(c.Request.Context(), c.Param("id"), req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Category updated successfully", item)
}

// delete godoc
// @Summary Hapus category (ADMIN only)
// @Description Ditolak jika category masih memiliki produk ACTIVE
// @Tags Categories
// @Security BearerAuth
// @Param id path string true "Category ID"
// @Success 204 "no content"
// @Failure 409 {object} map[string]interface{} "still has active products"
// @Router /categories/{id} [delete]
func (h *Handler) delete(c *gin.Context) {
	if err := h.service.Delete(c.Request.Context(), c.Param("id")); err != nil {
		response.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
