// Package idempotency membuktikan requirement KRITIKAL KEDUA seluruh
// project (03-BACKEND-CORE.md Iterasi 06; spec Section 42, 73, 80):
// callback payment gateway yang dikirim BERKALI-KALI, termasuk SECARA
// KONKUREN (bukan cuma sequential loop), hanya boleh memicu efek bisnis
// SATU KALI — payment jadi PAID sekali, order transisi sekali, stock-out
// tercatat sekali.
//
// WAJIB real PostgreSQL (Testcontainers) — idempotency di bawah row lock
// sungguhan tidak valid dites dengan mock/fake repository.
package idempotency

import (
	"context"
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
	"order-management/internal/payment"
	"order-management/internal/product"
	"order-management/internal/user"
	"order-management/internal/warehouse"
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
		SKU:        "SKU-IDEMPOTENCY-TEST",
		Name:       "Idempotency Test Product",
		Price:      decimal.NewFromInt(50000),
		CostPrice:  decimal.NewFromInt(25000),
		Weight:     decimal.NewFromInt(1),
		Status:     product.StatusActive,
	}
	if err := db.WithContext(ctx).Create(prod).Error; err != nil {
		t.Fatalf("seed product failed: %v", err)
	}

	wh := &warehouse.Warehouse{
		Code:    "WH-IDEMP",
		Name:    "Idempotency Test Warehouse",
		Address: "Jl. Test No. 2",
		City:    "Jakarta",
		Status:  warehouse.StatusActive,
	}
	if err := db.WithContext(ctx).Create(wh).Error; err != nil {
		t.Fatalf("seed warehouse failed: %v", err)
	}

	cust := &user.User{
		Name:         "Idempotency Test Customer",
		Email:        "idempotency-test@example.com",
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

func buildServices(db *gorm.DB, redisClient *redis.Client) (*order.Service, *payment.Service) {
	warehouseService := warehouse.NewService(warehouse.NewRepository(db))
	productService := product.NewService(product.NewRepository(db), redisClient, 10*time.Minute)
	inventoryService := inventory.NewService(inventory.NewRepository(db), db, nil)
	orderService := order.NewService(order.NewRepository(db), db, warehouseService, productService, inventoryService)
	paymentService := payment.NewService(payment.NewRepository(db), db, orderService, inventoryService)
	return orderService, paymentService
}

func TestPaymentCallback_Idempotent_ConcurrentDuplicateCallbacks(t *testing.T) {
	const initialStock = 100
	const orderQty = 3
	const concurrentCallbacks = 10

	db := testutil.SetupPostgres(t)
	redisClient := testutil.NewTestRedis(t)
	seed := seedBaseData(t, db, initialStock)
	orderSvc, paymentSvc := buildServices(db, redisClient)

	createdOrder, err := orderSvc.CreateOrder(context.Background(), seed.CustomerID, order.CreateOrderRequest{
		WarehouseID: seed.WarehouseID,
		Items:       []order.ItemInput{{ProductID: seed.ProductID, Quantity: orderQty}},
	})
	if err != nil {
		t.Fatalf("create order failed: %v", err)
	}

	createdPayment, err := paymentSvc.CreatePayment(context.Background(), seed.CustomerID, createdOrder.ID, payment.CreatePaymentRequest{})
	if err != nil {
		t.Fatalf("create payment failed: %v", err)
	}

	var wg sync.WaitGroup
	var successCount int64
	var errCount int64

	start := make(chan struct{})
	wg.Add(concurrentCallbacks)
	for i := 0; i < concurrentCallbacks; i++ {
		go func() {
			defer wg.Done()
			<-start // semua goroutine mengirim callback SETELAH barrier -> benar-benar konkuren, bukan sequential loop

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			err := paymentSvc.Callback(ctx, payment.CallbackRequest{
				TransactionID: createdPayment.TransactionID,
				OrderID:       createdOrder.ID,
				Status:        "PAID",
			})
			if err != nil {
				atomic.AddInt64(&errCount, 1)
				t.Logf("callback error (should never happen — callback is idempotent): %v", err)
				return
			}
			atomic.AddInt64(&successCount, 1)
		}()
	}
	close(start)
	wg.Wait()

	if errCount != 0 {
		t.Fatalf("expected all %d concurrent callbacks to succeed (idempotent no-op after the first), got %d errors", concurrentCallbacks, errCount)
	}
	if successCount != concurrentCallbacks {
		t.Fatalf("expected %d successful callback responses, got %d", concurrentCallbacks, successCount)
	}

	var p payment.Payment
	if err := db.Raw(`SELECT * FROM payments WHERE id = ?`, createdPayment.PaymentID).Scan(&p).Error; err != nil {
		t.Fatalf("failed to read payment: %v", err)
	}
	if p.Status != payment.StatusPaid {
		t.Fatalf("expected payment status PAID, got %s", p.Status)
	}
	if p.PaidAt == nil {
		t.Fatal("expected paid_at to be set")
	}

	var o order.Order
	if err := db.Raw(`SELECT * FROM orders WHERE id = ?`, createdOrder.ID).Scan(&o).Error; err != nil {
		t.Fatalf("failed to read order: %v", err)
	}
	if o.Status != order.StatusPaid {
		t.Fatalf("expected order status PAID, got %s", o.Status)
	}

	// Bukti utama idempotency: TEPAT SATU inventory_transaction STOCK_OUT
	// untuk order ini, meski 10 callback identik dikirim BERSAMAAN.
	var stockOutCount int64
	if err := db.Raw(
		`SELECT COUNT(*) FROM inventory_transactions WHERE type = 'STOCK_OUT' AND reference_type = 'ORDER' AND reference_id = ?`,
		createdOrder.ID,
	).Scan(&stockOutCount).Error; err != nil {
		t.Fatalf("failed to count STOCK_OUT transactions: %v", err)
	}
	if stockOutCount != 1 {
		t.Fatalf("expected EXACTLY 1 STOCK_OUT inventory_transaction despite %d concurrent identical callbacks, got %d (duplicate business effect!)", concurrentCallbacks, stockOutCount)
	}

	var inv inventory.Inventory
	if err := db.Raw(
		`SELECT * FROM inventories WHERE product_id = ? AND warehouse_id = ?`,
		seed.ProductID, seed.WarehouseID,
	).Scan(&inv).Error; err != nil {
		t.Fatalf("failed to read inventory: %v", err)
	}
	if inv.Quantity != initialStock-orderQty {
		t.Fatalf("expected quantity=%d after a SINGLE stock-out of %d (initial %d), got %d — stock was cut more than once", initialStock-orderQty, orderQty, initialStock, inv.Quantity)
	}
	if inv.ReservedQuantity != 0 {
		t.Fatalf("expected reserved_quantity=0 after stock-out settles the reservation, got %d", inv.ReservedQuantity)
	}
}
