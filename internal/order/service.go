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
}

func NewService(repo Repository, db TxRunner, warehouse WarehouseChecker, price PriceProvider, stock StockReserver) *Service {
	return &Service{repo: repo, db: db, warehouse: warehouse, price: price, stock: stock}
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
//  3. BEGIN TRANSACTION.
//  4. Pass 1: untuk setiap item (urutan ASC), CheckAndLock (row lock FOR
//     UPDATE + validasi available stock) DAN snapshot harga TERKINI (spec
//     Section 15). Item mana pun yang gagal -> seluruh transaksi batal,
//     TIDAK ADA row order/order_items yang pernah ditulis.
//  5. Generate order_number (lock counter table).
//  6. Create orders + order_items.
//  7. Pass 2: ReserveStock per item (menambah reserved_quantity + catat
//     inventory_transactions RESERVE, reference ke order yang baru dibuat).
//  8. COMMIT.
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

	var result Response
	err = s.db.Transaction(func(tx *gorm.DB) error {
		lines := make([]orderLine, 0, len(items))
		subtotal := decimal.Zero

		for _, it := range items {
			if err := s.stock.CheckAndLock(ctx, tx, it.ProductID, req.WarehouseID, it.Quantity); err != nil {
				return err
			}
			price, err := s.price.GetPriceSnapshot(ctx, it.ProductID)
			if err != nil {
				return err
			}
			lineSubtotal := price.Mul(decimal.NewFromInt(int64(it.Quantity)))
			lines = append(lines, orderLine{ProductID: it.ProductID, Quantity: it.Quantity, UnitPrice: price, Subtotal: lineSubtotal})
			subtotal = subtotal.Add(lineSubtotal)
		}

		tax := subtotal.Mul(taxRate).Round(2)
		grandTotal := subtotal.Add(tax).Add(shippingCost)

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
	return &result, nil
}
