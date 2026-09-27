package websocket

import (
	"sync"
	"testing"
)

type fakeConn struct {
	mu       sync.Mutex
	messages [][]byte
	closed   bool
}

func (f *fakeConn) WriteMessage(_ int, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, data)
	return nil
}

func (f *fakeConn) Close() error {
	f.closed = true
	return nil
}

func (f *fakeConn) messageCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.messages)
}

func TestManager_NotifyOnlyReachesOwner(t *testing.T) {
	m := NewManager()
	connA := &fakeConn{}
	connB := &fakeConn{}

	m.Register("user-a", connA)
	m.Register("user-b", connB)

	m.Notify("user-a", Event{Event: "ORDER_STATUS_UPDATED", Data: map[string]string{"order_id": "1"}})

	if got := connA.messageCount(); got != 1 {
		t.Fatalf("expected user-a to receive 1 message, got %d", got)
	}
	if got := connB.messageCount(); got != 0 {
		t.Fatalf("expected user-b to receive 0 messages (cross-talk!), got %d", got)
	}
}

func TestManager_MultipleConnectionsSameUser_AllReceive(t *testing.T) {
	m := NewManager()
	tabA := &fakeConn{}
	tabB := &fakeConn{}

	m.Register("user-a", tabA)
	m.Register("user-a", tabB)

	m.Notify("user-a", Event{Event: "ORDER_STATUS_UPDATED"})

	if got := tabA.messageCount(); got != 1 {
		t.Fatalf("expected tabA to receive 1 message, got %d", got)
	}
	if got := tabB.messageCount(); got != 1 {
		t.Fatalf("expected tabB to receive 1 message, got %d", got)
	}
}

func TestManager_UnregisterRemovesConnection(t *testing.T) {
	m := NewManager()
	conn := &fakeConn{}

	m.Register("user-a", conn)
	if got := m.ConnectionCount("user-a"); got != 1 {
		t.Fatalf("expected 1 connection after register, got %d", got)
	}

	m.Unregister("user-a", conn)
	if got := m.ConnectionCount("user-a"); got != 0 {
		t.Fatalf("expected 0 connections after unregister, got %d", got)
	}
}
