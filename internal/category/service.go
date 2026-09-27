package category

import (
	"context"
	"net/http"

	apperr "order-management/pkg/errors"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (*Response, error) {
	if _, err := s.repo.FindByName(ctx, req.Name); err == nil {
		return nil, apperr.ConflictErr(apperr.CategoryNameExists, "Category name already exists")
	} else if err != ErrNotFound {
		return nil, err
	}

	c := &Category{Name: req.Name, Description: req.Description}
	if err := s.repo.Create(ctx, c); err != nil {
		return nil, err
	}
	resp := toResponse(c)
	return &resp, nil
}

func (s *Service) GetByID(ctx context.Context, id string) (*Response, error) {
	c, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if err == ErrNotFound {
			return nil, apperr.New(http.StatusNotFound, apperr.CategoryNotFound, "Category not found")
		}
		return nil, err
	}
	resp := toResponse(c)
	return &resp, nil
}

func (s *Service) List(ctx context.Context, search string, page, limit int) ([]Response, int64, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	items, total, err := s.repo.List(ctx, search, page, limit)
	if err != nil {
		return nil, 0, err
	}

	resp := make([]Response, 0, len(items))
	for i := range items {
		resp = append(resp, toResponse(&items[i]))
	}
	return resp, total, nil
}

func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (*Response, error) {
	c, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if err == ErrNotFound {
			return nil, apperr.New(http.StatusNotFound, apperr.CategoryNotFound, "Category not found")
		}
		return nil, err
	}

	if req.Name != nil && *req.Name != c.Name {
		if existing, err := s.repo.FindByName(ctx, *req.Name); err == nil && existing.ID != c.ID {
			return nil, apperr.ConflictErr(apperr.CategoryNameExists, "Category name already exists")
		}
		c.Name = *req.Name
	}
	if req.Description != nil {
		c.Description = *req.Description
	}

	if err := s.repo.Update(ctx, c); err != nil {
		return nil, err
	}
	resp := toResponse(c)
	return &resp, nil
}

// Delete menolak penghapusan jika kategori masih punya produk ACTIVE (spec
// Section 28) — dicek SEBELUM delete dieksekusi, bukan dibiarkan gagal di
// level FK constraint (yang akan menghasilkan error database mentah yang
// tidak jelas ke client).
func (s *Service) Delete(ctx context.Context, id string) error {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		if err == ErrNotFound {
			return apperr.New(http.StatusNotFound, apperr.CategoryNotFound, "Category not found")
		}
		return err
	}

	hasActive, err := s.repo.HasActiveProducts(ctx, id)
	if err != nil {
		return err
	}
	if hasActive {
		return apperr.ConflictErr(apperr.CategoryHasActiveProducts, "Category still has active products and cannot be deleted")
	}

	return s.repo.Delete(ctx, id)
}
