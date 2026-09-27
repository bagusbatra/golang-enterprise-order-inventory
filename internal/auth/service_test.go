package auth

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"

	"order-management/internal/user"
	"order-management/pkg/jwt"
)

// fakeUserRepo adalah in-memory implementasi user.Repository khusus untuk
// unit test, menghindari dependency database sungguhan (spec Section 70
// hanya minta unit test, bukan integration test, untuk AuthService).
type fakeUserRepo struct {
	byEmail map[string]*user.User
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{byEmail: make(map[string]*user.User)}
}

func (f *fakeUserRepo) Create(ctx context.Context, u *user.User) error {
	u.ID = "user-" + u.Email
	f.byEmail[u.Email] = u
	return nil
}

func (f *fakeUserRepo) FindByID(ctx context.Context, id string) (*user.User, error) {
	for _, u := range f.byEmail {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, user.ErrNotFound
}

func (f *fakeUserRepo) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	if u, ok := f.byEmail[email]; ok {
		return u, nil
	}
	return nil, user.ErrNotFound
}

func (f *fakeUserRepo) List(ctx context.Context, filter user.ListFilter) ([]user.User, int64, error) {
	return nil, 0, nil
}

func (f *fakeUserRepo) Update(ctx context.Context, u *user.User) error {
	f.byEmail[u.Email] = u
	return nil
}

func newTestService(t *testing.T, repo *fakeUserRepo) *Service {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	manager := jwt.NewManager("test-access-secret", "test-refresh-secret", 15*time.Minute, 168*time.Hour)
	return NewService(repo, manager, client, 168*time.Hour)
}

func TestLogin_InvalidCredentials_WrongPassword(t *testing.T) {
	repo := newFakeUserRepo()
	hash, _ := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	repo.byEmail["a@b.com"] = &user.User{ID: "u1", Email: "a@b.com", PasswordHash: string(hash), Role: user.RoleCustomer, Status: user.StatusActive}

	svc := newTestService(t, repo)
	_, err := svc.Login(context.Background(), LoginRequest{Email: "a@b.com", Password: "wrong-password"})
	if err == nil {
		t.Fatal("expected error for wrong password, got nil")
	}
}

func TestLogin_InvalidCredentials_UnknownEmail(t *testing.T) {
	repo := newFakeUserRepo()
	svc := newTestService(t, repo)

	_, err := svc.Login(context.Background(), LoginRequest{Email: "unknown@b.com", Password: "whatever123"})
	if err == nil {
		t.Fatal("expected error for unknown email, got nil")
	}
}

func TestLogin_Success(t *testing.T) {
	repo := newFakeUserRepo()
	hash, _ := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	repo.byEmail["a@b.com"] = &user.User{ID: "u1", Email: "a@b.com", PasswordHash: string(hash), Role: user.RoleCustomer, Status: user.StatusActive}

	svc := newTestService(t, repo)
	resp, err := svc.Login(context.Background(), LoginRequest{Email: "a@b.com", Password: "correct-password"})
	if err != nil {
		t.Fatalf("expected login success, got error: %v", err)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Fatal("expected non-empty tokens")
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	repo := newFakeUserRepo()
	repo.byEmail["exists@b.com"] = &user.User{ID: "u1", Email: "exists@b.com"}

	svc := newTestService(t, repo)
	_, err := svc.Register(context.Background(), RegisterRequest{Name: "X", Email: "exists@b.com", Password: "password123"})
	if err == nil {
		t.Fatal("expected error for duplicate email, got nil")
	}
}

func TestRegister_DefaultsToCustomerRole(t *testing.T) {
	repo := newFakeUserRepo()
	svc := newTestService(t, repo)

	resp, err := svc.Register(context.Background(), RegisterRequest{Name: "New", Email: "new@b.com", Password: "password123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Role != string(user.RoleCustomer) {
		t.Fatalf("expected role CUSTOMER, got %s", resp.Role)
	}
}

// TestLogout_ThenRefresh_Revoked membuktikan refresh token yang sudah
// di-logout tidak bisa dipakai lagi (spec Section 25) meski signature-nya
// masih valid secara kriptografis.
func TestLogout_ThenRefresh_Revoked(t *testing.T) {
	repo := newFakeUserRepo()
	hash, _ := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	repo.byEmail["a@b.com"] = &user.User{ID: "u1", Email: "a@b.com", PasswordHash: string(hash), Role: user.RoleCustomer, Status: user.StatusActive}

	svc := newTestService(t, repo)
	ctx := context.Background()

	loginResp, err := svc.Login(ctx, LoginRequest{Email: "a@b.com", Password: "correct-password"})
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	if err := svc.Logout(ctx, LogoutRequest{RefreshToken: loginResp.RefreshToken}); err != nil {
		t.Fatalf("logout failed: %v", err)
	}

	_, err = svc.Refresh(ctx, RefreshRequest{RefreshToken: loginResp.RefreshToken})
	if err == nil {
		t.Fatal("expected refresh to fail after logout, got nil error")
	}
}
