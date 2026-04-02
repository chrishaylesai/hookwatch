package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chrishaylesai/hookwatch/internal/hub"
	"github.com/chrishaylesai/hookwatch/internal/models"
	"github.com/chrishaylesai/hookwatch/internal/store"
)

func TestRequestEndpointsListGetAndDelete(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	seedRequestData(t, db)
	router := NewRouter(db, hub.New(), authModeNone, nil, nil)

	listReq := httptest.NewRequest(http.MethodGet, "/api/tokens/token-1/requests?per_page=1&page=2&method=post", nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listRec.Code, http.StatusOK)
	}

	var listResp requestListResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if listResp.Total != 2 || listResp.Page != 2 || listResp.PerPage != 1 || listResp.TotalPages != 2 {
		t.Fatalf("list pagination = %+v, want total=2 page=2 per_page=1 total_pages=2", listResp)
	}
	if len(listResp.Data) != 1 || listResp.Data[0].UUID != "request-1" {
		t.Fatalf("list data = %+v, want request-1", listResp.Data)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/tokens/token-1/requests/request-2", nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", getRec.Code, http.StatusOK)
	}

	var getResp requestResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if getResp.UUID != "request-2" {
		t.Fatalf("get uuid = %s, want request-2", getResp.UUID)
	}
	if got := getResp.Headers["Content-Type"]; got != "application/json" {
		t.Fatalf("headers[Content-Type] = %#v, want application/json", got)
	}
	if got := getResp.FormData["event"]; got != "push" {
		t.Fatalf("form_data[event] = %#v, want push", got)
	}

	rawReq := httptest.NewRequest(http.MethodGet, "/api/tokens/token-1/requests/request-2/raw", nil)
	rawRec := httptest.NewRecorder()
	router.ServeHTTP(rawRec, rawReq)

	if rawRec.Code != http.StatusOK {
		t.Fatalf("raw status = %d, want %d", rawRec.Code, http.StatusOK)
	}
	if got := rawRec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("raw content-type = %q, want application/json", got)
	}
	if got := rawRec.Body.String(); got != `{"event":"push"}` {
		t.Fatalf("raw body = %q, want JSON body", got)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/tokens/token-1/requests/request-2", nil)
	deleteRec := httptest.NewRecorder()
	router.ServeHTTP(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", deleteRec.Code, http.StatusNoContent)
	}

	afterDeleteReq := httptest.NewRequest(http.MethodGet, "/api/tokens/token-1/requests/request-2", nil)
	afterDeleteRec := httptest.NewRecorder()
	router.ServeHTTP(afterDeleteRec, afterDeleteReq)

	if afterDeleteRec.Code != http.StatusNotFound {
		t.Fatalf("get after delete status = %d, want %d", afterDeleteRec.Code, http.StatusNotFound)
	}

	deleteAllReq := httptest.NewRequest(http.MethodDelete, "/api/tokens/token-1/requests", nil)
	deleteAllRec := httptest.NewRecorder()
	router.ServeHTTP(deleteAllRec, deleteAllReq)

	if deleteAllRec.Code != http.StatusNoContent {
		t.Fatalf("delete all status = %d, want %d", deleteAllRec.Code, http.StatusNoContent)
	}

	finalListReq := httptest.NewRequest(http.MethodGet, "/api/tokens/token-1/requests", nil)
	finalListRec := httptest.NewRecorder()
	router.ServeHTTP(finalListRec, finalListReq)

	if finalListRec.Code != http.StatusOK {
		t.Fatalf("final list status = %d, want %d", finalListRec.Code, http.StatusOK)
	}

	listResp = requestListResponse{}
	if err := json.Unmarshal(finalListRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode final list response: %v", err)
	}
	if listResp.Total != 0 || len(listResp.Data) != 0 {
		t.Fatalf("final list = %+v, want empty result", listResp)
	}
}

