package shipment

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"order-management/internal/order"
	"order-management/internal/websocket"
	apperr "order-management/pkg/errors"
)

// TxRunner — lihat internal/inventory/service.go untuk rationale yang sama.
type TxRunner interface {
	Transaction(fc func(tx *gorm.DB) error, opts ...*sql.TxOptions) error
}

// OrderGateway — titik integrasi dengan internal/order (Agent 3 sendiri),
// method passthrough yang sama dipakai Payment Service (Iterasi 06).
type OrderGateway interface {
	FindRawByID(ctx context.Context, orderID string) (*order.Order, error)
	LockForUpdate(ctx context.Context, tx *gorm.DB, orderID string) (*order.Order, error)
	UpdateStatus(ctx context.Context, tx *gorm.DB, orderID string, status order.Status) error
}

// EventPublisher — lihat internal/order/service.go untuk rationale yang
// sama. Opsional (nil-safe lewat SetEventPublisher).
type EventPublisher interface {
	Publish(ctx context.Context, stream, eventType, aggregateID string, payload any) error
}

const defaultCourier = "Simulated Courier"

type Service struct {
	repo   Repository
	db     TxRunner
	order  OrderGateway
	events EventPublisher
	ws     *websocket.Manager
}

func NewService(repo Repository, db TxRunner, orderGateway OrderGateway) *Service {
	return &Service{repo: repo, db: db, order: orderGateway}
}

func (s *Service) SetEventPublisher(p EventPublisher) {
	s.events = p
}

// SetWebSocketManager — lihat internal/order/service.go untuk rationale
// yang sama (Iterasi 10).
func (s *Service) SetWebSocketManager(m *websocket.Manager) {
	s.ws = m
}

func (s *Service) notifyStatusUpdated(customerID, orderID string, status order.Status) {
	if s.ws == nil {
		return
	}
	s.ws.Notify(customerID, websocket.Event{
		Event: "ORDER_STATUS_UPDATED",
		Data:  map[string]string{"order_id": orderID, "status": string(status)},
	})
}

// Pack (spec Section 51 diagram: PAID -> PROCESSING -> PACKED). Tidak ada
// endpoint terpisah untuk PROCESSING, jadi kedua transisi terjadi dalam
// SATU transaksi/panggilan — CanTransition tetap dipanggil untuk KEDUANYA
// (bukan langsung set PACKED) supaya state machine tetap satu-satunya
// sumber kebenaran, sesuai aturan di seluruh modul Agent 3.
func (s *Service) Pack(ctx context.Context, orderID string) (*order.Response, error) {
	var result *order.Order

	err := s.db.Transaction(func(tx *gorm.DB) error {
		o, err := s.order.LockForUpdate(ctx, tx, orderID)
		if err != nil {
			if errors.Is(err, order.ErrNotFound) {
				return apperr.New(http.StatusNotFound, apperr.OrderNotFound, "Order not found")
			}
			return err
		}

		if !order.CanTransition(o.Status, order.StatusProcessing) {
			return apperr.New(http.StatusUnprocessableEntity, apperr.OrderInvalidStatus, "Order must be PAID before it can be packed")
		}
		if err := s.order.UpdateStatus(ctx, tx, o.ID, order.StatusProcessing); err != nil {
			return err
		}
		if err := s.order.UpdateStatus(ctx, tx, o.ID, order.StatusPacked); err != nil {
			return err
		}

		o.Status = order.StatusPacked
		result = o
		return nil
	})
	if err != nil {
		return nil, err
	}

	resp := order.ToResponse(result)
	s.notifyStatusUpdated(result.CustomerID, orderID, order.StatusPacked)
	return &resp, nil
}

