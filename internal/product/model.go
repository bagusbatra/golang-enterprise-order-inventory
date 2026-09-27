package product

import (
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type Status string

const (
	StatusActive       Status = "ACTIVE"
	StatusInactive     Status = "INACTIVE"
	StatusDiscontinued Status = "DISCONTINUED"
)

// Product memetakan tabel `products` (docs/database-contract.md Section 3).
// Price & CostPrice memakai shopspring/decimal (Locked Decision C3 — lihat
// docs/architecture.md Section 8) supaya tidak ada precision loss floating
// point untuk nilai uang.
type Product struct {
	ID          string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	CategoryID  string `gorm:"type:uuid;index"`
	SKU         string `gorm:"uniqueIndex"`
	Name        string
	Description string
	Price       decimal.Decimal `gorm:"type:numeric(15,2)"`
	CostPrice   decimal.Decimal `gorm:"type:numeric(15,2);column:cost_price"`
	Weight      decimal.Decimal `gorm:"type:numeric(10,2)"`
	Status      Status
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

func (Product) TableName() string { return "products" }
