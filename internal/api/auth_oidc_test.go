package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/chrishaylesai/hookwatch/internal/auth"
	"github.com/chrishaylesai/hookwatch/internal/hub"
	"github.com/chrishaylesai/hookwatch/internal/models"
)

func TestAuthorizeOIDCSetsCookiesAndRedirects(t *testing.T) {
	t.Parallel()

	authSvc := &fakeAuthService{
		oidcFlow: &auth.OIDCAuthRequest{
			URL:          "https://issuer.example.com/authorize?state=test-state",
			State:        "test-state",
			Nonce:        "test-nonce",
			RedirectPath: "/admin",
		},
	}
	handler := newAuthHandler(authSvc, "oidc")

	req := httptest.NewRequest(http.MethodGet, "https://hookwatch.test/api/auth/oidc/authorize?redirect=/admin", nil)
	rec := httptest.NewRecorder()

	handler.authorizeOIDC(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	if location := rec.Header().Get("Location"); location != authSvc.oidcFlow.URL {
		t.Fatalf("location = %q, want %q", location, authSvc.oidcFlow.URL)
	}
	if authSvc.startBaseURL != "https://hookwatch.test" {
		t.Fatalf("start base URL = %q, want https://hookwatch.test", authSvc.startBaseURL)
	}
	if authSvc.startRedirectPath != "/admin" {
		t.Fatalf("start redirect path = %q, want /admin", authSvc.startRedirectPath)
	}

	cookies := rec.Result().Cookies()
	if len(cookies) != 3 {
		t.Fatalf("cookies = %d, want 3", len(cookies))
	}
}

func TestOIDCCallbackRedirectsAccountConflictToLogin(t *testing.T) {
	t.Parallel()

	authSvc := &fakeAuthService{completeErr: auth.ErrOIDCAccountConflict}
	handler := newAuthHandler(authSvc, "oidc")

	req := httptest.NewRequest(http.MethodGet, "https://hookwatch.test/api/auth/oidc/callback?state=test-state&code=test-code", nil)
	req.AddCookie(&http.Cookie{Name: auth.OIDCStateCookieName, Value: "test-state"})
	req.AddCookie(&http.Cookie{Name: auth.OIDCNonceCookieName, Value: "test-nonce"})
	req.AddCookie(&http.Cookie{Name: auth.OIDCRedirectCookieName, Value: "/admin"})
	rec := httptest.NewRecorder()

	handler.callbackOIDC(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}

	location, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	if location.Path != "/login" {
		t.Fatalf("redirect path = %q, want /login", location.Path)
	}
	if location.Query().Get("error") != "account_conflict" {
		t.Fatalf("error query = %q, want account_conflict", location.Query().Get("error"))
	}
	if location.Query().Get("redirect") != "/admin" {
		t.Fatalf("redirect query = %q, want /admin", location.Query().Get("redirect"))
	}
}

func TestRouterUsesModeSpecificAuthRoutes(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	authSvc := &fakeAuthService{
		oidcFlow: &auth.OIDCAuthRequest{
			URL:          "https://issuer.example.com/authorize",
			State:        "test-state",
			Nonce:        "test-nonce",
			RedirectPath: "/",
		},
		loginUser: &models.User{
			ID:          "user-1",
			Email:       "user@example.com",
			DisplayName: "User",
			GlobalRole:  "user",
			CreatedAt:   time.Now().UTC(),
		},
		loginSession: &models.Session{ID: "session-1"},
	}

	oidcRouter := NewRouter(db, hub.New(), "oidc", authSvc)
	oidcReq := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/authorize", nil)
	oidcRec := httptest.NewRecorder()
	oidcRouter.ServeHTTP(oidcRec, oidcReq)
	if oidcRec.Code != http.StatusFound {
		t.Fatalf("oidc authorize status = %d, want %d", oidcRec.Code, http.StatusFound)
	}

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader([]byte(`{"email":"user@example.com","password":"password123"}`)))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	oidcRouter.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusNotFound {
		t.Fatalf("oidc login status = %d, want %d", loginRec.Code, http.StatusNotFound)
	}

	localRouter := NewRouter(db, hub.New(), "local", authSvc)
	localLoginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader([]byte(`{"email":"user@example.com","password":"password123"}`)))
	localLoginReq.Header.Set("Content-Type", "application/json")
	localLoginRec := httptest.NewRecorder()
	localRouter.ServeHTTP(localLoginRec, localLoginReq)
	if localLoginRec.Code != http.StatusOK {
		t.Fatalf("local login status = %d, want %d", localLoginRec.Code, http.StatusOK)
	}

	var resp userResponse
	if err := json.Unmarshal(localLoginRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if resp.Email != "user@example.com" {
		t.Fatalf("email = %q, want user@example.com", resp.Email)
	}

	localOIDCReq := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/authorize", nil)
	localOIDCRec := httptest.NewRecorder()
	localRouter.ServeHTTP(localOIDCRec, localOIDCReq)
	if localOIDCRec.Code != http.StatusNotFound {
		t.Fatalf("local oidc authorize status = %d, want %d", localOIDCRec.Code, http.StatusNotFound)
	}
}

type fakeAuthService struct {
	oidcFlow          *auth.OIDCAuthRequest
	startErr          error
	completeErr       error
	loginUser         *models.User
	loginSession      *models.Session
	startBaseURL      string
	startRedirectPath string
	completeBaseURL   string
	completeCode      string
	completeNonce     string
}

func (f *fakeAuthService) SessionMiddleware(next http.Handler) http.Handler {
	return next
}

func (f *fakeAuthService) Register(ctx context.Context, email, displayName, password string) (*models.User, error) {
	return nil, auth.ErrUnsupportedAuthMode
}

func (f *fakeAuthService) Login(ctx context.Context, email, password, ip, userAgent string) (*models.User, *models.Session, error) {
	return f.loginUser, f.loginSession, nil
}

func (f *fakeAuthService) Logout(ctx context.Context, sessionID string) error {
	return nil
}

func (f *fakeAuthService) ValidateSession(ctx context.Context, sessionID string) (*models.User, error) {
	return nil, auth.ErrInvalidCredentials
}

func (f *fakeAuthService) StartOIDCAuth(baseURL, redirectPath string) (*auth.OIDCAuthRequest, error) {
	f.startBaseURL = baseURL
	f.startRedirectPath = redirectPath
	return f.oidcFlow, f.startErr
}

func (f *fakeAuthService) CompleteOIDCAuth(ctx context.Context, baseURL, code, expectedNonce, ip, userAgent string) (*models.User, *models.Session, error) {
	f.completeBaseURL = baseURL
	f.completeCode = code
	f.completeNonce = expectedNonce
	return f.loginUser, f.loginSession, f.completeErr
}

func (f *fakeAuthService) RunSessionCleanup(ctx context.Context, interval time.Duration, logger *slog.Logger) {
}
