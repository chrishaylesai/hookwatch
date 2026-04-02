package auth

import (
	"context"
	"net/http"

	"github.com/chrishaylesai/hookwatch/internal/models"
)

type sessionValidator interface {
	ValidateSession(ctx context.Context, sessionID string) (*models.User, error)
}

// SessionMiddleware extracts the session cookie, validates it, and injects
// the authenticated user into the request context. If no valid session is
// found, the request proceeds with a nil user (unauthenticated).
func (s *Service) SessionMiddleware(next http.Handler) http.Handler {
	return sessionMiddleware(s, next)
}

// SessionMiddleware extracts the session cookie, validates it, and injects
// the authenticated user into the request context. If no valid session is
// found, the request proceeds with a nil user (unauthenticated).
func (s *OIDCService) SessionMiddleware(next http.Handler) http.Handler {
	return sessionMiddleware(s, next)
}

func sessionMiddleware(validator sessionValidator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(SessionCookieName)
		if err != nil || cookie.Value == "" {
			next.ServeHTTP(w, r)
			return
		}

		user, err := validator.ValidateSession(r.Context(), cookie.Value)
		if err != nil {
			// Invalid/expired session — proceed as unauthenticated
			next.ServeHTTP(w, r)
			return
		}

		ctx := ContextWithUser(r.Context(), user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAuth is middleware that rejects unauthenticated requests with 401.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if UserFromContext(r.Context()) == nil {
			http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAdmin is middleware that rejects non-admin users with 403.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		if user == nil {
			http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
			return
		}
		if user.GlobalRole != "admin" {
			http.Error(w, `{"error":"admin access required"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
