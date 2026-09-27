package shipment

import "time"

type Status string

const (
	StatusReady     Status = "READY"
	StatusPickedUp  Status = "PICKED_UP"
	StatusInTransit Status = "IN_TRANSIT"
	StatusDelivered Status = "DELIVERED"
	StatusFailed    Status = "FAILED"
)

// Shipment memetakan tabel `shipments` (docs/database-contract.md Section
// 11). order_id UNIQUE — satu order hanya boleh punya satu shipment.
type Shipment struct {
	ID             string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrderID        string `gorm:"type:uuid;uniqueIndex"`
	Courier        string
	TrackingNumber string `gorm:"uniqueIndex"`
	Status         Status
	ShippedAt      *time.Time
	DeliveredAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (Shipment) TableName() string { return "shipments" }
