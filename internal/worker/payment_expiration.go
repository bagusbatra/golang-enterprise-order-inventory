package worker

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// PaymentExpirer adalah abstraksi minimal yang dibutuhkan worker ini dari
// internal/payment (Agent 3 sendiri) — dipisah sebagai interface supaya
// worker tidak import concrete *payment.Service secara langsung.
type PaymentExpirer interface {
	ExpirePendingPayments(ctx context.Context) (int, error)
}

// RunPaymentExpirationWorker menjalankan ExpirePendingPayments setiap
// `interval` (spec Iterasi 06: "misal setiap 1 menit"), berhenti graceful
// saat ctx di-cancel. Dipanggil sebagai fungsi `run` untuk Pool.Start biasa
// (lihat pool.go), sehingga otomatis ikut pola graceful shutdown yang sama
// dengan worker lain.
func RunPaymentExpirationWorker(ctx context.Context, logger *zap.Logger, expirer PaymentExpirer, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := expirer.ExpirePendingPayments(ctx)
			if err != nil {
				logger.Error("payment expiration worker run failed", zap.Error(err))
				continue
			}
			if n > 0 {
				logger.Info("payment expiration worker processed", zap.Int("count", n))
			}
		}
	}
}
