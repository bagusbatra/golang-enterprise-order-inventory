package audit

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("audit: not found")

type Repository interface {
	Create(ctx context.Context, a *AuditLog) error
	FindByID(ctx context.Context, id string) (*AuditLog, error)
	List(ctx context.Context, entity, userID string, page, limit int) ([]AuditLog, int64, error)
}

type gormRepository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) Create(ctx context.Context, a *AuditLog) error {
	return r.db.WithContext(ctx).Create(a).Error
}

func (r *gormRepository) FindByID(ctx context.Context, id string) (*AuditLog, error) {
	var a AuditLog
	if err := r.db.WithContext(ctx).First(&a, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &a, nil
}

func (r *gormRepository) List(ctx context.Context, entity, userID string, page, limit int) ([]AuditLog, int64, error) {
	q := r.db.WithContext(ctx).Model(&AuditLog{})
	if entity != "" {
		q = q.Where("entity = ?", entity)
	}
	if userID != "" {
		q = q.Where("user_id = ?", userID)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var items []AuditLog
	offset := (page - 1) * limit
	if err := q.Order("created_at desc").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}
