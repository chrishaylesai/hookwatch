package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"strings"
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
	oidcFlowCookieTTL = 10 * time.Minute
)

var (
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	ErrEmailTaken         = errors.New("auth: email already registered")
	ErrRegistrationClosed = errors.New("auth: registration is not allowed")
	ErrWeakPassword       = errors.New("auth: password must be at least 8 characters")
	ErrUnsupportedAuthMode = errors.New("auth: operation not supported for current auth mode")
	ErrOIDCEmailRequired   = errors.New("auth: oidc email claim is required")
	ErrOIDCAccountConflict = errors.New("auth: oidc account conflicts with existing user")
	ErrOIDCInvalidNonce    = errors.New("auth: invalid oidc nonce")
)

type contextKey string

const userContextKey contextKey = "auth_user"

const (
	OIDCStateCookieName    = "hookwatch_oidc_state"
	OIDCNonceCookieName    = "hookwatch_oidc_nonce"
	OIDCRedirectCookieName = "hookwatch_oidc_redirect"
)

// OIDCAuthRequest describes a pending OIDC authorization flow.
type OIDCAuthRequest struct {
	URL          string
	State        string
	Nonce        string
	RedirectPath string
}

// Authenticator is the common interface consumed by the API layer.
type Authenticator interface {
	SessionMiddleware(next http.Handler) http.Handler
	Register(ctx context.Context, email, displayName, password string) (*models.User, error)
	Login(ctx context.Context, email, password, ip, userAgent string) (*models.User, *models.Session, error)
	Logout(ctx context.Context, sessionID string) error
	ValidateSession(ctx context.Context, sessionID string) (*models.User, error)
	StartOIDCAuth(baseURL, redirectPath string) (*OIDCAuthRequest, error)
	CompleteOIDCAuth(ctx context.Context, baseURL, code, expectedNonce, ip, userAgent string) (*models.User, *models.Session, error)
	RunSessionCleanup(ctx context.Context, interval time.Duration, logger *slog.Logger)
}

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

	session, err := createSession(ctx, s.store, user.ID, ip, userAgent)
	if err != nil {
		return nil, nil, err
	}

	return user, session, nil
}

// Logout deletes the session.
func (s *Service) Logout(ctx context.Context, sessionID string) error {
	return logoutSession(ctx, s.store, sessionID)
}

// ValidateSession looks up a session and returns the associated user.
func (s *Service) ValidateSession(ctx context.Context, sessionID string) (*models.User, error) {
	return validateSession(ctx, s.store, sessionID)
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

func createSession(ctx context.Context, db *store.Store, userID, ip, userAgent string) (*models.Session, error) {
	now := time.Now().UTC()
	session := &models.Session{
		ID:        uuid.NewString(),
		UserID:    userID,
		CreatedAt: now,
		ExpiresAt: now.Add(SessionDuration),
		IP:        ip,
		UserAgent: userAgent,
	}

	if err := db.CreateSession(ctx, session); err != nil {
		return nil, err
	}

	return session, nil
}

func validateSession(ctx context.Context, db *store.Store, sessionID string) (*models.User, error) {
	session, err := db.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	user, err := db.GetUser(ctx, session.UserID)
	if err != nil {
		return nil, err
	}

	return user, nil
}

func logoutSession(ctx context.Context, db *store.Store, sessionID string) error {
	return db.DeleteSession(ctx, sessionID)
}

func (s *Service) StartOIDCAuth(baseURL, redirectPath string) (*OIDCAuthRequest, error) {
	return nil, ErrUnsupportedAuthMode
}

func (s *Service) CompleteOIDCAuth(ctx context.Context, baseURL, code, expectedNonce, ip, userAgent string) (*models.User, *models.Session, error) {
	return nil, nil, ErrUnsupportedAuthMode
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

func normalizeOIDCIssuer(issuer string) string {
	return strings.TrimRight(strings.TrimSpace(issuer), "/")
}

func newFlowSecret() (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf[:]), nil
}

func normalizeRedirectPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw[0] != '/' {
		return "/"
	}
	if len(raw) > 1 && raw[1] == '/' {
		return "/"
	}
	return raw
}

func SetOIDCFlowCookies(w http.ResponseWriter, flow *OIDCAuthRequest) {
	setCookie := func(name, value string) {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    value,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   int(oidcFlowCookieTTL.Seconds()),
		})
	}

	setCookie(OIDCStateCookieName, flow.State)
	setCookie(OIDCNonceCookieName, flow.Nonce)
	setCookie(OIDCRedirectCookieName, flow.RedirectPath)
}

func ClearOIDCFlowCookies(w http.ResponseWriter) {
	for _, name := range []string{OIDCStateCookieName, OIDCNonceCookieName, OIDCRedirectCookieName} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
		})
	}
}
