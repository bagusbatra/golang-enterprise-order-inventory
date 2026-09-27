package inventory

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"gorm.io/gorm"

	apperr "order-management/pkg/errors"

	"order-management/pkg/audit"
)

// TxRunner adalah subset method *gorm.DB yang dipakai Service untuk membuka
// transaksi. Diabstraksi jadi interface (bukan langsung *gorm.DB) supaya
// service_test.go bisa memakai fake yang menjalankan fc langsung tanpa
// koneksi database nyata — locking behavior sesungguhnya baru divalidasi
// dengan Testcontainers Postgres nyata di Iterasi 05 (concurrency test).
type TxRunner interface {
	Transaction(fc func(tx *gorm.DB) error, opts ...*sql.TxOptions) error
}

type Service struct {
	repo  Repository
	db    TxRunner
	audit audit.Logger
}

func NewService(repo Repository, db TxRunner, auditLogger audit.Logger) *Service {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}
	return &Service{repo: repo, db: db, audit: auditLogger}
}

// StockIn menambah quantity fisik (spec Section 31). Membuat row inventory
// baru (quantity=0) jika kombinasi product/warehouse ini belum pernah
// tercatat, supaya endpoint ini juga berfungsi sebagai "onboarding" stok
// pertama kali untuk kombinasi tersebut.
func (s *Service) StockIn(ctx context.Context, req StockInRequest) (*Response, error) {
	var result Response
	err := s.db.Transaction(func(tx *gorm.DB) error {
		inv, err := s.lockOrCreate(ctx, tx, req.ProductID, req.WarehouseID)
		if err != nil {
			return err
		}

		inv.Quantity += req.Quantity
		if err := s.repo.UpdateTx(ctx, tx, inv); err != nil {
			return err
		}

		desc := req.Description
		if err := s.repo.CreateTransaction(ctx, tx, &Transaction{
			InventoryID: inv.ID,
			Type:        TransactionStockIn,
			Quantity:    req.Quantity,
			Description: nilIfEmpty(desc),
		}); err != nil {
			return err
		}

		result = toResponse(inv)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// Adjust mengubah quantity dengan delta yang bisa negatif (spec Section 32),
// ADMIN only (di-enforce middleware RBAC, bukan di sini). Hasil quantity
// tidak boleh negatif — divalidasi DI BAWAH row lock supaya tidak ada race
// antara cek dan tulis. Audit log dicatat SETELAH commit sukses, mengikuti
// prinsip yang sama dengan publish event (architecture.md Section 3):
// kegagalan audit log tidak boleh membatalkan perubahan stok yang sah.
func (s *Service) Adjust(ctx context.Context, actorUserID string, req AdjustRequest) (*Response, error) {
	var result Response
	var oldQty int
	var invID string

	err := s.db.Transaction(func(tx *gorm.DB) error {
		inv, err := s.repo.LockForUpdate(ctx, tx, req.ProductID, req.WarehouseID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return apperr.New(http.StatusNotFound, apperr.InventoryNotFound, "Inventory not found")
			}
			return err
		}

		oldQty = inv.Quantity
		newQty := inv.Quantity + req.Quantity
		if newQty < 0 {
			return apperr.New(http.StatusUnprocessableEntity, apperr.InventoryNegative, "Adjustment would result in negative quantity")
		}

		invID = inv.ID
		inv.Quantity = newQty
		if err := s.repo.UpdateTx(ctx, tx, inv); err != nil {
			return err
		}

		desc := req.Description
		if err := s.repo.CreateTransaction(ctx, tx, &Transaction{
			InventoryID: inv.ID,
			Type:        TransactionAdjustment,
			Quantity:    req.Quantity,
			Description: &desc,
		}); err != nil {
			return err
		}

		result = toResponse(inv)
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.logAdjustAudit(ctx, actorUserID, invID, oldQty, result.Quantity, req.Description)
	return &result, nil
}

func (s *Service) GetByID(ctx context.Context, id string) (*Response, error) {
	inv, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, apperr.New(http.StatusNotFound, apperr.InventoryNotFound, "Inventory not found")
		}
		return nil, err
	}
	resp := toResponse(inv)
	return &resp, nil
}

func (s *Service) List(ctx context.Context, f ListFilter) ([]Response, int64, error) {
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

func (s *Service) ListTransactions(ctx context.Context, inventoryID string, page, limit int) ([]TransactionResponse, int64, error) {
	if _, err := s.repo.FindByID(ctx, inventoryID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, 0, apperr.New(http.StatusNotFound, apperr.InventoryNotFound, "Inventory not found")
		}
		return nil, 0, err
	}

	items, total, err := s.repo.ListTransactions(ctx, inventoryID, page, limit)
	if err != nil {
		return nil, 0, err
	}
	resp := make([]TransactionResponse, 0, len(items))
	for i := range items {
		resp = append(resp, toTransactionResponse(&items[i]))
	}
	return resp, total, nil
}

// CheckAndLock mengunci row inventory (FOR UPDATE) di dalam tx yang sama dan
// memvalidasi available stock cukup untuk qty, TANPA mengubah reserved_
// quantity. Dipakai Order Service di Pass 1 create-order: lock SEMUA item
// urut product_id ASC dan pastikan seluruh item valid dulu, SEBELUM menulis
// order/order_items — supaya order yang gagal validasi tidak pernah
// meninggalkan row order setengah jadi (lihat Service.ReserveStock untuk
// Pass 2 yang benar-benar mereservasi setelah order dibuat).
func (s *Service) CheckAndLock(ctx context.Context, tx *gorm.DB, productID, warehouseID string, qty int) error {
	inv, err := s.repo.LockForUpdate(ctx, tx, productID, warehouseID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apperr.New(http.StatusNotFound, apperr.ProductNotFound, "Product not found in this warehouse")
		}
		return err
	}
	if inv.AvailableStock() < qty {
		return apperr.New(http.StatusConflict, apperr.InsufficientStock, "Insufficient stock")
	}
	return nil
}

// ReserveStock adalah fungsi inti yang dipakai ulang Order Service saat
// create-order (spec Section 34/38). HARUS dipanggil di dalam transaksi
// (tx) yang sama dengan lock yang sudah/akan diambil caller — fungsi ini
// mengambil lock-nya sendiri lewat LockForUpdate, yang aman di-re-entry
// dalam transaksi PostgreSQL yang sama (bukan deadlock, hanya row lock yang
// sudah dipegang koneksi/transaksi ini sendiri).
//
// Assumption terdokumentasi: signature di 03-BACKEND-CORE.md Section 6.3
// tidak menyertakan referenceID; ditambahkan di sini agar inventory_
// transactions type RESERVE tetap traceable ke order_id sesuai database-
// contract.md Section 6 (reference_type/reference_id) — tanpa ini jejak
// audit reservasi tidak bisa ditelusuri balik ke order mana asalnya.
func (s *Service) ReserveStock(ctx context.Context, tx *gorm.DB, productID, warehouseID string, qty int, referenceID string) error {
	inv, err := s.repo.LockForUpdate(ctx, tx, productID, warehouseID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apperr.New(http.StatusNotFound, apperr.ProductNotFound, "Product not found in this warehouse")
		}
		return err
	}

	if inv.AvailableStock() < qty {
		return apperr.New(http.StatusConflict, apperr.InsufficientStock, "Insufficient stock")
	}

	inv.ReservedQuantity += qty
	if err := s.repo.UpdateTx(ctx, tx, inv); err != nil {
		return err
	}

	refType := "ORDER"
	return s.repo.CreateTransaction(ctx, tx, &Transaction{
		InventoryID:   inv.ID,
		Type:          TransactionReserve,
		Quantity:      qty,
		ReferenceType: &refType,
		ReferenceID:   &referenceID,
	})
}

