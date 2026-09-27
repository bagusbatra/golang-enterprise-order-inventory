// Entry point aplikasi order-management. Wiring domain Agent 2 (auth, user,
// category, product, warehouse) dilakukan di sini. Domain Agent 3
// (inventory, order, payment, shipment, notification, audit, worker,
// websocket) akan ditambahkan menyusul di bagian yang ditandai TODO di
// bawah — main.go adalah file bersama, penambahan route/wiring Agent 3
// harus additive (menambah, bukan menghapus wiring Agent 2 yang sudah ada).
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

	"order-management/internal/auth"
	"order-management/internal/category"
	"order-management/internal/config"
	"order-management/internal/inventory"
	"order-management/internal/middleware"
	"order-management/internal/order"
	"order-management/internal/product"
	"order-management/internal/user"
	"order-management/internal/warehouse"
	"order-management/pkg/audit"
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

	router := newRouter(cfg, log, db, redisClient, jwtManager)

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
	// TODO(Agent 3): panggil worker.Pool.Stop() di sini SEBELUM baris ini,
	// agar background worker berhenti graceful sebelum proses exit
	// (spec Section 89) — lihat docs/CHANGE_REQUESTS.md untuk koordinasi.
	log.Info("server stopped")
}

func newRouter(cfg *config.Config, log *zap.Logger, db *gorm.DB, redisClient *redis.Client, jwtManager *jwt.Manager) *gin.Engine {
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

	authService := auth.NewService(userRepo, jwtManager, redisClient, cfg.RefreshTokenTTL)
	userService := user.NewService(userRepo, audit.NopLogger{})
	categoryService := category.NewService(categoryRepo)
	productService := product.NewService(productRepo, redisClient, 10*time.Minute)
	warehouseService := warehouse.NewService(warehouseRepo)
	orderRepo := order.NewRepository(db)
	// audit.NopLogger{} sementara sampai internal/audit (Iterasi 11) di-wire
	// menggantikan seluruh pemakaian NopLogger di file ini sekaligus.
	inventoryService := inventory.NewService(inventoryRepo, db, audit.NopLogger{})
	// warehouseService & productService memenuhi order.WarehouseChecker &
	// order.PriceProvider secara implisit (docs/architecture.md Section 2);
	// inventoryService memenuhi order.StockReserver.
	orderService := order.NewService(orderRepo, db, warehouseService, productService, inventoryService)

	authHandler := auth.NewHandler(authService)
	userHandler := user.NewHandler(userService)
	categoryHandler := category.NewHandler(categoryService)
	productHandler := product.NewHandler(productService)
	warehouseHandler := warehouse.NewHandler(warehouseService)
	inventoryHandler := inventory.NewHandler(inventoryService)
	orderHandler := order.NewHandler(orderService)

	jwtAuth := middleware.JWTAuth(jwtManager)
	adminOnly := middleware.RequireRole("ADMIN")
	adminWarehouse := middleware.RequireRole("ADMIN", "WAREHOUSE")
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

	// TODO(Agent 3): daftarkan route payment/shipment/notification/audit +
	// GET /ws di sini setelah domain masing-masing selesai (lihat docs/
	// api-contract.md). Gunakan `protected` group yang sama untuk endpoint
	// yang butuh JWT, dan `v1` langsung untuk POST /payments/callback
	// (publik, simulasi gateway eksternal).

	return r
}
