// Package concurrency membuktikan requirement TERPENTING seluruh project
// (golang-enterprise-order-inventory-study-case.md Section 38/79/96):
//
//	Initial stock = 10
//	100 concurrent request, masing-masing quantity = 1
//	Expected: EXACTLY 10 success, 90 failed
//	Tidak boleh: stock < 0, success > 10, duplicate reservation
//
// WAJIB real PostgreSQL (Testcontainers) — locking behavior tidak valid
// dites dengan mock/fake repository (itu hanya membuktikan aritmatika,
// bukan row-level lock sungguhan di bawah concurrent access nyata).
package concurrency

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"order-management/internal/category"
	"order-management/internal/inventory"
	"order-management/internal/order"
	"order-management/internal/product"
	"order-management/internal/user"
	"order-management/internal/warehouse"
	apperr "order-management/pkg/errors"
	"order-management/tests/testutil"
)

type seeded struct {
	CustomerID  string
	WarehouseID string
	ProductID   string
}

func seedBaseData(t *testing.T, db *gorm.DB, initialStock int) seeded {
	t.Helper()
	ctx := context.Background()

	cat := &category.Category{Name: "Electronics"}
	if err := db.WithContext(ctx).Create(cat).Error; err != nil {
		t.Fatalf("seed category failed: %v", err)
	}

	prod := &product.Product{
		CategoryID: cat.ID,
		SKU:        "SKU-CONCURRENCY-TEST",
		Name:       "Concurrency Test Product",
		Price:      decimal.NewFromInt(100000),
		CostPrice:  decimal.NewFromInt(50000),
		Weight:     decimal.NewFromInt(1),
		Status:     product.StatusActive,
	}
	if err := db.WithContext(ctx).Create(prod).Error; err != nil {
		t.Fatalf("seed product failed: %v", err)
	}

	wh := &warehouse.Warehouse{
		Code:    "WH-TEST",
		Name:    "Test Warehouse",
		Address: "Jl. Test No. 1",
		City:    "Surabaya",
		Status:  warehouse.StatusActive,
	}
	if err := db.WithContext(ctx).Create(wh).Error; err != nil {
		t.Fatalf("seed warehouse failed: %v", err)
	}

	cust := &user.User{
		Name:         "Concurrency Test Customer",
		Email:        "concurrency-test@example.com",
		PasswordHash: "not-a-real-hash",
		Role:         user.RoleCustomer,
		Status:       user.StatusActive,
	}
	if err := db.WithContext(ctx).Create(cust).Error; err != nil {
		t.Fatalf("seed customer failed: %v", err)
	}

	inv := &inventory.Inventory{
		ProductID:        prod.ID,
		WarehouseID:      wh.ID,
		Quantity:         initialStock,
		ReservedQuantity: 0,
	}
	if err := db.WithContext(ctx).Create(inv).Error; err != nil {
		t.Fatalf("seed inventory failed: %v", err)
	}

	return seeded{CustomerID: cust.ID, WarehouseID: wh.ID, ProductID: prod.ID}
}

// resetInventoryAndOrders mengembalikan state ke stock awal & membersihkan
// order/order_items/inventory_transactions, supaya skenario 100-goroutine
// bisa diulang beberapa kali (spec: "jalankan berkali-kali, minimal 5x, untuk
// pastikan tidak flaky") TANPA perlu membuat ulang container Postgres setiap
// kali (jauh lebih cepat).
func resetInventoryAndOrders(t *testing.T, db *gorm.DB, s seeded, initialStock int) {
	t.Helper()
	if err := db.Exec(`DELETE FROM inventory_transactions`).Error; err != nil {
		t.Fatalf("reset inventory_transactions failed: %v", err)
	}
	if err := db.Exec(`DELETE FROM order_items`).Error; err != nil {
		t.Fatalf("reset order_items failed: %v", err)
	}
	if err := db.Exec(`DELETE FROM orders`).Error; err != nil {
		t.Fatalf("reset orders failed: %v", err)
	}
	if err := db.Exec(`DELETE FROM order_number_sequences`).Error; err != nil {
		t.Fatalf("reset order_number_sequences failed: %v", err)
	}
	if err := db.Exec(
		`UPDATE inventories SET quantity = ?, reserved_quantity = 0 WHERE product_id = ? AND warehouse_id = ?`,
		initialStock, s.ProductID, s.WarehouseID,
	).Error; err != nil {
		t.Fatalf("reset inventory row failed: %v", err)
	}
}

