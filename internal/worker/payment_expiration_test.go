package worker

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

type fakeExpirer struct {
	calls int32
}

func (f *fakeExpirer) ExpirePendingPayments(ctx context.Context) (int, error) {
	atomic.AddInt32(&f.calls, 1)
	return 0, nil
}

func TestRunPaymentExpirationWorker_TicksAndStopsGracefully(t *testing.T) {
	expirer := &fakeExpirer{}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		RunPaymentExpirationWorker(ctx, zap.NewNop(), expirer, 10*time.Millisecond)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&expirer.calls); got < 2 {
		t.Fatalf("expected at least 2 ticks to have fired, got %d", got)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop after context cancel")
	}
}
