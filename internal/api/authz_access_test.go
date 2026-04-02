package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chrishaylesai/hookwatch/internal/auth"
	"github.com/chrishaylesai/hookwatch/internal/hub"
	"github.com/chrishaylesai/hookwatch/internal/models"
	"github.com/chrishaylesai/hookwatch/internal/store"
)

type privateTokenAccessFixture struct {
	db      *store.Store
	router  http.Handler
	token   *models.Token
	request *models.Request
	owner   *models.User
	viewer  *models.User
	editor  *models.User
	admin   *models.User
	other   *models.User
}

func TestCreateTokenAssignsOwnerForAuthenticatedUser(t *testing.T) {
	t.Parallel()

	db := newTestStore(t)
	user := createAPIUser(t, db, "owner-1", "owner@example.com", "user")
	router := NewRouter(db, hub.New(), "local", &fakeAuthService{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/tokens", bytes.NewReader([]byte(`{"view_mode":"private"}`)))
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
	if resp.OwnerID == nil || *resp.OwnerID != user.ID {
		t.Fatalf("owner_id = %#v, want %q", resp.OwnerID, user.ID)
	}
	if resp.ViewMode != viewModePrivate {
		t.Fatalf("view_mode = %q, want %q", resp.ViewMode, viewModePrivate)
	}

	stored, err := db.GetToken(context.Background(), resp.UUID)
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if stored.OwnerID == nil || *stored.OwnerID != user.ID {
		t.Fatalf("stored owner_id = %#v, want %q", stored.OwnerID, user.ID)
	}
}

func TestPrivateViewReadAccessUsesRBAC(t *testing.T) {
	t.Parallel()

	fixture := newPrivateTokenAccessFixture(t)
	endpoints := []string{
		"/api/tokens/" + fixture.token.UUID,
		"/api/tokens/" + fixture.token.UUID + "/requests",
		"/api/tokens/" + fixture.token.UUID + "/requests/" + fixture.request.UUID,
		"/api/tokens/" + fixture.token.UUID + "/requests/" + fixture.request.UUID + "/raw",
	}
	cases := []struct {
		name   string
		user   *models.User
		status int
	}{
		{name: "anonymous", user: nil, status: http.StatusNotFound},
		{name: "other", user: fixture.other, status: http.StatusNotFound},
		{name: "owner", user: fixture.owner, status: http.StatusOK},
		{name: "viewer", user: fixture.viewer, status: http.StatusOK},
		{name: "editor", user: fixture.editor, status: http.StatusOK},
		{name: "admin", user: fixture.admin, status: http.StatusOK},
	}

	for _, endpoint := range endpoints {
		for _, tc := range cases {
			t.Run(endpoint+"_"+tc.name, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, endpoint, nil)
				req = requestWithUser(req, tc.user)
				rec := httptest.NewRecorder()
				fixture.router.ServeHTTP(rec, req)

				if rec.Code != tc.status {
					t.Fatalf("status = %d, want %d", rec.Code, tc.status)
				}
			})
		}
	}
}

func TestPrivateViewEventsUseRBAC(t *testing.T) {
	t.Parallel()

	fixture := newPrivateTokenAccessFixture(t)
	cases := []struct {
		name     string
		user     *models.User
		status   int
		contains string
	}{
		{name: "anonymous", user: nil, status: http.StatusNotFound},
		{name: "other", user: fixture.other, status: http.StatusNotFound},
		{name: "owner", user: fixture.owner, status: http.StatusOK, contains: ": connected"},
		{name: "viewer", user: fixture.viewer, status: http.StatusOK, contains: ": connected"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			req := httptest.NewRequest(http.MethodGet, "/api/tokens/"+fixture.token.UUID+"/events", nil).WithContext(ctx)
			req = requestWithUser(req, tc.user)
			rec := httptest.NewRecorder()

			done := make(chan struct{})
			go func() {
				fixture.router.ServeHTTP(rec, req)
				close(done)
			}()

			time.Sleep(20 * time.Millisecond)
			cancel()
			<-done

			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d", rec.Code, tc.status)
			}
			if tc.contains != "" && !strings.Contains(rec.Body.String(), tc.contains) {
				t.Fatalf("body = %q, want substring %q", rec.Body.String(), tc.contains)
			}
		})
	}
}

