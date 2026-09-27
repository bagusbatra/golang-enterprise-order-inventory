package warehouse

import "time"

type Status string

const (
	StatusActive   Status = "ACTIVE"
	StatusInactive Status = "INACTIVE"
)

// Warehouse memetakan tabel `warehouses` (docs/database-contract.md Section 4).
type Warehouse struct {
	ID        string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Code      string `gorm:"uniqueIndex"`
	Name      string
	Address   string
	City      string
	Status    Status
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Warehouse) TableName() string { return "warehouses" }
