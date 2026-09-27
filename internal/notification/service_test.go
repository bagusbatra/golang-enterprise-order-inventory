package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"order-management/internal/worker"
	apperr "order-management/pkg/errors"
)

type fakeRepo struct {
	items  map[string]*Notification
	nextID int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{items: map[string]*Notification{}}
}

func (f *fakeRepo) Create(ctx context.Context, n *Notification) error {
	f.nextID++
	n.ID = fmt.Sprintf("notif-%d", f.nextID)
	c := *n
	f.items[n.ID] = &c
	return nil
}

func (f *fakeRepo) ExistsByUserTypeMessage(ctx context.Context, userID, notifType, message string) (bool, error) {
	for _, n := range f.items {
		if n.UserID == userID && n.Type == notifType && n.Message == message {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeRepo) FindByID(ctx context.Context, id string) (*Notification, error) {
	n, ok := f.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	c := *n
	return &c, nil
}

func (f *fakeRepo) List(ctx context.Context, userID string, isRead *bool, page, limit int) ([]Notification, int64, error) {
	var out []Notification
	for _, n := range f.items {
		if n.UserID != userID {
			continue
		}
		if isRead != nil && n.IsRead != *isRead {
			continue
		}
		out = append(out, *n)
	}
	return out, int64(len(out)), nil
}

func (f *fakeRepo) MarkRead(ctx context.Context, id string) error {
	n, ok := f.items[id]
	if !ok {
		return ErrNotFound
	}
	n.IsRead = true
	return nil
}

func (f *fakeRepo) MarkAllRead(ctx context.Context, userID string) error {
	for _, n := range f.items {
		if n.UserID == userID {
			n.IsRead = true
		}
	}
	return nil
}

func TestCreateIfNotExists_CreatesOnce(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)

	if err := svc.CreateIfNotExists(context.Background(), "user-1", "PAYMENT_PAID", "Payment Successful", "msg-1"); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if len(repo.items) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(repo.items))
	}
}

// TestCreateIfNotExists_Idempotent_NoDuplicate mereproduksi spec Section 80:
// event dengan semantik sama diproses berkali-kali TIDAK menghasilkan
// notification duplikat.
func TestCreateIfNotExists_Idempotent_NoDuplicate(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)

	for i := 0; i < 5; i++ {
		if err := svc.CreateIfNotExists(context.Background(), "user-1", "PAYMENT_PAID", "Payment Successful", "Your payment for order ORD-1 has been confirmed."); err != nil {
			t.Fatalf("call #%d failed: %v", i+1, err)
		}
	}
	if len(repo.items) != 1 {
		t.Fatalf("expected exactly 1 notification despite 5 identical calls, got %d", len(repo.items))
	}
}

func TestCreateIfNotExists_DifferentMessage_CreatesSeparately(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)

	_ = svc.CreateIfNotExists(context.Background(), "user-1", "PAYMENT_PAID", "Payment Successful", "order ORD-1 confirmed")
	_ = svc.CreateIfNotExists(context.Background(), "user-1", "PAYMENT_PAID", "Payment Successful", "order ORD-2 confirmed")

	if len(repo.items) != 2 {
		t.Fatalf("expected 2 distinct notifications for 2 distinct orders, got %d", len(repo.items))
	}
}

func TestHandlePaymentEvent_CreatesNotificationWithSpecMessage(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)

	payload, _ := json.Marshal(map[string]string{
		"order_id": "order-1", "order_number": "ORD-20260927-000001", "customer_id": "customer-1",
	})
	event := worker.Event{EventType: "PAYMENT_PAID", Payload: payload}

	if err := svc.HandlePaymentEvent(context.Background(), event); err != nil {
		t.Fatalf("handler failed: %v", err)
	}

	var found *Notification
	for _, n := range repo.items {
		found = n
	}
	if found == nil {
		t.Fatal("expected a notification to be created")
	}
	if found.Title != "Payment Successful" {
		t.Fatalf("expected title 'Payment Successful', got %q", found.Title)
	}
	wantMessage := "Your payment for order ORD-20260927-000001 has been confirmed."
	if found.Message != wantMessage {
		t.Fatalf("expected message %q, got %q", wantMessage, found.Message)
	}
	if found.UserID != "customer-1" {
		t.Fatalf("expected notification addressed to customer-1, got %s", found.UserID)
	}
}

