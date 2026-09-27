package category

import (
	"context"
	"testing"
)

type fakeRepo struct {
	byName            map[string]*Category
	byID              map[string]*Category
	activeProductsFor map[string]bool
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		byName:            map[string]*Category{},
		byID:              map[string]*Category{},
		activeProductsFor: map[string]bool{},
	}
}

func (f *fakeRepo) Create(ctx context.Context, c *Category) error {
	c.ID = "cat-" + c.Name
	f.byName[c.Name] = c
	f.byID[c.ID] = c
	return nil
}

func (f *fakeRepo) FindByID(ctx context.Context, id string) (*Category, error) {
	if c, ok := f.byID[id]; ok {
		return c, nil
	}
	return nil, ErrNotFound
}

func (f *fakeRepo) FindByName(ctx context.Context, name string) (*Category, error) {
	if c, ok := f.byName[name]; ok {
		return c, nil
	}
	return nil, ErrNotFound
}

func (f *fakeRepo) List(ctx context.Context, search string, page, limit int) ([]Category, int64, error) {
	return nil, 0, nil
}

func (f *fakeRepo) Update(ctx context.Context, c *Category) error {
	f.byID[c.ID] = c
	f.byName[c.Name] = c
	return nil
}

func (f *fakeRepo) Delete(ctx context.Context, id string) error {
	delete(f.byID, id)
	return nil
}

func (f *fakeRepo) HasActiveProducts(ctx context.Context, categoryID string) (bool, error) {
	return f.activeProductsFor[categoryID], nil
}

func TestCreate_DuplicateName(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	ctx := context.Background()

	if _, err := svc.Create(ctx, CreateRequest{Name: "Laptop"}); err != nil {
		t.Fatalf("first create should succeed: %v", err)
	}
	_, err := svc.Create(ctx, CreateRequest{Name: "Laptop"})
	if err == nil {
		t.Fatal("expected error for duplicate category name, got nil")
	}
}

// TestDelete_WithActiveProducts_Rejected membuktikan spec Section 28:
// kategori yang masih punya produk ACTIVE tidak boleh dihapus.
func TestDelete_WithActiveProducts_Rejected(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	ctx := context.Background()

	created, err := svc.Create(ctx, CreateRequest{Name: "Gaming"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	repo.activeProductsFor[created.ID] = true

	err = svc.Delete(ctx, created.ID)
	if err == nil {
		t.Fatal("expected error deleting category with active products, got nil")
	}
}

func TestDelete_WithoutActiveProducts_Succeeds(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	ctx := context.Background()

	created, err := svc.Create(ctx, CreateRequest{Name: "Empty Category"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("expected delete to succeed, got error: %v", err)
	}
}
