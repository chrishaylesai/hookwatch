package auth_test

import (
	"context"
	"os"
	"testing"

	"github.com/chrishaylesai/hookwatch/internal/auth"
	"github.com/chrishaylesai/hookwatch/internal/store"
)

func setupTestStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(dir, store.Config{})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestRegisterFirstUserIsAdmin(t *testing.T) {
	db := setupTestStore(t)
	svc := auth.NewService(db, true)

	user, err := svc.Register(context.Background(), "admin@test.com", "Admin", "password123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	if user.GlobalRole != "admin" {
		t.Errorf("first user role = %q, want admin", user.GlobalRole)
	}
	if user.Email != "admin@test.com" {
		t.Errorf("email = %q, want admin@test.com", user.Email)
	}
}

func TestRegisterSecondUserIsRegular(t *testing.T) {
	db := setupTestStore(t)
	svc := auth.NewService(db, true)

	_, err := svc.Register(context.Background(), "admin@test.com", "Admin", "password123")
	if err != nil {
		t.Fatalf("register first: %v", err)
	}

	user, err := svc.Register(context.Background(), "user@test.com", "User", "password123")
	if err != nil {
		t.Fatalf("register second: %v", err)
	}

	if user.GlobalRole != "user" {
		t.Errorf("second user role = %q, want user", user.GlobalRole)
	}
}

func TestRegisterDuplicateEmail(t *testing.T) {
	db := setupTestStore(t)
	svc := auth.NewService(db, true)

	_, err := svc.Register(context.Background(), "dup@test.com", "First", "password123")
	if err != nil {
		t.Fatalf("register first: %v", err)
	}

	_, err = svc.Register(context.Background(), "dup@test.com", "Second", "password123")
	if err != auth.ErrEmailTaken {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
}

func TestRegisterWeakPassword(t *testing.T) {
	db := setupTestStore(t)
	svc := auth.NewService(db, true)

	_, err := svc.Register(context.Background(), "weak@test.com", "Weak", "short")
	if err != auth.ErrWeakPassword {
		t.Fatalf("expected ErrWeakPassword, got %v", err)
	}
}

func TestRegisterClosedButFirstUserAllowed(t *testing.T) {
	db := setupTestStore(t)
	svc := auth.NewService(db, false) // registration closed

	// First user should still be allowed (bootstrap)
	user, err := svc.Register(context.Background(), "first@test.com", "First", "password123")
	if err != nil {
		t.Fatalf("expected first user registration to succeed, got %v", err)
	}
	if user.GlobalRole != "admin" {
		t.Errorf("first user role = %q, want admin", user.GlobalRole)
	}

	// Second user should be rejected
	_, err = svc.Register(context.Background(), "second@test.com", "Second", "password123")
	if err != auth.ErrRegistrationClosed {
		t.Fatalf("expected ErrRegistrationClosed, got %v", err)
	}
}

func TestLoginAndValidateSession(t *testing.T) {
	db := setupTestStore(t)
	svc := auth.NewService(db, true)

	_, err := svc.Register(context.Background(), "login@test.com", "Login", "password123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	user, session, err := svc.Login(context.Background(), "login@test.com", "password123", "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if user.Email != "login@test.com" {
		t.Errorf("login user email = %q", user.Email)
	}
	if session.ID == "" {
		t.Fatal("session ID is empty")
	}

	// Validate the session
	validatedUser, err := svc.ValidateSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("validate session: %v", err)
	}
	if validatedUser.ID != user.ID {
		t.Errorf("validated user ID = %q, want %q", validatedUser.ID, user.ID)
	}
}

func TestLoginInvalidPassword(t *testing.T) {
	db := setupTestStore(t)
	svc := auth.NewService(db, true)

	_, err := svc.Register(context.Background(), "bad@test.com", "Bad", "password123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	_, _, err = svc.Login(context.Background(), "bad@test.com", "wrongpassword", "127.0.0.1", "test-agent")
	if err != auth.ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLoginNonexistentUser(t *testing.T) {
	db := setupTestStore(t)
	svc := auth.NewService(db, true)

	_, _, err := svc.Login(context.Background(), "nobody@test.com", "password123", "127.0.0.1", "test-agent")
	if err != auth.ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLogout(t *testing.T) {
	db := setupTestStore(t)
	svc := auth.NewService(db, true)

	_, err := svc.Register(context.Background(), "logout@test.com", "Logout", "password123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	_, session, err := svc.Login(context.Background(), "logout@test.com", "password123", "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	if err := svc.Logout(context.Background(), session.ID); err != nil {
		t.Fatalf("logout: %v", err)
	}

	// Session should no longer be valid
	_, err = svc.ValidateSession(context.Background(), session.ID)
	if err == nil {
		t.Fatal("expected session to be invalid after logout")
	}
}

func TestContextUser(t *testing.T) {
	ctx := context.Background()

	// No user in context
	if user := auth.UserFromContext(ctx); user != nil {
		t.Fatal("expected nil user from empty context")
	}

	// Store and retrieve user
	db := setupTestStore(t)
	svc := auth.NewService(db, true)

	registered, err := svc.Register(ctx, "ctx@test.com", "Ctx", "password123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	ctx = auth.ContextWithUser(ctx, registered)
	retrieved := auth.UserFromContext(ctx)
	if retrieved == nil || retrieved.ID != registered.ID {
		t.Fatal("context user mismatch")
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
