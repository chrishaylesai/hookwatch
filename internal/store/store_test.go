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

func TestListRequestsByTokenSupportsFiltersAndMissingToken(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 3, 30, 15, 0, 0, 0, time.UTC)

	if err := s.CreateToken(ctx, newToken("token-1", base)); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	postRequest := newRequest("token-1", "request-post", base.Add(time.Minute))
	postRequest.Method = "POST"
	postRequest.IP = "127.0.0.1"
	if err := s.CreateRequest(ctx, postRequest); err != nil {
		t.Fatalf("CreateRequest(post): %v", err)
	}

	getRequest := newRequest("token-1", "request-get", base.Add(2*time.Minute))
	getRequest.Method = "GET"
	getRequest.IP = "203.0.113.10"
	if err := s.CreateRequest(ctx, getRequest); err != nil {
		t.Fatalf("CreateRequest(get): %v", err)
	}

	page, err := s.ListRequestsByToken(ctx, "token-1", RequestListParams{
		Limit:  10,
		Method: "post",
	})
	if err != nil {
		t.Fatalf("ListRequestsByToken method filter: %v", err)
	}
	if page.Total != 1 || len(page.Requests) != 1 || page.Requests[0].UUID != "request-post" {
		t.Fatalf("ListRequestsByToken method filter = %+v, want only request-post", page.Requests)
	}

	page, err = s.ListRequestsByToken(ctx, "token-1", RequestListParams{
		Limit: 10,
		IP:    "203.0.113.10",
	})
	if err != nil {
		t.Fatalf("ListRequestsByToken ip filter: %v", err)
	}
	if page.Total != 1 || len(page.Requests) != 1 || page.Requests[0].UUID != "request-get" {
		t.Fatalf("ListRequestsByToken ip filter = %+v, want only request-get", page.Requests)
	}

	_, err = s.ListRequestsByToken(ctx, "missing-token", RequestListParams{Limit: 10})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("ListRequestsByToken missing token err = %v, want ErrNotFound", err)
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

func TestCreateRequestEnforcesTokenQuota(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 30, 15, 0, 0, 0, time.UTC)
	token := newToken("token-quota", now)
	token.MaxRequests = 1

	if err := s.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if err := s.CreateRequest(ctx, newRequest(token.UUID, "request-1", now.Add(time.Minute))); err != nil {
		t.Fatalf("CreateRequest(first): %v", err)
	}

	err := s.CreateRequest(ctx, newRequest(token.UUID, "request-2", now.Add(2*time.Minute)))
	if !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("CreateRequest(second) err = %v, want ErrQuotaExceeded", err)
	}

	total, err := s.CountRequestsByToken(ctx, token.UUID)
	if err != nil {
		t.Fatalf("CountRequestsByToken: %v", err)
	}
	if total != 1 {
		t.Fatalf("total requests = %d, want 1", total)
	}
}

func TestCreateTokenSetsDefaultExpiry(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 30, 15, 0, 0, 0, time.UTC)
	token := newToken("token-expiry", now)

	if err := s.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	stored, err := s.GetToken(ctx, token.UUID)
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}

	want := now.Add(DefaultTokenTTL)
	if !stored.ExpiresAt.Equal(want) {
		t.Fatalf("expires_at = %v, want %v", stored.ExpiresAt, want)
	}
}

func TestCreateTokenUsesConfiguredStoreDefaults(t *testing.T) {
	t.Parallel()

	cfg := Config{
		TokenTTL:    2 * time.Hour,
		MaxRequests: 25,
	}
	s, err := Open(t.TempDir(), cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	})

	ctx := context.Background()
	now := time.Date(2026, 3, 30, 15, 0, 0, 0, time.UTC)
	token := &models.Token{
		UUID:               "token-configured-defaults",
		ReceiveMode:        "public",
		ViewMode:           "public",
		DefaultStatus:      200,
		DefaultContent:     "",
		DefaultContentType: "text/plain",
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := s.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	stored, err := s.GetToken(ctx, token.UUID)
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if stored.MaxRequests != 25 {
		t.Fatalf("max_requests = %d, want 25", stored.MaxRequests)
	}
	if !stored.ExpiresAt.Equal(now.Add(2 * time.Hour)) {
		t.Fatalf("expires_at = %v, want %v", stored.ExpiresAt, now.Add(2*time.Hour))
	}
}

func TestDeleteExpiredTokensRemovesOnlyExpiredTokens(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 15, 0, 0, 0, time.UTC)

	expiredToken := newToken("token-expired", now.Add(-8*24*time.Hour))
	expiredToken.ExpiresAt = now.Add(-time.Minute)
	activeToken := newToken("token-active", now)
	activeToken.ExpiresAt = now.Add(time.Hour)

	if err := s.CreateToken(ctx, expiredToken); err != nil {
		t.Fatalf("CreateToken(expired): %v", err)
	}
	if err := s.CreateToken(ctx, activeToken); err != nil {
		t.Fatalf("CreateToken(active): %v", err)
	}
	if err := s.CreateRequest(ctx, newRequest(expiredToken.UUID, "request-expired", now)); err != nil {
		t.Fatalf("CreateRequest(expired): %v", err)
	}

	deleted, err := s.DeleteExpiredTokens(ctx, now)
	if err != nil {
		t.Fatalf("DeleteExpiredTokens: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}

	_, err = s.GetToken(ctx, expiredToken.UUID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetToken(expired) err = %v, want ErrNotFound", err)
	}
	_, err = s.GetRequest(ctx, expiredToken.UUID, "request-expired")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetRequest(expired) err = %v, want ErrNotFound", err)
	}
	_, err = s.GetToken(ctx, activeToken.UUID)
	if err != nil {
		t.Fatalf("GetToken(active): %v", err)
	}
}

func TestRunTokenCleanupDeletesExpiredTokens(t *testing.T) {
	s := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	now := time.Date(2026, 3, 31, 15, 0, 0, 0, time.UTC)
	expiredToken := newToken("token-cleanup", now.Add(-8*24*time.Hour))
	expiredToken.ExpiresAt = time.Now().UTC().Add(-time.Minute)
	if err := s.CreateToken(ctx, expiredToken); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	done := make(chan struct{})
	go func() {
		s.RunTokenCleanup(ctx, 10*time.Millisecond, nil)
		close(done)
	}()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := s.GetToken(context.Background(), expiredToken.UUID); errors.Is(err, ErrNotFound) {
			cancel()
			<-done
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	<-done
	t.Fatal("expired token was not cleaned up")
}

func newTestStore(t *testing.T) *Store {
	t.Helper()

	s, err := Open(t.TempDir(), Config{})
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
		MaxRequests:        DefaultMaxRequests,
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