func buildOrderService(db *gorm.DB, redisClient *redis.Client) *order.Service {
	warehouseService := warehouse.NewService(warehouse.NewRepository(db))
	productService := product.NewService(product.NewRepository(db), redisClient, 10*time.Minute)
	inventoryService := inventory.NewService(inventory.NewRepository(db), db, nil)
	return order.NewService(order.NewRepository(db), db, warehouseService, productService, inventoryService)
}

// TestConcurrentCheckout_ExactlyTenSucceed adalah TEST WAJIB LOLOS sebelum
// domain Order/Inventory boleh dilaporkan COMPLETED (03-BACKEND-CORE.md
// Section 5 & Iterasi 05) — dijalankan 5x berturut-turut pada container yang
// sama untuk membuktikan hasilnya deterministik, bukan kebetulan.
func TestConcurrentCheckout_ExactlyTenSucceed(t *testing.T) {
	const initialStock = 10
	const concurrentRequests = 100
	const runs = 5

	db := testutil.SetupPostgres(t)
	redisClient := testutil.NewTestRedis(t)
	seed := seedBaseData(t, db, initialStock)
	svc := buildOrderService(db, redisClient)

	for run := 1; run <= runs; run++ {
		t.Run(fmt.Sprintf("run_%d", run), func(t *testing.T) {
			resetInventoryAndOrders(t, db, seed, initialStock)

			var wg sync.WaitGroup
			var successCount int64
			var insufficientStockCount int64
			var unexpectedErrors int64

			start := make(chan struct{})
			wg.Add(concurrentRequests)
			for i := 0; i < concurrentRequests; i++ {
				go func() {
					defer wg.Done()
					<-start // semua goroutine mulai SETELAH barrier dibuka -> benar-benar konkuren

					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					_, err := svc.CreateOrder(ctx, seed.CustomerID, order.CreateOrderRequest{
						WarehouseID: seed.WarehouseID,
						Items:       []order.ItemInput{{ProductID: seed.ProductID, Quantity: 1}},
					})
					if err == nil {
						atomic.AddInt64(&successCount, 1)
						return
					}
					var appErr *apperr.AppError
					if errors.As(err, &appErr) && appErr.ErrCode == apperr.InsufficientStock {
						atomic.AddInt64(&insufficientStockCount, 1)
						return
					}
					atomic.AddInt64(&unexpectedErrors, 1)
					t.Logf("unexpected error from CreateOrder: %v", err)
				}()
			}
			close(start)
			wg.Wait()

			if unexpectedErrors != 0 {
				t.Fatalf("expected zero unexpected errors, got %d", unexpectedErrors)
			}
			if successCount != initialStock {
				t.Fatalf("expected EXACTLY %d successful orders, got %d", initialStock, successCount)
			}
			if insufficientStockCount != concurrentRequests-initialStock {
				t.Fatalf("expected EXACTLY %d INSUFFICIENT_STOCK failures, got %d", concurrentRequests-initialStock, insufficientStockCount)
			}

			var inv inventory.Inventory
			if err := db.Raw(
				`SELECT * FROM inventories WHERE product_id = ? AND warehouse_id = ?`,
				seed.ProductID, seed.WarehouseID,
			).Scan(&inv).Error; err != nil {
				t.Fatalf("failed to read final inventory state: %v", err)
			}

			if inv.Quantity != initialStock {
				t.Fatalf("quantity should remain %d (orders are not yet PAID, no stock-out) — got %d", initialStock, inv.Quantity)
			}
			if inv.ReservedQuantity != initialStock {
				t.Fatalf("expected reserved_quantity=%d (all initial stock reserved), got %d", initialStock, inv.ReservedQuantity)
			}
			if inv.AvailableStock() != 0 {
				t.Fatalf("expected available_stock=0 after exactly %d successful reservations, got %d", initialStock, inv.AvailableStock())
			}
			if inv.Quantity < 0 || inv.ReservedQuantity < 0 {
				t.Fatal("stock must never go negative")
			}
		})
	}
}