func TestRequestEndpointsMissingToken(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	router := NewRouter(db, hub.New(), authModeNone, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/tokens/missing-token/requests", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestReplayRequestEndpoint(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	seedRequestData(t, db)
	router := NewRouter(db, hub.New(), authModeNone, nil, nil)

	var capturedMethod string
	var capturedQuery string
	var capturedBody string
	var capturedHeader string
	var capturedReplayHeader string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedMethod = r.Method
		capturedQuery = r.URL.RawQuery
		capturedBody = string(body)
		capturedHeader = r.Header.Get("X-Test")
		capturedReplayHeader = r.Header.Get("X-Replay")
		w.Header().Set("X-Target", "ok")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("replayed"))
	}))
	defer target.Close()

	reqBody := `{
		"url":"` + target.URL + `/sink?fixed=true",
		"preserve_headers":true,
		"additional_headers":{"X-Replay":"true"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/tokens/token-1/requests/request-2/replay", strings.NewReader(reqBody))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if capturedMethod != http.MethodPost {
		t.Fatalf("captured method = %q, want POST", capturedMethod)
	}
	if capturedQuery != "fixed=true&foo=bar" {
		t.Fatalf("captured query = %q, want merged query", capturedQuery)
	}
	if capturedBody != `{"event":"push"}` {
		t.Fatalf("captured body = %q, want original request body", capturedBody)
	}
	if capturedHeader != "replay-source" {
		t.Fatalf("captured X-Test = %q, want replay-source", capturedHeader)
	}
	if capturedReplayHeader != "true" {
		t.Fatalf("captured X-Replay = %q, want true", capturedReplayHeader)
	}

	var resp replayResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode replay response: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("replay status = %d, want %d", resp.StatusCode, http.StatusAccepted)
	}
	if resp.Headers["X-Target"] != "ok" {
		t.Fatalf("replay headers = %+v, want X-Target", resp.Headers)
	}
	if resp.Body != "replayed" {
		t.Fatalf("replay body = %q, want replayed", resp.Body)
	}
}

func TestDiffRequestsEndpoint(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	seedRequestData(t, db)
	router := NewRouter(db, hub.New(), authModeNone, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/tokens/token-1/requests/diff?left=request-1&right=request-2", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp requestDiffResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode diff response: %v", err)
	}
	if resp.LeftRequestID != "request-1" || resp.RightRequestID != "request-2" {
		t.Fatalf("unexpected diff IDs: %+v", resp)
	}

	changed := map[string]bool{}
	for _, section := range resp.Sections {
		changed[section.Key] = section.Changed
	}
	if changed["method"] {
		t.Fatal("method should not be marked changed")
	}
	if !changed["body"] {
		t.Fatal("body should be marked changed")
	}
}

func TestOpenAPIGenerationEndpoint(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	seedRequestData(t, db)
	router := NewRouter(db, hub.New(), authModeNone, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/tokens/token-1/openapi.json", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp struct {
		OpenAPI string                         `json:"openapi"`
		Paths   map[string]map[string]any      `json:"paths"`
		Info    map[string]any                 `json:"info"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode spec response: %v", err)
	}
	if resp.OpenAPI != "3.0.3" {
		t.Fatalf("openapi = %q, want 3.0.3", resp.OpenAPI)
	}
	rootPath := resp.Paths["/"]
	if rootPath == nil {
		t.Fatalf("paths = %+v, want root path", resp.Paths)
	}
	if _, ok := rootPath["post"]; !ok {
		t.Fatalf("root path = %+v, want post operation", rootPath)
	}
	if _, ok := rootPath["get"]; !ok {
		t.Fatalf("root path = %+v, want get operation", rootPath)
	}
}

