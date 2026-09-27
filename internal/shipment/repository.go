package shipment

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("shipment: not found")

type Repository interface {
	Create(ctx context.Context, tx *gorm.DB, s *Shipment) error
	FindByID(ctx context.Context, id string) (*Shipment, error)
	// LockForUpdate — lihat internal/inventory/repository.go untuk rationale
	// yang sama: raw SQL SELECT...FOR UPDATE di dalam tx milik caller.
	LockForUpdate(ctx context.Context, tx *gorm.DB, id string) (*Shipment, error)
	UpdateTx(ctx context.Context, tx *gorm.DB, s *Shipment) error
}

type gormRepository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) Create(ctx context.Context, tx *gorm.DB, s *Shipment) error {
	return tx.WithContext(ctx).Create(s).Error
}

func (r *gormRepository) FindByID(ctx context.Context, id string) (*Shipment, error) {
	var s Shipment
	if err := r.db.WithContext(ctx).First(&s, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &s, nil
}

func (r *gormRepository) LockForUpdate(ctx context.Context, tx *gorm.DB, id string) (*Shipment, error) {
	var s Shipment
	err := tx.WithContext(ctx).Raw(`SELECT * FROM shipments WHERE id = ? FOR UPDATE`, id).Scan(&s).Error
	if err != nil {
		return nil, err
	}
	if s.ID == "" {
		return nil, ErrNotFound
	}
	return &s, nil
}

func (r *gormRepository) UpdateTx(ctx context.Context, tx *gorm.DB, s *Shipment) error {
	return tx.WithContext(ctx).Save(s).Error
}
