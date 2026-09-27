package worker

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestPool_StartStop_Graceful(t *testing.T) {
	pool := NewPool(3, zap.NewNop())

	var running int32
	started := make(chan struct{}, 3)

	pool.Start(context.Background(), func(ctx context.Context, workerID int) {
		atomic.AddInt32(&running, 1)
		started <- struct{}{}
		<-ctx.Done() // wajib berhenti hanya saat context di-cancel
		atomic.AddInt32(&running, -1)
	})

	for i := 0; i < 3; i++ {
		<-started
	}
	if got := atomic.LoadInt32(&running); got != 3 {
		t.Fatalf("expected 3 workers running, got %d", got)
	}

	stopped := make(chan struct{})
	go func() {
		pool.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() tidak selesai — kemungkinan goroutine leak / worker tidak merespons cancel")
	}

	if got := atomic.LoadInt32(&running); got != 0 {
		t.Fatalf("expected all workers stopped, got %d masih running", got)
	}
}

func TestPool_PanicInWorker_DoesNotCrashPool(t *testing.T) {
	pool := NewPool(1, zap.NewNop())
	done := make(chan struct{})

	pool.Start(context.Background(), func(ctx context.Context, workerID int) {
		defer close(done)
		panic("simulated worker panic")
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker tidak pernah selesai setelah panic")
	}

	pool.Stop() // tidak boleh panic/hang meski worker sudah panic sebelumnya
}
