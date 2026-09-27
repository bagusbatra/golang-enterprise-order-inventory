package order

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"order-management/internal/websocket"
	pkgaudit "order-management/pkg/audit"
	apperr "order-management/pkg/errors"
)

// TxRunner — lihat internal/inventory/service.go untuk rationale yang sama:
// diabstraksi dari *gorm.DB supaya service_test.go tidak butuh koneksi
// database nyata.
type TxRunner interface {
	Transaction(fc func(tx *gorm.DB) error, opts ...*sql.TxOptions) error
}

// WarehouseChecker adalah titik integrasi resmi dengan domain Agent 2
// (docs/architecture.md Section 2: warehouse.IsActive). Didefinisikan
// sebagai interface lokal (bukan import concrete *warehouse.Service) supaya
// order package tidak coupled ke implementasi warehouse dan tetap mudah
// dites — *warehouse.Service otomatis memenuhi ini (Go interface implicit).
type WarehouseChecker interface {
	IsActive(ctx context.Context, id string) (bool, error)
}

// PriceProvider adalah titik integrasi resmi dengan domain Agent 2
// (docs/architecture.md Section 2: product.GetPriceSnapshot).
type PriceProvider interface {
	GetPriceSnapshot(ctx context.Context, productID string) (decimal.Decimal, error)
}

// StockReserver adalah titik integrasi dengan internal/inventory (Agent 3
// sendiri) — CheckAndLock untuk Pass 1 (validasi semua item sebelum order
// ditulis), ReserveStock untuk Pass 2 (setelah order dibuat), ReleaseStock
// untuk cancellation.
type StockReserver interface {
	CheckAndLock(ctx context.Context, tx *gorm.DB, productID, warehouseID string, qty int) error
	ReserveStock(ctx context.Context, tx *gorm.DB, productID, warehouseID string, qty int, referenceID string) error
	ReleaseStock(ctx context.Context, tx *gorm.DB, productID, warehouseID string, qty int, referenceID string) error
}

// EventPublisher adalah titik integrasi dengan internal/worker (Redis
// Streams, Iterasi 07) — opsional (nil-safe, lihat SetEventPublisher).
// Dipanggil SETELAH transaksi commit (architecture.md Section 3: "Event
// dipublish SETELAH transaksi database commit, tidak pernah sebelum").
type EventPublisher interface {
	Publish(ctx context.Context, stream, eventType, aggregateID string, payload any) error
}

// taxRate & shippingCost — Locked Decision 03-BACKEND-CORE.md Section 3:
// tax = subtotal x 11%, shipping fixed Rp20.000, discount selalu 0.
var (
	taxRate      = decimal.NewFromInt(11).Div(decimal.NewFromInt(100))
	shippingCost = decimal.NewFromInt(20000)
)

type Service struct {
	repo      Repository
	db        TxRunner
	warehouse WarehouseChecker
	price     PriceProvider
	stock     StockReserver
	events    EventPublisher
	ws        *websocket.Manager
	audit     pkgaudit.Logger
}

func NewService(repo Repository, db TxRunner, warehouse WarehouseChecker, price PriceProvider, stock StockReserver) *Service {
	return &Service{repo: repo, db: db, warehouse: warehouse, price: price, stock: stock, audit: pkgaudit.NopLogger{}}
}

// SetAuditLogger menyuntikkan implementasi audit (Iterasi 11) untuk
// "Create order" & "Cancel order" (spec Section 20). nil-safe: kalau tidak
// pernah dipanggil, tetap memakai pkgaudit.NopLogger dari NewService.
func (s *Service) SetAuditLogger(a pkgaudit.Logger) {
	if a == nil {
		a = pkgaudit.NopLogger{}
	}
	s.audit = a
}

// SetEventPublisher menyuntikkan publisher Redis Streams (Iterasi 07) tanpa
// mengubah signature NewService (menghindari perubahan di seluruh call site
// yang sudah ada, termasuk test). Dibiarkan nil di test unit — Publish tidak
// pernah dipanggil kalau nil.
func (s *Service) SetEventPublisher(p EventPublisher) {
	s.events = p
}

