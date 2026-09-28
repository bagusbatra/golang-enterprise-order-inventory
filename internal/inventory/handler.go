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

// list godoc
// @Summary List inventory (ADMIN, WAREHOUSE)
// @Tags Inventory
// @Security BearerAuth
// @Produce json
// @Param page query int false "default 1"
// @Param limit query int false "default 20, max 100"
// @Param product_id query string false "filter"
// @Param warehouse_id query string false "filter"
// @Param low_stock query bool false "filter available_stock<=5"
// @Success 200 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{} "forbidden"
// @Router /inventory [get]
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

// get godoc
// @Summary Detail inventory (ADMIN, WAREHOUSE)
// @Tags Inventory
// @Security BearerAuth
// @Produce json
// @Param id path string true "Inventory ID"
// @Success 200 {object} Response
// @Failure 404 {object} map[string]interface{} "not found"
// @Router /inventory/{id} [get]
func (h *Handler) get(c *gin.Context) {
	item, err := h.service.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Inventory retrieved successfully", item)
}

// listTransactions godoc
// @Summary Riwayat transaksi inventory (ADMIN, WAREHOUSE)
// @Tags Inventory
// @Security BearerAuth
// @Produce json
// @Param id path string true "Inventory ID"
// @Param page query int false "default 1"
// @Param limit query int false "default 20, max 100"
// @Success 200 {object} map[string]interface{}
// @Router /inventory/{id}/transactions [get]
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

// stockIn godoc
// @Summary Stock-in (ADMIN, WAREHOUSE)
// @Tags Inventory
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body StockInRequest true "Data stock-in"
// @Success 201 {object} Response
// @Failure 403 {object} map[string]interface{} "forbidden"
// @Router /inventory/stock-in [post]
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

// adjust godoc
// @Summary Adjustment inventory (ADMIN only)
// @Description Tidak boleh membuat quantity negatif; menghasilkan audit log + inventory_transaction
// @Tags Inventory
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body AdjustRequest true "Data adjustment (quantity boleh negatif)"
// @Success 200 {object} Response
// @Failure 422 {object} map[string]interface{} "would result in negative quantity"
// @Router /inventory/adjust [post]
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
