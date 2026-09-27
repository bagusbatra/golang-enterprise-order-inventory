package shipment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"gorm.io/gorm"

	"order-management/internal/order"
	apperr "order-management/pkg/errors"
)

type fakeRepo struct {
	items  map[string]*Shipment
	nextID int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{items: map[string]*Shipment{}}
}

func (f *fakeRepo) Create(ctx context.Context, tx *gorm.DB, s *Shipment) error {
	f.nextID++
	s.ID = fmt.Sprintf("shipment-%d", f.nextID)
	c := *s
	f.items[s.ID] = &c
	return nil
}

func (f *fakeRepo) FindByID(ctx context.Context, id string) (*Shipment, error) {
	s, ok := f.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	c := *s
	return &c, nil
}

func (f *fakeRepo) LockForUpdate(ctx context.Context, tx *gorm.DB, id string) (*Shipment, error) {
	return f.FindByID(ctx, id)
}

func (f *fakeRepo) UpdateTx(ctx context.Context, tx *gorm.DB, s *Shipment) error {
	c := *s
	f.items[s.ID] = &c
	return nil
}

type fakeTxRunner struct{}

func (fakeTxRunner) Transaction(fc func(tx *gorm.DB) error, opts ...*sql.TxOptions) error {
	return fc(nil)
}

type fakeOrderGateway struct {
	orders map[string]*order.Order
}

func newFakeOrderGateway() *fakeOrderGateway {
	return &fakeOrderGateway{orders: map[string]*order.Order{}}
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

func newTestService() (*Service, *fakeRepo, *fakeOrderGateway) {
	repo := newFakeRepo()
	og := newFakeOrderGateway()
	svc := NewService(repo, fakeTxRunner{}, og)
	return svc, repo, og
}

func TestPack_FromPaid_TransitionsToPacked(t *testing.T) {
	svc, _, og := newTestService()
	og.orders["order-1"] = &order.Order{ID: "order-1", Status: order.StatusPaid}

	resp, err := svc.Pack(context.Background(), "order-1")
	if err != nil {
		t.Fatalf("pack failed: %v", err)
	}
	if resp.Status != string(order.StatusPacked) {
		t.Fatalf("expected status PACKED, got %s", resp.Status)
	}
	if og.orders["order-1"].Status != order.StatusPacked {
		t.Fatalf("expected persisted order status PACKED, got %s", og.orders["order-1"].Status)
	}
}

func TestPack_NotPaid_Rejected(t *testing.T) {
	svc, _, og := newTestService()
	og.orders["order-1"] = &order.Order{ID: "order-1", Status: order.StatusWaitingPayment}

	_, err := svc.Pack(context.Background(), "order-1")
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.ErrCode != apperr.OrderInvalidStatus {
		t.Fatalf("expected ORDER_INVALID_STATUS, got: %v", err)
	}
}

func TestShip_FromPacked_CreatesShipmentWithDefaultCourier(t *testing.T) {
	svc, repo, og := newTestService()
	og.orders["order-1"] = &order.Order{ID: "order-1", Status: order.StatusPacked, CustomerID: "customer-1"}

	resp, err := svc.Ship(context.Background(), "order-1", ShipRequest{})
	if err != nil {
		t.Fatalf("ship failed: %v", err)
	}
	if resp.Courier != defaultCourier {
		t.Fatalf("expected default courier %q, got %q", defaultCourier, resp.Courier)
	}
	if resp.Status != string(StatusInTransit) {
		t.Fatalf("expected shipment status IN_TRANSIT, got %s", resp.Status)
	}
	if resp.TrackingNumber == "" {
		t.Fatal("expected a generated tracking number")
	}
	if og.orders["order-1"].Status != order.StatusShipped {
		t.Fatalf("expected order status SHIPPED, got %s", og.orders["order-1"].Status)
	}
	if len(repo.items) != 1 {
		t.Fatalf("expected exactly 1 shipment record, got %d", len(repo.items))
	}
}

func TestShip_CustomCourier(t *testing.T) {
	svc, _, og := newTestService()
	og.orders["order-1"] = &order.Order{ID: "order-1", Status: order.StatusPacked}

	resp, err := svc.Ship(context.Background(), "order-1", ShipRequest{Courier: "JNE Express"})
	if err != nil {
		t.Fatalf("ship failed: %v", err)
	}
	if resp.Courier != "JNE Express" {
		t.Fatalf("expected custom courier, got %q", resp.Courier)
	}
}

func TestShip_NotPacked_Rejected(t *testing.T) {
	svc, _, og := newTestService()
	og.orders["order-1"] = &order.Order{ID: "order-1", Status: order.StatusPaid}

	_, err := svc.Ship(context.Background(), "order-1", ShipRequest{})
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.ErrCode != apperr.OrderInvalidStatus {
		t.Fatalf("expected ORDER_INVALID_STATUS (order must be PACKED first), got: %v", err)
	}
}

func TestDeliver_FromInTransit_CompletesOrder(t *testing.T) {
	svc, repo, og := newTestService()
	og.orders["order-1"] = &order.Order{ID: "order-1", Status: order.StatusShipped}
	repo.items["shipment-1"] = &Shipment{ID: "shipment-1", OrderID: "order-1", Status: StatusInTransit}

	resp, err := svc.Deliver(context.Background(), "shipment-1")
	if err != nil {
		t.Fatalf("deliver failed: %v", err)
	}
	if resp.Status != string(StatusDelivered) {
		t.Fatalf("expected shipment status DELIVERED, got %s", resp.Status)
	}
	if resp.DeliveredAt == nil {
		t.Fatal("expected delivered_at to be set")
	}
	if og.orders["order-1"].Status != order.StatusCompleted {
		t.Fatalf("expected order status COMPLETED, got %s", og.orders["order-1"].Status)
	}
}

func TestDeliver_AlreadyDelivered_Rejected(t *testing.T) {
	svc, repo, og := newTestService()
	og.orders["order-1"] = &order.Order{ID: "order-1", Status: order.StatusCompleted}
	repo.items["shipment-1"] = &Shipment{ID: "shipment-1", OrderID: "order-1", Status: StatusDelivered}

	_, err := svc.Deliver(context.Background(), "shipment-1")
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.ErrCode != apperr.ShipmentInvalidStatus {
		t.Fatalf("expected SHIPMENT_INVALID_STATUS, got: %v", err)
	}
}

func TestGetByID_OwnershipEnforcedForCustomer(t *testing.T) {
	svc, repo, og := newTestService()
	og.orders["order-1"] = &order.Order{ID: "order-1", CustomerID: "owner", Status: order.StatusShipped}
	repo.items["shipment-1"] = &Shipment{ID: "shipment-1", OrderID: "order-1", Status: StatusInTransit}

	if _, err := svc.GetByID(context.Background(), "owner", "CUSTOMER", "shipment-1"); err != nil {
		t.Fatalf("owner should be able to view own shipment: %v", err)
	}

	_, err := svc.GetByID(context.Background(), "someone-else", "CUSTOMER", "shipment-1")
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.ErrCode != apperr.AuthForbidden {
		t.Fatalf("expected AUTH_FORBIDDEN for a different customer, got: %v", err)
	}

	if _, err := svc.GetByID(context.Background(), "warehouse-staff", "WAREHOUSE", "shipment-1"); err != nil {
		t.Fatalf("non-customer roles should be able to view any shipment: %v", err)
	}
}
