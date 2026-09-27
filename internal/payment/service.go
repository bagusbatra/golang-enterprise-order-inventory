package payment

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

// TxRunner — lihat internal/inventory/service.go untuk rationale yang sama:
// diabstraksi dari *gorm.DB supaya service_test.go tidak butuh koneksi
// database nyata.
type TxRunner interface {
	Transaction(fc func(tx *gorm.DB) error, opts ...*sql.TxOptions) error
}

// OrderGateway adalah titik integrasi dengan internal/order (Agent 3
// sendiri) — diabstraksi sebagai interface lokal demi testability murni
// (bukan lintas-agent seperti WarehouseChecker/PriceProvider di order).
// *order.Service memenuhi ini lewat method passthrough di service.go-nya.
type OrderGateway interface {
	FindRawByID(ctx context.Context, orderID string) (*order.Order, error)
	LockForUpdate(ctx context.Context, tx *gorm.DB, orderID string) (*order.Order, error)
	UpdateStatus(ctx context.Context, tx *gorm.DB, orderID string, status order.Status) error
	ItemsByOrderID(ctx context.Context, tx *gorm.DB, orderID string) ([]order.OrderItem, error)
}

// StockAdjuster adalah titik integrasi dengan internal/inventory (Agent 3
// sendiri) untuk efek stock-out (payment PAID) dan release (expiration).
type StockAdjuster interface {
	StockOut(ctx context.Context, tx *gorm.DB, productID, warehouseID string, qty int, referenceID string) error
	ReleaseStock(ctx context.Context, tx *gorm.DB, productID, warehouseID string, qty int, referenceID string) error
}

// EventPublisher — lihat internal/order/service.go untuk rationale yang
// sama. Opsional (nil-safe lewat SetEventPublisher), dipanggil SETELAH
// transaksi commit.
type EventPublisher interface {
	Publish(ctx context.Context, stream, eventType, aggregateID string, payload any) error
}

const expirationWindow = 30 * time.Minute

type Service struct {
	repo   Repository
	db     TxRunner
	order  OrderGateway
	stock  StockAdjuster
	events EventPublisher
	ws     *websocket.Manager
}

func NewService(repo Repository, db TxRunner, orderGateway OrderGateway, stock StockAdjuster) *Service {
	return &Service{repo: repo, db: db, order: orderGateway, stock: stock}
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

// CreatePayment (spec Section 41): owner CUSTOMER saja. Order harus berada
// di status WAITING_PAYMENT — memakai order.CanTransition(status, PAID)
// sebagai pengecekan tunggal (state machine adalah SATU-SATUNYA sumber
// kebenaran urutan status, bukan pengecekan ad-hoc di sini).
func (s *Service) CreatePayment(ctx context.Context, actorUserID, orderID string, req CreatePaymentRequest) (*Response, error) {
	o, err := s.order.FindRawByID(ctx, orderID)
	if err != nil {
		if errors.Is(err, order.ErrNotFound) {
			return nil, apperr.New(http.StatusNotFound, apperr.OrderNotFound, "Order not found")
		}
		return nil, err
	}
	if o.CustomerID != actorUserID {
		return nil, apperr.ForbiddenErr("You do not have permission to pay for this order")
	}

	alreadyPaid, err := s.repo.ExistsPaidForOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if alreadyPaid {
		return nil, apperr.New(http.StatusConflict, apperr.PaymentAlreadyPaid, "This order has already been paid")
	}
	if !order.CanTransition(o.Status, order.StatusPaid) {
		return nil, apperr.New(http.StatusUnprocessableEntity, apperr.OrderInvalidStatus, "Order is not awaiting payment")
	}

	method := Method(req.PaymentMethod)
	if method == "" {
		method = MethodBankTransfer
	}

	p := &Payment{
		OrderID:       orderID,
		TransactionID: "TXN-" + uuid.NewString(),
		PaymentNumber: "PAY-" + uuid.NewString(),
		Amount:        o.GrandTotal,
		Status:        StatusPending,
		PaymentMethod: method,
		ExpiredAt:     time.Now().Add(expirationWindow),
	}
	if err := s.repo.Create(ctx, p); err != nil {
		return nil, err
	}

	resp := toResponse(p)
	return &resp, nil
}

func (s *Service) GetByID(ctx context.Context, actorUserID, actorRole, paymentID string) (*Response, error) {
	p, err := s.repo.FindByID(ctx, paymentID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, apperr.New(http.StatusNotFound, apperr.PaymentNotFound, "Payment not found")
		}
		return nil, err
	}

	if actorRole == "CUSTOMER" {
		o, err := s.order.FindRawByID(ctx, p.OrderID)
		if err != nil {
			return nil, err
		}
		if o.CustomerID != actorUserID {
			return nil, apperr.ForbiddenErr("You do not have permission to view this payment")
		}
	}

	resp := toResponse(p)
	return &resp, nil
}