// SetWebSocketManager menyuntikkan connection manager (Iterasi 10) — nil-safe
// sama seperti SetEventPublisher. Dipanggil SETELAH commit, bukan di dalam
// transaksi DB (spec Iterasi 10 FILES AFFECTED note).
func (s *Service) SetWebSocketManager(m *websocket.Manager) {
	s.ws = m
}

func (s *Service) notifyStatusUpdated(customerID, orderID string, status Status) {
	if s.ws == nil {
		return
	}
	s.ws.Notify(customerID, websocket.Event{
		Event: "ORDER_STATUS_UPDATED",
		Data:  map[string]string{"order_id": orderID, "status": string(status)},
	})
}

type orderLine struct {
	ProductID string
	Quantity  int
	UnitPrice decimal.Decimal
	Subtotal  decimal.Decimal
}

// CreateOrder mengimplementasikan flow create-order (spec Section 34) dengan
// concurrency safety sebagai prioritas nomor satu (spec Section 38/79/96):
//
//  1. Validasi warehouse ACTIVE.
//  2. Urutkan item berdasarkan product_id ASC — mencegah deadlock antar
//     request konkuren yang memesan kombinasi produk berbeda urutan.
//  3. Snapshot harga TERKINI setiap item (spec Section 15) — SEBELUM BEGIN
//     TRANSACTION, lihat komentar di lokasi pemanggilan untuk alasan
//     (menghindari koneksi database kedua dipakai SAAT transaksi lain
//     menahan row lock, yang bisa membuat connection pool saling menunggu).
//  4. BEGIN TRANSACTION.
//  5. Pass 1: untuk setiap item (urutan ASC), CheckAndLock (row lock FOR
//     UPDATE + validasi available stock). Item mana pun yang gagal ->
//     seluruh transaksi batal, TIDAK ADA row order/order_items yang pernah
//     ditulis.
//  6. Generate order_number (lock counter table).
//  7. Create orders + order_items.
//  8. Pass 2: ReserveStock per item (menambah reserved_quantity + catat
//     inventory_transactions RESERVE, reference ke order yang baru dibuat).
//  9. COMMIT.
//
// Publish event ORDER_CREATED (Iterasi 07) dilakukan oleh caller SETELAH
// fungsi ini sukses — bukan di sini, supaya tetap "publish setelah commit"
// (architecture.md Section 3).
func (s *Service) CreateOrder(ctx context.Context, customerID string, req CreateOrderRequest) (*Response, error) {
	active, err := s.warehouse.IsActive(ctx, req.WarehouseID)
	if err != nil {
		return nil, err
	}
	if !active {
		return nil, apperr.New(http.StatusUnprocessableEntity, apperr.WarehouseInactive, "Warehouse is not active")
	}

	items := make([]ItemInput, len(req.Items))
	copy(items, req.Items)
	sort.Slice(items, func(i, j int) bool { return items[i].ProductID < items[j].ProductID })

	// Snapshot harga SEBELUM membuka transaksi/lock — price lookup memakai
	// koneksi database TERPISAH dari transaksi lock inventory (titik
	// integrasi product.GetPriceSnapshot bukan bagian dari tx ini). Memanggil
	// query ber-koneksi-terpisah SAAT transaksi lain sedang menahan row lock
	// bisa membuat pool koneksi (yang ukurannya terbatas) saling menunggu:
	// pemenang lock butuh koneksi ekstra untuk price lookup, sementara
	// koneksi lain habis dipakai request yang justru menunggu lock yang
	// sama — deadlock di level connection pool, bukan di level row lock.
	// Mengambil harga dulu (di luar tx) menghilangkan risiko itu SEKALIGUS
	// memperpendek waktu row lock ditahan (lock hanya untuk operasi
	// inventory, bukan ikut menunggu round-trip ke tabel products).
	lines := make([]orderLine, 0, len(items))
	subtotal := decimal.Zero
	for _, it := range items {
		price, err := s.price.GetPriceSnapshot(ctx, it.ProductID)
		if err != nil {
			return nil, err
		}
		lineSubtotal := price.Mul(decimal.NewFromInt(int64(it.Quantity)))
		lines = append(lines, orderLine{ProductID: it.ProductID, Quantity: it.Quantity, UnitPrice: price, Subtotal: lineSubtotal})
		subtotal = subtotal.Add(lineSubtotal)
	}
	tax := subtotal.Mul(taxRate).Round(2)
	grandTotal := subtotal.Add(tax).Add(shippingCost)

	var result Response
	err = s.db.Transaction(func(tx *gorm.DB) error {
		for _, it := range items {
			if err := s.stock.CheckAndLock(ctx, tx, it.ProductID, req.WarehouseID, it.Quantity); err != nil {
				return err
			}
		}

		orderNumber, err := s.repo.NextOrderNumber(ctx, tx, time.Now())
		if err != nil {
			return err
		}

		o := &Order{
			OrderNumber:  orderNumber,
			CustomerID:   customerID,
			WarehouseID:  req.WarehouseID,
			Status:       StatusWaitingPayment,
			Subtotal:     subtotal,
			Discount:     decimal.Zero,
			Tax:          tax,
			ShippingCost: shippingCost,
			GrandTotal:   grandTotal,
		}
		for _, l := range lines {
			o.Items = append(o.Items, OrderItem{ProductID: l.ProductID, Quantity: l.Quantity, UnitPrice: l.UnitPrice, Subtotal: l.Subtotal})
		}

		if err := s.repo.Create(ctx, tx, o); err != nil {
			return err
		}

		for _, l := range lines {
			if err := s.stock.ReserveStock(ctx, tx, l.ProductID, req.WarehouseID, l.Quantity, o.ID); err != nil {
				return err
			}
		}

		result = toResponse(o)
		return nil
	})
	if err != nil {
		return nil, err
	}

	if s.events != nil {
		_ = s.events.Publish(ctx, "order_events", "ORDER_CREATED", result.ID, map[string]string{
			"order_id":    result.ID,
			"customer_id": customerID,
		})
	}
	_ = s.audit.Log(ctx, pkgaudit.Entry{
		UserID:   &customerID,
		Action:   "CREATE_ORDER",
		Entity:   "ORDER",
		EntityID: &result.ID,
		NewData:  map[string]string{"status": result.Status, "grand_total": result.GrandTotal},
	})
	return &result, nil
}

