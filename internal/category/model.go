package category

import (
	"time"

	"gorm.io/gorm"
)

// Category memetakan tabel `categories` (docs/database-contract.md Section 2).
// deleted_at dipakai GORM soft delete meski spec tidak meminta delete
// category jadi soft-delete secara eksplisit — dipertahankan agar konsisten
// dengan pola products, dan agar riwayat kategori pada order lama tidak hilang.
type Category struct {
	ID          string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name        string `gorm:"uniqueIndex"`
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

func (Category) TableName() string { return "categories" }
