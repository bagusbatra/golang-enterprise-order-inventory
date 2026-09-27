package user

import "time"

// Role dan Status adalah enum string yang divalidasi di service layer
// (bukan hanya di database CHECK constraint) supaya error message ke
// client lebih jelas daripada error constraint mentah dari Postgres.
type Role string

const (
	RoleAdmin     Role = "ADMIN"
	RoleSales     Role = "SALES"
	RoleWarehouse Role = "WAREHOUSE"
	RoleCustomer  Role = "CUSTOMER"
)

func (r Role) Valid() bool {
	switch r {
	case RoleAdmin, RoleSales, RoleWarehouse, RoleCustomer:
		return true
	}
	return false
}

type Status string

const (
	StatusActive    Status = "ACTIVE"
	StatusInactive  Status = "INACTIVE"
	StatusSuspended Status = "SUSPENDED"
)

func (s Status) Valid() bool {
	switch s {
	case StatusActive, StatusInactive, StatusSuspended:
		return true
	}
	return false
}

// User memetakan tabel `users` (docs/database-contract.md Section 1).
type User struct {
	ID           string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name         string
	Email        string `gorm:"uniqueIndex"`
	PasswordHash string
	Role         Role
	Status       Status
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (User) TableName() string { return "users" }
