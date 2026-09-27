// Package logger menyediakan structured logging (Zap) yang dipakai seluruh
// modul (spec Section 56). Caller WAJIB tidak pernah mencatat password, JWT,
// refresh token, atau data sensitif lain lewat logger ini — aturan ini
// tidak bisa dipaksakan secara teknis oleh package ini, harus dipatuhi di
// call-site (handler/service).
package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New membuat *zap.Logger sesuai level (dari .env LOG_LEVEL) dan environment
// (dari .env APP_ENV). Level tidak valid jatuh ke Info. Environment
// "development" memakai console encoder yang mudah dibaca manusia;
// selain itu memakai JSON encoder (production-friendly untuk log aggregator).
func New(level string, appEnv string) *zap.Logger {
	var lvl zapcore.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = zapcore.InfoLevel
	}

	var cfg zap.Config
	if appEnv == "development" {
		cfg = zap.NewDevelopmentConfig()
	} else {
		cfg = zap.NewProductionConfig()
	}
	cfg.Level = zap.NewAtomicLevelAt(lvl)

	l, err := cfg.Build()
	if err != nil {
		return zap.NewNop()
	}
	return l
}
