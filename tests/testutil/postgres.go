// Package testutil menyediakan infrastruktur test bersama (real PostgreSQL
// via Testcontainers, miniredis) untuk seluruh integration/concurrency/
// idempotency test milik Agent 3 (tests/concurrency, tests/idempotency,
// tests/integration).
package testutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// SetupPostgres membuat container PostgreSQL nyata, menerapkan seluruh
// migrations/*.up.sql, dan mengembalikan *gorm.DB yang siap dipakai —
// WAJIB untuk test yang memvalidasi row-level locking sungguhan (spec:
// "locking behavior tidak valid dites dengan mock/sqlite").
func SetupPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	ctx := context.Background()

	pgContainer, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("order_management_test"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container (Docker required for this test): %v", err)
	}
	t.Cleanup(func() {
		if err := pgContainer.Terminate(context.Background()); err != nil {
			t.Logf("failed to terminate postgres container: %v", err)
		}
	})

	// Root cause ditemukan lewat testing nyata (bukan asumsi): ConnectionString()
	// bawaan module memakai hostname "localhost", dan Go men-dial IPv6
	// (::1) LEBIH DULU untuk itu (RFC 6555 Happy Eyeballs) — di environment
	// ini loopback IPv6 blackhole/rusak, jadi setiap koneksi baru menunggu
	// penuh sampai timeout sebelum (kalau sempat) fallback ke IPv4. Ini
	// akar dari SEMUA hang koneksi yang pernah terjadi selama development,
	// bukan soal Docker Desktop lambat. Fix: paksa 127.0.0.1 secara
	// eksplisit, jangan pernah biarkan resolusi hostname ambigu.
	port, err := pgContainer.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatalf("failed to get mapped port: %v", err)
	}
	dsn := fmt.Sprintf(
		"host=127.0.0.1 port=%s user=postgres password=postgres dbname=order_management_test sslmode=disable",
		port.Port(),
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Fatalf("failed to connect to test postgres: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get underlying *sql.DB: %v", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		t.Fatalf("failed to ping test postgres: %v", err)
	}

	// Batasi pool koneksi client — Postgres punya max_connections terbatas
	// (default 100, dikurangi beberapa reserved). Tanpa batas ini, test
	// dengan banyak goroutine konkuren (concurrency test: 100 goroutine)
	// bisa memicu "sorry, too many clients already" di server, terutama di
	// bawah -race (instrumentasi race detector memperlambat tiap goroutine,
	// jadi lebih banyak yang overlap secara nyata di titik waktu yang sama).
	// Ini TIDAK terkait dengan root cause IPv6 di atas — itu soal koneksi
	// individual yang hang, ini soal JUMLAH koneksi simultan.
	sqlDB.SetMaxOpenConns(50)
	sqlDB.SetMaxIdleConns(50)

	applyMigrations(t, db)
	return db
}

// applyMigrations menjalankan seluruh *.up.sql di folder migrations/ secara
// berurutan (numerik) — cara paling sederhana untuk skema tanpa perlu
// bergantung pada golang-migrate CLI di dalam test.
func applyMigrations(t *testing.T, db *gorm.DB) {
	t.Helper()

	migDir, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("failed to resolve migrations dir: %v", err)
	}

	entries, err := os.ReadDir(migDir)
	if err != nil {
		t.Fatalf("failed to read migrations dir: %v", err)
	}

	var upFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			upFiles = append(upFiles, e.Name())
		}
	}
	sort.Strings(upFiles)

	for _, name := range upFiles {
		content, err := os.ReadFile(filepath.Join(migDir, name))
		if err != nil {
			t.Fatalf("failed to read migration %s: %v", name, err)
		}
		if err := db.Exec(string(content)).Error; err != nil {
			t.Fatalf("failed to apply migration %s: %v", name, err)
		}
	}
}

// NewTestRedis membuat *redis.Client di atas miniredis (in-memory) — cukup
// untuk kebutuhan product cache-aside di test, tanpa perlu container Redis
// terpisah.
func NewTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}
