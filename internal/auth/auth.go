package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/chrishaylesai/hookwatch/internal/models"
	"github.com/chrishaylesai/hookwatch/internal/store"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	SessionCookieName = "hookwatch_session"
	SessionDuration   = 7 * 24 * time.Hour
	bcryptCost        = 12
)

var (
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	ErrEmailTaken         = errors.New("auth: email already registered")
	ErrRegistrationClosed = errors.New("auth: registration is not allowed")
	ErrWeakPassword       = errors.New("auth: password must be at least 8 characters")
)

type contextKey string

const userContextKey contextKey = "auth_user"

// UserFromContext retrieves the authenticated user from the request context.
// Returns nil if no user is authenticated.
func UserFromContext(ctx context.Context) *models.User {
	user, _ := ctx.Value(userContextKey).(*models.User)
	return user
}

// ContextWithUser stores a user in the context.
func ContextWithUser(ctx context.Context, user *models.User) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

// Service handles authentication operations.
type Service struct {
	store             *store.Store
	allowRegistration bool
}

// NewService creates a new auth service.
func NewService(db *store.Store, allowRegistration bool) *Service {
	return &Service{
		store:             db,
		allowRegistration: allowRegistration,
	}
}

// Register creates a new user account with a password.
// The first registered user is automatically made admin.
func (s *Service) Register(ctx context.Context, email, displayName, password string) (*models.User, error) {
	if !s.allowRegistration {
		// Always allow registration if no users exist (first-user-is-admin bootstrap)
		count, err := s.store.CountUsers(ctx)
		if err != nil {
			return nil, err
		}
		if count > 0 {
			return nil, ErrRegistrationClosed
		}
	}

	if len(password) < 8 {
		return nil, ErrWeakPassword
	}

	// Check if email is already taken
	_, err := s.store.GetUserByEmail(ctx, email)
	if err == nil {
		return nil, ErrEmailTaken
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return nil, err
	}

	hashStr := string(hash)
	now := time.Now().UTC()

	// First user gets admin role
	role := "user"
	count, err := s.store.CountUsers(ctx)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		role = "admin"
	}

	user := &models.User{
		ID:           uuid.NewString(),
		Email:        email,
		DisplayName:  displayName,
		PasswordHash: &hashStr,
		GlobalRole:   role,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.store.CreateUser(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

// Login authenticates a user with email and password and creates a session.
func (s *Service) Login(ctx context.Context, email, password, ip, userAgent string) (*models.User, *models.Session, error) {
	user, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil, ErrInvalidCredentials
		}
		return nil, nil, err
	}

	if user.PasswordHash == nil {
		return nil, nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(password)); err != nil {
		return nil, nil, ErrInvalidCredentials
	}

	session, err := s.createSession(ctx, user.ID, ip, userAgent)
	if err != nil {
		return nil, nil, err
	}

	return user, session, nil
}

// Logout deletes the session.
func (s *Service) Logout(ctx context.Context, sessionID string) error {
	return s.store.DeleteSession(ctx, sessionID)
}

// ValidateSession looks up a session and returns the associated user.
func (s *Service) ValidateSession(ctx context.Context, sessionID string) (*models.User, error) {
	session, err := s.store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	user, err := s.store.GetUser(ctx, session.UserID)
	if err != nil {
		return nil, err
	}

	return user, nil
}

// SetSessionCookie sets the session cookie on the response.
func SetSessionCookie(w http.ResponseWriter, sessionID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(SessionDuration.Seconds()),
	})
}

// ClearSessionCookie removes the session cookie.
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (s *Service) createSession(ctx context.Context, userID, ip, userAgent string) (*models.Session, error) {
	now := time.Now().UTC()
	session := &models.Session{
		ID:        uuid.NewString(),
		UserID:    userID,
		CreatedAt: now,
		ExpiresAt: now.Add(SessionDuration),
		IP:        ip,
		UserAgent: userAgent,
	}

	if err := s.store.CreateSession(ctx, session); err != nil {
		return nil, err
	}

	return session, nil
}

// RunSessionCleanup periodically deletes expired sessions.
func (s *Service) RunSessionCleanup(ctx context.Context, interval time.Duration, logger *slog.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			deleted, err := s.store.DeleteExpiredSessions(ctx)
			if err != nil {
				logger.Error("failed to clean up expired sessions", "error", err)
				continue
			}
			if deleted > 0 {
				logger.Info("cleaned up expired sessions", "count", deleted)
			}
		}
	}
}
