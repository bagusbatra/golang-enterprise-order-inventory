package integration

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"order-management/internal/inventory"
	"order-management/internal/middleware"
	"order-management/pkg/jwt"
	"order-management/tests/testutil"
)

// TestAuthorization_CustomerCannotAdjustInventory mereproduksi spec Section
// 81 secara HTTP sungguhan (bukan cuma pengecekan di level service): CUSTOMER
// mencoba POST /api/v1/inventory/adjust -> 403 Forbidden. Memakai handler +
// middleware ASLI milik domain ini (internal/inventory, internal/middleware)
// lewat httptest.Server — bukan endpoint /users milik Agent 2 (spec Section
// 81 contoh kedua), tapi mekanisme middleware.RequireRole yang diuji di sini
// PERSIS SAMA dengan yang melindungi endpoint itu.
func TestAuthorization_CustomerCannotAdjustInventory(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := testutil.SetupPostgres(t)
	jwtManager := jwt.NewManager("access-secret", "refresh-secret", time.Hour, time.Hour)

	inventoryService := inventory.NewService(inventory.NewRepository(db), db, nil)
	inventoryHandler := inventory.NewHandler(inventoryService)

	r := gin.New()
	protected := r.Group("/api/v1")
	protected.Use(middleware.JWTAuth(jwtManager))
	adminOnly := middleware.RequireRole("ADMIN")
	adminWarehouse := middleware.RequireRole("ADMIN", "WAREHOUSE")
	inventoryHandler.RegisterRoutes(protected.Group("/inventory"), adminWarehouse, adminOnly)

	server := httptest.NewServer(r)
	defer server.Close()

	customerToken, err := jwtManager.GenerateAccessToken("customer-1", "CUSTOMER")
	if err != nil {
		t.Fatalf("failed to generate customer token: %v", err)
	}

	body := `{"product_id":"00000000-0000-0000-0000-000000000000","warehouse_id":"00000000-0000-0000-0000-000000000000","quantity":-1,"description":"unauthorized attempt"}`
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/inventory/adjust", strings.NewReader(body))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+customerToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for CUSTOMER on POST /inventory/adjust, got %d", resp.StatusCode)
	}
}

// TestAuthorization_AdminCanAdjustInventory membuktikan sisi sebaliknya
// (spec Section 81: "ADMIN dapat melakukan operasi tersebut jika permission
// mengizinkan") — role yang BENAR tidak diblokir oleh middleware yang sama.
func TestAuthorization_AdminCanAdjustInventory(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := testutil.SetupPostgres(t)
	seed := seedBaseData(t, db, 10, "SKU-AUTHZ-ADMIN", "authz-admin@example.com")
	jwtManager := jwt.NewManager("access-secret", "refresh-secret", time.Hour, time.Hour)

	inventoryService := inventory.NewService(inventory.NewRepository(db), db, nil)
	inventoryHandler := inventory.NewHandler(inventoryService)

	r := gin.New()
	protected := r.Group("/api/v1")
	protected.Use(middleware.JWTAuth(jwtManager))
	adminOnly := middleware.RequireRole("ADMIN")
	adminWarehouse := middleware.RequireRole("ADMIN", "WAREHOUSE")
	inventoryHandler.RegisterRoutes(protected.Group("/inventory"), adminWarehouse, adminOnly)

	server := httptest.NewServer(r)
	defer server.Close()

	adminToken, err := jwtManager.GenerateAccessToken("admin-1", "ADMIN")
	if err != nil {
		t.Fatalf("failed to generate admin token: %v", err)
	}

	body := `{"product_id":"` + seed.ProductID + `","warehouse_id":"` + seed.WarehouseID + `","quantity":5,"description":"authorized adjustment"}`
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/inventory/adjust", strings.NewReader(body))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for ADMIN on POST /inventory/adjust, got %d", resp.StatusCode)
	}
}
