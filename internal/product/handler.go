package product

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"order-management/internal/middleware"
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
// @Summary List product
// @Tags Products
// @Security BearerAuth
// @Produce json
// @Param page query int false "default 1"
// @Param limit query int false "default 20, max 100"
// @Param search query string false "filter"
// @Param category_id query string false "filter"
// @Param status query string false "ACTIVE|INACTIVE|DISCONTINUED"
// @Param sort query string false "name|price|created_at|updated_at"
// @Param order query string false "asc|desc"
// @Success 200 {object} map[string]interface{}
// @Router /products [get]
func (h *Handler) list(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	page, limit = response.NormalizePagination(page, limit)

	items, total, err := h.service.List(c.Request.Context(), ListFilter{
		Search:     c.Query("search"),
		CategoryID: c.Query("category_id"),
		Status:     c.Query("status"),
		Sort:       c.Query("sort"),
		Order:      c.Query("order"),
		Page:       page,
		Limit:      limit,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.List(c, "Products retrieved successfully", items, response.NewMeta(page, limit, total))
}

// get godoc
// @Summary Detail product (cache-aside Redis, TTL 10 menit)
// @Tags Products
// @Security BearerAuth
// @Produce json
// @Param id path string true "Product ID"
// @Success 200 {object} Response
// @Failure 404 {object} map[string]interface{} "not found"
// @Router /products/{id} [get]
func (h *Handler) get(c *gin.Context) {
	item, err := h.service.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Product retrieved successfully", item)
}

// create godoc
// @Summary Buat product baru (ADMIN only)
// @Description Menghasilkan audit log CREATE_PRODUCT
// @Tags Products
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body CreateRequest true "Data product"
// @Success 201 {object} Response
// @Failure 409 {object} map[string]interface{} "SKU already exists"
// @Router /products [post]
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

	item, err := h.service.Create(c.Request.Context(), middleware.GetUserID(c), req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusCreated, "Product created successfully", item)
}

// update godoc
// @Summary Update product (ADMIN only)
// @Description Invalidate cache Redis + menghasilkan audit log UPDATE_PRODUCT
// @Tags Products
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Product ID"
// @Param request body UpdateRequest true "Field yang diubah"
// @Success 200 {object} Response
// @Failure 404 {object} map[string]interface{} "not found"
// @Router /products/{id} [put]
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

	item, err := h.service.Update(c.Request.Context(), middleware.GetUserID(c), c.Param("id"), req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Product updated successfully", item)
}

// delete godoc
// @Summary Hapus product (soft delete, ADMIN only)
// @Tags Products
// @Security BearerAuth
// @Param id path string true "Product ID"
// @Success 204 "no content"
// @Failure 404 {object} map[string]interface{} "not found"
// @Router /products/{id} [delete]
func (h *Handler) delete(c *gin.Context) {
	if err := h.service.Delete(c.Request.Context(), c.Param("id")); err != nil {
		response.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