func (s *Service) GetByID(ctx context.Context, actorUserID, actorRole, orderID string) (*DetailResponse, error) {
	o, err := s.repo.FindByID(ctx, orderID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, apperr.New(http.StatusNotFound, apperr.OrderNotFound, "Order not found")
		}
		return nil, err
	}
	if actorRole == "CUSTOMER" && o.CustomerID != actorUserID {
		return nil, apperr.ForbiddenErr("You do not have permission to view this order")
	}
	resp := toDetailResponse(o)
	return &resp, nil
}

// List: CUSTOMER hanya melihat order miliknya sendiri (ownership dipaksa di
// sini, bukan opsional) — role lain boleh memfilter customer_id lewat query
// (spec Section 7 api-contract: "customer_id(admin/sales only)").
func (s *Service) List(ctx context.Context, actorUserID, actorRole, status, customerIDFilter string, page, limit int) ([]Response, int64, error) {
	f := ListFilter{Status: status, Page: page, Limit: limit}
	if actorRole == "CUSTOMER" {
		f.CustomerID = actorUserID
	} else if customerIDFilter != "" {
		f.CustomerID = customerIDFilter
	}

	items, total, err := s.repo.List(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	resp := make([]Response, 0, len(items))
	for i := range items {
		resp = append(resp, toResponse(&items[i]))
	}
	return resp, total, nil
}

// Cancel hanya bisa dari PENDING/WAITING_PAYMENT/PAID (spec Section 43,
// dijaga lewat CanTransition -> StatusCancelled). Reserved stock dilepas
// lewat ReleaseStock per item — aman dipanggil bahkan jika reserved_quantity
// sudah 0 (misal order sudah PAID lalu dibatalkan sebelum shipped: quantity
// fisik SUDAH dipotong saat stock-out, jadi pembatalan pasca-PAID di
// technical test ini secara sengaja TIDAK mengembalikan stok fisik —
// hanya melepas reserved_quantity yang mungkin masih tersisa).
func (s *Service) Cancel(ctx context.Context, actorUserID, actorRole, orderID string) (*Response, error) {
	var result Response
	var previousStatus Status
	err := s.db.Transaction(func(tx *gorm.DB) error {
		o, err := s.repo.LockForUpdate(ctx, tx, orderID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return apperr.New(http.StatusNotFound, apperr.OrderNotFound, "Order not found")
			}
			return err
		}

		if actorRole == "CUSTOMER" && o.CustomerID != actorUserID {
			return apperr.ForbiddenErr("You do not have permission to cancel this order")
		}

		if !CanTransition(o.Status, StatusCancelled) {
			return apperr.New(http.StatusUnprocessableEntity, apperr.OrderInvalidStatus, "Order cannot be cancelled from its current status")
		}
		previousStatus = o.Status

		// Reserved stock hanya aktif untuk PENDING/WAITING_PAYMENT. Order yang
		// sudah PAID: quantity fisik SUDAH dipotong saat stock-out dan
		// reserved_quantity SUDAH 0 — panggil ReleaseStock di sini hanya akan
		// menulis inventory_transaction RELEASE palsu (tidak ada apa pun yang
		// sebenarnya dilepas). Tidak ada flow restock/refund di scope
		// technical test ini, jadi cancel PAID sengaja tidak menyentuh
		// inventory sama sekali.
		if o.Status != StatusPaid {
			items, err := s.repo.ItemsByOrderID(ctx, tx, o.ID)
			if err != nil {
				return err
			}
			for _, item := range items {
				if err := s.stock.ReleaseStock(ctx, tx, item.ProductID, o.WarehouseID, item.Quantity, o.ID); err != nil {
					return err
				}
			}
		}

		if err := s.repo.UpdateStatus(ctx, tx, o.ID, StatusCancelled); err != nil {
			return err
		}
		o.Status = StatusCancelled
		result = toResponse(o)
		return nil
	})
	if err != nil {
		return nil, err
	}

	if s.events != nil {
		_ = s.events.Publish(ctx, "order_events", "ORDER_CANCELLED", orderID, map[string]string{"order_id": orderID})
	}
	_ = s.audit.Log(ctx, pkgaudit.Entry{
		UserID:   &actorUserID,
		Action:   "CANCEL_ORDER",
		Entity:   "ORDER",
		EntityID: &orderID,
		OldData:  map[string]string{"status": string(previousStatus)},
		NewData:  map[string]string{"status": string(StatusCancelled)},
	})
	s.notifyStatusUpdated(result.CustomerID, orderID, StatusCancelled)
	return &result, nil
}

