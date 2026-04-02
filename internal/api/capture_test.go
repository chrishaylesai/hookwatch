package api

import (
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chrishaylesai/hookwatch/internal/hub"
	"github.com/chrishaylesai/hookwatch/internal/models"
	"github.com/chrishaylesai/hookwatch/internal/store"
)

func TestCaptureWebhookStoresRequestAndUsesTokenDefaults(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 13, 0, 0, 0, time.UTC)
	tokenID := "550e8400-e29b-41d4-a716-446655440000"

	token := &models.Token{
		UUID:               tokenID,
		ReceiveMode:        "public",
		ViewMode:           "public",
		DefaultStatus:      http.StatusCreated,
		DefaultContent:     "captured",
		DefaultContentType: "text/plain",
		Timeout:            0,
		CORS:               false,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := db.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	router := NewRouter(db, hub.New(), authModeNone, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/"+tokenID+"/webhook/github?foo=bar", strings.NewReader("event=push&branch=main"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "hookwatch-test")
	req.Header.Set("X-GitHub-Event", "push")
	req.RemoteAddr = "203.0.113.5:4242"

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/plain" {
		t.Fatalf("content-type = %q, want text/plain", got)
	}
	if got := rec.Header().Get("X-Token-Id"); got != tokenID {
		t.Fatalf("X-Token-Id = %q, want %q", got, tokenID)
	}
	if got := rec.Header().Get("X-Request-Id"); got == "" {
		t.Fatal("missing X-Request-Id header")
	}
	if got := rec.Body.String(); got != "captured" {
		t.Fatalf("body = %q, want captured", got)
	}

	page, err := db.ListRequestsByToken(ctx, tokenID, storeRequestPageParams())
	if err != nil {
		t.Fatalf("ListRequestsByToken: %v", err)
	}
	if len(page.Requests) != 1 {
		t.Fatalf("captured requests = %d, want 1", len(page.Requests))
	}

	stored := page.Requests[0]
	if stored.Method != http.MethodPost {
		t.Fatalf("method = %q, want POST", stored.Method)
	}
	if stored.IP != "203.0.113.5" {
		t.Fatalf("ip = %q, want remote host", stored.IP)
	}
	if stored.Query != "foo=bar" {
		t.Fatalf("query = %q, want foo=bar", stored.Query)
	}
	if stored.URL != "http://example.com/"+tokenID+"/webhook/github?foo=bar" {
		t.Fatalf("url = %q, want full request URL", stored.URL)
	}
	if stored.Content != "event=push&branch=main" {
		t.Fatalf("content = %q, want raw body", stored.Content)
	}

	headers := decodeJSONMap(stored.Headers)
	if got := headers["X-Github-Event"]; got != "push" {
		t.Fatalf("headers[X-Github-Event] = %#v, want push", got)
	}

	formData := decodeJSONMap(stored.FormData)
	if got := formData["event"]; got != "push" {
		t.Fatalf("form_data[event] = %#v, want push", got)
	}
	if got := formData["branch"]; got != "main" {
		t.Fatalf("form_data[branch] = %#v, want main", got)
	}
}

