// Package websocket menyimpan koneksi WebSocket aktif per user_id, in-memory,
// khusus untuk instance aplikasi ini (spec Section 50, architecture.md
// Section 6). Ini SATU-SATUNYA tempat di seluruh sistem yang boleh memakai
// state in-memory Go untuk sesuatu yang "penting" — karena sifatnya
// transient (boleh hilang saat restart), bukan data bisnis lintas-instance
// seperti inventory/order/payment yang wajib lewat PostgreSQL.
package websocket

import (
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
)

// Conn adalah abstraksi minimal koneksi yang dibutuhkan Manager, supaya
// logic connection-tracking bisa dites tanpa membuka koneksi TCP sungguhan.
// *gorilla/websocket.Conn otomatis memenuhi interface ini; wiring endpoint
// GET /ws (upgrade HTTP -> WebSocket) ada di Iterasi 10.
type Conn interface {
	WriteMessage(messageType int, data []byte) error
	Close() error
}

// Event adalah format pesan yang dikirim ke client, sesuai docs/api-contract.md
// Section 12: {"event": "...", "data": {...}}.
type Event struct {
	Event string `json:"event"`
	Data  any    `json:"data"`
}

// Manager melacak koneksi aktif per user_id. Satu user boleh punya beberapa
// koneksi sekaligus (multi-tab/multi-device).
type Manager struct {
	mu    sync.RWMutex
	conns map[string][]Conn
}

func NewManager() *Manager {
	return &Manager{conns: make(map[string][]Conn)}
}

func (m *Manager) Register(userID string, conn Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conns[userID] = append(m.conns[userID], conn)
}

func (m *Manager) Unregister(userID string, conn Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()

	list := m.conns[userID]
	for i, c := range list {
		if c == conn {
			m.conns[userID] = append(list[:i], list[i+1:]...)
			break
		}
	}
	if len(m.conns[userID]) == 0 {
		delete(m.conns, userID)
	}
}

func (m *Manager) ConnectionCount(userID string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.conns[userID])
}

// Notify mengirim event ke SEMUA koneksi milik user_id ini SAJA. Ini yang
// menjamin customer A tidak pernah menerima event milik customer B (spec
// Section 81) — pemanggil (order/shipment service) selalu tahu persis
// user_id mana yang berhak menerima event tersebut.
func (m *Manager) Notify(userID string, event Event) {
	m.mu.RLock()
	conns := append([]Conn{}, m.conns[userID]...)
	m.mu.RUnlock()

	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	for _, c := range conns {
		_ = c.WriteMessage(websocket.TextMessage, payload)
	}
}