// TestHandlePaymentEvent_ProcessedTwice_NoDuplicate mereproduksi spec:
// consumer memproses event PAYMENT_PAID yang SAMA dua kali (misal karena
// redelivery setelah crash) -> hanya 1 notification.
func TestHandlePaymentEvent_ProcessedTwice_NoDuplicate(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)

	payload, _ := json.Marshal(map[string]string{
		"order_id": "order-1", "order_number": "ORD-20260927-000001", "customer_id": "customer-1",
	})
	event := worker.Event{EventType: "PAYMENT_PAID", Payload: payload}

	if err := svc.HandlePaymentEvent(context.Background(), event); err != nil {
		t.Fatalf("first handle failed: %v", err)
	}
	if err := svc.HandlePaymentEvent(context.Background(), event); err != nil {
		t.Fatalf("second handle failed: %v", err)
	}

	if len(repo.items) != 1 {
		t.Fatalf("expected exactly 1 notification after processing the same event twice, got %d", len(repo.items))
	}
}

func TestHandlePaymentEvent_IgnoresOtherEventTypes(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)

	payload, _ := json.Marshal(map[string]string{"order_id": "order-1", "customer_id": "customer-1"})
	event := worker.Event{EventType: "ORDER_CREATED", Payload: payload}

	if err := svc.HandlePaymentEvent(context.Background(), event); err != nil {
		t.Fatalf("expected nil error for irrelevant event type, got: %v", err)
	}
	if len(repo.items) != 0 {
		t.Fatalf("expected no notification for an unrelated event type, got %d", len(repo.items))
	}
}

func TestHandleOrderEvent_ShippedCreatesNotification(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)

	payload, _ := json.Marshal(map[string]string{"order_id": "order-1", "customer_id": "customer-1"})
	event := worker.Event{EventType: "ORDER_SHIPPED", Payload: payload}

	if err := svc.HandleOrderEvent(context.Background(), event); err != nil {
		t.Fatalf("handler failed: %v", err)
	}

	var found *Notification
	for _, n := range repo.items {
		found = n
	}
	if found == nil {
		t.Fatal("expected a notification to be created")
	}
	if found.Title != "Order Shipped" || found.Message != "Your order has been shipped." {
		t.Fatalf("unexpected notification content: %+v", found)
	}
}

func TestHandleOrderEvent_IgnoresNonShippedEvents(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)

	payload, _ := json.Marshal(map[string]string{"order_id": "order-1", "customer_id": "customer-1"})
	for _, eventType := range []string{"ORDER_CREATED", "ORDER_CANCELLED"} {
		event := worker.Event{EventType: eventType, Payload: payload}
		if err := svc.HandleOrderEvent(context.Background(), event); err != nil {
			t.Fatalf("expected nil error for %s, got: %v", eventType, err)
		}
	}
	if len(repo.items) != 0 {
		t.Fatalf("expected no notification for ORDER_CREATED/ORDER_CANCELLED, got %d", len(repo.items))
	}
}

func TestMarkRead_OwnershipEnforced(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	repo.items["n1"] = &Notification{ID: "n1", UserID: "owner", Title: "t", Message: "m"}

	if err := svc.MarkRead(context.Background(), "owner", "n1"); err != nil {
		t.Fatalf("owner should be able to mark own notification as read: %v", err)
	}
	if !repo.items["n1"].IsRead {
		t.Fatal("expected notification to be marked read")
	}

	repo.items["n2"] = &Notification{ID: "n2", UserID: "owner", Title: "t", Message: "m"}
	err := svc.MarkRead(context.Background(), "someone-else", "n2")
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.ErrCode != apperr.AuthForbidden {
		t.Fatalf("expected AUTH_FORBIDDEN for a different user, got: %v", err)
	}
}

func TestMarkAllRead(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	repo.items["n1"] = &Notification{ID: "n1", UserID: "user-1", Title: "t", Message: "m1"}
	repo.items["n2"] = &Notification{ID: "n2", UserID: "user-1", Title: "t", Message: "m2"}
	repo.items["n3"] = &Notification{ID: "n3", UserID: "user-2", Title: "t", Message: "m3"}

	if err := svc.MarkAllRead(context.Background(), "user-1"); err != nil {
		t.Fatalf("mark all read failed: %v", err)
	}
	if !repo.items["n1"].IsRead || !repo.items["n2"].IsRead {
		t.Fatal("expected both of user-1's notifications to be marked read")
	}
	if repo.items["n3"].IsRead {
		t.Fatal("expected user-2's notification to remain unread")
	}
}
