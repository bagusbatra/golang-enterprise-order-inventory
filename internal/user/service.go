package user

import (
	"context"
	"net/http"

	"golang.org/x/crypto/bcrypt"

	"order-management/pkg/audit"
	apperr "order-management/pkg/errors"
)

type Service struct {
	repo  Repository
	audit audit.Logger
}

func NewService(repo Repository, auditLogger audit.Logger) *Service {
	return &Service{repo: repo, audit: auditLogger}
}

// Create dipakai ADMIN untuk membuat user dengan role apa pun (berbeda dari
// register publik yang selalu memaksa role CUSTOMER — lihat internal/auth).
func (s *Service) Create(ctx context.Context, req CreateRequest) (*Response, error) {
	if _, err := s.repo.FindByEmail(ctx, req.Email); err == nil {
		return nil, apperr.New(http.StatusConflict, apperr.EmailExists, "Email already registered")
	} else if err != ErrNotFound {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	u := &User{
		Name:         req.Name,
		Email:        req.Email,
		PasswordHash: string(hash),
		Role:         Role(req.Role),
		Status:       StatusActive,
	}
	if err := s.repo.Create(ctx, u); err != nil {
		return nil, err
	}

	resp := toResponse(u)
	return &resp, nil
}

func (s *Service) GetByID(ctx context.Context, id string) (*Response, error) {
	u, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if err == ErrNotFound {
			return nil, apperr.NotFoundErr("user", "User not found")
		}
		return nil, err
	}
	resp := toResponse(u)
	return &resp, nil
}

func (s *Service) List(ctx context.Context, f ListFilter) ([]Response, int64, error) {
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 20
	}

	users, total, err := s.repo.List(ctx, f)
	if err != nil {
		return nil, 0, err
	}

	resp := make([]Response, 0, len(users))
	for i := range users {
		resp = append(resp, toResponse(&users[i]))
	}
	return resp, total, nil
}

// Update mengubah profil/role user. Perubahan role WAJIB menghasilkan audit
// log (spec Section 20) karena ini operasi sensitif yang bisa menaikkan
// privilege seseorang.
func (s *Service) Update(ctx context.Context, actorID, id string, req UpdateRequest) (*Response, error) {
	u, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if err == ErrNotFound {
			return nil, apperr.NotFoundErr("user", "User not found")
		}
		return nil, err
	}

	oldRole := u.Role

	if req.Name != nil {
		u.Name = *req.Name
	}
	if req.Email != nil {
		u.Email = *req.Email
	}
	roleChanged := false
	if req.Role != nil && Role(*req.Role) != u.Role {
		u.Role = Role(*req.Role)
		roleChanged = true
	}

	if err := s.repo.Update(ctx, u); err != nil {
		return nil, err
	}

	if roleChanged {
		newRole := u.Role
		_ = s.audit.Log(ctx, audit.Entry{
			UserID:   &actorID,
			Action:   "ROLE_CHANGE",
			Entity:   "USER",
			EntityID: &u.ID,
			OldData:  map[string]string{"role": string(oldRole)},
			NewData:  map[string]string{"role": string(newRole)},
		})
	}

	resp := toResponse(u)
	return &resp, nil
}

func (s *Service) UpdateStatus(ctx context.Context, id string, status string) (*Response, error) {
	u, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if err == ErrNotFound {
			return nil, apperr.NotFoundErr("user", "User not found")
		}
		return nil, err
	}

	u.Status = Status(status)
	if err := s.repo.Update(ctx, u); err != nil {
		return nil, err
	}

	resp := toResponse(u)
	return &resp, nil
}