// Ship (spec Section 51-52): order harus PACKED. Membuat tracking_number +
// shipment record, order->SHIPPED, publish ORDER_SHIPPED (Notification
// Worker Iterasi 08 yang membuat notification-nya, bukan di sini — sesuai
// arsitektur event-driven, architecture.md Section 3).
func (s *Service) Ship(ctx context.Context, orderID string, req ShipRequest) (*Response, error) {
	courier := req.Courier
	if courier == "" {
		courier = defaultCourier
	}

	var sh *Shipment
	var customerID string

	err := s.db.Transaction(func(tx *gorm.DB) error {
		o, err := s.order.LockForUpdate(ctx, tx, orderID)
		if err != nil {
			if errors.Is(err, order.ErrNotFound) {
				return apperr.New(http.StatusNotFound, apperr.OrderNotFound, "Order not found")
			}
			return err
		}
		if !order.CanTransition(o.Status, order.StatusShipped) {
			return apperr.New(http.StatusUnprocessableEntity, apperr.OrderInvalidStatus, "Order must be PACKED before it can be shipped")
		}

		now := time.Now()
		sh = &Shipment{
			OrderID:        o.ID,
			Courier:        courier,
			TrackingNumber: "TRK-" + uuid.NewString(),
			Status:         StatusInTransit,
			ShippedAt:      &now,
		}
		if err := s.repo.Create(ctx, tx, sh); err != nil {
			return err
		}
		if err := s.order.UpdateStatus(ctx, tx, o.ID, order.StatusShipped); err != nil {
			return err
		}

		customerID = o.CustomerID
		return nil
	})
	if err != nil {
		return nil, err
	}

	if s.events != nil {
		_ = s.events.Publish(ctx, "order_events", "ORDER_SHIPPED", orderID, map[string]string{
			"order_id":    orderID,
			"customer_id": customerID,
		})
	}
	s.notifyStatusUpdated(customerID, orderID, order.StatusShipped)

	resp := toResponse(sh)
	return &resp, nil
}

// Deliver (spec Section 52): shipment harus IN_TRANSIT atau PICKED_UP
// (docs/api-contract.md Section 9) -> DELIVERED, order -> COMPLETED.
func (s *Service) Deliver(ctx context.Context, shipmentID string) (*Response, error) {
	var result *Shipment
	var customerID string

	err := s.db.Transaction(func(tx *gorm.DB) error {
		sh, err := s.repo.LockForUpdate(ctx, tx, shipmentID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return apperr.New(http.StatusNotFound, apperr.ShipmentNotFound, "Shipment not found")
			}
			return err
		}
		if sh.Status != StatusInTransit && sh.Status != StatusPickedUp {
			return apperr.New(http.StatusUnprocessableEntity, apperr.ShipmentInvalidStatus, "Shipment must be IN_TRANSIT or PICKED_UP before it can be delivered")
		}

		now := time.Now()
		sh.Status = StatusDelivered
		sh.DeliveredAt = &now
		if err := s.repo.UpdateTx(ctx, tx, sh); err != nil {
			return err
		}

		o, err := s.order.LockForUpdate(ctx, tx, sh.OrderID)
		if err != nil {
			return err
		}
		if !order.CanTransition(o.Status, order.StatusCompleted) {
			return apperr.New(http.StatusUnprocessableEntity, apperr.OrderInvalidStatus, "Order is not in a state that can be completed")
		}
		if err := s.order.UpdateStatus(ctx, tx, o.ID, order.StatusCompleted); err != nil {
			return err
		}

		result = sh
		customerID = o.CustomerID
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.notifyStatusUpdated(customerID, result.OrderID, order.StatusCompleted)
	resp := toResponse(result)
	return &resp, nil
}

func (s *Service) GetByID(ctx context.Context, actorUserID, actorRole, id string) (*Response, error) {
	sh, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, apperr.New(http.StatusNotFound, apperr.ShipmentNotFound, "Shipment not found")
		}
		return nil, err
	}

	if actorRole == "CUSTOMER" {
		o, err := s.order.FindRawByID(ctx, sh.OrderID)
		if err != nil {
			return nil, err
		}
		if o.CustomerID != actorUserID {
			return nil, apperr.ForbiddenErr("You do not have permission to view this shipment")
		}
	}

	resp := toResponse(sh)
	return &resp, nil
}
