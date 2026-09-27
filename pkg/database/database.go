// Package database membuka koneksi PostgreSQL via GORM dengan connection
// pool yang dikonfigurasi eksplisit (spec Section 88) — jangan pernah
// membuat koneksi baru per-request, seluruh service memakai *gorm.DB yang
// dibuka sekali di sini saat startup.
package database

import (
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Config struct {
	Host            string
	Port            string
	User            string
	Password        string
	DBName          string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// Connect membuka koneksi PostgreSQL, mengonfigurasi connection pool, dan
// memverifikasinya dengan ping. GORM logger di-set Silent karena logging
// terstruktur aplikasi sudah ditangani middleware.Logger + Zap — bukan
// berarti query tidak dicatat sama sekali, hanya tidak dobel dengan
// mekanisme logging GORM bawaan.
func Connect(cfg Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("database: gagal konek: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("database: gagal ambil underlying *sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("database: ping gagal: %w", err)
	}

	return db, nil
}

// Ping dipakai health/readiness check (spec Section 64) untuk memverifikasi
// koneksi masih hidup tanpa membuka koneksi baru.
func Ping(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}
