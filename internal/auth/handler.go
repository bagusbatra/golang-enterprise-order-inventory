package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apperr "order-management/pkg/errors"
	"order-management/pkg/response"
	"order-management/pkg/validator"
)

// Handler mengekspos method individual (bukan satu RegisterRoutes) karena
// endpoint auth adalah campuran publik (register/login/refresh) dan
// terproteksi (logout) — pemasangan middleware per-route dilakukan oleh
// caller (cmd/server/main.go), bukan di sini.
type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.NewValidation("Invalid request body", nil))
		return
	}
	if err := validator.ValidateStruct(req); err != nil {
		response.Error(c, err)
		return
	}

	resp, err := h.service.Register(c.Request.Context(), req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusCreated, "Registration successful", resp)
}

func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.NewValidation("Invalid request body", nil))
		return
	}
	if err := validator.ValidateStruct(req); err != nil {
		response.Error(c, err)
		return
	}

	resp, err := h.service.Login(c.Request.Context(), req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Login successful", resp)
}

func (h *Handler) Refresh(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.NewValidation("Invalid request body", nil))
		return
	}
	if err := validator.ValidateStruct(req); err != nil {
		response.Error(c, err)
		return
	}

	resp, err := h.service.Refresh(c.Request.Context(), req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Token refreshed successfully", resp)
}

func (h *Handler) Logout(c *gin.Context) {
	var req LogoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.NewValidation("Invalid request body", nil))
		return
	}
	if err := validator.ValidateStruct(req); err != nil {
		response.Error(c, err)
		return
	}

	if err := h.service.Logout(c.Request.Context(), req); err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Logout successful", nil)
}
