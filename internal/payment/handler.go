package payment

import (
	"net/http"

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

// RegisterRoutes mendaftarkan endpoint yang butuh JWT: POST /orders/:id/payment
// (customerOnly) di grup /orders yang sudah ada, dan GET /payments/:id di
// grup /payments. POST /payments/callback TIDAK didaftarkan di sini — publik
// (simulasi gateway eksternal, tanpa JWT), didaftarkan langsung ke `v1` oleh
// caller lewat method Callback (lihat cmd/server/main.go).
func (h *Handler) RegisterRoutes(ordersGroup, paymentsGroup *gin.RouterGroup, customerOnly gin.HandlerFunc) {
	ordersGroup.POST("/:id/payment", customerOnly, h.create)
	paymentsGroup.GET("/:id", h.get)
}

func (h *Handler) create(c *gin.Context) {
	var req CreatePaymentRequest
	// Body opsional (payment_method?) — body kosong tetap valid, jadi
	// error binding di sini TIDAK dianggap fatal kecuali JSON-nya rusak.
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Error(c, apperr.NewValidation("Invalid request body", nil))
			return
		}
	}
	if err := validator.ValidateStruct(req); err != nil {
		response.Error(c, err)
		return
	}

	actorID := middleware.GetUserID(c)
	item, err := h.service.CreatePayment(c.Request.Context(), actorID, c.Param("id"), req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusCreated, "Payment created successfully", item)
}

func (h *Handler) get(c *gin.Context) {
	actorID := middleware.GetUserID(c)
	role := middleware.GetRole(c)

	item, err := h.service.GetByID(c.Request.Context(), actorID, role, c.Param("id"))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Payment retrieved successfully", item)
}

// Callback menangani POST /payments/callback — endpoint publik (simulasi
// gateway eksternal), TIDAK pakai middleware JWT.
func (h *Handler) Callback(c *gin.Context) {
	var req CallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.NewValidation("Invalid request body", nil))
		return
	}
	if err := validator.ValidateStruct(req); err != nil {
		response.Error(c, err)
		return
	}

	if err := h.service.Callback(c.Request.Context(), req); err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
