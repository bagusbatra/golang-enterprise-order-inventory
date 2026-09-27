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

func (h *Handler) pack(c *gin.Context) {
	item, err := h.service.Pack(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Order packed successfully", item)
}

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

func (h *Handler) deliver(c *gin.Context) {
	item, err := h.service.Deliver(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Shipment delivered successfully", item)
}

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