func TestPrivateViewMutationAccessUsesRBAC(t *testing.T) {
	t.Parallel()

	t.Run("viewer cannot update token", func(t *testing.T) {
		fixture := newPrivateTokenAccessFixture(t)
		req := httptest.NewRequest(http.MethodPut, "/api/tokens/"+fixture.token.UUID, bytes.NewReader([]byte(`{"timeout":2}`)))
		req = requestWithUser(req, fixture.viewer)
		rec := httptest.NewRecorder()
		fixture.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	t.Run("editor can update token", func(t *testing.T) {
		fixture := newPrivateTokenAccessFixture(t)
		req := httptest.NewRequest(http.MethodPut, "/api/tokens/"+fixture.token.UUID, bytes.NewReader([]byte(`{"timeout":2}`)))
		req = requestWithUser(req, fixture.editor)
		rec := httptest.NewRecorder()
		fixture.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		stored, err := fixture.db.GetToken(context.Background(), fixture.token.UUID)
		if err != nil {
			t.Fatalf("GetToken: %v", err)
		}
		if stored.Timeout != 2 {
			t.Fatalf("timeout = %d, want 2", stored.Timeout)
		}
	})

	t.Run("viewer cannot delete request", func(t *testing.T) {
		fixture := newPrivateTokenAccessFixture(t)
		req := httptest.NewRequest(http.MethodDelete, "/api/tokens/"+fixture.token.UUID+"/requests/"+fixture.request.UUID, nil)
		req = requestWithUser(req, fixture.viewer)
		rec := httptest.NewRecorder()
		fixture.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	t.Run("editor can delete request", func(t *testing.T) {
		fixture := newPrivateTokenAccessFixture(t)
		req := httptest.NewRequest(http.MethodDelete, "/api/tokens/"+fixture.token.UUID+"/requests/"+fixture.request.UUID, nil)
		req = requestWithUser(req, fixture.editor)
		rec := httptest.NewRecorder()
		fixture.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
		}
		if _, err := fixture.db.GetRequest(context.Background(), fixture.token.UUID, fixture.request.UUID); err == nil {
			t.Fatal("expected request to be deleted")
		}
	})

	t.Run("viewer cannot rotate secret", func(t *testing.T) {
		fixture := newPrivateTokenAccessFixture(t)
		req := httptest.NewRequest(http.MethodPost, "/api/tokens/"+fixture.token.UUID+"/rotate-secret", nil)
		req = requestWithUser(req, fixture.viewer)
		rec := httptest.NewRecorder()
		fixture.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	t.Run("editor can rotate secret", func(t *testing.T) {
		fixture := newPrivateTokenAccessFixture(t)
		req := httptest.NewRequest(http.MethodPost, "/api/tokens/"+fixture.token.UUID+"/rotate-secret", nil)
		req = requestWithUser(req, fixture.editor)
		rec := httptest.NewRecorder()
		fixture.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		var resp struct {
			ReceiveSecret *string `json:"receive_secret"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		if resp.ReceiveSecret == nil || *resp.ReceiveSecret == "" {
			t.Fatal("expected rotated receive_secret")
		}
	})

	t.Run("editor cannot delete token", func(t *testing.T) {
		fixture := newPrivateTokenAccessFixture(t)
		req := httptest.NewRequest(http.MethodDelete, "/api/tokens/"+fixture.token.UUID, nil)
		req = requestWithUser(req, fixture.editor)
		rec := httptest.NewRecorder()
		fixture.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	t.Run("admin can delete token", func(t *testing.T) {
		fixture := newPrivateTokenAccessFixture(t)
		req := httptest.NewRequest(http.MethodDelete, "/api/tokens/"+fixture.token.UUID, nil)
		req = requestWithUser(req, fixture.admin)
		rec := httptest.NewRecorder()
		fixture.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
		}
		if _, err := fixture.db.GetToken(context.Background(), fixture.token.UUID); err == nil {
			t.Fatal("expected token to be deleted")
		}
	})
}

func newPrivateTokenAccessFixture(t *testing.T) *privateTokenAccessFixture {
	t.Helper()

	db := newTestStore(t)
	now := time.Now().UTC()
	owner := createAPIUser(t, db, "owner-1", "owner@example.com", "user")
	viewer := createAPIUser(t, db, "viewer-1", "viewer@example.com", "user")
	editor := createAPIUser(t, db, "editor-1", "editor@example.com", "user")
	admin := createAPIUser(t, db, "admin-1", "admin@example.com", "admin")
	other := createAPIUser(t, db, "other-1", "other@example.com", "user")

	secret := "secret-1234"
	hash := hashReceiveSecret(secret)
	prefix := secret[:4]
	token := &models.Token{
		UUID:                "550e8400-e29b-41d4-a716-446655440111",
		OwnerID:             &owner.ID,
		ReceiveMode:         receiveModePrivate,
		ViewMode:            viewModePrivate,
		ReceiveSecretHash:   &hash,
		ReceiveSecretPrefix: &prefix,
		DefaultStatus:       http.StatusOK,
		DefaultContent:      "ok",
		DefaultContentType:  "text/plain",
		MaxRequests:         100,
		Timeout:             0,
		CORS:                false,
		CreatedAt:           now,
		UpdatedAt:           now,
		ExpiresAt:           now.Add(24 * time.Hour),
	}
	if err := db.CreateToken(context.Background(), token); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	request := &models.Request{
		UUID:      "request-1",
		TokenID:   token.UUID,
		IP:        "127.0.0.1",
		Hostname:  "example.test",
		Method:    "POST",
		UserAgent: "curl/8.0.0",
		Content:   `{"event":"push"}`,
		Query:     "foo=bar",
		Headers:   `{"Content-Type":"application/json"}`,
		FormData:  `{"event":"push"}`,
		URL:       "https://example.test/hook",
		CreatedAt: now.Add(time.Minute),
	}
	if err := db.CreateRequest(context.Background(), request); err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}

	if err := db.CreateHookGrant(context.Background(), &models.HookGrant{
		ID:        "grant-viewer",
		TokenID:   token.UUID,
		UserID:    viewer.ID,
		Role:      "viewer",
		GrantedBy: owner.ID,
		CreatedAt: now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("CreateHookGrant(viewer): %v", err)
	}
	if err := db.CreateHookGrant(context.Background(), &models.HookGrant{
		ID:        "grant-editor",
		TokenID:   token.UUID,
		UserID:    editor.ID,
		Role:      "editor",
		GrantedBy: owner.ID,
		CreatedAt: now.Add(3 * time.Minute),
	}); err != nil {
		t.Fatalf("CreateHookGrant(editor): %v", err)
	}

	return &privateTokenAccessFixture{
		db:      db,
		router:  NewRouter(db, hub.New(), "local", &fakeAuthService{}, nil),
		token:   token,
		request: request,
		owner:   owner,
		viewer:  viewer,
		editor:  editor,
		admin:   admin,
		other:   other,
	}
}

func createAPIUser(t *testing.T, db *store.Store, id, email, role string) *models.User {
	t.Helper()

	now := time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC)
	user := &models.User{
		ID:          id,
		Email:       email,
		DisplayName: email,
		GlobalRole:  role,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := db.CreateUser(context.Background(), user); err != nil {
		t.Fatalf("CreateUser(%s): %v", id, err)
	}
	return user
}

func requestWithUser(req *http.Request, user *models.User) *http.Request {
	if user == nil {
		return req
	}
	return req.WithContext(auth.ContextWithUser(req.Context(), user))
}
