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

// create godoc
// @Summary Buat payment untuk order (CUSTOMER, owner only)
// @Description payment_method default BANK_TRANSFER, expired_at = now+30m
// @Tags Payments
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Order ID"
// @Param request body CreatePaymentRequest false "payment_method opsional"
// @Success 201 {object} Response
// @Failure 422 {object} map[string]interface{} "order status invalid"
// @Router /orders/{id}/payment [post]
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

// get godoc
// @Summary Detail payment (owner atau ADMIN/SALES)
// @Tags Payments
// @Security BearerAuth
// @Produce json
// @Param id path string true "Payment ID"
// @Success 200 {object} Response
// @Failure 404 {object} map[string]interface{} "not found"
// @Router /payments/{id} [get]
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
// Callback godoc
// @Summary Payment gateway callback (simulasi, publik/tanpa JWT)
// @Description Idempotent — callback yang sama dikirim berkali-kali hanya menghasilkan efek bisnis satu kali (UNIQUE transaction_id + status check)
// @Tags Payments
// @Accept json
// @Produce json
// @Param request body CallbackRequest true "Callback dari payment gateway"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{} "payment not found"
// @Failure 422 {object} map[string]interface{} "invalid callback"
// @Router /payments/callback [post]
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
