// Package errors adalah registry error code terpusat (spec Section 60) yang
// dipakai lintas seluruh modul, baik milik Agent 2 maupun Agent 3, supaya
// tidak ada modul yang membuat kode error sendiri secara tidak konsisten.
package errors

import "net/http"

// Code adalah kode error stabil yang dikembalikan ke client lewat field
// "code" pada response envelope error (lihat pkg/response).
type Code string

const (
	AuthInvalidCredentials Code = "AUTH_INVALID_CREDENTIALS"
	AuthTokenExpired       Code = "AUTH_TOKEN_EXPIRED"
	AuthUnauthorized       Code = "AUTH_UNAUTHORIZED"
	AuthForbidden          Code = "AUTH_FORBIDDEN"

	ProductNotFound  Code = "PRODUCT_NOT_FOUND"
	ProductSKUExists Code = "PRODUCT_SKU_EXISTS"
	ProductInactive  Code = "PRODUCT_INACTIVE"

	CategoryNotFound          Code = "CATEGORY_NOT_FOUND"
	CategoryNameExists        Code = "CATEGORY_NAME_EXISTS"
	CategoryHasActiveProducts Code = "CATEGORY_HAS_ACTIVE_PRODUCTS"

	WarehouseNotFound   Code = "WAREHOUSE_NOT_FOUND"
	WarehouseInactive   Code = "WAREHOUSE_INACTIVE"
	WarehouseCodeExists Code = "WAREHOUSE_CODE_EXISTS"

	InventoryNotFound Code = "INVENTORY_NOT_FOUND"
	InsufficientStock Code = "INSUFFICIENT_STOCK"
	InventoryNegative Code = "INVENTORY_NEGATIVE"

	OrderNotFound         Code = "ORDER_NOT_FOUND"
	OrderInvalidStatus    Code = "ORDER_INVALID_STATUS"
	OrderAlreadyCancelled Code = "ORDER_ALREADY_CANCELLED"

	PaymentNotFound        Code = "PAYMENT_NOT_FOUND"
	PaymentAlreadyPaid     Code = "PAYMENT_ALREADY_PAID"
	PaymentExpired         Code = "PAYMENT_EXPIRED"
	PaymentInvalidCallback Code = "PAYMENT_INVALID_CALLBACK"

	ShipmentNotFound      Code = "SHIPMENT_NOT_FOUND"
	ShipmentInvalidStatus Code = "SHIPMENT_INVALID_STATUS"

	UserNotFound Code = "USER_NOT_FOUND"
	EmailExists  Code = "EMAIL_EXISTS"

	ValidationError Code = "VALIDATION_ERROR"
	NotFound        Code = "NOT_FOUND"
	InternalError   Code = "INTERNAL_ERROR"
)

// AppError adalah error terstruktur yang dipahami handler untuk memilih HTTP
// status code & response envelope yang tepat tanpa handler perlu tahu detail
// business logic penyebabnya.
type AppError struct {
	HTTPStatus int
	ErrCode    Code
	Msg        string
	FieldErrs  []FieldError
}

// FieldError merepresentasikan satu error validasi per-field, dipakai di
// array "errors" pada response envelope error.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *AppError) Error() string {
	return e.Msg
}

// New membuat AppError generik dengan HTTP status eksplisit.
func New(status int, code Code, msg string) *AppError {
	return &AppError{HTTPStatus: status, ErrCode: code, Msg: msg}
}

// NewValidation membuat AppError 422 khusus untuk kegagalan validasi input,
// membawa daftar field yang salah agar client bisa menampilkan pesan per-field.
func NewValidation(msg string, fields []FieldError) *AppError {
	return &AppError{HTTPStatus: http.StatusUnprocessableEntity, ErrCode: ValidationError, Msg: msg, FieldErrs: fields}
}

// Helper cepat untuk kasus yang sangat umum dipakai lintas modul.
func NotFoundErr(entity, msg string) *AppError {
	return New(http.StatusNotFound, NotFound, msg)
}

func ForbiddenErr(msg string) *AppError {
	return New(http.StatusForbidden, AuthForbidden, msg)
}

func UnauthorizedErr(msg string) *AppError {
	return New(http.StatusUnauthorized, AuthUnauthorized, msg)
}

func ConflictErr(code Code, msg string) *AppError {
	return New(http.StatusConflict, code, msg)
}
