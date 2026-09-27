package inventory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"gorm.io/gorm"

	apperr "order-management/pkg/errors"
)

// fakeRepo adalah in-memory Repository. Parameter tx (*gorm.DB) diterima
// tapi tidak pernah didereference — cukup untuk menguji ARITMATIKA business
// rule (spec Section 37). Perilaku row-level lock PostgreSQL sesungguhnya
// baru divalidasi dengan Testcontainers Postgres nyata di Iterasi 05.
type fakeRepo struct {
	items       map[string]*Inventory
	byProductWh map[string]*Inventory
	txs         []Transaction
	nextID      int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{items: map[string]*Inventory{}, byProductWh: map[string]*Inventory{}}
}

func key(productID, warehouseID string) string { return productID + "|" + warehouseID }

func (f *fakeRepo) seed(inv *Inventory) {
	f.nextID++
	inv.ID = fmt.Sprintf("inv-%d", f.nextID)
	stored := *inv
	f.items[inv.ID] = &stored
	f.byProductWh[key(inv.ProductID, inv.WarehouseID)] = &stored
	*inv = stored
}

func (f *fakeRepo) Create(ctx context.Context, inv *Inventory) error {
	f.seed(inv)
	return nil
}

func (f *fakeRepo) CreateTx(ctx context.Context, tx *gorm.DB, inv *Inventory) error {
	if _, exists := f.byProductWh[key(inv.ProductID, inv.WarehouseID)]; exists {
		return errors.New("fakeRepo: duplicate product_id+warehouse_id")
	}
	f.seed(inv)
	return nil
}

func (f *fakeRepo) FindByID(ctx context.Context, id string) (*Inventory, error) {
	if inv, ok := f.items[id]; ok {
		c := *inv
		return &c, nil
	}
	return nil, ErrNotFound
}

func (f *fakeRepo) FindByProductWarehouse(ctx context.Context, productID, warehouseID string) (*Inventory, error) {
	if inv, ok := f.byProductWh[key(productID, warehouseID)]; ok {
		c := *inv
		return &c, nil
	}
	return nil, ErrNotFound
}

func (f *fakeRepo) List(ctx context.Context, filter ListFilter) ([]Inventory, int64, error) {
	return nil, 0, nil
}

func (f *fakeRepo) Update(ctx context.Context, inv *Inventory) error {
	f.store(inv)
	return nil
}

func (f *fakeRepo) LockForUpdate(ctx context.Context, tx *gorm.DB, productID, warehouseID string) (*Inventory, error) {
	inv, ok := f.byProductWh[key(productID, warehouseID)]
	if !ok {
		return nil, ErrNotFound
	}
	c := *inv
	return &c, nil
}

func (f *fakeRepo) UpdateTx(ctx context.Context, tx *gorm.DB, inv *Inventory) error {
	f.store(inv)
	return nil
}

func (f *fakeRepo) store(inv *Inventory) {
	c := *inv
	f.items[inv.ID] = &c
	f.byProductWh[key(inv.ProductID, inv.WarehouseID)] = &c
}

func (f *fakeRepo) CreateTransaction(ctx context.Context, tx *gorm.DB, t *Transaction) error {
	f.txs = append(f.txs, *t)
	return nil
}

func (f *fakeRepo) ListTransactions(ctx context.Context, inventoryID string, page, limit int) ([]Transaction, int64, error) {
	var out []Transaction
	for _, t := range f.txs {
		if t.InventoryID == inventoryID {
			out = append(out, t)
		}
	}
	return out, int64(len(out)), nil
}

// fakeTxRunner menjalankan fc langsung tanpa membuka koneksi database nyata
// — cukup untuk menguji urutan langkah & aritmatika service, bukan atomicity
// sungguhan (itu tugas Testcontainers di Iterasi 05).
type fakeTxRunner struct{}

func (fakeTxRunner) Transaction(fc func(tx *gorm.DB) error, opts ...*sql.TxOptions) error {
	return fc(nil)
}

func seedInventory(t *testing.T, repo *fakeRepo, productID, warehouseID string, quantity, reserved int) {
	t.Helper()
	inv := &Inventory{ProductID: productID, WarehouseID: warehouseID, Quantity: quantity, ReservedQuantity: reserved}
	if err := repo.Create(context.Background(), inv); err != nil {
		t.Fatalf("seed inventory failed: %v", err)
	}
}

