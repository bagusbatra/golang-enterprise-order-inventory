package order

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	apperr "order-management/pkg/errors"
)

// --- fakes ---------------------------------------------------------------

type fakeOrderRepo struct {
	orders    map[string]*Order
	itemsByID map[string][]OrderItem
	seq       map[string]int
	nextID    int
}

func newFakeOrderRepo() *fakeOrderRepo {
	return &fakeOrderRepo{orders: map[string]*Order{}, itemsByID: map[string][]OrderItem{}, seq: map[string]int{}}
}

func (f *fakeOrderRepo) NextOrderNumber(ctx context.Context, tx *gorm.DB, date time.Time) (string, error) {
	d := date.Format("2006-01-02")
	f.seq[d]++
	return fmt.Sprintf("ORD-%s-%06d", date.Format("20060102"), f.seq[d]), nil
}

func (f *fakeOrderRepo) Create(ctx context.Context, tx *gorm.DB, o *Order) error {
	f.nextID++
	o.ID = fmt.Sprintf("order-%d", f.nextID)
	stored := *o
	f.orders[o.ID] = &stored

	items := make([]OrderItem, len(o.Items))
	for i, it := range o.Items {
		it.OrderID = o.ID
		items[i] = it
	}
	f.itemsByID[o.ID] = items
	return nil
}

func (f *fakeOrderRepo) FindByID(ctx context.Context, id string) (*Order, error) {
	o, ok := f.orders[id]
	if !ok {
		return nil, ErrNotFound
	}
	c := *o
	c.Items = f.itemsByID[id]
	return &c, nil
}

func (f *fakeOrderRepo) List(ctx context.Context, flt ListFilter) ([]Order, int64, error) {
	return nil, 0, nil
}

func (f *fakeOrderRepo) LockForUpdate(ctx context.Context, tx *gorm.DB, id string) (*Order, error) {
	o, ok := f.orders[id]
	if !ok {
		return nil, ErrNotFound
	}
	c := *o
	return &c, nil
}

func (f *fakeOrderRepo) UpdateStatus(ctx context.Context, tx *gorm.DB, id string, status Status) error {
	o, ok := f.orders[id]
	if !ok {
		return ErrNotFound
	}
	o.Status = status
	return nil
}

func (f *fakeOrderRepo) ItemsByOrderID(ctx context.Context, tx *gorm.DB, orderID string) ([]OrderItem, error) {
	return f.itemsByID[orderID], nil
}

type fakeTxRunner struct{}

func (fakeTxRunner) Transaction(fc func(tx *gorm.DB) error, opts ...*sql.TxOptions) error {
	return fc(nil)
}

type fakeWarehouseChecker struct{ active map[string]bool }

func (f fakeWarehouseChecker) IsActive(ctx context.Context, id string) (bool, error) {
	return f.active[id], nil
}

type fakePriceProvider struct{ prices map[string]decimal.Decimal }

func (f fakePriceProvider) GetPriceSnapshot(ctx context.Context, productID string) (decimal.Decimal, error) {
	p, ok := f.prices[productID]
	if !ok {
		return decimal.Zero, apperr.New(404, apperr.ProductNotFound, "product not found")
	}
	return p, nil
}

// fakeStockReserver melacak available stock per product+warehouse DAN urutan
// pemanggilan CheckAndLock, supaya bisa memverifikasi lock diambil urut
// product_id ASC (mencegah deadlock, spec Iterasi 04).
type fakeStockReserver struct {
	available  map[string]int
	checkOrder []string
	reserved   []string
	released   []string
}

func stockKey(productID, warehouseID string) string { return productID + "|" + warehouseID }

func newFakeStockReserver() *fakeStockReserver {
	return &fakeStockReserver{available: map[string]int{}}
}

func (f *fakeStockReserver) CheckAndLock(ctx context.Context, tx *gorm.DB, productID, warehouseID string, qty int) error {
	f.checkOrder = append(f.checkOrder, productID)
	if f.available[stockKey(productID, warehouseID)] < qty {
		return apperr.New(409, apperr.InsufficientStock, "insufficient stock")
	}
	return nil
}

