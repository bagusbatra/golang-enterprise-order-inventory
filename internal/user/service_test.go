package user

import (
	"context"
	"testing"

	"order-management/pkg/audit"
)

type fakeRepo struct {
	byEmail map[string]*User
	byID    map[string]*User
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{byEmail: map[string]*User{}, byID: map[string]*User{}}
}

func (f *fakeRepo) Create(ctx context.Context, u *User) error {
	u.ID = "user-" + u.Email
	f.byEmail[u.Email] = u
	f.byID[u.ID] = u
	return nil
}

func (f *fakeRepo) FindByID(ctx context.Context, id string) (*User, error) {
	if u, ok := f.byID[id]; ok {
		return u, nil
	}
	return nil, ErrNotFound
}

func (f *fakeRepo) FindByEmail(ctx context.Context, email string) (*User, error) {
	if u, ok := f.byEmail[email]; ok {
		return u, nil
	}
	return nil, ErrNotFound
}

func (f *fakeRepo) List(ctx context.Context, filter ListFilter) ([]User, int64, error) {
	return nil, 0, nil
}

func (f *fakeRepo) Update(ctx context.Context, u *User) error {
	f.byID[u.ID] = u
	f.byEmail[u.Email] = u
	return nil
}

// spyAuditLogger mencatat setiap panggilan Log() supaya test bisa
// memverifikasi role change benar-benar menghasilkan audit log (spec
// Section 20).
type spyAuditLogger struct {
	entries []audit.Entry
}

func (s *spyAuditLogger) Log(ctx context.Context, entry audit.Entry) error {
	s.entries = append(s.entries, entry)
	return nil
}

func TestCreate_DuplicateEmail(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, &spyAuditLogger{})
	ctx := context.Background()

	req := CreateRequest{Name: "A", Email: "a@b.com", Password: "password123", Role: "CUSTOMER"}
	if _, err := svc.Create(ctx, req); err != nil {
		t.Fatalf("first create should succeed: %v", err)
	}
	_, err := svc.Create(ctx, req)
	if err == nil {
		t.Fatal("expected error for duplicate email, got nil")
	}
}

// TestUpdate_RoleChange_CreatesAuditLog memverifikasi role change WAJIB
// menghasilkan audit log (spec Section 20), sedangkan update tanpa role
// change TIDAK boleh menghasilkan audit log (hindari noise).
func TestUpdate_RoleChange_CreatesAuditLog(t *testing.T) {
	repo := newFakeRepo()
	spy := &spyAuditLogger{}
	svc := NewService(repo, spy)
	ctx := context.Background()

	created, err := svc.Create(ctx, CreateRequest{Name: "A", Email: "a@b.com", Password: "password123", Role: "CUSTOMER"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	newRole := "ADMIN"
	if _, err := svc.Update(ctx, "actor-1", created.ID, UpdateRequest{Role: &newRole}); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	if len(spy.entries) != 1 {
		t.Fatalf("expected exactly 1 audit log entry for role change, got %d", len(spy.entries))
	}
	if spy.entries[0].Action != "ROLE_CHANGE" {
		t.Fatalf("expected action ROLE_CHANGE, got %s", spy.entries[0].Action)
	}
}

func TestUpdate_NoRoleChange_NoAuditLog(t *testing.T) {
	repo := newFakeRepo()
	spy := &spyAuditLogger{}
	svc := NewService(repo, spy)
	ctx := context.Background()

	created, err := svc.Create(ctx, CreateRequest{Name: "A", Email: "a@b.com", Password: "password123", Role: "CUSTOMER"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	newName := "A Updated"
	if _, err := svc.Update(ctx, "actor-1", created.ID, UpdateRequest{Name: &newName}); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	if len(spy.entries) != 0 {
		t.Fatalf("expected no audit log entry when role unchanged, got %d", len(spy.entries))
	}
}
