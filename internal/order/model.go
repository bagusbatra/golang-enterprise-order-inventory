package order

import (
	"time"

	"github.com/shopspring/decimal"
)

// Order memetakan tabel `orders` (docs/database-contract.md Section 8).
// Dibuat langsung dengan status WAITING_PAYMENT (Locked Decision
// 03-BACKEND-CORE.md Section 3) — PENDING tetap ada di enum Status untuk
// kelengkapan spec Section 16, tapi tidak pernah dipersist oleh create flow.
type Order struct {
	ID           string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrderNumber  string `gorm:"uniqueIndex"`
	CustomerID   string `gorm:"type:uuid;index"`
	WarehouseID  string `gorm:"type:uuid"`
	Status       Status `gorm:"index"`
	Subtotal     decimal.Decimal
	Discount     decimal.Decimal
	Tax          decimal.Decimal
	ShippingCost decimal.Decimal
	GrandTotal   decimal.Decimal
	CreatedAt    time.Time `gorm:"index"`
	UpdatedAt    time.Time

	Items []OrderItem `gorm:"foreignKey:OrderID"`
}

func (Order) TableName() string { return "orders" }

// OrderItem memetakan tabel `order_items`. UnitPrice adalah SNAPSHOT harga
// produk saat order dibuat (spec Section 15) — tidak pernah diambil ulang
// dari products.price setelahnya.
type OrderItem struct {
	ID        string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrderID   string `gorm:"type:uuid;index"`
	ProductID string `gorm:"type:uuid;index"`
	Quantity  int
	UnitPrice decimal.Decimal
	Subtotal  decimal.Decimal
	CreatedAt time.Time
}

func (OrderItem) TableName() string { return "order_items" }