func (f *fakeStockReserver) ReserveStock(ctx context.Context, tx *gorm.DB, productID, warehouseID string, qty int, referenceID string) error {
	k := stockKey(productID, warehouseID)
	if f.available[k] < qty {
		return apperr.New(409, apperr.InsufficientStock, "insufficient stock")
	}
	f.available[k] -= qty
	f.reserved = append(f.reserved, fmt.Sprintf("%s:%d:%s", k, qty, referenceID))
	return nil
}

func (f *fakeStockReserver) ReleaseStock(ctx context.Context, tx *gorm.DB, productID, warehouseID string, qty int, referenceID string) error {
	k := stockKey(productID, warehouseID)
	f.available[k] += qty
	f.released = append(f.released, fmt.Sprintf("%s:%d:%s", k, qty, referenceID))
	return nil
}

// --- tests ---------------------------------------------------------------

func newTestService(warehouseActive map[string]bool, prices map[string]decimal.Decimal, stock *fakeStockReserver) (*Service, *fakeOrderRepo) {
	repo := newFakeOrderRepo()
	svc := NewService(repo, fakeTxRunner{}, fakeWarehouseChecker{active: warehouseActive}, fakePriceProvider{prices: prices}, stock)
	return svc, repo
}

// TestCreateOrder_Section75Scenario mereproduksi Acceptance Test spec
// Section 75: stock=10, beli 2 -> Order=WAITING_PAYMENT, Reserved=2 (available
// turun dari 10 ke 8), grand_total = subtotal + tax(11%) + shipping(20000).
func TestCreateOrder_Section75Scenario(t *testing.T) {
	stock := newFakeStockReserver()
	stock.available[stockKey("product-1", "wh-1")] = 10

	svc, _ := newTestService(
		map[string]bool{"wh-1": true},
		map[string]decimal.Decimal{"product-1": decimal.NewFromInt(100000)},
		stock,
	)

	resp, err := svc.CreateOrder(context.Background(), "customer-1", CreateOrderRequest{
		WarehouseID: "wh-1",
		Items:       []ItemInput{{ProductID: "product-1", Quantity: 2}},
	})
	if err != nil {
		t.Fatalf("create order failed: %v", err)
	}

	if resp.Status != string(StatusWaitingPayment) {
		t.Fatalf("expected status WAITING_PAYMENT, got %s", resp.Status)
	}
	if resp.Subtotal != "200000.00" {
		t.Fatalf("expected subtotal=200000.00, got %s", resp.Subtotal)
	}
	if resp.Tax != "22000.00" {
		t.Fatalf("expected tax=22000.00 (11%%), got %s", resp.Tax)
	}
	if resp.ShippingCost != "20000.00" {
		t.Fatalf("expected shipping_cost=20000.00, got %s", resp.ShippingCost)
	}
	if resp.GrandTotal != "242000.00" {
		t.Fatalf("expected grand_total=242000.00, got %s", resp.GrandTotal)
	}

	if got := stock.available[stockKey("product-1", "wh-1")]; got != 8 {
		t.Fatalf("expected available stock=8 after reserving 2 of 10, got %d", got)
	}
	if len(stock.reserved) != 1 {
		t.Fatalf("expected exactly 1 ReserveStock call, got %d", len(stock.reserved))
	}
}

func TestCreateOrder_WarehouseInactive_Rejected(t *testing.T) {
	stock := newFakeStockReserver()
	svc, repo := newTestService(
		map[string]bool{"wh-1": false},
		map[string]decimal.Decimal{"product-1": decimal.NewFromInt(1000)},
		stock,
	)

	_, err := svc.CreateOrder(context.Background(), "customer-1", CreateOrderRequest{
		WarehouseID: "wh-1",
		Items:       []ItemInput{{ProductID: "product-1", Quantity: 1}},
	})
	if err == nil {
		t.Fatal("expected error for inactive warehouse")
	}
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.ErrCode != apperr.WarehouseInactive {
		t.Fatalf("expected WAREHOUSE_INACTIVE error, got: %v", err)
	}
	if len(repo.orders) != 0 {
		t.Fatal("no order should be created when warehouse is inactive")
	}
}

