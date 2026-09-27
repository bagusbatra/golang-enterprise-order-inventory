package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	pkgaudit "order-management/pkg/audit"
)

type fakeRepo struct {
	items  map[string]*AuditLog
	nextID int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{items: map[string]*AuditLog{}}
}

func (f *fakeRepo) Create(ctx context.Context, a *AuditLog) error {
	f.nextID++
	a.ID = fmt.Sprintf("audit-%d", f.nextID)
	c := *a
	f.items[a.ID] = &c
	return nil
}

func (f *fakeRepo) FindByID(ctx context.Context, id string) (*AuditLog, error) {
	a, ok := f.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	c := *a
	return &c, nil
}

func (f *fakeRepo) List(ctx context.Context, entity, userID string, page, limit int) ([]AuditLog, int64, error) {
	var out []AuditLog
	for _, a := range f.items {
		if entity != "" && a.Entity != entity {
			continue
		}
		if userID != "" && (a.UserID == nil || *a.UserID != userID) {
			continue
		}
		out = append(out, *a)
	}
	return out, int64(len(out)), nil
}

func TestLog_MarshalsOldAndNewData(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)

	userID := "user-1"
	entityID := "inv-1"
	err := svc.Log(context.Background(), pkgaudit.Entry{
		UserID:   &userID,
		Action:   "INVENTORY_ADJUST",
		Entity:   "inventory",
		EntityID: &entityID,
		OldData:  map[string]int{"quantity": 10},
		NewData:  map[string]int{"quantity": 7},
	})
	if err != nil {
		t.Fatalf("log failed: %v", err)
	}
	if len(repo.items) != 1 {
		t.Fatalf("expected 1 audit log, got %d", len(repo.items))
	}

	var stored *AuditLog
	for _, a := range repo.items {
		stored = a
	}
	if stored.Action != "INVENTORY_ADJUST" || stored.Entity != "inventory" {
		t.Fatalf("unexpected action/entity: %+v", stored)
	}

	var oldData map[string]int
	if err := json.Unmarshal(stored.OldData, &oldData); err != nil {
		t.Fatalf("failed to unmarshal old_data: %v", err)
	}
	if oldData["quantity"] != 10 {
		t.Fatalf("expected old_data.quantity=10, got %d", oldData["quantity"])
	}

	var newData map[string]int
	if err := json.Unmarshal(stored.NewData, &newData); err != nil {
		t.Fatalf("failed to unmarshal new_data: %v", err)
	}
	if newData["quantity"] != 7 {
		t.Fatalf("expected new_data.quantity=7, got %d", newData["quantity"])
	}
}

func TestLog_NilDataFields_StoredAsNil(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)

	if err := svc.Log(context.Background(), pkgaudit.Entry{Action: "PAYMENT_CALLBACK", Entity: "PAYMENT"}); err != nil {
		t.Fatalf("log failed: %v", err)
	}

	var stored *AuditLog
	for _, a := range repo.items {
		stored = a
	}
	if stored.OldData != nil || stored.NewData != nil {
		t.Fatalf("expected nil old_data/new_data when Entry doesn't set them, got old=%s new=%s", stored.OldData, stored.NewData)
	}
	if stored.UserID != nil {
		t.Fatal("expected nil user_id for a system/gateway-triggered entry")
	}
}

func TestGetByID_NotFound(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)

	if _, err := svc.GetByID(context.Background(), "does-not-exist"); err == nil {
		t.Fatal("expected an error for a non-existent audit log")
	}
}

func TestList_FiltersByEntityAndUser(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)

	userA := "user-a"
	userB := "user-b"
	_ = svc.Log(context.Background(), pkgaudit.Entry{UserID: &userA, Action: "CREATE_ORDER", Entity: "ORDER"})
	_ = svc.Log(context.Background(), pkgaudit.Entry{UserID: &userB, Action: "CREATE_ORDER", Entity: "ORDER"})
	_ = svc.Log(context.Background(), pkgaudit.Entry{UserID: &userA, Action: "INVENTORY_ADJUST", Entity: "inventory"})

	items, total, err := svc.List(context.Background(), "ORDER", "", 1, 20)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected 2 ORDER audit logs, got %d", total)
	}
	_ = items

	items, total, err = svc.List(context.Background(), "", "user-a", 1, 20)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected 2 audit logs for user-a, got %d", total)
	}
	_ = items
}
