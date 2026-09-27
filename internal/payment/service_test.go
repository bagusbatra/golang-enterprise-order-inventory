package payment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"order-management/internal/order"
	apperr "order-management/pkg/errors"
)

// --- fakes ---------------------------------------------------------------

type fakePaymentRepo struct {
	byID map[string]*Payment
	byTx map[string]*Payment
	next int
}

func newFakePaymentRepo() *fakePaymentRepo {
	return &fakePaymentRepo{byID: map[string]*Payment{}, byTx: map[string]*Payment{}}
}

func (f *fakePaymentRepo) Create(ctx context.Context, p *Payment) error {
	f.next++
	p.ID = fmt.Sprintf("payment-%d", f.next)
	stored := *p
	f.byID[p.ID] = &stored
	f.byTx[p.TransactionID] = &stored
	return nil
}

func (f *fakePaymentRepo) FindByID(ctx context.Context, id string) (*Payment, error) {
	p, ok := f.byID[id]
	if !ok {
		return nil, ErrNotFound
	}
	c := *p
	return &c, nil
}

func (f *fakePaymentRepo) ExistsPaidForOrder(ctx context.Context, orderID string) (bool, error) {
	for _, p := range f.byID {
		if p.OrderID == orderID && p.Status == StatusPaid {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakePaymentRepo) LockForUpdateByTransactionID(ctx context.Context, tx *gorm.DB, transactionID string) (*Payment, error) {
	p, ok := f.byTx[transactionID]
	if !ok {
		return nil, ErrNotFound
	}
	c := *p
	return &c, nil
}

func (f *fakePaymentRepo) LockForUpdateByID(ctx context.Context, tx *gorm.DB, id string) (*Payment, error) {
	p, ok := f.byID[id]
	if !ok {
		return nil, ErrNotFound
	}
	c := *p
	return &c, nil
}

func (f *fakePaymentRepo) UpdateTx(ctx context.Context, tx *gorm.DB, p *Payment) error {
	c := *p
	f.byID[p.ID] = &c
	f.byTx[p.TransactionID] = &c
	return nil
}

func (f *fakePaymentRepo) UpdateStatusTxIfPending(ctx context.Context, tx *gorm.DB, id string, newStatus Status) (bool, error) {
	p, ok := f.byID[id]
	if !ok || p.Status != StatusPending {
		return false, nil
	}
	p.Status = newStatus
	return true, nil
}

func (f *fakePaymentRepo) ListPendingExpired(ctx context.Context, before time.Time, limit int) ([]Payment, error) {
	var out []Payment
	for _, p := range f.byID {
		if p.Status == StatusPending && p.ExpiredAt.Before(before) {
			out = append(out, *p)
		}
	}
	return out, nil
}

type fakeTxRunner struct{}

func (fakeTxRunner) Transaction(fc func(tx *gorm.DB) error, opts ...*sql.TxOptions) error {
	return fc(nil)
}

type fakeOrderGateway struct {
	orders map[string]*order.Order
	items  map[string][]order.OrderItem
}

func newFakeOrderGateway() *fakeOrderGateway {
	return &fakeOrderGateway{orders: map[string]*order.Order{}, items: map[string][]order.OrderItem{}}
}

func (f *fakeOrderGateway) FindRawByID(ctx context.Context, orderID string) (*order.Order, error) {
	o, ok := f.orders[orderID]
	if !ok {
		return nil, order.ErrNotFound
	}
	c := *o
	return &c, nil
}

func (f *fakeOrderGateway) LockForUpdate(ctx context.Context, tx *gorm.DB, orderID string) (*order.Order, error) {
	return f.FindRawByID(ctx, orderID)
}

func (f *fakeOrderGateway) UpdateStatus(ctx context.Context, tx *gorm.DB, orderID string, status order.Status) error {
	o, ok := f.orders[orderID]
	if !ok {
		return order.ErrNotFound
	}
	o.Status = status
	return nil
}

func (f *fakeOrderGateway) ItemsByOrderID(ctx context.Context, tx *gorm.DB, orderID string) ([]order.OrderItem, error) {
	return f.items[orderID], nil
}

type fakeStockAdjuster struct {
	stockOutCalls int
	releaseCalls  int
}

func (f *fakeStockAdjuster) StockOut(ctx context.Context, tx *gorm.DB, productID, warehouseID string, qty int, referenceID string) error {
	f.stockOutCalls++
	return nil
}

func (f *fakeStockAdjuster) ReleaseStock(ctx context.Context, tx *gorm.DB, productID, warehouseID string, qty int, referenceID string) error {
	f.releaseCalls++
	return nil
}

// --- tests ---------------------------------------------------------------

func newTestService() (*Service, *fakePaymentRepo, *fakeOrderGateway, *fakeStockAdjuster) {
	repo := newFakePaymentRepo()
	og := newFakeOrderGateway()
	stock := &fakeStockAdjuster{}
	svc := NewService(repo, fakeTxRunner{}, og, stock)
	return svc, repo, og, stock
}

func TestCreatePayment_Success(t *testing.T) {
	svc, _, og, _ := newTestService()
	og.orders["order-1"] = &order.Order{ID: "order-1", CustomerID: "customer-1", Status: order.StatusWaitingPayment, GrandTotal: decimal.NewFromInt(242000)}

	resp, err := svc.CreatePayment(context.Background(), "customer-1", "order-1", CreatePaymentRequest{})
	if err != nil {
		t.Fatalf("create payment failed: %v", err)
	}
	if resp.Status != string(StatusPending) {
		t.Fatalf("expected status PENDING, got %s", resp.Status)
	}
	if resp.PaymentMethod != string(MethodBankTransfer) {
		t.Fatalf("expected default payment_method BANK_TRANSFER, got %s", resp.PaymentMethod)
	}
	if resp.Amount != "242000.00" {
		t.Fatalf("expected amount snapshot from order grand_total, got %s", resp.Amount)
	}
}

func TestCreatePayment_NotOwner_Forbidden(t *testing.T) {
	svc, _, og, _ := newTestService()
	og.orders["order-1"] = &order.Order{ID: "order-1", CustomerID: "owner", Status: order.StatusWaitingPayment}

	_, err := svc.CreatePayment(context.Background(), "someone-else", "order-1", CreatePaymentRequest{})
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.ErrCode != apperr.AuthForbidden {
		t.Fatalf("expected AUTH_FORBIDDEN error, got: %v", err)
	}
}

func TestCreatePayment_AlreadyPaid_Rejected(t *testing.T) {
	svc, repo, og, _ := newTestService()
	og.orders["order-1"] = &order.Order{ID: "order-1", CustomerID: "customer-1", Status: order.StatusPaid, GrandTotal: decimal.NewFromInt(1000)}
	repo.byID["existing"] = &Payment{ID: "existing", OrderID: "order-1", Status: StatusPaid}

	_, err := svc.CreatePayment(context.Background(), "customer-1", "order-1", CreatePaymentRequest{})
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.ErrCode != apperr.PaymentAlreadyPaid {
		t.Fatalf("expected PAYMENT_ALREADY_PAID error, got: %v", err)
	}
}

func TestCreatePayment_OrderNotAwaitingPayment_Rejected(t *testing.T) {
	svc, _, og, _ := newTestService()
	og.orders["order-1"] = &order.Order{ID: "order-1", CustomerID: "customer-1", Status: order.StatusCancelled, GrandTotal: decimal.NewFromInt(1000)}

	_, err := svc.CreatePayment(context.Background(), "customer-1", "order-1", CreatePaymentRequest{})
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.ErrCode != apperr.OrderInvalidStatus {
		t.Fatalf("expected ORDER_INVALID_STATUS error, got: %v", err)
	}
}

// TestCallback_Idempotent_RepeatedCallbacksSequential mereproduksi spec
// Section 73/80: callback yang sama dikirim berkali-kali -> payment updated
// SEKALI, order transisi SEKALI, TIDAK ADA duplicate stock-out.
func TestCallback_Idempotent_RepeatedCallbacksSequential(t *testing.T) {
	svc, repo, og, stock := newTestService()
	og.orders["order-1"] = &order.Order{ID: "order-1", WarehouseID: "wh-1", Status: order.StatusWaitingPayment}
	og.items["order-1"] = []order.OrderItem{{ProductID: "product-1", Quantity: 2}}
	repo.byID["payment-1"] = &Payment{ID: "payment-1", OrderID: "order-1", TransactionID: "txn-1", Status: StatusPending}
	repo.byTx["txn-1"] = repo.byID["payment-1"]

	for i := 0; i < 10; i++ {
		if err := svc.Callback(context.Background(), CallbackRequest{TransactionID: "txn-1", OrderID: "order-1", Status: "PAID"}); err != nil {
			t.Fatalf("callback #%d should succeed (idempotent no-op after first), got error: %v", i+1, err)
		}
	}

	if stock.stockOutCalls != 1 {
		t.Fatalf("expected exactly 1 StockOut call across 10 identical callbacks, got %d", stock.stockOutCalls)
	}
	if og.orders["order-1"].Status != order.StatusPaid {
		t.Fatalf("expected order status PAID, got %s", og.orders["order-1"].Status)
	}
	if repo.byID["payment-1"].Status != StatusPaid {
		t.Fatalf("expected payment status PAID, got %s", repo.byID["payment-1"].Status)
	}
}

func TestCallback_OrderIDMismatch_Rejected(t *testing.T) {
	svc, repo, og, _ := newTestService()
	og.orders["order-1"] = &order.Order{ID: "order-1", Status: order.StatusWaitingPayment}
	repo.byID["payment-1"] = &Payment{ID: "payment-1", OrderID: "order-1", TransactionID: "txn-1", Status: StatusPending}
	repo.byTx["txn-1"] = repo.byID["payment-1"]

	err := svc.Callback(context.Background(), CallbackRequest{TransactionID: "txn-1", OrderID: "wrong-order", Status: "PAID"})
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.ErrCode != apperr.PaymentInvalidCallback {
		t.Fatalf("expected PAYMENT_INVALID_CALLBACK error, got: %v", err)
	}
}

func TestCallback_UnknownTransactionID_NotFound(t *testing.T) {
	svc, _, _, _ := newTestService()
	err := svc.Callback(context.Background(), CallbackRequest{TransactionID: "does-not-exist", OrderID: "order-1", Status: "PAID"})
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.ErrCode != apperr.PaymentNotFound {
		t.Fatalf("expected PAYMENT_NOT_FOUND error, got: %v", err)
	}
}

func TestCallback_AlreadyExpired_Rejected(t *testing.T) {
	svc, repo, og, stock := newTestService()
	og.orders["order-1"] = &order.Order{ID: "order-1", Status: order.StatusExpired}
	repo.byID["payment-1"] = &Payment{ID: "payment-1", OrderID: "order-1", TransactionID: "txn-1", Status: StatusExpired}
	repo.byTx["txn-1"] = repo.byID["payment-1"]

	err := svc.Callback(context.Background(), CallbackRequest{TransactionID: "txn-1", OrderID: "order-1", Status: "PAID"})
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.ErrCode != apperr.PaymentExpired {
		t.Fatalf("expected PAYMENT_EXPIRED error, got: %v", err)
	}
	if stock.stockOutCalls != 0 {
		t.Fatal("no stock-out should happen for an already-expired payment")
	}
}

// TestExpirePendingPayments_ReleasesStockAndTransitionsOrder mereproduksi
// spec Section 78: worker expiration -> Order=EXPIRED, stock released.
func TestExpirePendingPayments_ReleasesStockAndTransitionsOrder(t *testing.T) {
	svc, repo, og, stock := newTestService()
	og.orders["order-1"] = &order.Order{ID: "order-1", WarehouseID: "wh-1", Status: order.StatusWaitingPayment}
	og.items["order-1"] = []order.OrderItem{{ProductID: "product-1", Quantity: 3}}
	repo.byID["payment-1"] = &Payment{
		ID: "payment-1", OrderID: "order-1", TransactionID: "txn-1",
		Status: StatusPending, ExpiredAt: time.Now().Add(-time.Minute),
	}

	count, err := svc.ExpirePendingPayments(context.Background())
	if err != nil {
		t.Fatalf("ExpirePendingPayments failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 payment expired, got %d", count)
	}
	if repo.byID["payment-1"].Status != StatusExpired {
		t.Fatalf("expected payment status EXPIRED, got %s", repo.byID["payment-1"].Status)
	}
	if og.orders["order-1"].Status != order.StatusExpired {
		t.Fatalf("expected order status EXPIRED, got %s", og.orders["order-1"].Status)
	}
	if stock.releaseCalls != 1 {
		t.Fatalf("expected exactly 1 ReleaseStock call, got %d", stock.releaseCalls)
	}
}

func TestExpirePendingPayments_NotYetExpired_Untouched(t *testing.T) {
	svc, repo, _, stock := newTestService()
	repo.byID["payment-1"] = &Payment{
		ID: "payment-1", OrderID: "order-1", TransactionID: "txn-1",
		Status: StatusPending, ExpiredAt: time.Now().Add(time.Hour),
	}

	count, err := svc.ExpirePendingPayments(context.Background())
	if err != nil {
		t.Fatalf("ExpirePendingPayments failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 payments expired (not yet past expired_at), got %d", count)
	}
	if stock.releaseCalls != 0 {
		t.Fatal("no release should happen for a payment that has not expired yet")
	}
}
