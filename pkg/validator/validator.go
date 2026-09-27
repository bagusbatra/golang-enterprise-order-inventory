// Package validator membungkus go-playground/validator agar error validasi
// bisa langsung dikonversi ke format []apperr.FieldError yang dipakai
// response envelope error (spec Section 58), tanpa tiap handler menulis
// konversi ini berulang-ulang.
package validator

import (
	"strings"

	"github.com/go-playground/validator/v10"

	apperr "order-management/pkg/errors"
)

var instance = validator.New()

// ValidateStruct menjalankan validasi tag `validate:"..."` pada struct DTO.
// Mengembalikan nil jika valid, atau *apperr.AppError (422 VALIDATION_ERROR)
// berisi daftar field yang gagal jika tidak valid.
func ValidateStruct(s any) error {
	err := instance.Struct(s)
	if err == nil {
		return nil
	}

	validationErrs, ok := err.(validator.ValidationErrors)
	if !ok {
		return apperr.NewValidation("Validation failed", nil)
	}

	fields := make([]apperr.FieldError, 0, len(validationErrs))
	for _, fe := range validationErrs {
		fields = append(fields, apperr.FieldError{
			Field:   toSnakeField(fe.Field()),
			Message: humanizeTag(fe),
		})
	}
	return apperr.NewValidation("Validation failed", fields)
}

// toSnakeField mengubah nama field Go (PascalCase) menjadi snake_case yang
// konsisten dengan penamaan field JSON di seluruh API contract.
func toSnakeField(field string) string {
	var b strings.Builder
	for i, r := range field {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte('_')
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}

func humanizeTag(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "field is required"
	case "email":
		return "invalid email format"
	case "min":
		return "value is below minimum (" + fe.Param() + ")"
	case "max":
		return "value exceeds maximum (" + fe.Param() + ")"
	case "gt":
		return "value must be greater than " + fe.Param()
	case "gte":
		return "value must be greater than or equal to " + fe.Param()
	case "oneof":
		return "value must be one of: " + fe.Param()
	default:
		return "invalid value"
	}
}