// TestCreateOrder_InsufficientStock_NoOrderPersisted membuktikan Pass 1 (lock
// + check semua item) gagal SEBELUM order/order_items ditulis sama sekali —
// tidak ada row order yang "setengah jadi" saat salah satu item kurang stok.
func TestCreateOrder_InsufficientStock_NoOrderPersisted(t *testing.T) {
	stock := newFakeStockReserver()
	stock.available[stockKey("product-1", "wh-1")] = 1

	svc, repo := newTestService(
		map[string]bool{"wh-1": true},
		map[string]decimal.Decimal{"product-1": decimal.NewFromInt(1000)},
		stock,
	)

	_, err := svc.CreateOrder(context.Background(), "customer-1", CreateOrderRequest{
		WarehouseID: "wh-1",
		Items:       []ItemInput{{ProductID: "product-1", Quantity: 5}},
	})
	if err == nil {
		t.Fatal("expected INSUFFICIENT_STOCK error")
	}
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.ErrCode != apperr.InsufficientStock {
		t.Fatalf("expected INSUFFICIENT_STOCK error, got: %v", err)
	}
	if len(repo.orders) != 0 {
		t.Fatal("no order should be persisted when any item fails stock check")
	}
	if len(stock.reserved) != 0 {
		t.Fatal("ReserveStock should never be called when Pass 1 check fails")
	}
}

// TestCreateOrder_LocksItemsInProductIDAscendingOrder memverifikasi urutan
// lock SELALU ascending berdasarkan product_id, terlepas dari urutan item di
// request — ini yang mencegah deadlock antar request konkuren yang memesan
// kombinasi produk berbeda urutan (spec Iterasi 04).
func TestCreateOrder_LocksItemsInProductIDAscendingOrder(t *testing.T) {
	stock := newFakeStockReserver()
	stock.available[stockKey("product-b", "wh-1")] = 10
	stock.available[stockKey("product-a", "wh-1")] = 10

	svc, _ := newTestService(
		map[string]bool{"wh-1": true},
		map[string]decimal.Decimal{
			"product-a": decimal.NewFromInt(1000),
			"product-b": decimal.NewFromInt(2000),
		},
		stock,
	)

	// Request sengaja menaruh product-b duluan.
	_, err := svc.CreateOrder(context.Background(), "customer-1", CreateOrderRequest{
		WarehouseID: "wh-1",
		Items: []ItemInput{
			{ProductID: "product-b", Quantity: 1},
			{ProductID: "product-a", Quantity: 1},
		},
	})
	if err != nil {
		t.Fatalf("create order failed: %v", err)
	}

	if len(stock.checkOrder) != 2 || stock.checkOrder[0] != "product-a" || stock.checkOrder[1] != "product-b" {
		t.Fatalf("expected lock order [product-a, product-b], got %v", stock.checkOrder)
	}
}

func TestCancel_FromWaitingPayment_ReleasesReservedStock(t *testing.T) {
	stock := newFakeStockReserver()
	stock.available[stockKey("product-1", "wh-1")] = 10

	svc, repo := newTestService(
		map[string]bool{"wh-1": true},
		map[string]decimal.Decimal{"product-1": decimal.NewFromInt(1000)},
		stock,
	)

	created, err := svc.CreateOrder(context.Background(), "customer-1", CreateOrderRequest{
		WarehouseID: "wh-1",
		Items:       []ItemInput{{ProductID: "product-1", Quantity: 3}},
	})
	if err != nil {
		t.Fatalf("create order failed: %v", err)
	}

	cancelled, err := svc.Cancel(context.Background(), "customer-1", "CUSTOMER", created.ID)
	if err != nil {
		t.Fatalf("cancel failed: %v", err)
	}
	if cancelled.Status != string(StatusCancelled) {
		t.Fatalf("expected status CANCELLED, got %s", cancelled.Status)
	}
	if len(stock.released) != 1 {
		t.Fatalf("expected exactly 1 ReleaseStock call, got %d", len(stock.released))
	}
	if repo.orders[created.ID].Status != StatusCancelled {
		t.Fatal("order status should be persisted as CANCELLED")
	}
}

