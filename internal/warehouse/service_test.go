package warehouse

import (
	"context"
	"testing"
)

type fakeRepo struct {
	byCode map[string]*Warehouse
	byID   map[string]*Warehouse
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{byCode: map[string]*Warehouse{}, byID: map[string]*Warehouse{}}
}

func (f *fakeRepo) Create(ctx context.Context, w *Warehouse) error {
	w.ID = "wh-" + w.Code
	f.byCode[w.Code] = w
	f.byID[w.ID] = w
	return nil
}

func (f *fakeRepo) FindByID(ctx context.Context, id string) (*Warehouse, error) {
	if w, ok := f.byID[id]; ok {
		return w, nil
	}
	return nil, ErrNotFound
}

func (f *fakeRepo) FindByCode(ctx context.Context, code string) (*Warehouse, error) {
	if w, ok := f.byCode[code]; ok {
		return w, nil
	}
	return nil, ErrNotFound
}

func (f *fakeRepo) List(ctx context.Context, status string, page, limit int) ([]Warehouse, int64, error) {
	return nil, 0, nil
}

func (f *fakeRepo) Update(ctx context.Context, w *Warehouse) error {
	f.byID[w.ID] = w
	f.byCode[w.Code] = w
	return nil
}

func (f *fakeRepo) Delete(ctx context.Context, id string) error {
	delete(f.byID, id)
	return nil
}

func TestCreate_DuplicateCode(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	ctx := context.Background()

	if _, err := svc.Create(ctx, CreateRequest{Code: "SBY", Name: "Surabaya", Address: "Jl. A", City: "Surabaya"}); err != nil {
		t.Fatalf("first create should succeed: %v", err)
	}
	_, err := svc.Create(ctx, CreateRequest{Code: "SBY", Name: "Surabaya 2", Address: "Jl. B", City: "Surabaya"})
	if err == nil {
		t.Fatal("expected error for duplicate warehouse code, got nil")
	}
}

// TestIsActive membuktikan integrasi resmi dengan Order Service milik
// Agent 3 (spec Section 29, docs/architecture.md Section 2): warehouse
// INACTIVE tidak boleh dipilih untuk order baru.
func TestIsActive(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	ctx := context.Background()

	created, err := svc.Create(ctx, CreateRequest{Code: "JKT", Name: "Jakarta", Address: "Jl. C", City: "Jakarta"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	active, err := svc.IsActive(ctx, created.ID)
	if err != nil || !active {
		t.Fatalf("expected newly created warehouse to be active, got active=%v err=%v", active, err)
	}

	inactive := "INACTIVE"
	if _, err := svc.Update(ctx, created.ID, UpdateRequest{Status: &inactive}); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	active, err = svc.IsActive(ctx, created.ID)
	if err != nil || active {
		t.Fatalf("expected warehouse to be inactive after update, got active=%v err=%v", active, err)
	}
}

func TestIsActive_NotFound_ReturnsFalseNoError(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)

	active, err := svc.IsActive(context.Background(), "does-not-exist")
	if err != nil {
		t.Fatalf("expected no error for not-found warehouse, got: %v", err)
	}
	if active {
		t.Fatal("expected active=false for non-existent warehouse")
	}
}
