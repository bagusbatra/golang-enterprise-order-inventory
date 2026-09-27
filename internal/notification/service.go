package notification

import (
	"context"
	"errors"
	"net/http"

	apperr "order-management/pkg/errors"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// CreateIfNotExists mengimplementasikan idempotency (spec Section 80,
// 03-BACKEND-CORE.md Iterasi 08). Tabel `notifications` (database-contract.md
// Section 12, LOCKED) TIDAK punya kolom reference_id/event_type terpisah,
// jadi dedupe key yang dipakai adalah (user_id, type, message) EXACT MATCH.
// Ini valid karena caller (lihat consumer.go) SELALU membangun `message`
// secara deterministik dari data event (order_number, teks statis) — dua
// event dengan semantik identik menghasilkan message yang identik, dan
// event yang berbeda menghasilkan message yang berbeda. Efek bisnis lain
// (stock-out, transisi status) sudah dijamin idempotent di domain masing-
// masing (inventory/order/payment); ini cukup untuk notification saja.
func (s *Service) CreateIfNotExists(ctx context.Context, userID, notifType, title, message string) error {
	exists, err := s.repo.ExistsByUserTypeMessage(ctx, userID, notifType, message)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return s.repo.Create(ctx, &Notification{UserID: userID, Type: notifType, Title: title, Message: message})
}

func (s *Service) List(ctx context.Context, userID string, isRead *bool, page, limit int) ([]Response, int64, error) {
	items, total, err := s.repo.List(ctx, userID, isRead, page, limit)
	if err != nil {
		return nil, 0, err
	}
	resp := make([]Response, 0, len(items))
	for i := range items {
		resp = append(resp, toResponse(&items[i]))
	}
	return resp, total, nil
}

func (s *Service) MarkRead(ctx context.Context, actorUserID, id string) error {
	n, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apperr.New(http.StatusNotFound, apperr.NotFound, "Notification not found")
		}
		return err
	}
	if n.UserID != actorUserID {
		return apperr.ForbiddenErr("You do not have permission to modify this notification")
	}
	return s.repo.MarkRead(ctx, id)
}

func (s *Service) MarkAllRead(ctx context.Context, userID string) error {
	return s.repo.MarkAllRead(ctx, userID)
}
