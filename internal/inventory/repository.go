package inventory

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("inventory: not found")

type ListFilter struct {
	ProductID   string
	WarehouseID string
	LowStock    bool // available_stock (quantity - reserved_quantity) <= 5
	Page        int
	Limit       int
}

type Repository interface {
	Create(ctx context.Context, inv *Inventory) error
	CreateTx(ctx context.Context, tx *gorm.DB, inv *Inventory) error
	FindByID(ctx context.Context, id string) (*Inventory, error)
	FindByProductWarehouse(ctx context.Context, productID, warehouseID string) (*Inventory, error)
	List(ctx context.Context, f ListFilter) ([]Inventory, int64, error)
	Update(ctx context.Context, inv *Inventory) error

	// LockForUpdate mengambil row inventory dengan SELECT ... FOR UPDATE di
	// dalam transaksi `tx` milik caller (spec Section 5: transaction boundary
	// dikontrol service, bukan repository). Ini fungsi inti yang dipakai
	// ulang InventoryService (stock-in/adjust) DAN OrderService (create
	// order) — caller WAJIB sudah berada di dalam db.Transaction(...).
	LockForUpdate(ctx context.Context, tx *gorm.DB, productID, warehouseID string) (*Inventory, error)
	UpdateTx(ctx context.Context, tx *gorm.DB, inv *Inventory) error
	CreateTransaction(ctx context.Context, tx *gorm.DB, t *Transaction) error
	ListTransactions(ctx context.Context, inventoryID string, page, limit int) ([]Transaction, int64, error)
}

type gormRepository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) Create(ctx context.Context, inv *Inventory) error {
	return r.db.WithContext(ctx).Create(inv).Error
}

func (r *gormRepository) CreateTx(ctx context.Context, tx *gorm.DB, inv *Inventory) error {
	return tx.WithContext(ctx).Create(inv).Error
}

func (r *gormRepository) FindByID(ctx context.Context, id string) (*Inventory, error) {
	var inv Inventory
	if err := r.db.WithContext(ctx).First(&inv, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &inv, nil
}

func (r *gormRepository) FindByProductWarehouse(ctx context.Context, productID, warehouseID string) (*Inventory, error) {
	var inv Inventory
	err := r.db.WithContext(ctx).
		First(&inv, "product_id = ? AND warehouse_id = ?", productID, warehouseID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &inv, nil
}

func (r *gormRepository) List(ctx context.Context, f ListFilter) ([]Inventory, int64, error) {
	q := r.db.WithContext(ctx).Model(&Inventory{})
	if f.ProductID != "" {
		q = q.Where("product_id = ?", f.ProductID)
	}
	if f.WarehouseID != "" {
		q = q.Where("warehouse_id = ?", f.WarehouseID)
	}
	if f.LowStock {
		q = q.Where("(quantity - reserved_quantity) <= 5")
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

	var items []Inventory
	offset := (page - 1) * limit
	if err := q.Order("created_at desc").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *gormRepository) Update(ctx context.Context, inv *Inventory) error {
	return r.db.WithContext(ctx).Save(inv).Error
}

// LockForUpdate memakai raw SQL SELECT ... FOR UPDATE (bukan GORM
// clause.Locking) agar perilaku locking predictable & eksplisit — ini query
// internal dengan parameter aman (bukan "query dari input user" yang
// dilarang spec Section 63). HARUS dipanggil di dalam transaksi `tx` yang
// SAMA dengan write berikutnya, supaya check-then-act (available stock)
// benar-benar atomic terhadap request konkuren lain.
func (r *gormRepository) LockForUpdate(ctx context.Context, tx *gorm.DB, productID, warehouseID string) (*Inventory, error) {
	var inv Inventory
	err := tx.WithContext(ctx).Raw(
		`SELECT * FROM inventories WHERE product_id = ? AND warehouse_id = ? FOR UPDATE`,
		productID, warehouseID,
	).Scan(&inv).Error
	if err != nil {
		return nil, err
	}
	if inv.ID == "" {
		return nil, ErrNotFound
	}
	return &inv, nil
}

// UpdateTx sama seperti Update tapi lewat *gorm.DB tx milik caller, dipakai
// setelah LockForUpdate di dalam transaksi yang sama (bukan transaksi baru).
func (r *gormRepository) UpdateTx(ctx context.Context, tx *gorm.DB, inv *Inventory) error {
	return tx.WithContext(ctx).Save(inv).Error
}

func (r *gormRepository) CreateTransaction(ctx context.Context, tx *gorm.DB, t *Transaction) error {
	return tx.WithContext(ctx).Create(t).Error
}

func (r *gormRepository) ListTransactions(ctx context.Context, inventoryID string, page, limit int) ([]Transaction, int64, error) {
	q := r.db.WithContext(ctx).Model(&Transaction{}).Where("inventory_id = ?", inventoryID)

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

	var items []Transaction
	offset := (page - 1) * limit
	if err := q.Order("created_at desc").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}
