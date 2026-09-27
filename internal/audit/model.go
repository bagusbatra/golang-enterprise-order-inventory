package audit

import (
	"encoding/json"
	"time"
)

// AuditLog memetakan tabel `audit_logs` (docs/database-contract.md Section
// 13, spec Section 20).
type AuditLog struct {
	ID        string          `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID    *string         `gorm:"type:uuid;index"`
	Action    string          `gorm:"index"`
	Entity    string          `gorm:"index"`
	EntityID  *string         `gorm:"type:uuid"`
	OldData   json.RawMessage `gorm:"type:jsonb"`
	NewData   json.RawMessage `gorm:"type:jsonb"`
	IPAddress string
	UserAgent string
	CreatedAt time.Time `gorm:"index"`
}

func (AuditLog) TableName() string { return "audit_logs" }
