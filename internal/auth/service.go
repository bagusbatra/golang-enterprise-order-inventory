package auth

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"

	"order-management/internal/user"
	apperr "order-management/pkg/errors"
	"order-management/pkg/jwt"
)

// Service mengimplementasikan register/login/refresh/logout (spec Section
// 22-25). Refresh token state disimpan di Redis (bukan di PostgreSQL) agar
// revoke saat logout langsung efektif tanpa perlu tabel tambahan — sesuai
// aturan Redis usage di docs/architecture.md Section 5 (session/token state
// BOLEH di Redis, ini bukan financial/inventory data).
type Service struct {
	users      user.Repository
	jwtManager *jwt.Manager
	redis      *redis.Client
	refreshTTL time.Duration
}

func NewService(users user.Repository, jwtManager *jwt.Manager, redisClient *redis.Client, refreshTTL time.Duration) *Service {
	return &Service{users: users, jwtManager: jwtManager, redis: redisClient, refreshTTL: refreshTTL}
}

func refreshKey(userID, tokenID string) string {
	return fmt.Sprintf("refresh:%s:%s", userID, tokenID)
}

// Register selalu memaksa role CUSTOMER (spec Section 22) — pembuatan user
// dengan role lain hanya lewat POST /users oleh ADMIN (internal/user).
func (s *Service) Register(ctx context.Context, req RegisterRequest) (*RegisterResponse, error) {
	if _, err := s.users.FindByEmail(ctx, req.Email); err == nil {
		return nil, apperr.New(http.StatusConflict, apperr.EmailExists, "Email already registered")
	} else if err != user.ErrNotFound {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	u := &user.User{
		Name:         req.Name,
		Email:        req.Email,
		PasswordHash: string(hash),
		Role:         user.RoleCustomer,
		Status:       user.StatusActive,
	}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, err
	}

	return &RegisterResponse{ID: u.ID, Name: u.Name, Email: u.Email, Role: string(u.Role)}, nil
}

func (s *Service) Login(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
	u, err := s.users.FindByEmail(ctx, req.Email)
	if err != nil {
		if err == user.ErrNotFound {
			return nil, apperr.New(http.StatusUnauthorized, apperr.AuthInvalidCredentials, "Invalid email or password")
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		return nil, apperr.New(http.StatusUnauthorized, apperr.AuthInvalidCredentials, "Invalid email or password")
	}

	if u.Status != user.StatusActive {
		return nil, apperr.New(http.StatusUnauthorized, apperr.AuthUnauthorized, "Account is not active")
	}

	accessToken, err := s.jwtManager.GenerateAccessToken(u.ID, string(u.Role))
	if err != nil {
		return nil, err
	}

	refreshToken, tokenID, err := s.jwtManager.GenerateRefreshToken(u.ID, string(u.Role))
	if err != nil {
		return nil, err
	}

	if err := s.redis.Set(ctx, refreshKey(u.ID, tokenID), "valid", s.refreshTTL).Err(); err != nil {
		return nil, err
	}

	return &LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    900, // 15 menit, selaras dengan JWT_ACCESS_SECRET TTL di config
	}, nil
}

// Refresh memvalidasi signature+expiry refresh token, LALU memastikan
// key-nya masih ada di Redis (belum di-revoke oleh logout) sebelum
// menerbitkan access token baru — dua langkah ini sengaja terpisah karena
// signature valid tidak berarti token belum di-revoke (spec Section 24).
func (s *Service) Refresh(ctx context.Context, req RefreshRequest) (*RefreshResponse, error) {
	claims, err := s.jwtManager.ParseRefreshToken(req.RefreshToken)
	if err != nil {
		return nil, apperr.New(http.StatusUnauthorized, apperr.AuthTokenExpired, "Refresh token is invalid or expired")
	}

	exists, err := s.redis.Exists(ctx, refreshKey(claims.UserID, claims.ID)).Result()
	if err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, apperr.New(http.StatusUnauthorized, apperr.AuthTokenExpired, "Refresh token has been revoked")
	}

	accessToken, err := s.jwtManager.GenerateAccessToken(claims.UserID, claims.Role)
	if err != nil {
		return nil, err
	}

	return &RefreshResponse{AccessToken: accessToken, ExpiresIn: 900}, nil
}

// Logout merevoke refresh token dengan menghapus key-nya di Redis (spec
// Section 25) sehingga percobaan refresh berikutnya dengan token yang sama
// akan ditolak meski signature masih valid.
func (s *Service) Logout(ctx context.Context, req LogoutRequest) error {
	claims, err := s.jwtManager.ParseRefreshToken(req.RefreshToken)
	if err != nil {
		// Token sudah invalid/expired dianggap "sudah logout" — idempotent,
		// tidak perlu mengembalikan error ke client.
		return nil
	}
	return s.redis.Del(ctx, refreshKey(claims.UserID, claims.ID)).Err()
}
