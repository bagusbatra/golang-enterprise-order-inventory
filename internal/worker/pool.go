// Package worker menyediakan pool goroutine dengan graceful shutdown
// (spec Section 89) yang dipakai oleh seluruh background worker milik
// Agent 3 (order events, payment expiration, notification, dsb — Iterasi
// 06-08). Pool sendiri tidak tahu apa isi job-nya; setiap domain
// mendaftarkan fungsi `run` sendiri lewat Start.
package worker

import (
	"context"
	"sync"

	"go.uber.org/zap"
)

// Pool menjalankan N goroutine ("worker") yang masing-masing memanggil
// fungsi run yang sama. run bertanggung jawab melakukan polling/consume
// sendiri (misal XREADGROUP di internal/worker/stream.go) dan HARUS
// mengembalikan kontrol ketika ctx di-cancel — Pool tidak akan
// membunuh goroutine secara paksa.
type Pool struct {
	workerCount int
	logger      *zap.Logger

	wg     sync.WaitGroup
	cancel context.CancelFunc
}

// NewPool membuat Pool dengan jumlah worker dari .env WORKER_COUNT.
// workerCount <= 0 dianggap invalid dan dipaksa jadi 1 agar Start tidak
// pernah menghasilkan pool kosong yang diam-diam tidak memproses apa pun.
func NewPool(workerCount int, logger *zap.Logger) *Pool {
	if workerCount <= 0 {
		workerCount = 1
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Pool{workerCount: workerCount, logger: logger}
}

// Start menjalankan `run` di masing-masing worker goroutine. Panic di dalam
// run di-recover per-worker (log lalu worker tersebut berhenti) supaya satu
// worker yang panic tidak menjatuhkan seluruh pool.
func (p *Pool) Start(ctx context.Context, run func(ctx context.Context, workerID int)) {
	workerCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel

	for i := 0; i < p.workerCount; i++ {
		p.wg.Add(1)
		go func(id int) {
			defer p.wg.Done()
			defer func() {
				if r := recover(); r != nil {
					p.logger.Error("worker panic recovered",
						zap.Int("worker_id", id),
						zap.Any("panic", r),
					)
				}
			}()
			run(workerCtx, id)
		}(i)
	}
}

// Stop meng-cancel context semua worker (sinyal "berhenti menerima job baru")
// lalu menunggu sampai job yang sedang berjalan selesai (WaitGroup.Wait) —
// urutan ini yang membuat shutdown graceful sesuai spec Section 89.
func (p *Pool) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()
}
