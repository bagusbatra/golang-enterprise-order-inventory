package user

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

// RegisterRoutes mendaftarkan seluruh endpoint /users. Group yang diterima
// di sini SUDAH melewati JWTAuth + RequireRole(ADMIN) — didaftarkan oleh
// caller (cmd/server/main.go), handler ini tidak perlu tahu detail RBAC.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.list)
	rg.GET("/:id", h.get)
	rg.POST("", h.create)
	rg.PUT("/:id", h.update)
	rg.PATCH("/:id/status", h.updateStatus)
}

// list godoc
// @Summary List user (ADMIN only)
// @Tags Users
// @Security BearerAuth
// @Produce json
// @Param page query int false "default 1"
// @Param limit query int false "default 20, max 100"
// @Param search query string false "filter"
// @Param role query string false "ADMIN|SALES|WAREHOUSE|CUSTOMER"
// @Param status query string false "ACTIVE|INACTIVE|SUSPENDED"
// @Success 200 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{} "forbidden"
// @Router /users [get]
func (h *Handler) list(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	page, limit = response.NormalizePagination(page, limit)

	items, total, err := h.service.List(c.Request.Context(), ListFilter{
		Search: c.Query("search"),
		Role:   c.Query("role"),
		Status: c.Query("status"),
		Page:   page,
		Limit:  limit,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.List(c, "Users retrieved successfully", items, response.NewMeta(page, limit, total))
}

// get godoc
// @Summary Detail user (ADMIN only)
// @Tags Users
// @Security BearerAuth
// @Produce json
// @Param id path string true "User ID"
// @Success 200 {object} Response
// @Failure 404 {object} map[string]interface{} "not found"
// @Router /users/{id} [get]
func (h *Handler) get(c *gin.Context) {
	item, err := h.service.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "User retrieved successfully", item)
}

// create godoc
// @Summary Buat user baru dengan role apa pun (ADMIN only)
// @Tags Users
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body CreateRequest true "Data user"
// @Success 201 {object} Response
// @Failure 422 {object} map[string]interface{} "validation error"
// @Router /users [post]
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
	response.Success(c, http.StatusCreated, "User created successfully", item)
}

// update godoc
// @Summary Update profil/role user (ADMIN only)
// @Description Role change menghasilkan audit log otomatis
// @Tags Users
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "User ID"
// @Param request body UpdateRequest true "Field yang diubah"
// @Success 200 {object} Response
// @Failure 404 {object} map[string]interface{} "not found"
// @Router /users/{id} [put]
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

	actorID := middleware.GetUserID(c)
	item, err := h.service.Update(c.Request.Context(), actorID, c.Param("id"), req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "User updated successfully", item)
}

// updateStatus godoc
// @Summary Ubah status user (ADMIN only)
// @Tags Users
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "User ID"
// @Param request body UpdateStatusRequest true "Status baru"
// @Success 200 {object} Response
// @Failure 404 {object} map[string]interface{} "not found"
// @Router /users/{id}/status [patch]
func (h *Handler) updateStatus(c *gin.Context) {
	var req UpdateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.NewValidation("Invalid request body", nil))
		return
	}
	if err := validator.ValidateStruct(req); err != nil {
		response.Error(c, err)
		return
	}

	item, err := h.service.UpdateStatus(c.Request.Context(), c.Param("id"), req.Status)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "User status updated successfully", item)
}
