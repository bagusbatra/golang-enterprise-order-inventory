// Package integration mereproduksi spec Section 71 (Integration Test) dan
// Acceptance Test Section 75-81 secara end-to-end terhadap real PostgreSQL
// + Redis (Testcontainers). Section 79 (Concurrency) dan sebagian Section 80
// (duplicate callback KONKUREN) sudah divalidasi lebih dalam di
// tests/concurrency dan tests/idempotency — tidak diulang di sini, hanya
// direferensikan.
package integration

import (
	"context"
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
	"order-management/internal/shipment"
	"order-management/internal/user"
	"order-management/internal/warehouse"
	"order-management/tests/testutil"
)

type seeded struct {
	CustomerID  string
	WarehouseID string
	ProductID   string
}

func seedBaseData(t *testing.T, db *gorm.DB, initialStock int, sku, email string) seeded {
	t.Helper()
	ctx := context.Background()

	cat := &category.Category{Name: "Electronics-" + sku}
	if err := db.WithContext(ctx).Create(cat).Error; err != nil {
		t.Fatalf("seed category failed: %v", err)
	}

	prod := &product.Product{
		CategoryID: cat.ID,
		SKU:        sku,
		Name:       "ASUS ROG",
		Price:      decimal.NewFromInt(15000000),
		CostPrice:  decimal.NewFromInt(12000000),
		Weight:     decimal.NewFromInt(3),
		Status:     product.StatusActive,
	}
	if err := db.WithContext(ctx).Create(prod).Error; err != nil {
		t.Fatalf("seed product failed: %v", err)
	}

	wh := &warehouse.Warehouse{
		Code:    "WH-" + sku,
		Name:    "Surabaya",
		Address: "Jl. Test",
		City:    "Surabaya",
		Status:  warehouse.StatusActive,
	}
	if err := db.WithContext(ctx).Create(wh).Error; err != nil {
		t.Fatalf("seed warehouse failed: %v", err)
	}

	cust := &user.User{
		Name:         "Integration Test Customer",
		Email:        email,
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

type services struct {
	Order     *order.Service
	Payment   *payment.Service
	Shipment  *shipment.Service
	Inventory *inventory.Service
}

func buildServices(db *gorm.DB, redisClient *redis.Client) services {
	warehouseService := warehouse.NewService(warehouse.NewRepository(db))
	productService := product.NewService(product.NewRepository(db), redisClient, 10*time.Minute)
	inventoryService := inventory.NewService(inventory.NewRepository(db), db, nil)
	orderService := order.NewService(order.NewRepository(db), db, warehouseService, productService, inventoryService)
	paymentService := payment.NewService(payment.NewRepository(db), db, orderService, inventoryService)
	shipmentService := shipment.NewService(shipment.NewRepository(db), db, orderService)
	return services{Order: orderService, Payment: paymentService, Shipment: shipmentService, Inventory: inventoryService}
}

func inventoryState(t *testing.T, svc services, productID, warehouseID string) inventory.Response {
	t.Helper()
	items, _, err := svc.Inventory.List(context.Background(), inventory.ListFilter{
		ProductID: productID, WarehouseID: warehouseID, Page: 1, Limit: 10,
	})
	if err != nil {
		t.Fatalf("failed to read inventory state: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected exactly 1 inventory row for this product/warehouse, got %d", len(items))
	}
	return items[0]
}

// TestFullLifecycle_CreateOrderToCompleted mereproduksi spec Section 71
// (Integration Test scenario) DAN Section 75-76 (Acceptance Test — Critical
// Scenario A / Payment) dalam satu alur berkesinambungan.
func TestFullLifecycle_CreateOrderToCompleted(t *testing.T) {
	const initialStock = 10
	const orderQty = 2

	db := testutil.SetupPostgres(t)
	redisClient := testutil.NewTestRedis(t)
	seed := seedBaseData(t, db, initialStock, "SKU-LIFECYCLE", "lifecycle@example.com")
	svc := buildServices(db, redisClient)
	ctx := context.Background()

	// --- Create order (spec Section 75: Order=WAITING_PAYMENT, Reserved=2, Available=8, Payment=PENDING) ---
	createdOrder, err := svc.Order.CreateOrder(ctx, seed.CustomerID, order.CreateOrderRequest{
		WarehouseID: seed.WarehouseID,
		Items:       []order.ItemInput{{ProductID: seed.ProductID, Quantity: orderQty}},
	})
	if err != nil {
		t.Fatalf("create order failed: %v", err)
	}
	if createdOrder.Status != string(order.StatusWaitingPayment) {
		t.Fatalf("expected order status WAITING_PAYMENT, got %s", createdOrder.Status)
	}

	inv := inventoryState(t, svc, seed.ProductID, seed.WarehouseID)
	if inv.Quantity != initialStock {
		t.Fatalf("expected quantity=%d (not yet PAID), got %d", initialStock, inv.Quantity)
	}
	if inv.ReservedQuantity != orderQty {
		t.Fatalf("expected reserved_quantity=%d, got %d", orderQty, inv.ReservedQuantity)
	}
	if inv.AvailableStock != initialStock-orderQty {
		t.Fatalf("expected available_stock=%d, got %d", initialStock-orderQty, inv.AvailableStock)
	}

	createdPayment, err := svc.Payment.CreatePayment(ctx, seed.CustomerID, createdOrder.ID, payment.CreatePaymentRequest{})
	if err != nil {
		t.Fatalf("create payment failed: %v", err)
	}
	if createdPayment.Status != string(payment.StatusPending) {
		t.Fatalf("expected payment status PENDING, got %s", createdPayment.Status)
	}

	// --- Payment callback -> PAID (spec Section 76: Payment=PAID, Order=PAID) ---
	if err := svc.Payment.Callback(ctx, payment.CallbackRequest{
		TransactionID: createdPayment.TransactionID,
		OrderID:       createdOrder.ID,
		Status:        "PAID",
	}); err != nil {
		t.Fatalf("payment callback failed: %v", err)
	}

	orderAfterPaid, err := svc.Order.GetByID(ctx, seed.CustomerID, "CUSTOMER", createdOrder.ID)
	if err != nil {
		t.Fatalf("get order failed: %v", err)
	}
	if orderAfterPaid.Status != string(order.StatusPaid) {
		t.Fatalf("expected order status PAID, got %s", orderAfterPaid.Status)
	}

	paymentAfterCallback, err := svc.Payment.GetByID(ctx, seed.CustomerID, "CUSTOMER", createdPayment.PaymentID)
	if err != nil {
		t.Fatalf("get payment failed: %v", err)
	}
	if paymentAfterCallback.Status != string(payment.StatusPaid) {
		t.Fatalf("expected payment status PAID, got %s", paymentAfterCallback.Status)
	}

	// "Inventory belum boleh salah hitung" (Section 76) — stock-out harus
	// tepat: quantity berkurang qty, reserved kembali 0.
	inv = inventoryState(t, svc, seed.ProductID, seed.WarehouseID)
	if inv.Quantity != initialStock-orderQty {
		t.Fatalf("expected quantity=%d after stock-out, got %d", initialStock-orderQty, inv.Quantity)
	}
	if inv.ReservedQuantity != 0 {
		t.Fatalf("expected reserved_quantity=0 after stock-out settles the reservation, got %d", inv.ReservedQuantity)
	}

	// --- Process -> Pack -> Ship -> Deliver -> COMPLETED (spec Section 71) ---
	packedOrder, err := svc.Shipment.Pack(ctx, createdOrder.ID)
	if err != nil {
		t.Fatalf("pack failed: %v", err)
	}
	if packedOrder.Status != string(order.StatusPacked) {
		t.Fatalf("expected order status PACKED, got %s", packedOrder.Status)
	}

	shippedShipment, err := svc.Shipment.Ship(ctx, createdOrder.ID, shipment.ShipRequest{})
	if err != nil {
		t.Fatalf("ship failed: %v", err)
	}
	if shippedShipment.Status != string(shipment.StatusInTransit) {
		t.Fatalf("expected shipment status IN_TRANSIT, got %s", shippedShipment.Status)
	}
	if shippedShipment.TrackingNumber == "" {
		t.Fatal("expected a generated tracking number")
	}

	orderAfterShip, err := svc.Order.GetByID(ctx, seed.CustomerID, "CUSTOMER", createdOrder.ID)
	if err != nil {
		t.Fatalf("get order failed: %v", err)
	}
	if orderAfterShip.Status != string(order.StatusShipped) {
		t.Fatalf("expected order status SHIPPED, got %s", orderAfterShip.Status)
	}

	deliveredShipment, err := svc.Shipment.Deliver(ctx, shippedShipment.ID)
	if err != nil {
		t.Fatalf("deliver failed: %v", err)
	}
	if deliveredShipment.Status != string(shipment.StatusDelivered) {
		t.Fatalf("expected shipment status DELIVERED, got %s", deliveredShipment.Status)
	}

	finalOrder, err := svc.Order.GetByID(ctx, seed.CustomerID, "CUSTOMER", createdOrder.ID)
	if err != nil {
		t.Fatalf("get order failed: %v", err)
	}
	if finalOrder.Status != string(order.StatusCompleted) {
		t.Fatalf("expected FINAL order status COMPLETED, got %s", finalOrder.Status)
	}
}

// TestCancellation_ReleasesReservedStock mereproduksi spec Section 77.
func TestCancellation_ReleasesReservedStock(t *testing.T) {
	const initialStock = 10
	const orderQty = 3

	db := testutil.SetupPostgres(t)
	redisClient := testutil.NewTestRedis(t)
	seed := seedBaseData(t, db, initialStock, "SKU-CANCEL", "cancel@example.com")
	svc := buildServices(db, redisClient)
	ctx := context.Background()

	created, err := svc.Order.CreateOrder(ctx, seed.CustomerID, order.CreateOrderRequest{
		WarehouseID: seed.WarehouseID,
		Items:       []order.ItemInput{{ProductID: seed.ProductID, Quantity: orderQty}},
	})
	if err != nil {
		t.Fatalf("create order failed: %v", err)
	}

	cancelled, err := svc.Order.Cancel(ctx, seed.CustomerID, "CUSTOMER", created.ID)
	if err != nil {
		t.Fatalf("cancel failed: %v", err)
	}
	if cancelled.Status != string(order.StatusCancelled) {
		t.Fatalf("expected order status CANCELLED, got %s", cancelled.Status)
	}

	inv := inventoryState(t, svc, seed.ProductID, seed.WarehouseID)
	if inv.ReservedQuantity != 0 {
		t.Fatalf("expected reserved stock released (reserved_quantity=0), got %d", inv.ReservedQuantity)
	}
	if inv.Quantity != initialStock {
		t.Fatalf("expected quantity untouched (order never reached PAID), got %d", inv.Quantity)
	}
}

// TestExpiration_ReleasesReservedStockAndExpiresOrder mereproduksi spec
// Section 78. expired_at di-backdate langsung (simulasi "setelah 30 menit")
// alih-alih menunggu — worker (internal/worker/payment_expiration.go)
// hanya memanggil ExpirePendingPayments secara periodik, logic-nya sama.
func TestExpiration_ReleasesReservedStockAndExpiresOrder(t *testing.T) {
	const initialStock = 10
	const orderQty = 4

	db := testutil.SetupPostgres(t)
	redisClient := testutil.NewTestRedis(t)
	seed := seedBaseData(t, db, initialStock, "SKU-EXPIRE", "expire@example.com")
	svc := buildServices(db, redisClient)
	ctx := context.Background()

	created, err := svc.Order.CreateOrder(ctx, seed.CustomerID, order.CreateOrderRequest{
		WarehouseID: seed.WarehouseID,
		Items:       []order.ItemInput{{ProductID: seed.ProductID, Quantity: orderQty}},
	})
	if err != nil {
		t.Fatalf("create order failed: %v", err)
	}

	createdPayment, err := svc.Payment.CreatePayment(ctx, seed.CustomerID, created.ID, payment.CreatePaymentRequest{})
	if err != nil {
		t.Fatalf("create payment failed: %v", err)
	}

	if err := db.Exec(`UPDATE payments SET expired_at = ? WHERE id = ?`, time.Now().Add(-time.Minute), createdPayment.PaymentID).Error; err != nil {
		t.Fatalf("failed to backdate expired_at: %v", err)
	}

	count, err := svc.Payment.ExpirePendingPayments(ctx)
	if err != nil {
		t.Fatalf("ExpirePendingPayments failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 payment expired, got %d", count)
	}

	orderAfter, err := svc.Order.GetByID(ctx, seed.CustomerID, "CUSTOMER", created.ID)
	if err != nil {
		t.Fatalf("get order failed: %v", err)
	}
	if orderAfter.Status != string(order.StatusExpired) {
		t.Fatalf("expected order status EXPIRED, got %s", orderAfter.Status)
	}

	paymentAfter, err := svc.Payment.GetByID(ctx, seed.CustomerID, "CUSTOMER", createdPayment.PaymentID)
	if err != nil {
		t.Fatalf("get payment failed: %v", err)
	}
	if paymentAfter.Status != string(payment.StatusExpired) {
		t.Fatalf("expected payment status EXPIRED, got %s", paymentAfter.Status)
	}

	inv := inventoryState(t, svc, seed.ProductID, seed.WarehouseID)
	if inv.ReservedQuantity != 0 {
		t.Fatalf("expected reserved stock released after expiration, got %d", inv.ReservedQuantity)
	}
}

// TestDuplicateCallback_SequentialTenTimes_NoDoubleEffect mereproduksi spec
// Section 80 secara sequential (versi KONKUREN yang lebih ketat sudah ada
// di tests/idempotency/payment_callback_idempotency_test.go).
func TestDuplicateCallback_SequentialTenTimes_NoDoubleEffect(t *testing.T) {
	const initialStock = 100
	const orderQty = 5

	db := testutil.SetupPostgres(t)
	redisClient := testutil.NewTestRedis(t)
	seed := seedBaseData(t, db, initialStock, "SKU-DUPCALLBACK", "dupcallback@example.com")
	svc := buildServices(db, redisClient)
	ctx := context.Background()

	created, err := svc.Order.CreateOrder(ctx, seed.CustomerID, order.CreateOrderRequest{
		WarehouseID: seed.WarehouseID,
		Items:       []order.ItemInput{{ProductID: seed.ProductID, Quantity: orderQty}},
	})
	if err != nil {
		t.Fatalf("create order failed: %v", err)
	}

	createdPayment, err := svc.Payment.CreatePayment(ctx, seed.CustomerID, created.ID, payment.CreatePaymentRequest{})
	if err != nil {
		t.Fatalf("create payment failed: %v", err)
	}

	for i := 0; i < 10; i++ {
		if err := svc.Payment.Callback(ctx, payment.CallbackRequest{
			TransactionID: createdPayment.TransactionID,
			OrderID:       created.ID,
			Status:        "PAID",
		}); err != nil {
			t.Fatalf("callback #%d should succeed (idempotent no-op after the first), got: %v", i+1, err)
		}
	}

	var stockOutCount int64
	if err := db.Raw(
		`SELECT COUNT(*) FROM inventory_transactions WHERE type = 'STOCK_OUT' AND reference_type = 'ORDER' AND reference_id = ?`,
		created.ID,
	).Scan(&stockOutCount).Error; err != nil {
		t.Fatalf("failed to count STOCK_OUT transactions: %v", err)
	}
	if stockOutCount != 1 {
		t.Fatalf("expected EXACTLY 1 STOCK_OUT transaction despite 10 identical callbacks, got %d", stockOutCount)
	}

	inv := inventoryState(t, svc, seed.ProductID, seed.WarehouseID)
	if inv.Quantity != initialStock-orderQty {
		t.Fatalf("expected quantity=%d after a single stock-out, got %d (stock cut more than once!)", initialStock-orderQty, inv.Quantity)
	}
}
