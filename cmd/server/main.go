// Entry point aplikasi order-management. Wiring domain Agent 2 (auth, user,
// category, product, warehouse) DAN Agent 3 (inventory, order, payment,
// shipment, notification, audit, worker, websocket) — main.go adalah file
// bersama, seluruh penambahan Agent 3 dilakukan additive (menambah, bukan
// menghapus wiring Agent 2 yang sudah ada).
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"

	internalaudit "order-management/internal/audit"
	"order-management/internal/auth"
	"order-management/internal/category"
	"order-management/internal/config"
	"order-management/internal/inventory"
	"order-management/internal/middleware"
	"order-management/internal/notification"
	"order-management/internal/order"
	"order-management/internal/payment"
	"order-management/internal/product"
	"order-management/internal/shipment"
	"order-management/internal/user"
	"order-management/internal/warehouse"
	"order-management/internal/websocket"
	"order-management/internal/worker"
	"order-management/pkg/database"
	"order-management/pkg/jwt"
	"order-management/pkg/logger"
	pkgredis "order-management/pkg/redis"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	log := logger.New(cfg.LogLevel, cfg.AppEnv)
	defer func() { _ = log.Sync() }()

	ctx := context.Background()

	db, err := database.Connect(database.Config{
		Host:            cfg.DatabaseHost,
		Port:            cfg.DatabasePort,
		User:            cfg.DatabaseUser,
		Password:        cfg.DatabasePassword,
		DBName:          cfg.DatabaseName,
		SSLMode:         cfg.DatabaseSSLMode,
		MaxOpenConns:    cfg.DatabaseMaxOpenConns,
		MaxIdleConns:    cfg.DatabaseMaxIdleConns,
		ConnMaxLifetime: cfg.DatabaseConnMaxLifetime,
		ConnMaxIdleTime: cfg.DatabaseConnMaxIdleTime,
	})
	if err != nil {
		log.Fatal("failed to connect to database", zap.Error(err))
	}

	redisClient, err := pkgredis.Connect(ctx, pkgredis.Config{
		Host:     cfg.RedisHost,
		Port:     cfg.RedisPort,
		Password: cfg.RedisPassword,
	})
	if err != nil {
		log.Fatal("failed to connect to redis", zap.Error(err))
	}

	jwtManager := jwt.NewManager(cfg.JWTAccessSecret, cfg.JWTRefreshSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)

	router, paymentService, notificationService := newRouter(cfg, log, db, redisClient, jwtManager)

	srv := &http.Server{
		Addr:    ":" + cfg.AppPort,
		Handler: router,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("server failed to start", zap.Error(err))
		}
	}()
	log.Info("server started", zap.String("port", cfg.AppPort), zap.String("env", cfg.AppEnv))

	workerCtx, workerCancel := context.WithCancel(context.Background())

	// Payment expiration worker (spec Iterasi 06) — jalan periodik setiap 1
	// menit, mengikuti pola graceful shutdown yang sama dengan worker
	// lain (Pool.Start/Stop, spec Section 89).
	expirationPool := worker.NewPool(1, log)
	expirationPool.Start(workerCtx, func(ctx context.Context, workerID int) {
		worker.RunPaymentExpirationWorker(ctx, log, paymentService, time.Minute)
	})

	// Consumer group order_events/payment_events (spec Iterasi 07, Section
	// 45-48) — minimal WORKER_COUNT worker per stream (default 4, dari
	// .env). Handler notification (Iterasi 08): HandleOrderEvent hanya
	// bereaksi ke ORDER_SHIPPED, HandlePaymentEvent hanya ke PAYMENT_PAID —
	// event lain di stream yang sama diabaikan (return nil, bukan error).
	orderEventsPool := worker.NewPool(cfg.WorkerCount, log)
	orderConsumer := worker.NewOrderEventConsumer(redisClient, log, notificationService.HandleOrderEvent)
	orderEventsPool.Start(workerCtx, orderConsumer.Run)

	paymentEventsPool := worker.NewPool(cfg.WorkerCount, log)
	paymentConsumer := worker.NewPaymentEventConsumer(redisClient, log, notificationService.HandlePaymentEvent)
	paymentEventsPool.Start(workerCtx, paymentConsumer.Run)

	// Graceful shutdown sesuai spec Section 65: tunggu SIGTERM/SIGINT,
	// stop menerima request baru, beri waktu request aktif selesai,
	// baru benar-benar exit.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", zap.Error(err))
	}

	workerCancel()
	expirationPool.Stop()
	orderEventsPool.Stop()
	paymentEventsPool.Stop()
	log.Info("server stopped")
}

