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

func (h *Handler) get(c *gin.Context) {
	item, err := h.service.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "User retrieved successfully", item)
}

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
