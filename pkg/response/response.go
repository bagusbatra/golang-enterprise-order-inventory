// Package response menyediakan helper untuk menghasilkan response envelope
// yang konsisten (spec Section 58) di seluruh handler, baik milik Agent 2
// maupun Agent 3. Jangan membangun JSON response secara manual di handler.
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apperr "order-management/pkg/errors"
)

type envelope struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type listEnvelope struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data"`
	Meta    Meta   `json:"meta"`
}

type errorEnvelope struct {
	Success bool                `json:"success"`
	Message string              `json:"message"`
	Code    apperr.Code         `json:"code"`
	Errors  []apperr.FieldError `json:"errors,omitempty"`
}

// Meta adalah metadata pagination standar untuk seluruh endpoint list.
type Meta struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// NewMeta menghitung total_pages dari total & limit sehingga caller tidak
// perlu menghitung pembagian pembulatan ke atas secara manual di tiap tempat.
func NewMeta(page, limit int, total int64) Meta {
	totalPages := 0
	if limit > 0 {
		totalPages = int((total + int64(limit) - 1) / int64(limit))
	}
	return Meta{Page: page, Limit: limit, Total: total, TotalPages: totalPages}
}

// Success mengirim response sukses untuk single resource.
func Success(c *gin.Context, status int, message string, data any) {
	c.JSON(status, envelope{Success: true, Message: message, Data: data})
}

// List mengirim response sukses untuk list resource dengan meta pagination.
func List(c *gin.Context, message string, data any, meta Meta) {
	c.JSON(http.StatusOK, listEnvelope{Success: true, Message: message, Data: data, Meta: meta})
}

// Error mengirim response error terstruktur. Jika err adalah *apperr.AppError,
// HTTP status/code/field errors diambil darinya; selain itu dianggap
// kesalahan tak terduga (500 INTERNAL_ERROR) — detail asli TIDAK dikirim ke
// client (hanya dicatat oleh logger di middleware Recovery/Logger) supaya
// tidak membocorkan informasi internal.
func Error(c *gin.Context, err error) {
	if ae, ok := err.(*apperr.AppError); ok {
		c.JSON(ae.HTTPStatus, errorEnvelope{
			Success: false,
			Message: ae.Msg,
			Code:    ae.ErrCode,
			Errors:  ae.FieldErrs,
		})
		return
	}

	c.JSON(http.StatusInternalServerError, errorEnvelope{
		Success: false,
		Message: "Internal server error",
		Code:    apperr.InternalError,
	})
}
