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
)

func TestCreateTokenAcceptsConfigurableResponseFields(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	router := NewRouter(db, hub.New(), authModeNone)

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

func TestCreateTokenForcesPublicViewModeInAnonymousAuth(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	router := NewRouter(db, hub.New(), authModeNone)

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

func TestCreatePrivateTokenReturnsReceiveSecretAndStoresHashOnly(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	router := NewRouter(db, hub.New(), authModeNone)

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
	router := NewRouter(db, hub.New(), authModeNone)

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

func TestGetTokenHidesPrivateViewHooks(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 14, 0, 0, 0, time.UTC)
	token := &models.Token{
		UUID:               "550e8400-e29b-41d4-a716-446655440010",
		ReceiveMode:        receiveModePublic,
		ViewMode:           viewModePrivate,
		DefaultStatus:      http.StatusOK,
		DefaultContent:     "",
		DefaultContentType: "text/plain",
		Timeout:            0,
		CORS:               false,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := db.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	router := NewRouter(db, hub.New(), authModeNone)
	req := httptest.NewRequest(http.MethodGet, "/api/tokens/"+token.UUID, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
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

	router := NewRouter(db, hub.New(), authModeNone)
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

	router := NewRouter(db, hub.New(), authModeNone)
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
	if !stored.ExpiresAt.After(now.Add(6 * 24 * time.Hour)) {
		t.Fatalf("expires_at = %v, want it refreshed beyond %v", stored.ExpiresAt, now.Add(6*24*time.Hour))
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
	}
	if err := db.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	router := NewRouter(db, hub.New(), authModeNone)
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
	router := NewRouter(db, hub.New(), authModeNone)

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
	}
	if err := db.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	router := NewRouter(db, hub.New(), authModeNone)
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
	}
	if err := db.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	router := NewRouter(db, hub.New(), authModeNone)
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
	}
	if err := db.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	router := NewRouter(db, hub.New(), authModeNone)
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
	}
	if err := db.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	router := NewRouter(db, hub.New(), authModeNone)
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
	}
	if err := db.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	router := NewRouter(db, hub.New(), authModeNone)
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
