package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chrishaylesai/hookwatch/internal/hub"
	"github.com/chrishaylesai/hookwatch/internal/models"
	"github.com/chrishaylesai/hookwatch/internal/store"
)

func TestCreateTokenAcceptsConfigurableResponseFields(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	router := NewRouter(db, hub.New(), authModeNone, nil, nil)

	body := []byte(`{
		"default_status": 201,
		"default_content": "{\"ok\":true}",
		"default_content_type": "application/json",
		"max_requests": 25,
		"timeout": 3,
		"cors": true
	}`)

	req := httptest.NewRequest(http.MethodPost, "/api/tokens", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var resp tokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if resp.DefaultStatus != 201 {
		t.Fatalf("default_status = %d, want 201", resp.DefaultStatus)
	}
	if resp.DefaultContent != `{"ok":true}` {
		t.Fatalf("default_content = %q, want configured content", resp.DefaultContent)
	}
	if resp.DefaultContentType != "application/json" {
		t.Fatalf("default_content_type = %q, want application/json", resp.DefaultContentType)
	}
	if resp.Timeout != 3 {
		t.Fatalf("timeout = %d, want 3", resp.Timeout)
	}
	if resp.MaxRequests != 25 {
		t.Fatalf("max_requests = %d, want 25", resp.MaxRequests)
	}
	if !resp.CORS {
		t.Fatal("cors = false, want true")
	}
	if !resp.ExpiresAt.After(resp.CreatedAt) {
		t.Fatalf("expires_at = %v, want later than created_at %v", resp.ExpiresAt, resp.CreatedAt)
	}
	if resp.ReceiveSecret != nil {
		t.Fatalf("receive_secret = %q, want nil for public token", *resp.ReceiveSecret)
	}
}

func TestListTokensInAnonymousModeReturnsActiveTokens(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	router := NewRouter(db, hub.New(), authModeNone, nil, nil)

	active := &models.Token{
		UUID:               "active-token",
		ReceiveMode:        receiveModePublic,
		ViewMode:           viewModePublic,
		DefaultStatus:      http.StatusOK,
		DefaultContent:     "",
		DefaultContentType: "text/plain",
		MaxRequests:        10,
		CreatedAt:          now.Add(-time.Minute),
		UpdatedAt:          now.Add(-time.Minute),
		ExpiresAt:          now.Add(24 * time.Hour),
	}
	if err := db.CreateToken(ctx, active); err != nil {
		t.Fatalf("CreateToken(active): %v", err)
	}

	persistent := &models.Token{
		UUID:               "persistent-token",
		ReceiveMode:        receiveModePublic,
		ViewMode:           viewModePublic,
		Persistent:         true,
		DefaultStatus:      http.StatusOK,
		DefaultContent:     "",
		DefaultContentType: "text/plain",
		MaxRequests:        10,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := db.CreateToken(ctx, persistent); err != nil {
		t.Fatalf("CreateToken(persistent): %v", err)
	}

	expired := &models.Token{
		UUID:               "expired-token",
		ReceiveMode:        receiveModePublic,
		ViewMode:           viewModePublic,
		DefaultStatus:      http.StatusOK,
		DefaultContent:     "",
		DefaultContentType: "text/plain",
		MaxRequests:        10,
		CreatedAt:          now.Add(-48 * time.Hour),
		UpdatedAt:          now.Add(-48 * time.Hour),
		ExpiresAt:          now.Add(-time.Minute),
	}
	if err := db.CreateToken(ctx, expired); err != nil {
		t.Fatalf("CreateToken(expired): %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/tokens?limit=10", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp tokenListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if resp.Total != 2 {
		t.Fatalf("total = %d, want 2", resp.Total)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("len(data) = %d, want 2", len(resp.Data))
	}
	if resp.Data[0].UUID != "persistent-token" || !resp.Data[0].CanDelete {
		t.Fatalf("first token = %+v, want persistent-token with can_delete=true", resp.Data[0])
	}
	if resp.Data[1].UUID != "active-token" || !resp.Data[1].CanDelete {
		t.Fatalf("second token = %+v, want active-token with can_delete=true", resp.Data[1])
	}
}

func TestListTokensRequiresAuthenticationWhenEnabled(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	router := NewRouter(db, hub.New(), "local", &fakeAuthService{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/tokens", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestListTokensForAuthenticatedUserReturnsOwnedAndGrantedTokens(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	router := NewRouter(db, hub.New(), "local", &fakeAuthService{}, nil)

	user := createAPIUser(t, db, "user-1", "user@example.com", "user")
	owner := createAPIUser(t, db, "owner-1", "owner@example.com", "user")

	owned := &models.Token{
		UUID:               "owned-token",
		OwnerID:            &user.ID,
		ReceiveMode:        receiveModePublic,
		ViewMode:           viewModePrivate,
		DefaultStatus:      http.StatusOK,
		DefaultContent:     "",
		DefaultContentType: "text/plain",
		MaxRequests:        10,
		CreatedAt:          now,
		UpdatedAt:          now,
		ExpiresAt:          now.Add(24 * time.Hour),
	}
	if err := db.CreateToken(ctx, owned); err != nil {
		t.Fatalf("CreateToken(owned): %v", err)
	}

	granted := &models.Token{
		UUID:               "granted-token",
		OwnerID:            &owner.ID,
		ReceiveMode:        receiveModePublic,
		ViewMode:           viewModePrivate,
		DefaultStatus:      http.StatusOK,
		DefaultContent:     "",
		DefaultContentType: "text/plain",
		MaxRequests:        10,
		CreatedAt:          now.Add(time.Minute),
		UpdatedAt:          now.Add(time.Minute),
		ExpiresAt:          now.Add(25 * time.Hour),
	}
	if err := db.CreateToken(ctx, granted); err != nil {
		t.Fatalf("CreateToken(granted): %v", err)
	}

	unrelated := &models.Token{
		UUID:               "unrelated-token",
		OwnerID:            &owner.ID,
		ReceiveMode:        receiveModePublic,
		ViewMode:           viewModePrivate,
		DefaultStatus:      http.StatusOK,
		DefaultContent:     "",
		DefaultContentType: "text/plain",
		MaxRequests:        10,
		CreatedAt:          now.Add(2 * time.Minute),
		UpdatedAt:          now.Add(2 * time.Minute),
		ExpiresAt:          now.Add(26 * time.Hour),
	}
	if err := db.CreateToken(ctx, unrelated); err != nil {
		t.Fatalf("CreateToken(unrelated): %v", err)
	}

	if err := db.CreateHookGrant(ctx, &models.HookGrant{
		ID:        "grant-1",
		TokenID:   granted.UUID,
		UserID:    user.ID,
		Role:      "viewer",
		GrantedBy: owner.ID,
		CreatedAt: now.Add(3 * time.Minute),
	}); err != nil {
		t.Fatalf("CreateHookGrant: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/tokens?limit=10", nil)
	req = requestWithUser(req, user)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp tokenListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if resp.Total != 2 {
		t.Fatalf("total = %d, want 2", resp.Total)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("len(data) = %d, want 2", len(resp.Data))
	}
	if resp.Data[0].UUID != "granted-token" || resp.Data[0].AccessRole != "viewer" || resp.Data[0].CanDelete {
		t.Fatalf("first token = %+v, want granted-token viewer can_delete=false", resp.Data[0])
	}
	if resp.Data[1].UUID != "owned-token" || resp.Data[1].AccessRole != "owner" || !resp.Data[1].CanDelete {
		t.Fatalf("second token = %+v, want owned-token owner can_delete=true", resp.Data[1])
	}
}

func TestAdminListTokensReturnsAllActiveTokens(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	router := NewRouter(db, hub.New(), "local", &fakeAuthService{}, nil)

	admin := createAPIUser(t, db, "admin-1", "admin@example.com", "admin")
	owner := createAPIUser(t, db, "owner-1", "owner@example.com", "user")
	user := createAPIUser(t, db, "user-1", "user@example.com", "user")

	owned := &models.Token{
		UUID:               "owned-token",
		OwnerID:            &owner.ID,
		ReceiveMode:        receiveModePublic,
		ViewMode:           viewModePrivate,
		DefaultStatus:      http.StatusOK,
		DefaultContent:     "",
		DefaultContentType: "text/plain",
		MaxRequests:        10,
		CreatedAt:          now,
		UpdatedAt:          now,
		ExpiresAt:          now.Add(24 * time.Hour),
	}
	if err := db.CreateToken(ctx, owned); err != nil {
		t.Fatalf("CreateToken(owned): %v", err)
	}

	anonymous := &models.Token{
		UUID:               "anonymous-token",
		ReceiveMode:        receiveModePublic,
		ViewMode:           viewModePublic,
		DefaultStatus:      http.StatusOK,
		DefaultContent:     "",
		DefaultContentType: "text/plain",
		MaxRequests:        10,
		CreatedAt:          now.Add(time.Minute),
		UpdatedAt:          now.Add(time.Minute),
		ExpiresAt:          now.Add(25 * time.Hour),
	}
	if err := db.CreateToken(ctx, anonymous); err != nil {
		t.Fatalf("CreateToken(anonymous): %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/tokens?limit=10", nil)
	req = requestWithUser(req, admin)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp tokenListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if resp.Total != 2 {
		t.Fatalf("total = %d, want 2", resp.Total)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("len(data) = %d, want 2", len(resp.Data))
	}
	if resp.Data[0].UUID != "anonymous-token" || resp.Data[0].OwnerDisplay != "Anonymous" || !resp.Data[0].CanDelete {
		t.Fatalf("first token = %+v, want anonymous-token owner_display=Anonymous can_delete=true", resp.Data[0])
	}
	if resp.Data[1].UUID != "owned-token" || resp.Data[1].OwnerDisplay != owner.Email || !resp.Data[1].CanDelete {
		t.Fatalf("second token = %+v, want owned-token owner_display=%s can_delete=true", resp.Data[1], owner.Email)
	}

	nonAdminReq := httptest.NewRequest(http.MethodGet, "/api/admin/tokens?limit=10", nil)
	nonAdminReq = requestWithUser(nonAdminReq, user)
	nonAdminRec := httptest.NewRecorder()
	router.ServeHTTP(nonAdminRec, nonAdminReq)
	if nonAdminRec.Code != http.StatusForbidden {
		t.Fatalf("non-admin status = %d, want %d", nonAdminRec.Code, http.StatusForbidden)
	}
}

func TestCreateTokenForcesPublicViewModeInAnonymousAuth(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	router := NewRouter(db, hub.New(), authModeNone, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/tokens", bytes.NewReader([]byte(`{
		"receive_mode":"public",
		"view_mode":"private"
	}`)))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var resp tokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if resp.ViewMode != viewModePublic {
		t.Fatalf("view_mode = %q, want %q", resp.ViewMode, viewModePublic)
	}
}

func TestCreatePersistentTokenRequiresAuthentication(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	router := NewRouter(db, hub.New(), authModeNone, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/tokens", bytes.NewReader([]byte(`{"persistent":true}`)))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreatePersistentTokenForAuthenticatedOwner(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	user := createAPIUser(t, db, "owner-persistent", "owner-persistent@example.com", "user")
	router := NewRouter(db, hub.New(), "local", &fakeAuthService{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/tokens", bytes.NewReader([]byte(`{
		"persistent":true,
		"view_mode":"private",
		"signature_provider":"github",
		"signature_secret":"topsecret"
	}`)))
	req = requestWithUser(req, user)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var resp tokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !resp.Persistent {
		t.Fatal("expected persistent token response")
	}
	if !resp.SignatureConfigured {
		t.Fatal("expected signature secret to be marked configured")
	}
	if resp.SignatureProvider != "github" {
		t.Fatalf("signature_provider = %q, want github", resp.SignatureProvider)
	}

	stored, err := db.GetToken(context.Background(), resp.UUID)
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if !stored.Persistent {
		t.Fatal("expected stored token to be persistent")
	}
	if stored.SignatureSecret == nil || *stored.SignatureSecret != "topsecret" {
		t.Fatalf("signature_secret = %#v, want stored secret", stored.SignatureSecret)
	}
}

func TestCreatePrivateTokenReturnsReceiveSecretAndStoresHashOnly(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	router := NewRouter(db, hub.New(), authModeNone, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/tokens", bytes.NewReader([]byte(`{
		"receive_mode":"private"
	}`)))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var resp tokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if resp.ReceiveSecret == nil || *resp.ReceiveSecret == "" {
		t.Fatal("expected receive_secret in create response")
	}
	if len(*resp.ReceiveSecret) != 43 {
		t.Fatalf("receive_secret len = %d, want 43", len(*resp.ReceiveSecret))
	}
	if resp.ReceiveSecretPrefix == nil || *resp.ReceiveSecretPrefix != (*resp.ReceiveSecret)[:4] {
		t.Fatalf("receive_secret_prefix = %#v, want first 4 chars of secret", resp.ReceiveSecretPrefix)
	}

	token, err := db.GetToken(context.Background(), resp.UUID)
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if token.ReceiveSecretHash == nil || *token.ReceiveSecretHash == "" {
		t.Fatal("expected receive_secret_hash to be stored")
	}
	if token.ReceiveSecretHash != nil && *token.ReceiveSecretHash == *resp.ReceiveSecret {
		t.Fatal("stored hash should not equal raw secret")
	}
	if !validateReceiveSecret(token, *resp.ReceiveSecret) {
		t.Fatal("validateReceiveSecret returned false for generated secret")
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/tokens/"+resp.UUID, nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", getRec.Code, http.StatusOK)
	}

	var getResp tokenResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("json.Unmarshal get response: %v", err)
	}
	if getResp.ReceiveSecret != nil {
		t.Fatal("receive_secret should not be returned from GET token")
	}
	if getResp.ReceiveSecretPrefix == nil || *getResp.ReceiveSecretPrefix != (*resp.ReceiveSecret)[:4] {
		t.Fatalf("receive_secret_prefix = %#v, want persisted prefix", getResp.ReceiveSecretPrefix)
	}
}

func TestCreateTokenRejectsInvalidAccessModes(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	router := NewRouter(db, hub.New(), authModeNone, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/tokens", bytes.NewReader([]byte(`{
		"receive_mode":"locked",
		"view_mode":"public"
	}`)))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetTokenReturnsGoneWhenExpired(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 14, 0, 0, 0, time.UTC)
	token := &models.Token{
		UUID:               "550e8400-e29b-41d4-a716-446655440099",
		ReceiveMode:        receiveModePublic,
		ViewMode:           viewModePublic,
		DefaultStatus:      http.StatusOK,
		DefaultContent:     "",
		DefaultContentType: "text/plain",
		Timeout:            0,
		CORS:               false,
		CreatedAt:          now.Add(-8 * 24 * time.Hour),
		UpdatedAt:          now.Add(-8 * 24 * time.Hour),
		ExpiresAt:          now.Add(-time.Minute),
	}
	if err := db.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	router := NewRouter(db, hub.New(), authModeNone, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/tokens/"+token.UUID, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusGone {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusGone)
	}
}

func TestGetTokenRefreshesExpiry(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	token := &models.Token{
		UUID:               "550e8400-e29b-41d4-a716-446655440098",
		ReceiveMode:        receiveModePublic,
		ViewMode:           viewModePublic,
		DefaultStatus:      http.StatusOK,
		DefaultContent:     "",
		DefaultContentType: "text/plain",
		Timeout:            0,
		CORS:               false,
		CreatedAt:          now.Add(-time.Hour),
		UpdatedAt:          now.Add(-time.Hour),
		ExpiresAt:          now.Add(time.Hour),
	}
	if err := db.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	router := NewRouter(db, hub.New(), authModeNone, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/tokens/"+token.UUID, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	stored, err := db.GetToken(ctx, token.UUID)
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	wantMinimum := time.Now().UTC().Add(store.DefaultTokenTTL - time.Hour)
	if !stored.ExpiresAt.After(wantMinimum) {
		t.Fatalf("expires_at = %v, want it refreshed beyond %v", stored.ExpiresAt, wantMinimum)
	}
}

func TestUpdateTokenRejectsInvalidTimeout(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 14, 0, 0, 0, time.UTC)
	token := &models.Token{
		UUID:               "550e8400-e29b-41d4-a716-446655440001",
		ReceiveMode:        "public",
		ViewMode:           "public",
		DefaultStatus:      http.StatusOK,
		DefaultContent:     "",
		DefaultContentType: "text/plain",
		Timeout:            0,
		CORS:               false,
		CreatedAt:          now,
		UpdatedAt:          now,
		ExpiresAt:          activeExpiresAt(),
	}
	if err := db.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	router := NewRouter(db, hub.New(), authModeNone, nil, nil)
	req := httptest.NewRequest(http.MethodPut, "/api/tokens/"+token.UUID, bytes.NewReader([]byte(`{"timeout":11}`)))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateTokenRejectsInvalidMaxRequests(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	router := NewRouter(db, hub.New(), authModeNone, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/tokens", bytes.NewReader([]byte(`{"max_requests":0}`)))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdateTokenPublicToPrivateReturnsOneTimeReceiveSecret(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 14, 0, 0, 0, time.UTC)
	token := &models.Token{
		UUID:               "550e8400-e29b-41d4-a716-446655440012",
		ReceiveMode:        receiveModePublic,
		ViewMode:           viewModePublic,
		DefaultStatus:      http.StatusOK,
		DefaultContent:     "",
		DefaultContentType: "text/plain",
		Timeout:            0,
		CORS:               false,
		CreatedAt:          now,
		UpdatedAt:          now,
		ExpiresAt:          activeExpiresAt(),
	}
	if err := db.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	router := NewRouter(db, hub.New(), authModeNone, nil, nil)
	req := httptest.NewRequest(http.MethodPut, "/api/tokens/"+token.UUID, bytes.NewReader([]byte(`{"receive_mode":"private"}`)))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp tokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if resp.ReceiveSecret == nil || *resp.ReceiveSecret == "" {
		t.Fatal("expected one-time receive_secret on public->private update")
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/api/tokens/"+token.UUID, nil)
	secondRec := httptest.NewRecorder()
	router.ServeHTTP(secondRec, secondReq)

	if secondRec.Code != http.StatusOK {
		t.Fatalf("second get status = %d, want %d", secondRec.Code, http.StatusOK)
	}

	var secondResp tokenResponse
	if err := json.Unmarshal(secondRec.Body.Bytes(), &secondResp); err != nil {
		t.Fatalf("json.Unmarshal second response: %v", err)
	}
	if secondResp.ReceiveSecret != nil {
		t.Fatal("receive_secret should not persist after update response")
	}
}

func TestUpdateTokenPrivateToPublicClearsReceiveSecretState(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 14, 0, 0, 0, time.UTC)
	secret := "abcd1234secret"
	hash := hashReceiveSecret(secret)
	prefix := secret[:4]
	token := &models.Token{
		UUID:                "550e8400-e29b-41d4-a716-446655440013",
		ReceiveMode:         receiveModePrivate,
		ViewMode:            viewModePublic,
		ReceiveSecretHash:   &hash,
		ReceiveSecretPrefix: &prefix,
		DefaultStatus:       http.StatusOK,
		DefaultContent:      "",
		DefaultContentType:  "text/plain",
		Timeout:             0,
		CORS:                false,
		CreatedAt:           now,
		UpdatedAt:           now,
		ExpiresAt:           activeExpiresAt(),
	}
	if err := db.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	router := NewRouter(db, hub.New(), authModeNone, nil, nil)
	req := httptest.NewRequest(http.MethodPut, "/api/tokens/"+token.UUID, bytes.NewReader([]byte(`{"receive_mode":"public"}`)))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	stored, err := db.GetToken(ctx, token.UUID)
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if stored.ReceiveSecretHash != nil || stored.ReceiveSecretPrefix != nil {
		t.Fatal("expected receive secret hash and prefix to be cleared when receive_mode becomes public")
	}
}

func TestRotateReceiveSecretReturnsNewSecretAndUpdatesStoredHash(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 14, 0, 0, 0, time.UTC)
	oldSecret := "abcd1234secret"
	oldHash := hashReceiveSecret(oldSecret)
	oldPrefix := oldSecret[:4]
	token := &models.Token{
		UUID:                "550e8400-e29b-41d4-a716-446655440014",
		ReceiveMode:         receiveModePrivate,
		ViewMode:            viewModePublic,
		ReceiveSecretHash:   &oldHash,
		ReceiveSecretPrefix: &oldPrefix,
		DefaultStatus:       http.StatusOK,
		DefaultContent:      "",
		DefaultContentType:  "text/plain",
		Timeout:             0,
		CORS:                false,
		CreatedAt:           now,
		UpdatedAt:           now,
		ExpiresAt:           activeExpiresAt(),
	}
	if err := db.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	router := NewRouter(db, hub.New(), authModeNone, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/tokens/"+token.UUID+"/rotate-secret", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp struct {
		ReceiveSecret       *string `json:"receive_secret"`
		ReceiveSecretPrefix *string `json:"receive_secret_prefix"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if resp.ReceiveSecret == nil || *resp.ReceiveSecret == "" {
		t.Fatal("expected rotated receive_secret")
	}
	if *resp.ReceiveSecret == oldSecret {
		t.Fatal("expected rotated secret to differ from old secret")
	}
	if resp.ReceiveSecretPrefix == nil || *resp.ReceiveSecretPrefix != (*resp.ReceiveSecret)[:4] {
		t.Fatalf("receive_secret_prefix = %#v, want first 4 chars of new secret", resp.ReceiveSecretPrefix)
	}

	stored, err := db.GetToken(ctx, token.UUID)
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if stored.ReceiveSecretHash == nil || *stored.ReceiveSecretHash == oldHash {
		t.Fatal("expected stored secret hash to change after rotation")
	}
	if !validateReceiveSecret(stored, *resp.ReceiveSecret) {
		t.Fatal("new stored hash does not validate rotated secret")
	}
	if validateReceiveSecret(stored, oldSecret) {
		t.Fatal("old secret should be invalid after rotation")
	}
}

func TestUpdateTokenUsesConfiguredTTLWhenRefreshingExpiry(t *testing.T) {
	db, err := store.Open(t.TempDir(), store.Config{TokenTTL: 2 * time.Hour})
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Fatalf("store.Close: %v", closeErr)
		}
	})

	restore := timeNow
	now := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	timeNow = func() time.Time { return now }
	t.Cleanup(func() {
		timeNow = restore
	})

	ctx := context.Background()
	token := &models.Token{
		UUID:               "550e8400-e29b-41d4-a716-446655440016",
		ReceiveMode:        receiveModePublic,
		ViewMode:           viewModePublic,
		DefaultStatus:      http.StatusOK,
		DefaultContent:     "",
		DefaultContentType: "text/plain",
		Timeout:            0,
		CORS:               false,
		CreatedAt:          now.Add(-time.Hour),
		UpdatedAt:          now.Add(-time.Hour),
		ExpiresAt:          now.Add(time.Hour),
	}
	if err := db.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	router := NewRouter(db, hub.New(), authModeNone, nil, nil)
	req := httptest.NewRequest(http.MethodPut, "/api/tokens/"+token.UUID, bytes.NewReader([]byte(`{"default_status":204}`)))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	stored, err := db.GetToken(ctx, token.UUID)
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	want := now.Add(2 * time.Hour)
	if !stored.ExpiresAt.Equal(want) {
		t.Fatalf("expires_at = %v, want %v", stored.ExpiresAt, want)
	}
}

func TestRotateReceiveSecretUsesConfiguredTTLWhenRefreshingExpiry(t *testing.T) {
	db, err := store.Open(t.TempDir(), store.Config{TokenTTL: 2 * time.Hour})
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Fatalf("store.Close: %v", closeErr)
		}
	})

	restore := timeNow
	now := time.Date(2026, 4, 2, 11, 0, 0, 0, time.UTC)
	timeNow = func() time.Time { return now }
	t.Cleanup(func() {
		timeNow = restore
	})

	ctx := context.Background()
	oldSecret := "abcd1234secret"
	oldHash := hashReceiveSecret(oldSecret)
	oldPrefix := oldSecret[:4]
	token := &models.Token{
		UUID:                "550e8400-e29b-41d4-a716-446655440017",
		ReceiveMode:         receiveModePrivate,
		ViewMode:            viewModePublic,
		ReceiveSecretHash:   &oldHash,
		ReceiveSecretPrefix: &oldPrefix,
		DefaultStatus:       http.StatusOK,
		DefaultContent:      "",
		DefaultContentType:  "text/plain",
		Timeout:             0,
		CORS:                false,
		CreatedAt:           now.Add(-time.Hour),
		UpdatedAt:           now.Add(-time.Hour),
		ExpiresAt:           now.Add(time.Hour),
	}
	if err := db.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	router := NewRouter(db, hub.New(), authModeNone, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/tokens/"+token.UUID+"/rotate-secret", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	stored, err := db.GetToken(ctx, token.UUID)
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	want := now.Add(2 * time.Hour)
	if !stored.ExpiresAt.Equal(want) {
		t.Fatalf("expires_at = %v, want %v", stored.ExpiresAt, want)
	}
}

func TestRotateReceiveSecretRejectsPublicToken(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 14, 0, 0, 0, time.UTC)
	token := &models.Token{
		UUID:               "550e8400-e29b-41d4-a716-446655440015",
		ReceiveMode:        receiveModePublic,
		ViewMode:           viewModePublic,
		DefaultStatus:      http.StatusOK,
		DefaultContent:     "",
		DefaultContentType: "text/plain",
		Timeout:            0,
		CORS:               false,
		CreatedAt:          now,
		UpdatedAt:          now,
		ExpiresAt:          activeExpiresAt(),
	}
	if err := db.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	router := NewRouter(db, hub.New(), authModeNone, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/tokens/"+token.UUID+"/rotate-secret", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCaptureWebhookHonorsConfiguredTimeout(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 14, 0, 0, 0, time.UTC)
	tokenID := "550e8400-e29b-41d4-a716-446655440002"
	token := &models.Token{
		UUID:               tokenID,
		ReceiveMode:        "public",
		ViewMode:           "public",
		DefaultStatus:      http.StatusAccepted,
		DefaultContent:     "delayed",
		DefaultContentType: "text/plain",
		Timeout:            1,
		CORS:               false,
		CreatedAt:          now,
		UpdatedAt:          now,
		ExpiresAt:          activeExpiresAt(),
	}
	if err := db.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	router := NewRouter(db, hub.New(), authModeNone, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/"+tokenID, nil)
	rec := httptest.NewRecorder()

	start := time.Now()
	router.ServeHTTP(rec, req)
	elapsed := time.Since(start)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	if elapsed < 900*time.Millisecond {
		t.Fatalf("elapsed = %v, want about 1s delay", elapsed)
	}
}
