// Package config memuat konfigurasi aplikasi dari environment variable
// (spec Section 7) via godotenv. Semua secret (JWT, DB password) HANYA
// boleh dibaca dari sini, jangan hardcode di tempat lain.
package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppName string
	AppEnv  string
	AppPort string

	DatabaseHost            string
	DatabasePort            string
	DatabaseUser            string
	DatabasePassword        string
	DatabaseName            string
	DatabaseSSLMode         string
	DatabaseMaxOpenConns    int
	DatabaseMaxIdleConns    int
	DatabaseConnMaxLifetime time.Duration
	DatabaseConnMaxIdleTime time.Duration

	RedisHost     string
	RedisPort     string
	RedisPassword string

	JWTAccessSecret  string
	JWTRefreshSecret string
	AccessTokenTTL   time.Duration
	RefreshTokenTTL  time.Duration

	WorkerCount int

	LogLevel string
}

// Load membaca file .env jika ada (tidak error jika tidak ada, misal di
// environment production yang sudah menyuntikkan env var langsung), lalu
// membangun Config dari environment variable dengan default yang aman.
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		AppName: getEnv("APP_NAME", "order-management"),
		AppEnv:  getEnv("APP_ENV", "development"),
		AppPort: getEnv("APP_PORT", "8080"),

		DatabaseHost:     getEnv("DATABASE_HOST", "localhost"),
		DatabasePort:     getEnv("DATABASE_PORT", "5432"),
		DatabaseUser:     getEnv("DATABASE_USER", "postgres"),
		DatabasePassword: getEnv("DATABASE_PASSWORD", "postgres"),
		DatabaseName:     getEnv("DATABASE_NAME", "order_management"),
		DatabaseSSLMode:  getEnv("DATABASE_SSLMODE", "disable"),

		DatabaseMaxOpenConns:    getEnvInt("DATABASE_MAX_OPEN_CONNS", 25),
		DatabaseMaxIdleConns:    getEnvInt("DATABASE_MAX_IDLE_CONNS", 10),
		DatabaseConnMaxLifetime: getEnvDuration("DATABASE_CONN_MAX_LIFETIME", 30*time.Minute),
		DatabaseConnMaxIdleTime: getEnvDuration("DATABASE_CONN_MAX_IDLE_TIME", 5*time.Minute),

		RedisHost:     getEnv("REDIS_HOST", "localhost"),
		RedisPort:     getEnv("REDIS_PORT", "6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),

		JWTAccessSecret:  getEnv("JWT_ACCESS_SECRET", "change-this-access-secret"),
		JWTRefreshSecret: getEnv("JWT_REFRESH_SECRET", "change-this-refresh-secret"),
		AccessTokenTTL:   getEnvDuration("ACCESS_TOKEN_EXPIRE", 15*time.Minute),
		RefreshTokenTTL:  getEnvDuration("REFRESH_TOKEN_EXPIRE", 168*time.Hour),

		WorkerCount: getEnvInt("WORKER_COUNT", 4),

		LogLevel: getEnv("LOG_LEVEL", "info"),
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return i
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