// ReleaseStock membalikkan reservasi (cancellation Section 43, expiration
// Section 44) — mengurangi reserved_quantity, TIDAK menyentuh quantity fisik
// (barang belum pernah keluar gudang untuk order yang belum PAID).
func (s *Service) ReleaseStock(ctx context.Context, tx *gorm.DB, productID, warehouseID string, qty int, referenceID string) error {
	inv, err := s.repo.LockForUpdate(ctx, tx, productID, warehouseID)
	if err != nil {
		return err
	}

	inv.ReservedQuantity -= qty
	if inv.ReservedQuantity < 0 {
		inv.ReservedQuantity = 0
	}
	if err := s.repo.UpdateTx(ctx, tx, inv); err != nil {
		return err
	}

	refType := "ORDER"
	return s.repo.CreateTransaction(ctx, tx, &Transaction{
		InventoryID:   inv.ID,
		Type:          TransactionRelease,
		Quantity:      qty,
		ReferenceType: &refType,
		ReferenceID:   &referenceID,
	})
}

func (s *Service) lockOrCreate(ctx context.Context, tx *gorm.DB, productID, warehouseID string) (*Inventory, error) {
	inv, err := s.repo.LockForUpdate(ctx, tx, productID, warehouseID)
	if err == nil {
		return inv, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	newInv := &Inventory{ProductID: productID, WarehouseID: warehouseID, Quantity: 0, ReservedQuantity: 0}
	if createErr := s.repo.CreateTx(ctx, tx, newInv); createErr != nil {
		// Kemungkinan race: request lain membuat row yang sama lebih dulu
		// (melanggar UNIQUE(product_id,warehouse_id)) -> lock row yang sudah
		// ada itu alih-alih gagal total.
		locked, lockErr := s.repo.LockForUpdate(ctx, tx, productID, warehouseID)
		if lockErr != nil {
			return nil, createErr
		}
		return locked, nil
	}
	return newInv, nil
}

func (s *Service) logAdjustAudit(ctx context.Context, actorUserID, inventoryID string, oldQty, newQty int, description string) {
	var userIDPtr *string
	if actorUserID != "" {
		userIDPtr = &actorUserID
	}
	_ = s.audit.Log(ctx, audit.Entry{
		UserID:   userIDPtr,
		Action:   "INVENTORY_ADJUST",
		Entity:   "inventory",
		EntityID: &inventoryID,
		OldData:  map[string]int{"quantity": oldQty},
		NewData:  map[string]any{"quantity": newQty, "description": description},
	})
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
