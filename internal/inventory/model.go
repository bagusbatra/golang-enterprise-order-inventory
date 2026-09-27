package inventory

import "time"

// Inventory memetakan tabel `inventories` (docs/database-contract.md Section
// 5). available_stock SENGAJA tidak jadi kolom fisik — dihitung di service
// layer lewat AvailableStock(), sesuai contract.
type Inventory struct {
	ID               string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ProductID        string `gorm:"type:uuid;index"`
	WarehouseID      string `gorm:"type:uuid;index"`
	Quantity         int
	ReservedQuantity int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (Inventory) TableName() string { return "inventories" }

func (i Inventory) AvailableStock() int {
	return i.Quantity - i.ReservedQuantity
}

type TransactionType string

const (
	TransactionStockIn    TransactionType = "STOCK_IN"
	TransactionStockOut   TransactionType = "STOCK_OUT"
	TransactionReserve    TransactionType = "RESERVE"
	TransactionRelease    TransactionType = "RELEASE"
	TransactionAdjustment TransactionType = "ADJUSTMENT"
)

// Transaction memetakan tabel `inventory_transactions` (docs/
// database-contract.md Section 6) — jejak audit setiap perubahan quantity/
// reserved_quantity, dipakai juga oleh GET /inventory/:id/transactions.
type Transaction struct {
	ID            string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	InventoryID   string `gorm:"type:uuid;index"`
	Type          TransactionType
	Quantity      int
	ReferenceType *string
	ReferenceID   *string `gorm:"type:uuid"`
	Description   *string
	CreatedAt     time.Time
}

func (Transaction) TableName() string { return "inventory_transactions" }
