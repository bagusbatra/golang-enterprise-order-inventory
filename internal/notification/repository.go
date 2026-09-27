package notification

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("notification: not found")

type Repository interface {
	Create(ctx context.Context, n *Notification) error
	// ExistsByUserTypeMessage adalah dedupe check idempotency (lihat
	// Service.CreateIfNotExists untuk rationale).
	ExistsByUserTypeMessage(ctx context.Context, userID, notifType, message string) (bool, error)
	FindByID(ctx context.Context, id string) (*Notification, error)
	List(ctx context.Context, userID string, isRead *bool, page, limit int) ([]Notification, int64, error)
	MarkRead(ctx context.Context, id string) error
	MarkAllRead(ctx context.Context, userID string) error
}

type gormRepository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) Create(ctx context.Context, n *Notification) error {
	return r.db.WithContext(ctx).Create(n).Error
}

func (r *gormRepository) ExistsByUserTypeMessage(ctx context.Context, userID, notifType, message string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Notification{}).
		Where("user_id = ? AND type = ? AND message = ?", userID, notifType, message).
		Count(&count).Error
	return count > 0, err
}

func (r *gormRepository) FindByID(ctx context.Context, id string) (*Notification, error) {
	var n Notification
	if err := r.db.WithContext(ctx).First(&n, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &n, nil
}

func (r *gormRepository) List(ctx context.Context, userID string, isRead *bool, page, limit int) ([]Notification, int64, error) {
	q := r.db.WithContext(ctx).Model(&Notification{}).Where("user_id = ?", userID)
	if isRead != nil {
		q = q.Where("is_read = ?", *isRead)
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

	var items []Notification
	offset := (page - 1) * limit
	if err := q.Order("created_at desc").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *gormRepository) MarkRead(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Model(&Notification{}).Where("id = ?", id).Update("is_read", true).Error
}

func (r *gormRepository) MarkAllRead(ctx context.Context, userID string) error {
	return r.db.WithContext(ctx).Model(&Notification{}).
		Where("user_id = ? AND is_read = ?", userID, false).
		Update("is_read", true).Error
}
