package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/chrishaylesai/hookwatch/internal/models"
	"github.com/chrishaylesai/hookwatch/internal/store"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

// OIDCService handles OIDC-based authentication flows.
type OIDCService struct {
	store        *store.Store
	issuer       string
	clientID     string
	clientSecret string
	provider     *oidc.Provider
	verifier     *oidc.IDTokenVerifier
}

type oidcClaims struct {
	Subject string `json:"sub"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Nonce   string `json:"nonce"`
}

// NewOIDCService discovers the OIDC provider and prepares an auth service.
func NewOIDCService(ctx context.Context, db *store.Store, issuer, clientID, clientSecret string) (*OIDCService, error) {
	issuer = normalizeOIDCIssuer(issuer)

	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("discover oidc provider: %w", err)
	}

	return &OIDCService{
		store:        db,
		issuer:       issuer,
		clientID:     strings.TrimSpace(clientID),
		clientSecret: strings.TrimSpace(clientSecret),
		provider:     provider,
		verifier:     provider.Verifier(&oidc.Config{ClientID: strings.TrimSpace(clientID)}),
	}, nil
}

func (s *OIDCService) Register(ctx context.Context, email, displayName, password string) (*models.User, error) {
	return nil, ErrUnsupportedAuthMode
}

func (s *OIDCService) Login(ctx context.Context, email, password, ip, userAgent string) (*models.User, *models.Session, error) {
	return nil, nil, ErrUnsupportedAuthMode
}

func (s *OIDCService) Logout(ctx context.Context, sessionID string) error {
	return logoutSession(ctx, s.store, sessionID)
}

func (s *OIDCService) ValidateSession(ctx context.Context, sessionID string) (*models.User, error) {
	return validateSession(ctx, s.store, sessionID)
}

func (s *OIDCService) StartOIDCAuth(baseURL, redirectPath string) (*OIDCAuthRequest, error) {
	state, err := newFlowSecret()
	if err != nil {
		return nil, err
	}
	nonce, err := newFlowSecret()
	if err != nil {
		return nil, err
	}

	redirectPath = normalizeRedirectPath(redirectPath)
	authURL := s.oauthConfig(baseURL).AuthCodeURL(state, oidc.Nonce(nonce))

	return &OIDCAuthRequest{
		URL:          authURL,
		State:        state,
		Nonce:        nonce,
		RedirectPath: redirectPath,
	}, nil
}

func (s *OIDCService) CompleteOIDCAuth(ctx context.Context, baseURL, code, expectedNonce, ip, userAgent string) (*models.User, *models.Session, error) {
	token, err := s.oauthConfig(baseURL).Exchange(ctx, code)
	if err != nil {
		return nil, nil, fmt.Errorf("exchange oidc code: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || strings.TrimSpace(rawIDToken) == "" {
		return nil, nil, fmt.Errorf("oidc response missing id_token")
	}

	idToken, err := s.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, nil, fmt.Errorf("verify oidc id token: %w", err)
	}

	var claims oidcClaims
	if err := idToken.Claims(&claims); err != nil {
		return nil, nil, fmt.Errorf("decode oidc claims: %w", err)
	}

	if claims.Subject == "" {
		claims.Subject = idToken.Subject
	}
	if strings.TrimSpace(claims.Nonce) != strings.TrimSpace(expectedNonce) {
		return nil, nil, ErrOIDCInvalidNonce
	}

	user, err := s.resolveUser(ctx, claims)
	if err != nil {
		return nil, nil, err
	}

	session, err := createSession(ctx, s.store, user.ID, ip, userAgent)
	if err != nil {
		return nil, nil, err
	}

	return user, session, nil
}

func (s *OIDCService) RunSessionCleanup(ctx context.Context, interval time.Duration, logger *slog.Logger) {
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

func (s *OIDCService) oauthConfig(baseURL string) *oauth2.Config {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")

	return &oauth2.Config{
		ClientID:     s.clientID,
		ClientSecret: s.clientSecret,
		Endpoint:     s.provider.Endpoint(),
		RedirectURL:  baseURL + "/api/auth/oidc/callback",
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}
}

func (s *OIDCService) resolveUser(ctx context.Context, claims oidcClaims) (*models.User, error) {
	subject := strings.TrimSpace(claims.Subject)
	if subject == "" {
		return nil, fmt.Errorf("oidc subject claim is required")
	}

	email := strings.TrimSpace(claims.Email)
	if email == "" {
		return nil, ErrOIDCEmailRequired
	}

	user, err := s.store.GetUserByOIDC(ctx, s.issuer, subject)
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}

	if _, err := s.store.GetUserByEmail(ctx, email); err == nil {
		return nil, ErrOIDCAccountConflict
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}

	role := "user"
	count, err := s.store.CountUsers(ctx)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		role = "admin"
	}

	displayName := strings.TrimSpace(claims.Name)
	if displayName == "" {
		displayName = email
	}

	now := time.Now().UTC()
	provider := s.issuer

	user = &models.User{
		ID:           uuid.NewString(),
		Email:        email,
		DisplayName:  displayName,
		OIDCProvider: &provider,
		OIDCSubject:  &subject,
		GlobalRole:   role,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.store.CreateUser(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}
