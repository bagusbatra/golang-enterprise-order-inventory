package product

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"

	"order-management/pkg/audit"
)

// fakeRepo adalah in-memory implementasi Repository khusus unit test, tanpa
// bergantung pada PostgreSQL sungguhan (spec Section 70).
type fakeRepo struct {
	bySKU map[string]*Product
	byID  map[string]*Product
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{bySKU: map[string]*Product{}, byID: map[string]*Product{}}
}

func (f *fakeRepo) Create(ctx context.Context, p *Product) error {
	p.ID = "prod-" + p.SKU
	f.bySKU[p.SKU] = p
	f.byID[p.ID] = p
	return nil
}

func (f *fakeRepo) FindByID(ctx context.Context, id string) (*Product, error) {
	if p, ok := f.byID[id]; ok {
		return p, nil
	}
	return nil, ErrNotFound
}

func (f *fakeRepo) FindBySKU(ctx context.Context, sku string) (*Product, error) {
	if p, ok := f.bySKU[sku]; ok {
		return p, nil
	}
	return nil, ErrNotFound
}

func (f *fakeRepo) List(ctx context.Context, filter ListFilter) ([]Product, int64, error) {
	return nil, 0, nil
}

func (f *fakeRepo) Update(ctx context.Context, p *Product) error {
	f.byID[p.ID] = p
	f.bySKU[p.SKU] = p
	return nil
}

func (f *fakeRepo) Delete(ctx context.Context, id string) error {
	delete(f.byID, id)
	return nil
}

func newTestService() *Service {
	// redisClient nil -> cache-aside otomatis dilewati (lihat Service.NewService).
	return NewService(newFakeRepo(), nil, 0, audit.NopLogger{})
}

func TestCreate_DuplicateSKU(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	req := CreateRequest{
		CategoryID: "cat-1", SKU: "SKU-001", Name: "Product A",
		Price: decimal.NewFromInt(100000), CostPrice: decimal.NewFromInt(50000), Weight: decimal.NewFromInt(1),
	}
	if _, err := svc.Create(ctx, "actor-1", req); err != nil {
		t.Fatalf("first create should succeed: %v", err)
	}

	_, err := svc.Create(ctx, "actor-1", req)
	if err == nil {
		t.Fatal("expected error for duplicate SKU, got nil")
	}
}

func TestCreate_InvalidPrice_Rejected(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	req := CreateRequest{
		CategoryID: "cat-1", SKU: "SKU-002", Name: "Product B",
		Price: decimal.NewFromInt(0), CostPrice: decimal.NewFromInt(50000), Weight: decimal.NewFromInt(1),
	}
	_, err := svc.Create(ctx, "actor-1", req)
	if err == nil {
		t.Fatal("expected error for price=0, got nil")
	}
}

func TestCreate_NegativeWeight_Rejected(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	req := CreateRequest{
		CategoryID: "cat-1", SKU: "SKU-003", Name: "Product C",
		Price: decimal.NewFromInt(100000), CostPrice: decimal.NewFromInt(50000), Weight: decimal.NewFromInt(-1),
	}
	_, err := svc.Create(ctx, "actor-1", req)
	if err == nil {
		t.Fatal("expected error for negative weight, got nil")
	}
}

// TestGetPriceSnapshot_InactiveProduct_Rejected membuktikan order tidak bisa
// dibuat dari produk yang tidak ACTIVE (integrasi dengan Order Service
// milik Agent 3 — docs/architecture.md Section 2).
func TestGetPriceSnapshot_InactiveProduct_Rejected(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	created, err := svc.Create(ctx, "actor-1", CreateRequest{
		CategoryID: "cat-1", SKU: "SKU-004", Name: "Product D",
		Price: decimal.NewFromInt(100000), CostPrice: decimal.NewFromInt(50000), Weight: decimal.NewFromInt(1),
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	inactive := "INACTIVE"
	if _, err := svc.Update(ctx, "actor-1", created.ID, UpdateRequest{Status: &inactive}); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	_, err = svc.GetPriceSnapshot(ctx, created.ID)
	if err == nil {
		t.Fatal("expected error when snapshotting price of inactive product, got nil")
	}
}

func TestGetPriceSnapshot_UsesCurrentPrice(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	created, err := svc.Create(ctx, "actor-1", CreateRequest{
		CategoryID: "cat-1", SKU: "SKU-005", Name: "Product E",
		Price: decimal.NewFromInt(23000000), CostPrice: decimal.NewFromInt(20000000), Weight: decimal.NewFromInt(1),
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	price, err := svc.GetPriceSnapshot(ctx, created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !price.Equal(decimal.NewFromInt(23000000)) {
		t.Fatalf("expected price 23000000, got %s", price.String())
	}
}
