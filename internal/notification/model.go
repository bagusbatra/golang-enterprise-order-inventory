package notification

import "time"

// Notification memetakan tabel `notifications` (docs/database-contract.md
// Section 12). TIDAK ADA kolom reference_id/event_type terpisah di
// contract yang sudah LOCKED — lihat Service.CreateIfNotExists untuk
// bagaimana idempotency tetap dijamin hanya dengan kolom yang ada.
type Notification struct {
	ID        string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID    string `gorm:"type:uuid;index"`
	Type      string
	Title     string
	Message   string
	IsRead    bool `gorm:"column:is_read"`
	CreatedAt time.Time
}

func (Notification) TableName() string { return "notifications" }
