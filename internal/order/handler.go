package order

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

// RegisterRoutes mendaftarkan endpoint /orders (docs/api-contract.md Section
// 7). customerSalesOnly = RequireRole("CUSTOMER","SALES") untuk create.
// customerOrAdmin = RequireRole("CUSTOMER","ADMIN") untuk cancel (ownership
// CUSTOMER divalidasi di service, bukan middleware). GET list/detail dibuka
// untuk semua role terautentikasi — ownership/visibility diputuskan service.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, customerSalesOnly, customerOrAdmin gin.HandlerFunc) {
	rg.GET("", h.list)
	rg.GET("/:id", h.get)
	rg.POST("", customerSalesOnly, h.create)
	rg.POST("/:id/cancel", customerOrAdmin, h.cancel)
}

func (h *Handler) create(c *gin.Context) {
	var req CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.NewValidation("Invalid request body", nil))
		return
	}
	if err := validator.ValidateStruct(req); err != nil {
		response.Error(c, err)
		return
	}

	customerID := middleware.GetUserID(c)
	item, err := h.service.CreateOrder(c.Request.Context(), customerID, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusCreated, "Order created successfully", item)
}

func (h *Handler) get(c *gin.Context) {
	actorID := middleware.GetUserID(c)
	role := middleware.GetRole(c)

	item, err := h.service.GetByID(c.Request.Context(), actorID, role, c.Param("id"))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Order retrieved successfully", item)
}

func (h *Handler) list(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	actorID := middleware.GetUserID(c)
	role := middleware.GetRole(c)

	items, total, err := h.service.List(c.Request.Context(), actorID, role, c.Query("status"), c.Query("customer_id"), page, limit)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.List(c, "Orders retrieved successfully", items, response.NewMeta(page, limit, total))
}

func (h *Handler) cancel(c *gin.Context) {
	actorID := middleware.GetUserID(c)
	role := middleware.GetRole(c)

	item, err := h.service.Cancel(c.Request.Context(), actorID, role, c.Param("id"))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Order cancelled successfully", item)
}
