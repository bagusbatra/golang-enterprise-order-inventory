package product

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("product: not found")

// sortWhitelist mencegah query parameter `sort` masuk mentah-mentah ke SQL
// ORDER BY (spec Section 62) — field di luar daftar ini diabaikan, fallback
// ke default.
var sortWhitelist = map[string]bool{
	"name":       true,
	"price":      true,
	"created_at": true,
	"updated_at": true,
}

type ListFilter struct {
	Search     string
	CategoryID string
	Status     string
	Sort       string
	Order      string
	Page       int
	Limit      int
}

type Repository interface {
	Create(ctx context.Context, p *Product) error
	FindByID(ctx context.Context, id string) (*Product, error)
	FindBySKU(ctx context.Context, sku string) (*Product, error)
	List(ctx context.Context, f ListFilter) ([]Product, int64, error)
	Update(ctx context.Context, p *Product) error
	Delete(ctx context.Context, id string) error
}

type gormRepository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) Create(ctx context.Context, p *Product) error {
	return r.db.WithContext(ctx).Create(p).Error
}

func (r *gormRepository) FindByID(ctx context.Context, id string) (*Product, error) {
	var p Product
	if err := r.db.WithContext(ctx).First(&p, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (r *gormRepository) FindBySKU(ctx context.Context, sku string) (*Product, error) {
	var p Product
	if err := r.db.WithContext(ctx).First(&p, "sku = ?", sku).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (r *gormRepository) List(ctx context.Context, f ListFilter) ([]Product, int64, error) {
	q := r.db.WithContext(ctx).Model(&Product{})

	if f.Search != "" {
		q = q.Where("name ILIKE ? OR sku ILIKE ?", "%"+f.Search+"%", "%"+f.Search+"%")
	}
	if f.CategoryID != "" {
		q = q.Where("category_id = ?", f.CategoryID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	sortField := "created_at"
	if sortWhitelist[f.Sort] {
		sortField = f.Sort
	}
	sortOrder := "desc"
	if f.Order == "asc" {
		sortOrder = "asc"
	}

	var items []Product
	offset := (f.Page - 1) * f.Limit
	if err := q.Order(sortField + " " + sortOrder).Offset(offset).Limit(f.Limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *gormRepository) Update(ctx context.Context, p *Product) error {
	return r.db.WithContext(ctx).Save(p).Error
}

func (r *gormRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&Product{}, "id = ?", id).Error
}
