package auth_test

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chrishaylesai/hookwatch/internal/auth"
)

func TestOIDCProvisioningCreatesUsersAndSessions(t *testing.T) {
	db := setupTestStore(t)
	provider := newFakeOIDCProvider(t)
	defer provider.Close()

	svc, err := auth.NewOIDCService(context.Background(), db, provider.Issuer(), "hookwatch-client", "hookwatch-secret")
	if err != nil {
		t.Fatalf("NewOIDCService: %v", err)
	}

	provider.SetCode("first-code", fakeOIDCClaims{
		Subject: "subject-1",
		Email:   "first@example.com",
		Name:    "First User",
		Nonce:   "nonce-1",
	})

	firstUser, firstSession, err := svc.CompleteOIDCAuth(context.Background(), "http://app.test", "first-code", "nonce-1", "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("CompleteOIDCAuth first user: %v", err)
	}
	if firstUser.GlobalRole != "admin" {
		t.Fatalf("first user role = %q, want admin", firstUser.GlobalRole)
	}
	if firstSession.ID == "" {
		t.Fatal("first session ID is empty")
	}

	validated, err := svc.ValidateSession(context.Background(), firstSession.ID)
	if err != nil {
		t.Fatalf("ValidateSession: %v", err)
	}
	if validated.ID != firstUser.ID {
		t.Fatalf("validated user ID = %q, want %q", validated.ID, firstUser.ID)
	}

	provider.SetCode("second-code", fakeOIDCClaims{
		Subject: "subject-2",
		Email:   "second@example.com",
		Name:    "Second User",
		Nonce:   "nonce-2",
	})

	secondUser, _, err := svc.CompleteOIDCAuth(context.Background(), "http://app.test", "second-code", "nonce-2", "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("CompleteOIDCAuth second user: %v", err)
	}
	if secondUser.GlobalRole != "user" {
		t.Fatalf("second user role = %q, want user", secondUser.GlobalRole)
	}
}

func TestOIDCExistingSubjectReturnsExistingUser(t *testing.T) {
	db := setupTestStore(t)
	provider := newFakeOIDCProvider(t)
	defer provider.Close()

	svc, err := auth.NewOIDCService(context.Background(), db, provider.Issuer(), "hookwatch-client", "hookwatch-secret")
	if err != nil {
		t.Fatalf("NewOIDCService: %v", err)
	}

	provider.SetCode("first", fakeOIDCClaims{
		Subject: "subject-1",
		Email:   "user@example.com",
		Name:    "Original Name",
		Nonce:   "nonce-1",
	})
	firstUser, _, err := svc.CompleteOIDCAuth(context.Background(), "http://app.test", "first", "nonce-1", "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("CompleteOIDCAuth first: %v", err)
	}

	provider.SetCode("second", fakeOIDCClaims{
		Subject: "subject-1",
		Email:   "updated@example.com",
		Name:    "Updated Name",
		Nonce:   "nonce-2",
	})
	secondUser, _, err := svc.CompleteOIDCAuth(context.Background(), "http://app.test", "second", "nonce-2", "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("CompleteOIDCAuth second: %v", err)
	}

	if secondUser.ID != firstUser.ID {
		t.Fatalf("user ID = %q, want existing %q", secondUser.ID, firstUser.ID)
	}
	if secondUser.Email != firstUser.Email {
		t.Fatalf("email = %q, want persisted %q", secondUser.Email, firstUser.Email)
	}
}

func TestOIDCMissingEmailIsRejected(t *testing.T) {
	db := setupTestStore(t)
	provider := newFakeOIDCProvider(t)
	defer provider.Close()

	svc, err := auth.NewOIDCService(context.Background(), db, provider.Issuer(), "hookwatch-client", "hookwatch-secret")
	if err != nil {
		t.Fatalf("NewOIDCService: %v", err)
	}

	provider.SetCode("missing-email", fakeOIDCClaims{
		Subject: "subject-1",
		Nonce:   "nonce-1",
	})

	_, _, err = svc.CompleteOIDCAuth(context.Background(), "http://app.test", "missing-email", "nonce-1", "127.0.0.1", "test-agent")
	if err != auth.ErrOIDCEmailRequired {
		t.Fatalf("err = %v, want %v", err, auth.ErrOIDCEmailRequired)
	}
}

