package order

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("order: not found")

type ListFilter struct {
	CustomerID string
	Status     string
	Page       int
	Limit      int
}

type Repository interface {
	// NextOrderNumber mengimplementasikan Locked Decision "Order number
	// generation" (03-BACKEND-CORE.md Section 3): SELECT...FOR UPDATE pada
	// counter harian, increment, format ORD-YYYYMMDD-XXXXXX. HARUS dipanggil
	// di dalam transaksi `tx` yang sama dengan create order.
	NextOrderNumber(ctx context.Context, tx *gorm.DB, date time.Time) (string, error)

	// Create menyimpan order + seluruh order_items sekaligus (GORM nested
	// insert lewat association Items) di dalam transaksi `tx`.
	Create(ctx context.Context, tx *gorm.DB, o *Order) error

	FindByID(ctx context.Context, id string) (*Order, error)
	List(ctx context.Context, f ListFilter) ([]Order, int64, error)

	// LockForUpdate mengunci SATU row order (tanpa items) di dalam tx —
	// dipakai Cancel supaya dua request cancel/pack/ship konkuren pada order
	// yang sama tidak saling menimpa status.
	LockForUpdate(ctx context.Context, tx *gorm.DB, id string) (*Order, error)
	UpdateStatus(ctx context.Context, tx *gorm.DB, id string, status Status) error
	ItemsByOrderID(ctx context.Context, tx *gorm.DB, orderID string) ([]OrderItem, error)
}

type gormRepository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

// NextOrderNumber: upsert row kosong dulu (ON CONFLICT DO NOTHING, aman
// dipanggil berkali-kali) supaya SELECT FOR UPDATE di bawah selalu punya row
// untuk dikunci walau ini order pertama di tanggal tersebut.
func (r *gormRepository) NextOrderNumber(ctx context.Context, tx *gorm.DB, date time.Time) (string, error) {
	dateStr := date.Format("2006-01-02")

	if err := tx.WithContext(ctx).Exec(
		`INSERT INTO order_number_sequences (sequence_date, last_number) VALUES (?, 0) ON CONFLICT (sequence_date) DO NOTHING`,
		dateStr,
	).Error; err != nil {
		return "", err
	}

	var lastNumber int
	if err := tx.WithContext(ctx).Raw(
		`SELECT last_number FROM order_number_sequences WHERE sequence_date = ? FOR UPDATE`,
		dateStr,
	).Scan(&lastNumber).Error; err != nil {
		return "", err
	}

	nextNumber := lastNumber + 1
	if err := tx.WithContext(ctx).Exec(
		`UPDATE order_number_sequences SET last_number = ? WHERE sequence_date = ?`,
		nextNumber, dateStr,
	).Error; err != nil {
		return "", err
	}

	return fmt.Sprintf("ORD-%s-%06d", date.Format("20060102"), nextNumber), nil
}

func (r *gormRepository) Create(ctx context.Context, tx *gorm.DB, o *Order) error {
	return tx.WithContext(ctx).Create(o).Error
}

func (r *gormRepository) FindByID(ctx context.Context, id string) (*Order, error) {
	var o Order
	if err := r.db.WithContext(ctx).Preload("Items").First(&o, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &o, nil
}

func (r *gormRepository) List(ctx context.Context, f ListFilter) ([]Order, int64, error) {
	q := r.db.WithContext(ctx).Model(&Order{})
	if f.CustomerID != "" {
		q = q.Where("customer_id = ?", f.CustomerID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page, limit := f.Page, f.Limit
	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var items []Order
	offset := (page - 1) * limit
	if err := q.Order("created_at desc").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *gormRepository) LockForUpdate(ctx context.Context, tx *gorm.DB, id string) (*Order, error) {
	var o Order
	err := tx.WithContext(ctx).Raw(`SELECT * FROM orders WHERE id = ? FOR UPDATE`, id).Scan(&o).Error
	if err != nil {
		return nil, err
	}
	if o.ID == "" {
		return nil, ErrNotFound
	}
	return &o, nil
}

func (r *gormRepository) UpdateStatus(ctx context.Context, tx *gorm.DB, id string, status Status) error {
	return tx.WithContext(ctx).Model(&Order{}).Where("id = ?", id).Update("status", status).Error
}

func (r *gormRepository) ItemsByOrderID(ctx context.Context, tx *gorm.DB, orderID string) ([]OrderItem, error) {
	var items []OrderItem
	if err := tx.WithContext(ctx).Where("order_id = ?", orderID).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}