func TestCaptureStoresSignatureValidation(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	secret := "topsecret"
	tokenID := "550e8400-e29b-41d4-a716-446655440123"
	token := &models.Token{
		UUID:               tokenID,
		ReceiveMode:        "public",
		ViewMode:           "public",
		SignatureProvider:  "github",
		SignatureSecret:    &secret,
		DefaultStatus:      http.StatusOK,
		DefaultContent:     "",
		DefaultContentType: "text/plain",
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := db.CreateToken(context.Background(), token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	router := NewRouter(db, hub.New(), authModeNone, nil, nil)
	body := `{"event":"signed"}`
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	captureReq := httptest.NewRequest(http.MethodPost, "/"+tokenID, strings.NewReader(body))
	captureReq.Header.Set("X-Hub-Signature-256", signature)
	captureReq.Header.Set("Content-Type", "application/json")
	captureRec := httptest.NewRecorder()
	router.ServeHTTP(captureRec, captureReq)

	if captureRec.Code != http.StatusOK {
		t.Fatalf("capture status = %d, want %d", captureRec.Code, http.StatusOK)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/tokens/"+tokenID+"/requests", nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listRec.Code, http.StatusOK)
	}

	var listResp requestListResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listResp.Data) != 1 {
		t.Fatalf("captured requests = %d, want 1", len(listResp.Data))
	}
	if listResp.Data[0].SignatureValidation.Status != signatureStatusValid {
		t.Fatalf("signature status = %q, want %q", listResp.Data[0].SignatureValidation.Status, signatureStatusValid)
	}
	if listResp.Data[0].SignatureValidation.Provider == nil || *listResp.Data[0].SignatureValidation.Provider != "github" {
		t.Fatalf("signature provider = %#v, want github", listResp.Data[0].SignatureValidation.Provider)
	}
}

func newTestStore(t *testing.T) *store.Store {
	t.Helper()

	db, err := store.Open(t.TempDir(), store.Config{})
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatalf("store.Close: %v", err)
		}
	})

	return db
}

func seedRequestData(t *testing.T, db *store.Store) {
	t.Helper()

	ctx := context.Background()
	createdAt := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)
	token := &models.Token{
		UUID:               "token-1",
		ReceiveMode:        "public",
		ViewMode:           "public",
		DefaultStatus:      http.StatusOK,
		DefaultContent:     "",
		DefaultContentType: "text/plain",
		Timeout:            0,
		CORS:               false,
		CreatedAt:          createdAt,
		UpdatedAt:          createdAt,
	}
	if err := db.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	requests := []*models.Request{
		{
			UUID:      "request-1",
			TokenID:   "token-1",
			IP:        "127.0.0.1",
			Hostname:  "example.test",
			Method:    "POST",
			UserAgent: "curl/8.0.0",
			Content:   `{"event":"older"}`,
			Query:     "foo=bar",
			Headers:   `{"Content-Type":"application/json","X-Test":"replay-source"}`,
			FormData:  `{"event":"older"}`,
			URL:       "https://example.test/token-1",
			CreatedAt: createdAt.Add(time.Minute),
		},
		{
			UUID:      "request-2",
			TokenID:   "token-1",
			IP:        "127.0.0.1",
			Hostname:  "example.test",
			Method:    "POST",
			UserAgent: "curl/8.0.0",
			Content:   `{"event":"push"}`,
			Query:     "foo=bar",
			Headers:   `{"Content-Type":"application/json","X-Test":"replay-source"}`,
			FormData:  `{"event":"push"}`,
			URL:       "https://example.test/token-1",
			CreatedAt: createdAt.Add(2 * time.Minute),
		},
		{
			UUID:      "request-3",
			TokenID:   "token-1",
			IP:        "203.0.113.10",
			Hostname:  "example.test",
			Method:    "GET",
			UserAgent: "curl/8.0.0",
			Content:   "",
			Query:     "",
			Headers:   `{"Content-Type":"text/plain"}`,
			FormData:  `{}`,
			URL:       "https://example.test/token-1",
			CreatedAt: createdAt.Add(3 * time.Minute),
		},
	}

	for _, req := range requests {
		if err := db.CreateRequest(ctx, req); err != nil {
			t.Fatalf("CreateRequest(%s): %v", req.UUID, err)
		}
	}
}
