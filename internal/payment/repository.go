package payment

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("payment: not found")

type Repository interface {
	Create(ctx context.Context, p *Payment) error
	FindByID(ctx context.Context, id string) (*Payment, error)
	ExistsPaidForOrder(ctx context.Context, orderID string) (bool, error)

	// LockForUpdateByTransactionID mengunci row payment (FOR UPDATE) dalam
	// tx yang sama dengan callback — dipakai untuk memeriksa status SEBELUM
	// efek samping apa pun (spec Section 42, idempotency callback).
	LockForUpdateByTransactionID(ctx context.Context, tx *gorm.DB, transactionID string) (*Payment, error)
	LockForUpdateByID(ctx context.Context, tx *gorm.DB, id string) (*Payment, error)
	UpdateTx(ctx context.Context, tx *gorm.DB, p *Payment) error

	// UpdateStatusTxIfPending menjalankan UPDATE ... WHERE status='PENDING'
	// (bukan cuma dicek di SELECT) — idempotency guard eksplisit untuk
	// worker expiration (spec Iterasi 06): kalau worker run 2x bersamaan,
	// hanya satu yang benar-benar mengubah row.
	UpdateStatusTxIfPending(ctx context.Context, tx *gorm.DB, id string, newStatus Status) (bool, error)

	// ListPendingExpired mencari kandidat (SELECT polos, tanpa lock) untuk
	// worker expiration — locking sesungguhnya terjadi per-row lewat
	// LockForUpdateByID di dalam transaksi masing-masing.
	ListPendingExpired(ctx context.Context, before time.Time, limit int) ([]Payment, error)
}

type gormRepository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) Create(ctx context.Context, p *Payment) error {
	return r.db.WithContext(ctx).Create(p).Error
}

func (r *gormRepository) FindByID(ctx context.Context, id string) (*Payment, error) {
	var p Payment
	if err := r.db.WithContext(ctx).First(&p, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (r *gormRepository) ExistsPaidForOrder(ctx context.Context, orderID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Payment{}).
		Where("order_id = ? AND status = ?", orderID, StatusPaid).
		Count(&count).Error
	return count > 0, err
}

func (r *gormRepository) LockForUpdateByTransactionID(ctx context.Context, tx *gorm.DB, transactionID string) (*Payment, error) {
	var p Payment
	err := tx.WithContext(ctx).Raw(
		`SELECT * FROM payments WHERE transaction_id = ? FOR UPDATE`, transactionID,
	).Scan(&p).Error
	if err != nil {
		return nil, err
	}
	if p.ID == "" {
		return nil, ErrNotFound
	}
	return &p, nil
}

func (r *gormRepository) LockForUpdateByID(ctx context.Context, tx *gorm.DB, id string) (*Payment, error) {
	var p Payment
	err := tx.WithContext(ctx).Raw(
		`SELECT * FROM payments WHERE id = ? FOR UPDATE`, id,
	).Scan(&p).Error
	if err != nil {
		return nil, err
	}
	if p.ID == "" {
		return nil, ErrNotFound
	}
	return &p, nil
}

func (r *gormRepository) UpdateTx(ctx context.Context, tx *gorm.DB, p *Payment) error {
	return tx.WithContext(ctx).Save(p).Error
}

func (r *gormRepository) UpdateStatusTxIfPending(ctx context.Context, tx *gorm.DB, id string, newStatus Status) (bool, error) {
	result := tx.WithContext(ctx).Exec(
		`UPDATE payments SET status = ?, updated_at = now() WHERE id = ? AND status = ?`,
		newStatus, id, StatusPending,
	)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func (r *gormRepository) ListPendingExpired(ctx context.Context, before time.Time, limit int) ([]Payment, error) {
	var items []Payment
	err := r.db.WithContext(ctx).
		Where("status = ? AND expired_at < ?", StatusPending, before).
		Order("expired_at asc").
		Limit(limit).
		Find(&items).Error
	return items, err
}