func TestCaptureWebhookMissingTokenReturnsNotFound(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	router := NewRouter(db, hub.New(), authModeNone, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/550e8400-e29b-41d4-a716-446655440000/webhook", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestCaptureWebhookExpiredTokenReturnsGone(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 13, 10, 0, 0, time.UTC)
	tokenID := "550e8400-e29b-41d4-a716-446655440003"

	token := &models.Token{
		UUID:               tokenID,
		ReceiveMode:        receiveModePublic,
		ViewMode:           viewModePublic,
		DefaultStatus:      http.StatusOK,
		DefaultContent:     "expired",
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
	req := httptest.NewRequest(http.MethodPost, "/"+tokenID+"/webhook", strings.NewReader("payload"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusGone {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusGone)
	}
}

func TestCaptureWebhookReturnsGoneWhenQuotaExceeded(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 13, 12, 0, 0, time.UTC)
	tokenID := "550e8400-e29b-41d4-a716-446655440097"

	token := &models.Token{
		UUID:               tokenID,
		ReceiveMode:        receiveModePublic,
		ViewMode:           viewModePublic,
		DefaultStatus:      http.StatusOK,
		DefaultContent:     "ok",
		DefaultContentType: "text/plain",
		MaxRequests:        1,
		Timeout:            0,
		CORS:               false,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := db.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	router := NewRouter(db, hub.New(), authModeNone, nil, nil)

	firstReq := httptest.NewRequest(http.MethodPost, "/"+tokenID+"/incoming", strings.NewReader("first"))
	firstRec := httptest.NewRecorder()
	router.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("first status = %d, want %d", firstRec.Code, http.StatusOK)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/"+tokenID+"/incoming", strings.NewReader("second"))
	secondRec := httptest.NewRecorder()
	router.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusGone {
		t.Fatalf("second status = %d, want %d", secondRec.Code, http.StatusGone)
	}

	page, err := db.ListRequestsByToken(ctx, tokenID, storeRequestPageParams())
	if err != nil {
		t.Fatalf("ListRequestsByToken: %v", err)
	}
	if len(page.Requests) != 1 {
		t.Fatalf("captured requests = %d, want 1", len(page.Requests))
	}
}

func TestCaptureWebhookRejectsPrivateReceiveModeWithoutSecret(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 13, 15, 0, 0, time.UTC)
	tokenID := "550e8400-e29b-41d4-a716-446655440004"
	secret := "abcd1234secret"
	hash := hashReceiveSecret(secret)
	prefix := secret[:4]

	token := &models.Token{
		UUID:                tokenID,
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

	router := NewRouter(db, hub.New(), authModeNone, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/"+tokenID+"/incoming", strings.NewReader("payload"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestCaptureWebhookAcceptsHeaderSecretAndScrubsItFromStoredRequest(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 13, 16, 0, 0, time.UTC)
	tokenID := "550e8400-e29b-41d4-a716-446655440005"
	secret := "header-secret-1234"
	hash := hashReceiveSecret(secret)
	prefix := secret[:4]

	token := &models.Token{
		UUID:                tokenID,
		ReceiveMode:         receiveModePrivate,
		ViewMode:            viewModePublic,
		ReceiveSecretHash:   &hash,
		ReceiveSecretPrefix: &prefix,
		DefaultStatus:       http.StatusOK,
		DefaultContent:      "ok",
		DefaultContentType:  "text/plain",
		Timeout:             0,
		CORS:                false,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := db.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	router := NewRouter(db, hub.New(), authModeNone, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/"+tokenID+"/incoming?secret=wrong", strings.NewReader("payload"))
	req.Header.Set("X-Hook-Secret", secret)
	req.Header.Set("Authorization", "Basic dXNlcjp3cm9uZw==")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	page, err := db.ListRequestsByToken(ctx, tokenID, storeRequestPageParams())
	if err != nil {
		t.Fatalf("ListRequestsByToken: %v", err)
	}
	if len(page.Requests) != 1 {
		t.Fatalf("captured requests = %d, want 1", len(page.Requests))
	}

	stored := page.Requests[0]
	if strings.Contains(stored.Query, "secret=") {
		t.Fatalf("query = %q, secret should be scrubbed", stored.Query)
	}
	headers := decodeJSONMap(stored.Headers)
	if _, ok := headers["X-Hook-Secret"]; ok {
		t.Fatal("stored headers should not include X-Hook-Secret")
	}
}

func TestCaptureWebhookAcceptsQuerySecretAndScrubsURL(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 13, 17, 0, 0, time.UTC)
	tokenID := "550e8400-e29b-41d4-a716-446655440006"
	secret := "query-secret-1234"
	hash := hashReceiveSecret(secret)
	prefix := secret[:4]

	token := &models.Token{
		UUID:                tokenID,
		ReceiveMode:         receiveModePrivate,
		ViewMode:            viewModePublic,
		ReceiveSecretHash:   &hash,
		ReceiveSecretPrefix: &prefix,
		DefaultStatus:       http.StatusAccepted,
		DefaultContent:      "accepted",
		DefaultContentType:  "text/plain",
		Timeout:             0,
		CORS:                false,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := db.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	router := NewRouter(db, hub.New(), authModeNone, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/"+tokenID+"/incoming?foo=bar&secret="+secret, strings.NewReader("payload"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	page, err := db.ListRequestsByToken(ctx, tokenID, storeRequestPageParams())
	if err != nil {
		t.Fatalf("ListRequestsByToken: %v", err)
	}
	stored := page.Requests[0]
	if stored.Query != "foo=bar" {
		t.Fatalf("query = %q, want foo=bar", stored.Query)
	}
	if stored.URL != "http://example.com/"+tokenID+"/incoming?foo=bar" {
		t.Fatalf("url = %q, want scrubbed URL", stored.URL)
	}
}

func TestCaptureWebhookAcceptsBasicAuthSecretAndScrubsAuthorization(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 13, 18, 0, 0, time.UTC)
	tokenID := "550e8400-e29b-41d4-a716-446655440007"
	secret := "basic-secret-1234"
	hash := hashReceiveSecret(secret)
	prefix := secret[:4]

	token := &models.Token{
		UUID:                tokenID,
		ReceiveMode:         receiveModePrivate,
		ViewMode:            viewModePublic,
		ReceiveSecretHash:   &hash,
		ReceiveSecretPrefix: &prefix,
		DefaultStatus:       http.StatusOK,
		DefaultContent:      "ok",
		DefaultContentType:  "text/plain",
		Timeout:             0,
		CORS:                false,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := db.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	router := NewRouter(db, hub.New(), authModeNone, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/"+tokenID+"/incoming", strings.NewReader("payload"))
	req.SetBasicAuth("", secret)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	page, err := db.ListRequestsByToken(ctx, tokenID, storeRequestPageParams())
	if err != nil {
		t.Fatalf("ListRequestsByToken: %v", err)
	}
	headers := decodeJSONMap(page.Requests[0].Headers)
	if _, ok := headers["Authorization"]; ok {
		t.Fatal("stored headers should not include Authorization when basic auth secret is used")
	}
}

func TestCaptureWebhookAddsCORSHeadersWhenEnabled(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 13, 30, 0, 0, time.UTC)
	tokenID := "550e8400-e29b-41d4-a716-446655440003"

	token := &models.Token{
		UUID:               tokenID,
		ReceiveMode:        "public",
		ViewMode:           "public",
		DefaultStatus:      http.StatusOK,
		DefaultContent:     "",
		DefaultContentType: "text/plain",
		Timeout:            0,
		CORS:               true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := db.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	router := NewRouter(db, hub.New(), authModeNone, nil, nil)
	req := httptest.NewRequest(http.MethodOptions, "/"+tokenID+"/preflight", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("allow-origin = %q, want *", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Fatal("missing Access-Control-Allow-Methods")
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Fatal("missing Access-Control-Allow-Headers")
	}
	if got := rec.Header().Get("Access-Control-Expose-Headers"); !strings.Contains(got, "X-Request-Id") {
		t.Fatalf("expose-headers = %q, want X-Request-Id", got)
	}
}

func TestNonTokenPathFallsBackToSPA(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	router := NewRouter(db, hub.New(), authModeNone, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("content-type = %q, want html", got)
	}
	if !strings.Contains(rec.Body.String(), "<!doctype html>") {
		t.Fatal("expected SPA index response")
	}
}

func TestEncodeFormDataMultipart(t *testing.T) {
	t.Parallel()

	body := &strings.Builder{}
	writer := multipartWriter(t, body)
	if err := writer.WriteField("event", "push"); err != nil {
		t.Fatalf("WriteField: %v", err)
	}
	part, err := writer.CreateFormFile("artifact", "payload.json")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write([]byte(`{"ok":true}`)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	encoded, err := encodeFormData(writer.FormDataContentType(), []byte(body.String()))
	if err != nil {
		t.Fatalf("encodeFormData: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(encoded), &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if got := decoded["event"]; got != "push" {
		t.Fatalf("event = %#v, want push", got)
	}
	files, ok := decoded["artifact"].([]any)
	if !ok || len(files) != 1 {
		t.Fatalf("artifact = %#v, want one file entry", decoded["artifact"])
	}
}

func multipartWriter(t *testing.T, body *strings.Builder) *multipart.Writer {
	t.Helper()

	writer := multipart.NewWriter(body)
	return writer
}

func storeRequestPageParams() store.RequestListParams {
	return store.RequestListParams{Limit: 10}
}