func TestCancel_FromPaid_DoesNotTouchInventory(t *testing.T) {
	stock := newFakeStockReserver()
	svc, repo := newTestService(map[string]bool{"wh-1": true}, nil, stock)

	repo.orders["order-paid"] = &Order{ID: "order-paid", CustomerID: "customer-1", WarehouseID: "wh-1", Status: StatusPaid}
	repo.itemsByID["order-paid"] = []OrderItem{{ProductID: "product-1", Quantity: 3}}

	resp, err := svc.Cancel(context.Background(), "customer-1", "CUSTOMER", "order-paid")
	if err != nil {
		t.Fatalf("cancel failed: %v", err)
	}
	if resp.Status != string(StatusCancelled) {
		t.Fatalf("expected CANCELLED, got %s", resp.Status)
	}
	if len(stock.released) != 0 {
		t.Fatal("cancelling a PAID order must not call ReleaseStock (no restock flow in scope)")
	}
}

func TestCancel_InvalidStatus_Rejected(t *testing.T) {
	stock := newFakeStockReserver()
	svc, repo := newTestService(map[string]bool{"wh-1": true}, nil, stock)
	repo.orders["order-shipped"] = &Order{ID: "order-shipped", CustomerID: "customer-1", WarehouseID: "wh-1", Status: StatusShipped}

	_, err := svc.Cancel(context.Background(), "customer-1", "CUSTOMER", "order-shipped")
	if err == nil {
		t.Fatal("expected error cancelling a SHIPPED order")
	}
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.ErrCode != apperr.OrderInvalidStatus {
		t.Fatalf("expected ORDER_INVALID_STATUS error, got: %v", err)
	}
}

func TestCancel_OtherCustomersOrder_Forbidden(t *testing.T) {
	stock := newFakeStockReserver()
	svc, repo := newTestService(map[string]bool{"wh-1": true}, nil, stock)
	repo.orders["order-1"] = &Order{ID: "order-1", CustomerID: "owner-customer", WarehouseID: "wh-1", Status: StatusWaitingPayment}

	_, err := svc.Cancel(context.Background(), "someone-else", "CUSTOMER", "order-1")
	if err == nil {
		t.Fatal("expected forbidden error when a different customer tries to cancel")
	}
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.ErrCode != apperr.AuthForbidden {
		t.Fatalf("expected AUTH_FORBIDDEN error, got: %v", err)
	}
}

func TestGetByID_OwnershipEnforcedForCustomer(t *testing.T) {
	stock := newFakeStockReserver()
	svc, repo := newTestService(map[string]bool{"wh-1": true}, nil, stock)
	repo.orders["order-1"] = &Order{ID: "order-1", CustomerID: "owner-customer", WarehouseID: "wh-1", Status: StatusWaitingPayment}

	if _, err := svc.GetByID(context.Background(), "owner-customer", "CUSTOMER", "order-1"); err != nil {
		t.Fatalf("owner should be able to view their own order: %v", err)
	}

	_, err := svc.GetByID(context.Background(), "someone-else", "CUSTOMER", "order-1")
	if err == nil {
		t.Fatal("expected forbidden error for a different customer")
	}

	if _, err := svc.GetByID(context.Background(), "warehouse-staff", "WAREHOUSE", "order-1"); err != nil {
		t.Fatalf("non-customer roles should be able to view any order: %v", err)
	}
}
