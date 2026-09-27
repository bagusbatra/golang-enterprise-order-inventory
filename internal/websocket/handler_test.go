package websocket_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	ws "order-management/internal/websocket"
	"order-management/pkg/jwt"
)

// TestHandler_NoCrossTalkBetweenUsers adalah VALIDATION eksplisit spec
// Iterasi 10: "Test dengan 2 client (2 JWT berbeda), pastikan tidak ada
// cross-talk" — dua koneksi WebSocket NYATA (bukan fake Conn) lewat HTTP
// server sungguhan, JWT sungguhan lewat query param `?token=`.
func TestHandler_NoCrossTalkBetweenUsers(t *testing.T) {
	gin.SetMode(gin.TestMode)

	jwtManager := jwt.NewManager("access-secret", "refresh-secret", time.Hour, time.Hour)
	manager := ws.NewManager()
	handler := ws.NewHandler(manager, jwtManager, zap.NewNop())

	r := gin.New()
	handler.RegisterRoutes(r)
	server := httptest.NewServer(r)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	tokenA, err := jwtManager.GenerateAccessToken("user-a", "CUSTOMER")
	if err != nil {
		t.Fatalf("failed to generate token for user-a: %v", err)
	}
	tokenB, err := jwtManager.GenerateAccessToken("user-b", "CUSTOMER")
	if err != nil {
		t.Fatalf("failed to generate token for user-b: %v", err)
	}

	connA, _, err := websocket.DefaultDialer.Dial(wsURL+"/ws?token="+tokenA, nil)
	if err != nil {
		t.Fatalf("client A failed to connect: %v", err)
	}
	defer connA.Close()

	connB, _, err := websocket.DefaultDialer.Dial(wsURL+"/ws?token="+tokenB, nil)
	if err != nil {
		t.Fatalf("client B failed to connect: %v", err)
	}
	defer connB.Close()

	// Beri waktu server menyelesaikan upgrade+Register di goroutine masing-
	// masing sebelum Notify dipanggil dari test ini.
	waitForConnection(t, manager, "user-a")
	waitForConnection(t, manager, "user-b")

	manager.Notify("user-a", ws.Event{
		Event: "ORDER_STATUS_UPDATED",
		Data:  map[string]string{"order_id": "order-1", "status": "SHIPPED"},
	})

	_ = connA.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := connA.ReadMessage()
	if err != nil {
		t.Fatalf("client A (owner of the order) should have received the event: %v", err)
	}
	var received ws.Event
	if err := json.Unmarshal(msg, &received); err != nil {
		t.Fatalf("failed to decode event received by client A: %v", err)
	}
	if received.Event != "ORDER_STATUS_UPDATED" {
		t.Fatalf("unexpected event type received by client A: %s", received.Event)
	}

	_ = connB.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if _, _, err := connB.ReadMessage(); err == nil {
		t.Fatal("client B should NOT receive an event addressed to user-a (cross-talk!)")
	}
}

func waitForConnection(t *testing.T, manager *ws.Manager, userID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if manager.ConnectionCount(userID) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s to register its connection", userID)
}
