package inventory

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

// RegisterRoutes mendaftarkan endpoint /inventory (docs/api-contract.md
// Section 6). adminWarehouse mengizinkan ADMIN & WAREHOUSE; adminOnly hanya
// ADMIN (dipakai khusus POST /inventory/adjust).
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, adminWarehouse, adminOnly gin.HandlerFunc) {
	rg.GET("", adminWarehouse, h.list)
	rg.GET("/:id", adminWarehouse, h.get)
	rg.GET("/:id/transactions", adminWarehouse, h.listTransactions)
	rg.POST("/stock-in", adminWarehouse, h.stockIn)
	rg.POST("/adjust", adminOnly, h.adjust)
}

func (h *Handler) list(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	page, limit = response.NormalizePagination(page, limit)
	lowStock, _ := strconv.ParseBool(c.Query("low_stock"))

	f := ListFilter{
		ProductID:   c.Query("product_id"),
		WarehouseID: c.Query("warehouse_id"),
		LowStock:    lowStock,
		Page:        page,
		Limit:       limit,
	}

	items, total, err := h.service.List(c.Request.Context(), f)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.List(c, "Inventory retrieved successfully", items, response.NewMeta(f.Page, f.Limit, total))
}

func (h *Handler) get(c *gin.Context) {
	item, err := h.service.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Inventory retrieved successfully", item)
}

func (h *Handler) listTransactions(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	page, limit = response.NormalizePagination(page, limit)

	items, total, err := h.service.ListTransactions(c.Request.Context(), c.Param("id"), page, limit)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.List(c, "Inventory transactions retrieved successfully", items, response.NewMeta(page, limit, total))
}

func (h *Handler) stockIn(c *gin.Context) {
	var req StockInRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.NewValidation("Invalid request body", nil))
		return
	}
	if err := validator.ValidateStruct(req); err != nil {
		response.Error(c, err)
		return
	}

	item, err := h.service.StockIn(c.Request.Context(), req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusCreated, "Stock-in recorded successfully", item)
}

func (h *Handler) adjust(c *gin.Context) {
	var req AdjustRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.NewValidation("Invalid request body", nil))
		return
	}
	if err := validator.ValidateStruct(req); err != nil {
		response.Error(c, err)
		return
	}

	actorID := middleware.GetUserID(c)
	item, err := h.service.Adjust(c.Request.Context(), actorID, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Inventory adjusted successfully", item)
}
