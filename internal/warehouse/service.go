package warehouse

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
	if _, err := s.repo.FindByCode(ctx, req.Code); err == nil {
		return nil, apperr.ConflictErr(apperr.WarehouseCodeExists, "Warehouse code already exists")
	} else if err != ErrNotFound {
		return nil, err
	}

	w := &Warehouse{Code: req.Code, Name: req.Name, Address: req.Address, City: req.City, Status: StatusActive}
	if err := s.repo.Create(ctx, w); err != nil {
		return nil, err
	}
	resp := toResponse(w)
	return &resp, nil
}

func (s *Service) GetByID(ctx context.Context, id string) (*Response, error) {
	w, err := s.get(ctx, id)
	if err != nil {
		return nil, err
	}
	resp := toResponse(w)
	return &resp, nil
}

func (s *Service) List(ctx context.Context, status string, page, limit int) ([]Response, int64, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	items, total, err := s.repo.List(ctx, status, page, limit)
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
	w, err := s.get(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		w.Name = *req.Name
	}
	if req.Address != nil {
		w.Address = *req.Address
	}
	if req.City != nil {
		w.City = *req.City
	}
	if req.Status != nil {
		w.Status = Status(*req.Status)
	}

	if err := s.repo.Update(ctx, w); err != nil {
		return nil, err
	}
	resp := toResponse(w)
	return &resp, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	if _, err := s.get(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

func (s *Service) get(ctx context.Context, id string) (*Warehouse, error) {
	w, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if err == ErrNotFound {
			return nil, apperr.New(http.StatusNotFound, apperr.WarehouseNotFound, "Warehouse not found")
		}
		return nil, err
	}
	return w, nil
}

// IsActive adalah titik integrasi resmi dengan domain Agent 3 (order
// service memanggil ini sebelum membuat order — spec Section 29, docs/
// architecture.md Section 2). Sengaja mengembalikan (bool, error) polos,
// bukan *apperr.AppError, supaya Agent 3 bebas memilih error code order-nya
// sendiri (WAREHOUSE_INACTIVE) tanpa coupling ke error code domain ini.
func (s *Service) IsActive(ctx context.Context, id string) (bool, error) {
	w, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if err == ErrNotFound {
			return false, nil
		}
		return false, err
	}
	return w.Status == StatusActive, nil
}