func newRouter(cfg *config.Config, log *zap.Logger, db *gorm.DB, redisClient *redis.Client, jwtManager *jwt.Manager) (*gin.Engine, *payment.Service, *notification.Service) {
	if cfg.AppEnv != "development" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(middleware.RequestID(), middleware.Logger(log), middleware.Recovery(log), middleware.CORS())

	r.GET("/health", func(c *gin.Context) {
		dbStatus := "ok"
		if err := database.Ping(db); err != nil {
			dbStatus = "down"
		}
		redisStatus := "ok"
		if err := redisClient.Ping(c.Request.Context()).Err(); err != nil {
			redisStatus = "down"
		}
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"services": gin.H{
				"database": dbStatus,
				"redis":    redisStatus,
			},
		})
	})

	r.GET("/ready", func(c *gin.Context) {
		if err := database.Ping(db); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready", "reason": "database"})
			return
		}
		if err := redisClient.Ping(c.Request.Context()).Err(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready", "reason": "redis"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	// --- Wiring domain (Agent 2) ---
	userRepo := user.NewRepository(db)
	categoryRepo := category.NewRepository(db)
	productRepo := product.NewRepository(db)
	warehouseRepo := warehouse.NewRepository(db)
	inventoryRepo := inventory.NewRepository(db)

	// auditService mengimplementasikan pkg/audit.Logger (interface netral
	// Agent 2) — menggantikan SELURUH pemakaian audit.NopLogger{} di file
	// ini (Iterasi 11).
	auditService := internalaudit.NewService(internalaudit.NewRepository(db))

	authService := auth.NewService(userRepo, jwtManager, redisClient, cfg.RefreshTokenTTL, auditService)
	userService := user.NewService(userRepo, auditService)
	categoryService := category.NewService(categoryRepo)
	productService := product.NewService(productRepo, redisClient, 10*time.Minute, auditService)
	warehouseService := warehouse.NewService(warehouseRepo)
	orderRepo := order.NewRepository(db)
	inventoryService := inventory.NewService(inventoryRepo, db, auditService)
	// warehouseService & productService memenuhi order.WarehouseChecker &
	// order.PriceProvider secara implisit (docs/architecture.md Section 2);
	// inventoryService memenuhi order.StockReserver.
	orderService := order.NewService(orderRepo, db, warehouseService, productService, inventoryService)
	paymentRepo := payment.NewRepository(db)
	// orderService memenuhi payment.OrderGateway, inventoryService memenuhi
	// payment.StockAdjuster (keduanya lewat method passthrough yang sudah
	// ditambahkan khusus untuk titik integrasi ini).
	paymentService := payment.NewService(paymentRepo, db, orderService, inventoryService)
	notificationService := notification.NewService(notification.NewRepository(db))
	// orderService memenuhi shipment.OrderGateway juga (method passthrough
	// yang sama dipakai Payment Service).
	shipmentService := shipment.NewService(shipment.NewRepository(db), db, orderService)

	// Publish event Redis Streams (Iterasi 07) SETELAH commit — lihat
	// masing-masing Service untuk event apa yang dipublish (ORDER_CREATED/
	// ORDER_CANCELLED/PAYMENT_PAID/ORDER_SHIPPED).
	eventPublisher := worker.NewRedisPublisher(redisClient, log)
	orderService.SetEventPublisher(eventPublisher)
	paymentService.SetEventPublisher(eventPublisher)
	shipmentService.SetEventPublisher(eventPublisher)

	// Audit log (Iterasi 11) untuk "Create order"/"Cancel order" dan
	// "Payment callback" (spec Section 20) — inventory adjustment sudah
	// diaudit sejak Iterasi 03 lewat parameter auditService di atas.
	orderService.SetAuditLogger(auditService)
	paymentService.SetAuditLogger(auditService)

	// WebSocket connection manager (Iterasi 10) — Notify dipanggil SETELAH
	// commit oleh Order/Payment/Shipment Service (spec Section 50 & FILES
	// AFFECTED note), bukan di dalam transaksi DB.
	wsManager := websocket.NewManager()
	orderService.SetWebSocketManager(wsManager)
	paymentService.SetWebSocketManager(wsManager)
	shipmentService.SetWebSocketManager(wsManager)

	authHandler := auth.NewHandler(authService)
	userHandler := user.NewHandler(userService)
	categoryHandler := category.NewHandler(categoryService)
	productHandler := product.NewHandler(productService)
	warehouseHandler := warehouse.NewHandler(warehouseService)
	inventoryHandler := inventory.NewHandler(inventoryService)
	orderHandler := order.NewHandler(orderService)
	paymentHandler := payment.NewHandler(paymentService)
	notificationHandler := notification.NewHandler(notificationService)
	shipmentHandler := shipment.NewHandler(shipmentService)
	auditHandler := internalaudit.NewHandler(auditService)

	jwtAuth := middleware.JWTAuth(jwtManager)
	adminOnly := middleware.RequireRole("ADMIN")
	adminWarehouse := middleware.RequireRole("ADMIN", "WAREHOUSE")
	warehouseOnly := middleware.RequireRole("WAREHOUSE")
	customerOnly := middleware.RequireRole("CUSTOMER")
	customerSalesOnly := middleware.RequireRole("CUSTOMER", "SALES")
	customerOrAdmin := middleware.RequireRole("CUSTOMER", "ADMIN")
	loginRateLimit := middleware.RateLimit(redisClient, log, "login", 5, time.Minute)
	generalRateLimit := middleware.RateLimit(redisClient, log, "general", 100, time.Minute)

	v1 := r.Group("/api/v1")
	v1.Use(generalRateLimit)

	// Auth: register/login/refresh publik, logout wajib token.
	authGroup := v1.Group("/auth")
	authGroup.POST("/register", authHandler.Register)
	authGroup.POST("/login", loginRateLimit, authHandler.Login)
	authGroup.POST("/refresh", authHandler.Refresh)
	authGroup.POST("/logout", jwtAuth, authHandler.Logout)

	// Sisanya seluruh /api/v1 butuh JWT (spec: semua endpoint kecuali auth
	// register/login/refresh, health/ready/swagger, payments/callback).
	protected := v1.Group("")
	protected.Use(jwtAuth)

	usersGroup := protected.Group("/users")
	usersGroup.Use(adminOnly)
	userHandler.RegisterRoutes(usersGroup)

	categoryHandler.RegisterRoutes(protected.Group("/categories"), adminOnly)
	productHandler.RegisterRoutes(protected.Group("/products"), adminOnly)
	warehouseHandler.RegisterRoutes(protected.Group("/warehouses"), adminOnly)
	inventoryHandler.RegisterRoutes(protected.Group("/inventory"), adminWarehouse, adminOnly)
	orderHandler.RegisterRoutes(protected.Group("/orders"), customerSalesOnly, customerOrAdmin)
	paymentHandler.RegisterRoutes(protected.Group("/orders"), protected.Group("/payments"), customerOnly)
	notificationHandler.RegisterRoutes(protected.Group("/notifications"))
	shipmentHandler.RegisterRoutes(protected.Group("/orders"), protected.Group("/shipments"), warehouseOnly)
	auditHandler.RegisterRoutes(protected.Group("/audit-logs", adminOnly))

	// POST /payments/callback: publik (simulasi gateway eksternal), TIDAK
	// pakai middleware jwtAuth — divalidasi lewat transaction_id di service.
	v1.POST("/payments/callback", paymentHandler.Callback)

	// GET /ws: didaftarkan di root `r` (sejajar /health, /ready), BUKAN di
	// bawah `protected` — JWT-nya divalidasi manual dari query param di
	// dalam handler sendiri (Locked Decision), bukan via middleware.JWTAuth.
	wsHandler := websocket.NewHandler(wsManager, jwtManager, log)
	wsHandler.RegisterRoutes(r)

	return r, paymentService, notificationService
}