// Titik integrasi resmi dengan Payment Service (Agent 3 sendiri, Iterasi
// 06) — payment callback & expiration worker perlu mengubah status order
// DALAM TRANSAKSI YANG SAMA dengan perubahan status payment (idempotency).
// Diekspos sebagai passthrough tipis ke Repository (bukan reach-into
// internal struct) agar Payment tidak perlu tahu apa pun soal *gorm.DB
// wiring order selain lewat Service ini.

// FindRawByID mengembalikan *Order mentah (tanpa DTO/ownership check) —
// dipakai Payment Service untuk membaca customer_id/status/grand_total saat
// membuat payment baru.
func (s *Service) FindRawByID(ctx context.Context, orderID string) (*Order, error) {
	return s.repo.FindByID(ctx, orderID)
}

func (s *Service) LockForUpdate(ctx context.Context, tx *gorm.DB, orderID string) (*Order, error) {
	return s.repo.LockForUpdate(ctx, tx, orderID)
}

func (s *Service) UpdateStatus(ctx context.Context, tx *gorm.DB, orderID string, status Status) error {
	return s.repo.UpdateStatus(ctx, tx, orderID, status)
}

func (s *Service) ItemsByOrderID(ctx context.Context, tx *gorm.DB, orderID string) ([]OrderItem, error) {
	return s.repo.ItemsByOrderID(ctx, tx, orderID)
}