func TestOIDCEmailConflictIsRejected(t *testing.T) {
	db := setupTestStore(t)
	local := auth.NewService(db, true)
	if _, err := local.Register(context.Background(), "shared@example.com", "Local User", "password123"); err != nil {
		t.Fatalf("Register local user: %v", err)
	}

	provider := newFakeOIDCProvider(t)
	defer provider.Close()

	svc, err := auth.NewOIDCService(context.Background(), db, provider.Issuer(), "hookwatch-client", "hookwatch-secret")
	if err != nil {
		t.Fatalf("NewOIDCService: %v", err)
	}

	provider.SetCode("conflict", fakeOIDCClaims{
		Subject: "subject-1",
		Email:   "shared@example.com",
		Name:    "OIDC User",
		Nonce:   "nonce-1",
	})

	_, _, err = svc.CompleteOIDCAuth(context.Background(), "http://app.test", "conflict", "nonce-1", "127.0.0.1", "test-agent")
	if err != auth.ErrOIDCAccountConflict {
		t.Fatalf("err = %v, want %v", err, auth.ErrOIDCAccountConflict)
	}
}

type fakeOIDCProvider struct {
	server *httptest.Server
	signer *rsa.PrivateKey
	codes  map[string]fakeOIDCClaims
}

type fakeOIDCClaims struct {
	Subject string
	Email   string
	Name    string
	Nonce   string
}

func newFakeOIDCProvider(t *testing.T) *fakeOIDCProvider {
	t.Helper()

	signer, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}

	provider := &fakeOIDCProvider{
		signer: signer,
		codes:  make(map[string]fakeOIDCClaims),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", provider.handleDiscovery)
	mux.HandleFunc("/token", provider.handleToken)
	mux.HandleFunc("/keys", provider.handleKeys)
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	provider.server = httptest.NewServer(mux)
	return provider
}

func (p *fakeOIDCProvider) Close() {
	p.server.Close()
}

func (p *fakeOIDCProvider) Issuer() string {
	return p.server.URL
}

func (p *fakeOIDCProvider) SetCode(code string, claims fakeOIDCClaims) {
	p.codes[code] = claims
}

func (p *fakeOIDCProvider) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"issuer":                 p.Issuer(),
		"authorization_endpoint": p.Issuer() + "/authorize",
		"token_endpoint":         p.Issuer() + "/token",
		"jwks_uri":               p.Issuer() + "/keys",
		"id_token_signing_alg_values_supported": []string{"RS256"},
	})
}

func (p *fakeOIDCProvider) handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	claims, ok := p.codes[r.Form.Get("code")]
	if !ok {
		http.Error(w, "unknown code", http.StatusBadRequest)
		return
	}

	idToken, err := p.signIDToken(claims)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token": "access-token",
		"id_token":     idToken,
		"token_type":   "Bearer",
		"expires_in":   3600,
	})
}

func (p *fakeOIDCProvider) handleKeys(w http.ResponseWriter, r *http.Request) {
	pub := p.signer.PublicKey
	e := big.NewInt(int64(pub.E)).Bytes()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"keys": []map[string]string{
			{
				"kty": "RSA",
				"alg": "RS256",
				"use": "sig",
				"kid": "test-key",
				"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(e),
			},
		},
	})
}

func (p *fakeOIDCProvider) signIDToken(claims fakeOIDCClaims) (string, error) {
	now := time.Now().UTC()

	header, err := json.Marshal(map[string]string{
		"alg": "RS256",
		"typ": "JWT",
		"kid": "test-key",
	})
	if err != nil {
		return "", err
	}

	payload, err := json.Marshal(map[string]any{
		"iss":   p.Issuer(),
		"sub":   claims.Subject,
		"aud":   "hookwatch-client",
		"exp":   now.Add(time.Hour).Unix(),
		"iat":   now.Unix(),
		"email": claims.Email,
		"name":  claims.Name,
		"nonce": claims.Nonce,
	})
	if err != nil {
		return "", err
	}

	encodedHeader := base64.RawURLEncoding.EncodeToString(header)
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signingInput := encodedHeader + "." + encodedPayload

	sum := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, p.signer, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}
