package warehouse

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("warehouse: not found")

type Repository interface {
	Create(ctx context.Context, w *Warehouse) error
	FindByID(ctx context.Context, id string) (*Warehouse, error)
	FindByCode(ctx context.Context, code string) (*Warehouse, error)
	List(ctx context.Context, status string, page, limit int) ([]Warehouse, int64, error)
	Update(ctx context.Context, w *Warehouse) error
	Delete(ctx context.Context, id string) error
}

type gormRepository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) Create(ctx context.Context, w *Warehouse) error {
	return r.db.WithContext(ctx).Create(w).Error
}

func (r *gormRepository) FindByID(ctx context.Context, id string) (*Warehouse, error) {
	var w Warehouse
	if err := r.db.WithContext(ctx).First(&w, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &w, nil
}

func (r *gormRepository) FindByCode(ctx context.Context, code string) (*Warehouse, error) {
	var w Warehouse
	if err := r.db.WithContext(ctx).First(&w, "code = ?", code).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &w, nil
}

func (r *gormRepository) List(ctx context.Context, status string, page, limit int) ([]Warehouse, int64, error) {
	q := r.db.WithContext(ctx).Model(&Warehouse{})
	if status != "" {
		q = q.Where("status = ?", status)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []Warehouse
	offset := (page - 1) * limit
	if err := q.Order("created_at desc").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *gormRepository) Update(ctx context.Context, w *Warehouse) error {
	return r.db.WithContext(ctx).Save(w).Error
}

func (r *gormRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&Warehouse{}, "id = ?", id).Error
}