// Callback (spec Section 42, 73, 80) — SATU-SATUNYA jalur yang mengubah
// payment jadi PAID. Idempotent WAJIB: status payment diperiksa SEBELUM
// efek samping apa pun, di bawah row lock (FOR UPDATE) dalam transaksi yang
// sama dengan update payment+order+stock-out, supaya callback ganda yang
// datang BERSAMAAN (bukan cuma sequential) tidak pernah memicu efek bisnis
// dobel — callback kedua akan menunggu lock milik callback pertama, lalu
// melihat status sudah PAID dan berhenti tanpa efek apa pun.
func (s *Service) Callback(ctx context.Context, req CallbackRequest) error {
	if req.Status != "PAID" {
		return apperr.New(http.StatusUnprocessableEntity, apperr.PaymentInvalidCallback, "Unsupported callback status")
	}

	var didProcess bool
	var paymentID, orderNumber, customerID string

	err := s.db.Transaction(func(tx *gorm.DB) error {
		p, err := s.repo.LockForUpdateByTransactionID(ctx, tx, req.TransactionID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return apperr.New(http.StatusNotFound, apperr.PaymentNotFound, "Payment not found")
			}
			return err
		}

		if p.OrderID != req.OrderID {
			return apperr.New(http.StatusUnprocessableEntity, apperr.PaymentInvalidCallback, "transaction_id does not match order_id")
		}

		switch p.Status {
		case StatusPaid:
			return nil // idempotent no-op — sudah diproses sebelumnya, sukses tanpa efek ganda.
		case StatusExpired:
			return apperr.New(http.StatusConflict, apperr.PaymentExpired, "Payment has already expired")
		case StatusPending:
			// lanjut ke efek samping di bawah.
		default:
			return apperr.New(http.StatusUnprocessableEntity, apperr.PaymentInvalidCallback, "Payment is not in a payable state")
		}

		now := time.Now()
		p.Status = StatusPaid
		p.PaidAt = &now
		if err := s.repo.UpdateTx(ctx, tx, p); err != nil {
			return err
		}

		o, err := s.order.LockForUpdate(ctx, tx, p.OrderID)
		if err != nil {
			return err
		}
		if !order.CanTransition(o.Status, order.StatusPaid) {
			return apperr.New(http.StatusUnprocessableEntity, apperr.PaymentInvalidCallback, "Order is not awaiting payment")
		}
		if err := s.order.UpdateStatus(ctx, tx, o.ID, order.StatusPaid); err != nil {
			return err
		}

		items, err := s.order.ItemsByOrderID(ctx, tx, o.ID)
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := s.stock.StockOut(ctx, tx, item.ProductID, o.WarehouseID, item.Quantity, o.ID); err != nil {
				return err
			}
		}

		didProcess = true
		paymentID = p.ID
		orderNumber = o.OrderNumber
		customerID = o.CustomerID
		return nil
	})
	if err != nil {
		return err
	}

	// Publish SETELAH commit (architecture.md Section 3), dan HANYA kalau
	// callback ini benar-benar memicu transisi PENDING->PAID — bukan untuk
	// idempotent no-op (payment yang sudah PAID sebelumnya), supaya
	// PAYMENT_PAID juga tidak pernah dipublish dobel walau callback datang
	// berkali-kali (idempotency berlaku juga di jalur event, bukan cuma DB).
	// order_number & customer_id disertakan supaya Notification Worker
	// (Iterasi 08) bisa membangun pesan & menentukan penerima tanpa query
	// balik ke database.
	if didProcess {
		if s.events != nil {
			_ = s.events.Publish(ctx, "payment_events", "PAYMENT_PAID", req.OrderID, map[string]string{
				"order_id":     req.OrderID,
				"payment_id":   paymentID,
				"order_number": orderNumber,
				"customer_id":  customerID,
			})
		}
		s.notifyStatusUpdated(customerID, req.OrderID, order.StatusPaid)
	}
	return nil
}

// ExpirePendingPayments (spec Iterasi 06, worker periodik) — mencari
// kandidat lewat SELECT polos, lalu memproses SATU PER SATU dalam transaksi
// terpisah supaya satu payment yang gagal tidak membatalkan payment lain
// yang seharusnya berhasil expired.
func (s *Service) ExpirePendingPayments(ctx context.Context) (int, error) {
	candidates, err := s.repo.ListPendingExpired(ctx, time.Now(), 100)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, p := range candidates {
		expired, err := s.expireOne(ctx, p.ID)
		if err != nil {
			return count, err
		}
		if expired {
			count++
		}
	}
	return count, nil
}

func (s *Service) expireOne(ctx context.Context, paymentID string) (bool, error) {
	var didExpire, orderExpired bool
	var customerID, orderID string
	err := s.db.Transaction(func(tx *gorm.DB) error {
		p, err := s.repo.LockForUpdateByID(ctx, tx, paymentID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil
			}
			return err
		}
		if p.Status != StatusPending {
			return nil // sudah diproses worker lain / sudah PAID lebih dulu -> idempotent no-op.
		}

		// UPDATE ... WHERE status='PENDING' eksplisit (bukan cuma dicek di
		// SELECT) — idempotency guard tambahan sesuai spec Iterasi 06, di
		// atas row lock FOR UPDATE yang sudah diambil di atas.
		updated, err := s.repo.UpdateStatusTxIfPending(ctx, tx, p.ID, StatusExpired)
		if err != nil {
			return err
		}
		if !updated {
			return nil
		}

		o, err := s.order.LockForUpdate(ctx, tx, p.OrderID)
		if err != nil {
			return err
		}
		if order.CanTransition(o.Status, order.StatusExpired) {
			if err := s.order.UpdateStatus(ctx, tx, o.ID, order.StatusExpired); err != nil {
				return err
			}
			items, err := s.order.ItemsByOrderID(ctx, tx, o.ID)
			if err != nil {
				return err
			}
			for _, item := range items {
				if err := s.stock.ReleaseStock(ctx, tx, item.ProductID, o.WarehouseID, item.Quantity, o.ID); err != nil {
					return err
				}
			}
			orderExpired = true
			customerID = o.CustomerID
			orderID = o.ID
		}

		didExpire = true
		return nil
	})
	if err == nil && orderExpired {
		s.notifyStatusUpdated(customerID, orderID, order.StatusExpired)
	}
	return didExpire, err
}
