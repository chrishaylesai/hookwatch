package api

import (
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

func TestRequestEndpointsListGetAndDelete(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	seedRequestData(t, db)
	router := NewRouter(db, hub.New(), authModeNone)

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
	router := NewRouter(db, hub.New(), authModeNone)

	req := httptest.NewRequest(http.MethodGet, "/api/tokens/missing-token/requests", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestListRequestsHidesPrivateViewHooks(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)
	token := &models.Token{
		UUID:               "550e8400-e29b-41d4-a716-446655440011",
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
	req := httptest.NewRequest(http.MethodGet, "/api/tokens/"+token.UUID+"/requests", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
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
			Headers:   `{"Content-Type":"application/json"}`,
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
			Headers:   `{"Content-Type":"application/json"}`,
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
