package product

import "github.com/shopspring/decimal"

type CreateRequest struct {
	CategoryID  string `json:"category_id" validate:"required"`
	SKU         string `json:"sku" validate:"required"`
	Name        string `json:"name" validate:"required"`
	Description string `json:"description"`
	// Price/CostPrice/Weight sengaja TIDAK divalidasi lewat tag `validate`
	// (go-playground/validator tidak mendukung perbandingan numerik pada
	// struct decimal.Decimal via reflection) — divalidasi manual di service
	// layer (lihat Service.Create), konsisten dengan prinsip business rule
	// harus ada di service, bukan hanya di binding tag.
	Price     decimal.Decimal `json:"price"`
	CostPrice decimal.Decimal `json:"cost_price"`
	Weight    decimal.Decimal `json:"weight"`
}

type UpdateRequest struct {
	CategoryID  *string          `json:"category_id"`
	Name        *string          `json:"name"`
	Description *string          `json:"description"`
	Price       *decimal.Decimal `json:"price"`
	CostPrice   *decimal.Decimal `json:"cost_price"`
	Weight      *decimal.Decimal `json:"weight"`
	Status      *string          `json:"status" validate:"omitempty,oneof=ACTIVE INACTIVE DISCONTINUED"`
}

type Response struct {
	ID          string          `json:"id"`
	CategoryID  string          `json:"category_id"`
	SKU         string          `json:"sku"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Price       decimal.Decimal `json:"price"`
	CostPrice   decimal.Decimal `json:"cost_price"`
	Weight      decimal.Decimal `json:"weight"`
	Status      string          `json:"status"`
}

func toResponse(p *Product) Response {
	return Response{
		ID:          p.ID,
		CategoryID:  p.CategoryID,
		SKU:         p.SKU,
		Name:        p.Name,
		Description: p.Description,
		Price:       p.Price,
		CostPrice:   p.CostPrice,
		Weight:      p.Weight,
		Status:      string(p.Status),
	}
}
