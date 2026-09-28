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

// Register godoc
// @Summary Register akun baru
// @Description Selalu membuat user dengan role CUSTOMER
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body RegisterRequest true "Data registrasi"
// @Success 201 {object} RegisterResponse
// @Failure 422 {object} map[string]interface{} "validation error / email sudah terdaftar"
// @Router /auth/register [post]
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

// Login godoc
// @Summary Login
// @Description Rate limited 5 req/menit/IP
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body LoginRequest true "Credential"
// @Success 200 {object} LoginResponse
// @Failure 401 {object} map[string]interface{} "invalid credentials"
// @Failure 429 {object} map[string]interface{} "too many requests"
// @Router /auth/login [post]
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

// Refresh godoc
// @Summary Refresh access token
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body RefreshRequest true "Refresh token"
// @Success 200 {object} RefreshResponse
// @Failure 401 {object} map[string]interface{} "token expired/revoked"
// @Router /auth/refresh [post]
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

// Logout godoc
// @Summary Logout (revoke refresh token)
// @Tags Auth
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body LogoutRequest true "Refresh token yang akan direvoke"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{} "unauthorized"
// @Router /auth/logout [post]
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
