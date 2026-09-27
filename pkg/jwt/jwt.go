// Package jwt membungkus golang-jwt/jwt untuk generate & validasi access
// dan refresh token (spec Section 22-24). Claims sengaja minimal (user_id,
// role) agar token tidak membawa data sensitif lebih dari yang diperlukan
// middleware RBAC.
package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	ErrInvalidToken = errors.New("jwt: token tidak valid")
	ErrExpiredToken = errors.New("jwt: token sudah expired")
)

// Claims adalah payload JWT kustom yang dipakai baik untuk access token
// maupun refresh token (dibedakan lewat secret & TTL, bukan lewat field).
type Claims struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// Manager menyimpan secret & TTL untuk access/refresh token. Dibuat sekali
// saat startup (dari config), lalu di-inject ke auth service & middleware.
type Manager struct {
	accessSecret  []byte
	refreshSecret []byte
	accessTTL     time.Duration
	refreshTTL    time.Duration
}

func NewManager(accessSecret, refreshSecret string, accessTTL, refreshTTL time.Duration) *Manager {
	return &Manager{
		accessSecret:  []byte(accessSecret),
		refreshSecret: []byte(refreshSecret),
		accessTTL:     accessTTL,
		refreshTTL:    refreshTTL,
	}
}

// GenerateAccessToken membuat access token (default TTL 15 menit, spec
// Section 23) untuk user_id & role tertentu.
func (m *Manager) GenerateAccessToken(userID, role string) (string, error) {
	return m.generate(userID, role, m.accessSecret, m.accessTTL)
}

// GenerateRefreshToken membuat refresh token (default TTL 7 hari). ID unik
// token (jti) dipakai sebagai bagian key Redis saat disimpan/direvoke
// (lihat internal/auth service, Iterasi 03).
func (m *Manager) GenerateRefreshToken(userID, role string) (string, string, error) {
	tokenID := uuid.NewString()
	claims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(m.refreshTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ID:        tokenID,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.refreshSecret)
	if err != nil {
		return "", "", err
	}
	return signed, claims.ID, nil
}

func (m *Manager) generate(userID, role string, secret []byte, ttl time.Duration) (string, error) {
	claims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

// ParseAccessToken memvalidasi signature & expiry access token, mengembalikan
// claims jika valid.
func (m *Manager) ParseAccessToken(tokenStr string) (*Claims, error) {
	return m.parse(tokenStr, m.accessSecret)
}

// ParseRefreshToken sama seperti ParseAccessToken tapi dengan refresh secret.
// Pemanggil (auth service) WAJIB tetap mengecek status revoke di Redis
// setelah signature/expiry valid — package ini tidak tahu apa pun soal Redis.
func (m *Manager) ParseRefreshToken(tokenStr string) (*Claims, error) {
	return m.parse(tokenStr, m.refreshSecret)
}

func (m *Manager) parse(tokenStr string, secret []byte) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		return secret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}
	if !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
