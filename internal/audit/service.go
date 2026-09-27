package audit

import (
	"context"
	"encoding/json"
	"errors"

	pkgaudit "order-management/pkg/audit"
	apperr "order-management/pkg/errors"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Log mengimplementasikan pkg/audit.Logger — interface netral yang sudah
// disiapkan Agent 2 (pkg/audit/audit.go) supaya domain manapun (Agent 2:
// auth/product/user, Agent 3: inventory/order/payment) bisa mencatat audit
// tanpa import langsung ke internal/audit. Di-inject sebagai concrete
// implementation saat wiring di main.go, menggantikan pkgaudit.NopLogger.
func (s *Service) Log(ctx context.Context, entry pkgaudit.Entry) error {
	oldData, err := marshalOrNil(entry.OldData)
	if err != nil {
		return err
	}
	newData, err := marshalOrNil(entry.NewData)
	if err != nil {
		return err
	}

	return s.repo.Create(ctx, &AuditLog{
		UserID:    entry.UserID,
		Action:    entry.Action,
		Entity:    entry.Entity,
		EntityID:  entry.EntityID,
		OldData:   oldData,
		NewData:   newData,
		IPAddress: entry.IPAddress,
		UserAgent: entry.UserAgent,
	})
}

func marshalOrNil(v any) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}

func (s *Service) GetByID(ctx context.Context, id string) (*Response, error) {
	a, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, apperr.NotFoundErr("audit_log", "Audit log not found")
		}
		return nil, err
	}
	resp := toResponse(a)
	return &resp, nil
}

func (s *Service) List(ctx context.Context, entity, userID string, page, limit int) ([]Response, int64, error) {
	items, total, err := s.repo.List(ctx, entity, userID, page, limit)
	if err != nil {
		return nil, 0, err
	}
	resp := make([]Response, 0, len(items))
	for i := range items {
		resp = append(resp, toResponse(&items[i]))
	}
	return resp, total, nil
}
