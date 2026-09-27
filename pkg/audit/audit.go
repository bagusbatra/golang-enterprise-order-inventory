// Package audit mendefinisikan interface Logger yang dipakai lintas domain
// (auth, user, category, product milik Agent 2; inventory, order, payment
// milik Agent 3) untuk mencatat audit log (spec Section 20, 53) tanpa
// domain pemanggil perlu tahu detail implementasi/skema tabel audit_logs.
//
// Implementasi konkret (baca/tulis ke tabel audit_logs) adalah milik
// Agent 3 (internal/audit). Sebelum itu di-wire di main.go, seluruh
// domain memakai NopLogger agar tidak ada dependency cycle/blocking.
// Dicatat sebagai notifikasi (bukan contract change) di
// docs/CHANGE_REQUESTS.md.
package audit

import "context"

// Logger adalah kontrak audit logging yang dipakai lintas domain.
type Logger interface {
	Log(ctx context.Context, entry Entry) error
}

// Entry merepresentasikan satu baris audit_logs (spec Section 20).
type Entry struct {
	UserID    *string // nil jika aksi dilakukan sistem, bukan user tertentu
	Action    string
	Entity    string
	EntityID  *string
	OldData   any
	NewData   any
	IPAddress string
	UserAgent string
}

// NopLogger adalah implementasi no-op, dipakai sebagai default sampai
// Agent 3 menyediakan implementasi nyata dan di-inject di main.go.
type NopLogger struct{}

func (NopLogger) Log(ctx context.Context, entry Entry) error {
	return nil
}
