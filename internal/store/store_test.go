package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chrishaylesai/hookwatch/internal/models"
)

func TestCreateRequestRequiresExistingToken(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	now := time.Date(2026, 3, 30, 15, 0, 0, 0, time.UTC)

	err := s.CreateRequest(context.Background(), &models.Request{
		UUID:      "request-1",
		TokenID:   "missing-token",
		IP:        "127.0.0.1",
		Hostname:  "example.test",
		Method:    "POST",
		UserAgent: "curl/8.0.0",
		Content:   "{}",
		Query:     "",
		Headers:   "{}",
		FormData:  "{}",
		URL:       "https://example.test/missing-token",
		CreatedAt: now,
	})
	if err == nil {
		t.Fatal("CreateRequest succeeded for a missing token")
	}
}

func TestListTokensAndRequestsProvidePagination(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 3, 30, 15, 0, 0, 0, time.UTC)

	for i := 1; i <= 3; i++ {
		tokenTime := base.Add(time.Duration(i) * time.Minute)
		tokenID := "token-" + string(rune('0'+i))
		if err := s.CreateToken(ctx, newToken(tokenID, tokenTime)); err != nil {
			t.Fatalf("CreateToken(%s): %v", tokenID, err)
		}
	}

	tokenPage, err := s.ListTokens(ctx, TokenListParams{
		Limit:  2,
		Offset: 1,
		SortBy: "created_at",
		Order:  "desc",
	})
	if err != nil {
		t.Fatalf("ListTokens: %v", err)
	}
	if tokenPage.Total != 3 {
		t.Fatalf("ListTokens total = %d, want 3", tokenPage.Total)
	}
	if len(tokenPage.Tokens) != 2 {
		t.Fatalf("ListTokens len = %d, want 2", len(tokenPage.Tokens))
	}
	if got := tokenPage.Tokens[0].UUID; got != "token-2" {
		t.Fatalf("ListTokens[0] = %s, want token-2", got)
	}
	if got := tokenPage.Tokens[1].UUID; got != "token-1" {
		t.Fatalf("ListTokens[1] = %s, want token-1", got)
	}

	for i := 1; i <= 3; i++ {
		reqTime := base.Add(time.Duration(i) * time.Minute)
		requestID := "request-" + string(rune('0'+i))
		if err := s.CreateRequest(ctx, newRequest("token-1", requestID, reqTime)); err != nil {
			t.Fatalf("CreateRequest(%s): %v", requestID, err)
		}
	}

	requestPage, err := s.ListRequestsByToken(ctx, "token-1", RequestListParams{
		Limit:  2,
		Offset: 1,
		SortBy: "created_at",
		Order:  "desc",
	})
	if err != nil {
		t.Fatalf("ListRequestsByToken: %v", err)
	}
	if requestPage.Total != 3 {
		t.Fatalf("ListRequestsByToken total = %d, want 3", requestPage.Total)
	}
	if len(requestPage.Requests) != 2 {
		t.Fatalf("ListRequestsByToken len = %d, want 2", len(requestPage.Requests))
	}
	if got := requestPage.Requests[0].UUID; got != "request-2" {
		t.Fatalf("ListRequestsByToken[0] = %s, want request-2", got)
	}
	if got := requestPage.Requests[1].UUID; got != "request-1" {
		t.Fatalf("ListRequestsByToken[1] = %s, want request-1", got)
	}
}

func TestMissingMutationsReturnErrNotFound(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 30, 15, 0, 0, 0, time.UTC)

	err := s.UpdateToken(ctx, newToken("missing-token", now))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateToken err = %v, want ErrNotFound", err)
	}

	err = s.DeleteToken(ctx, "missing-token")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteToken err = %v, want ErrNotFound", err)
	}

	if err := s.CreateToken(ctx, newToken("token-1", now)); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	err = s.DeleteRequest(ctx, "token-1", "missing-request")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteRequest err = %v, want ErrNotFound", err)
	}

	err = s.DeleteAllRequestsByToken(ctx, "missing-token")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteAllRequestsByToken err = %v, want ErrNotFound", err)
	}
}

func TestDeleteTokenCascadesRequests(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 30, 15, 0, 0, 0, time.UTC)

	if err := s.CreateToken(ctx, newToken("token-1", now)); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if err := s.CreateRequest(ctx, newRequest("token-1", "request-1", now.Add(time.Minute))); err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}

	if err := s.DeleteToken(ctx, "token-1"); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}

	_, err := s.GetToken(ctx, "token-1")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetToken err = %v, want ErrNotFound", err)
	}

	_, err = s.GetRequest(ctx, "token-1", "request-1")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetRequest err = %v, want ErrNotFound", err)
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()

	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	})

	return s
}

func newToken(id string, ts time.Time) *models.Token {
	return &models.Token{
		UUID:               id,
		ReceiveMode:        "public",
		ViewMode:           "public",
		DefaultStatus:      200,
		DefaultContent:     "",
		DefaultContentType: "text/plain",
		Timeout:            0,
		CORS:               false,
		CreatedAt:          ts,
		UpdatedAt:          ts,
	}
}

func newRequest(tokenID, requestID string, ts time.Time) *models.Request {
	return &models.Request{
		UUID:      requestID,
		TokenID:   tokenID,
		IP:        "127.0.0.1",
		Hostname:  "example.test",
		Method:    "POST",
		UserAgent: "curl/8.0.0",
		Content:   `{"ok":true}`,
		Query:     "",
		Headers:   "{}",
		FormData:  "{}",
		URL:       "https://example.test/" + tokenID,
		CreatedAt: ts,
	}
}
