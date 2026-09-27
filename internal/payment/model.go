package payment

import (
	"time"

	"github.com/shopspring/decimal"
)

type Status string

const (
	StatusPending Status = "PENDING"
	StatusPaid    Status = "PAID"
	StatusFailed  Status = "FAILED"
	StatusExpired Status = "EXPIRED"
)

type Method string

const (
	MethodBankTransfer   Method = "BANK_TRANSFER"
	MethodVirtualAccount Method = "VIRTUAL_ACCOUNT"
	MethodEWallet        Method = "E_WALLET"
)

// Payment memetakan tabel `payments` (docs/database-contract.md Section 10).
// TransactionID adalah kunci idempotency callback (UNIQUE di level DB) —
// lihat Service.Callback.
type Payment struct {
	ID            string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrderID       string `gorm:"type:uuid;index"`
	TransactionID string `gorm:"uniqueIndex"`
	PaymentNumber string `gorm:"uniqueIndex"`
	Amount        decimal.Decimal
	Status        Status `gorm:"index"`
	PaymentMethod Method
	PaidAt        *time.Time
	ExpiredAt     time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (Payment) TableName() string { return "payments" }
