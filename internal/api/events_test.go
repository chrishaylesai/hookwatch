package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chrishaylesai/hookwatch/internal/hub"
	"github.com/chrishaylesai/hookwatch/internal/models"
	"github.com/go-chi/chi/v5"
)

func TestEventsEndpointStreamsRequestCreated(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	eventHub := hub.New()
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 15, 0, 0, 0, time.UTC)
	tokenID := "550e8400-e29b-41d4-a716-446655440020"
	token := &models.Token{
		UUID:               tokenID,
		ReceiveMode:        receiveModePublic,
		ViewMode:           viewModePublic,
		DefaultStatus:      http.StatusOK,
		DefaultContent:     "ok",
		DefaultContentType: "text/plain",
		Timeout:            0,
		CORS:               false,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := db.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	streamHandler := newEventHandler(db, eventHub)
	streamCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	streamReq := httptest.NewRequest(http.MethodGet, "/api/tokens/"+tokenID+"/events", nil).WithContext(streamCtx)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("tokenId", tokenID)
	streamReq = streamReq.WithContext(context.WithValue(streamReq.Context(), chi.RouteCtxKey, routeCtx))
	streamRec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		streamHandler.stream(streamRec, streamReq)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)

	router := NewRouter(db, eventHub, authModeNone, nil)
	captureReq := httptest.NewRequest(http.MethodPost, "/"+tokenID+"/incoming", strings.NewReader(`{"ok":true}`))
	captureReq.Header.Set("Content-Type", "application/json")
	captureRec := httptest.NewRecorder()
	router.ServeHTTP(captureRec, captureReq)
	if captureRec.Code != http.StatusOK {
		t.Fatalf("capture status = %d, want %d", captureRec.Code, http.StatusOK)
	}

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	body := streamRec.Body.String()
	if !strings.Contains(body, "event: request.created") {
		t.Fatalf("stream body missing request.created event: %q", body)
	}
	if !strings.Contains(body, `"total":1`) {
		t.Fatalf("stream body missing total count: %q", body)
	}
	if !strings.Contains(body, `"token_id":"`+tokenID+`"`) {
		t.Fatalf("stream body missing token id: %q", body)
	}
}

func TestEventsEndpointHidesPrivateViewHooks(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	eventHub := hub.New()
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 15, 0, 0, 0, time.UTC)
	tokenID := "550e8400-e29b-41d4-a716-446655440021"
	token := &models.Token{
		UUID:               tokenID,
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

	router := NewRouter(db, eventHub, authModeNone, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/tokens/"+tokenID+"/events", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestWriteSSEEventFormatsIDTypeAndData(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	payload, err := json.Marshal(map[string]any{"ok": true})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	if err := writeSSEEvent(rec, hub.Event{
		ID:   "123",
		Type: "token.updated",
		Data: payload,
	}); err != nil {
		t.Fatalf("writeSSEEvent: %v", err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "id: 123") {
		t.Fatalf("body missing id: %q", body)
	}
	if !strings.Contains(body, "event: token.updated") {
		t.Fatalf("body missing type: %q", body)
	}
	if !strings.Contains(body, `data: {"ok":true}`) {
		t.Fatalf("body missing payload: %q", body)
	}
}
