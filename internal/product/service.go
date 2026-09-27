package product

import (
	"context"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"

	apperr "order-management/pkg/errors"
)

type Service struct {
	repo  Repository
	cache *cache
}

// NewService menerima redisClient yang boleh nil (misal saat unit test
// tanpa Redis) — cache-aside otomatis dilewati (selalu fallback ke
// PostgreSQL) tanpa error, karena Redis bukan source of truth (docs/
// architecture.md Section 5).
func NewService(repo Repository, redisClient *redis.Client, cacheTTL time.Duration) *Service {
	return &Service{repo: repo, cache: newCache(redisClient, cacheTTL)}
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (*Response, error) {
	if err := validatePricing(req.Price, req.CostPrice, req.Weight); err != nil {
		return nil, err
	}

	if _, err := s.repo.FindBySKU(ctx, req.SKU); err == nil {
		return nil, apperr.ConflictErr(apperr.ProductSKUExists, "SKU already exists")
	} else if err != ErrNotFound {
		return nil, err
	}

	p := &Product{
		CategoryID:  req.CategoryID,
		SKU:         req.SKU,
		Name:        req.Name,
		Description: req.Description,
		Price:       req.Price,
		CostPrice:   req.CostPrice,
		Weight:      req.Weight,
		Status:      StatusActive,
	}
	if err := s.repo.Create(ctx, p); err != nil {
		return nil, err
	}
	resp := toResponse(p)
	return &resp, nil
}

// GetByID adalah endpoint cache-aside: cek Redis dulu, miss -> Postgres ->
// isi Redis (spec Section 27).
func (s *Service) GetByID(ctx context.Context, id string) (*Response, error) {
	if cached, ok := s.cache.Get(ctx, id); ok {
		resp := toResponse(cached)
		return &resp, nil
	}

	p, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if err == ErrNotFound {
			return nil, apperr.New(http.StatusNotFound, apperr.ProductNotFound, "Product not found")
		}
		return nil, err
	}

	s.cache.Set(ctx, p)
	resp := toResponse(p)
	return &resp, nil
}

func (s *Service) List(ctx context.Context, f ListFilter) ([]Response, int64, error) {
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 20
	}

	items, total, err := s.repo.List(ctx, f)
	if err != nil {
		return nil, 0, err
	}

	resp := make([]Response, 0, len(items))
	for i := range items {
		resp = append(resp, toResponse(&items[i]))
	}
	return resp, total, nil
}

func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (*Response, error) {
	p, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if err == ErrNotFound {
			return nil, apperr.New(http.StatusNotFound, apperr.ProductNotFound, "Product not found")
		}
		return nil, err
	}

	if req.CategoryID != nil {
		p.CategoryID = *req.CategoryID
	}
	if req.Name != nil {
		p.Name = *req.Name
	}
	if req.Description != nil {
		p.Description = *req.Description
	}
	if req.Price != nil {
		p.Price = *req.Price
	}
	if req.CostPrice != nil {
		p.CostPrice = *req.CostPrice
	}
	if req.Weight != nil {
		p.Weight = *req.Weight
	}
	if req.Status != nil {
		p.Status = Status(*req.Status)
	}

	if err := validatePricing(p.Price, p.CostPrice, p.Weight); err != nil {
		return nil, err
	}

	if err := s.repo.Update(ctx, p); err != nil {
		return nil, err
	}

	// Invalidate SETELAH update commit ke database — urutan ini penting:
	// jika invalidate dilakukan sebelum update selesai lalu update gagal,
	// request lain bisa mengisi cache dengan data lama tepat sebelum
	// update akhirnya berhasil (race window).
	s.cache.Invalidate(ctx, id)

	resp := toResponse(p)
	return &resp, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		if err == ErrNotFound {
			return apperr.New(http.StatusNotFound, apperr.ProductNotFound, "Product not found")
		}
		return err
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.cache.Invalidate(ctx, id)
	return nil
}

// GetPriceSnapshot adalah titik integrasi resmi dengan Order Service milik
// Agent 3 (docs/architecture.md Section 2) — dipanggil saat create-order
// untuk mengambil harga TERKINI yang akan di-snapshot ke order_items.unit_price
// (spec Section 15). Sengaja BYPASS cache (langsung ke repository) supaya
// jalur finansial kritikal ini tidak bergantung pada kebenaran cache.
func (s *Service) GetPriceSnapshot(ctx context.Context, productID string) (decimal.Decimal, error) {
	p, err := s.repo.FindByID(ctx, productID)
	if err != nil {
		if err == ErrNotFound {
			return decimal.Zero, apperr.New(http.StatusNotFound, apperr.ProductNotFound, "Product not found")
		}
		return decimal.Zero, err
	}
	if p.Status != StatusActive {
		return decimal.Zero, apperr.New(http.StatusUnprocessableEntity, apperr.ProductInactive, "Product is not active")
	}
	return p.Price, nil
}

func validatePricing(price, costPrice, weight decimal.Decimal) error {
	if price.LessThanOrEqual(decimal.Zero) {
		return apperr.NewValidation("Invalid product pricing", []apperr.FieldError{
			{Field: "price", Message: "price must be greater than 0"},
		})
	}
	if costPrice.LessThanOrEqual(decimal.Zero) {
		return apperr.NewValidation("Invalid product pricing", []apperr.FieldError{
			{Field: "cost_price", Message: "cost_price must be greater than 0"},
		})
	}
	if weight.LessThan(decimal.Zero) {
		return apperr.NewValidation("Invalid product pricing", []apperr.FieldError{
			{Field: "weight", Message: "weight must be greater than or equal to 0"},
		})
	}
	return nil
}