// TestReserveStock_Section37Scenario mereproduksi tepat contoh spec Section
// 37: quantity=10, reserved=3 (available=7) -> reserve 5 -> sukses,
// reserved=8, available=2 -> reserve 3 lagi -> DITOLAK karena available
// sebelumnya hanya 2.
func TestReserveStock_Section37Scenario(t *testing.T) {
	repo := newFakeRepo()
	seedInventory(t, repo, "product-1", "warehouse-1", 10, 3)
	svc := NewService(repo, fakeTxRunner{}, nil)
	ctx := context.Background()

	if err := svc.ReserveStock(ctx, nil, "product-1", "warehouse-1", 5, "order-1"); err != nil {
		t.Fatalf("expected reserve of 5 to succeed (available was 7), got error: %v", err)
	}

	inv, err := repo.FindByProductWarehouse(ctx, "product-1", "warehouse-1")
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if inv.ReservedQuantity != 8 {
		t.Fatalf("expected reserved_quantity=8, got %d", inv.ReservedQuantity)
	}
	if inv.AvailableStock() != 2 {
		t.Fatalf("expected available_stock=2, got %d", inv.AvailableStock())
	}

	err = svc.ReserveStock(ctx, nil, "product-1", "warehouse-1", 3, "order-2")
	if err == nil {
		t.Fatal("expected reserve of 3 to be rejected (available was only 2), got success")
	}
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.ErrCode != apperr.InsufficientStock {
		t.Fatalf("expected INSUFFICIENT_STOCK error, got: %v", err)
	}

	// Quantity fisik tidak boleh berubah oleh reservasi yang ditolak.
	inv, _ = repo.FindByProductWarehouse(ctx, "product-1", "warehouse-1")
	if inv.Quantity != 10 || inv.ReservedQuantity != 8 {
		t.Fatalf("state should be unchanged after rejected reserve, got quantity=%d reserved=%d", inv.Quantity, inv.ReservedQuantity)
	}
}

func TestReleaseStock_RestoresReservedQuantity(t *testing.T) {
	repo := newFakeRepo()
	seedInventory(t, repo, "product-1", "warehouse-1", 10, 8)
	svc := NewService(repo, fakeTxRunner{}, nil)
	ctx := context.Background()

	if err := svc.ReleaseStock(ctx, nil, "product-1", "warehouse-1", 5, "order-1"); err != nil {
		t.Fatalf("release failed: %v", err)
	}

	inv, _ := repo.FindByProductWarehouse(ctx, "product-1", "warehouse-1")
	if inv.ReservedQuantity != 3 {
		t.Fatalf("expected reserved_quantity=3 after release, got %d", inv.ReservedQuantity)
	}
}

func TestStockIn_CreatesInventoryWhenNotExists(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, fakeTxRunner{}, nil)

	resp, err := svc.StockIn(context.Background(), StockInRequest{
		ProductID: "product-new", WarehouseID: "warehouse-1", Quantity: 20,
	})
	if err != nil {
		t.Fatalf("stock-in failed: %v", err)
	}
	if resp.Quantity != 20 || resp.ReservedQuantity != 0 {
		t.Fatalf("expected fresh inventory quantity=20 reserved=0, got quantity=%d reserved=%d", resp.Quantity, resp.ReservedQuantity)
	}
	if len(repo.txs) != 1 || repo.txs[0].Type != TransactionStockIn {
		t.Fatalf("expected exactly 1 STOCK_IN transaction record, got %+v", repo.txs)
	}
}

func TestStockIn_AddsToExistingInventory(t *testing.T) {
	repo := newFakeRepo()
	seedInventory(t, repo, "product-1", "warehouse-1", 10, 0)
	svc := NewService(repo, fakeTxRunner{}, nil)

	resp, err := svc.StockIn(context.Background(), StockInRequest{
		ProductID: "product-1", WarehouseID: "warehouse-1", Quantity: 5,
	})
	if err != nil {
		t.Fatalf("stock-in failed: %v", err)
	}
	if resp.Quantity != 15 {
		t.Fatalf("expected quantity=15 after stock-in, got %d", resp.Quantity)
	}
}

func TestAdjust_ResultingNegative_Rejected(t *testing.T) {
	repo := newFakeRepo()
	seedInventory(t, repo, "product-1", "warehouse-1", 5, 0)
	svc := NewService(repo, fakeTxRunner{}, nil)

	_, err := svc.Adjust(context.Background(), "admin-1", AdjustRequest{
		ProductID: "product-1", WarehouseID: "warehouse-1", Quantity: -10, Description: "koreksi stok opname",
	})
	if err == nil {
		t.Fatal("expected adjustment resulting in negative quantity to be rejected")
	}
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.ErrCode != apperr.InventoryNegative {
		t.Fatalf("expected INVENTORY_NEGATIVE error, got: %v", err)
	}

	inv, _ := repo.FindByProductWarehouse(context.Background(), "product-1", "warehouse-1")
	if inv.Quantity != 5 {
		t.Fatalf("quantity should be unchanged after rejected adjustment, got %d", inv.Quantity)
	}
}

func TestAdjust_NegativeDelta_ValidResult_Succeeds(t *testing.T) {
	repo := newFakeRepo()
	seedInventory(t, repo, "product-1", "warehouse-1", 10, 0)
	svc := NewService(repo, fakeTxRunner{}, nil)

	resp, err := svc.Adjust(context.Background(), "admin-1", AdjustRequest{
		ProductID: "product-1", WarehouseID: "warehouse-1", Quantity: -3, Description: "barang rusak",
	})
	if err != nil {
		t.Fatalf("adjust failed: %v", err)
	}
	if resp.Quantity != 7 {
		t.Fatalf("expected quantity=7 after adjustment of -3, got %d", resp.Quantity)
	}
	if len(repo.txs) != 1 || repo.txs[0].Type != TransactionAdjustment {
		t.Fatalf("expected exactly 1 ADJUSTMENT transaction record, got %+v", repo.txs)
	}
}
