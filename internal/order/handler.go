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

// create godoc
// @Summary Buat order baru (CUSTOMER, SALES)
// @Description Concurrency-safe stock reservation via PostgreSQL row lock
// @Tags Orders
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body CreateOrderRequest true "Warehouse & item order"
// @Success 201 {object} Response
// @Failure 409 {object} map[string]interface{} "insufficient stock"
// @Failure 422 {object} map[string]interface{} "warehouse inactive"
// @Router /orders [post]
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

// get godoc
// @Summary Detail order (CUSTOMER: own only, lainnya sesuai permission)
// @Tags Orders
// @Security BearerAuth
// @Produce json
// @Param id path string true "Order ID"
// @Success 200 {object} DetailResponse
// @Failure 403 {object} map[string]interface{} "forbidden"
// @Failure 404 {object} map[string]interface{} "not found"
// @Router /orders/{id} [get]
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

// list godoc
// @Summary List order (CUSTOMER: own only, lainnya sesuai permission)
// @Tags Orders
// @Security BearerAuth
// @Produce json
// @Param page query int false "default 1"
// @Param limit query int false "default 20, max 100"
// @Param status query string false "filter"
// @Param customer_id query string false "ADMIN/SALES only"
// @Success 200 {object} map[string]interface{}
// @Router /orders [get]
func (h *Handler) list(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	page, limit = response.NormalizePagination(page, limit)
	actorID := middleware.GetUserID(c)
	role := middleware.GetRole(c)

	items, total, err := h.service.List(c.Request.Context(), actorID, role, c.Query("status"), c.Query("customer_id"), page, limit)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.List(c, "Orders retrieved successfully", items, response.NewMeta(page, limit, total))
}

// cancel godoc
// @Summary Batalkan order (CUSTOMER: own, ADMIN)
// @Description Hanya dari status PENDING/WAITING_PAYMENT/PAID; release reserved stock
// @Tags Orders
// @Security BearerAuth
// @Produce json
// @Param id path string true "Order ID"
// @Success 200 {object} Response
// @Failure 422 {object} map[string]interface{} "invalid order status"
// @Router /orders/{id}/cancel [post]
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
