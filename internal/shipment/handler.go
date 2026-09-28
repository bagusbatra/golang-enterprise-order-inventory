package shipment

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"order-management/internal/middleware"
	apperr "order-management/pkg/errors"
	"order-management/pkg/response"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes: pack/ship terdaftar di grup /orders yang sudah ada,
// deliver/get di grup /shipments. warehouseOnly = RequireRole("WAREHOUSE")
// — role lain -> 403 (spec Section 52, 81).
func (h *Handler) RegisterRoutes(ordersGroup, shipmentsGroup *gin.RouterGroup, warehouseOnly gin.HandlerFunc) {
	ordersGroup.POST("/:id/pack", warehouseOnly, h.pack)
	ordersGroup.POST("/:id/ship", warehouseOnly, h.ship)
	shipmentsGroup.POST("/:id/deliver", warehouseOnly, h.deliver)
	shipmentsGroup.GET("/:id", h.get)
}

// pack godoc
// @Summary Pack order (WAREHOUSE only)
// @Description Transisi PAID -> PACKED
// @Tags Shipments
// @Security BearerAuth
// @Produce json
// @Param id path string true "Order ID"
// @Success 200 {object} internal_order.Response
// @Failure 403 {object} map[string]interface{} "forbidden"
// @Failure 422 {object} map[string]interface{} "invalid order status"
// @Router /orders/{id}/pack [post]
func (h *Handler) pack(c *gin.Context) {
	item, err := h.service.Pack(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Order packed successfully", item)
}

// ship godoc
// @Summary Ship order (WAREHOUSE only)
// @Description Transisi PACKED -> SHIPPED, generate tracking_number, kirim notifikasi + WS event
// @Tags Shipments
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Order ID"
// @Param request body ShipRequest false "courier opsional, default Simulated Courier"
// @Success 201 {object} internal_order.Response
// @Failure 422 {object} map[string]interface{} "invalid order status"
// @Router /orders/{id}/ship [post]
func (h *Handler) ship(c *gin.Context) {
	var req ShipRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Error(c, apperr.NewValidation("Invalid request body", nil))
			return
		}
	}

	item, err := h.service.Ship(c.Request.Context(), c.Param("id"), req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusCreated, "Order shipped successfully", item)
}

// deliver godoc
// @Summary Deliver shipment (WAREHOUSE only)
// @Description Transisi IN_TRANSIT/PICKED_UP -> DELIVERED, order -> COMPLETED
// @Tags Shipments
// @Security BearerAuth
// @Produce json
// @Param id path string true "Shipment ID"
// @Success 200 {object} Response
// @Failure 422 {object} map[string]interface{} "invalid shipment status"
// @Router /shipments/{id}/deliver [post]
func (h *Handler) deliver(c *gin.Context) {
	item, err := h.service.Deliver(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Shipment delivered successfully", item)
}

// get godoc
// @Summary Detail shipment (owner atau ADMIN/SALES/WAREHOUSE)
// @Tags Shipments
// @Security BearerAuth
// @Produce json
// @Param id path string true "Shipment ID"
// @Success 200 {object} Response
// @Failure 404 {object} map[string]interface{} "not found"
// @Router /shipments/{id} [get]
func (h *Handler) get(c *gin.Context) {
	actorID := middleware.GetUserID(c)
	role := middleware.GetRole(c)

	item, err := h.service.GetByID(c.Request.Context(), actorID, role, c.Param("id"))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Shipment retrieved successfully", item)
}
