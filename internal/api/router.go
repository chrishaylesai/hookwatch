package api

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/chrishaylesai/hookwatch"
	"github.com/chrishaylesai/hookwatch/internal/auth"
	"github.com/chrishaylesai/hookwatch/internal/authz"
	"github.com/chrishaylesai/hookwatch/internal/hub"
	"github.com/chrishaylesai/hookwatch/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter creates the HTTP router with all routes.
func NewRouter(db *store.Store, eventHub *hub.Hub, authMode string, authService auth.Authenticator) http.Handler {
	r := chi.NewRouter()
	policy := authz.NewPolicy(db, authMode)
	tokenHandler := newTokenHandler(db, eventHub, authMode, policy)
	requestHandler := newRequestHandler(db, policy)
	captureHandler := newCaptureHandler(db, eventHub)
	eventHandler := newEventHandler(db, eventHub, policy)

	authH := newAuthHandler(authService, authMode)
	grantH := newGrantHandler(db, policy)
	adminH := newAdminHandler(db)

	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Inject authenticated user into context when auth is enabled
	if authMode != "none" && authService != nil {
		r.Use(authService.SessionMiddleware)
	}

	// API routes
	r.Route("/api", func(r chi.Router) {
		// Auth info endpoint (always available)
		r.Get("/auth/info", authH.authInfo)

		// Auth routes (only when auth is enabled)
		if authMode == "local" {
			r.Post("/auth/register", authH.register)
			r.Post("/auth/login", authH.login)
			r.Post("/auth/logout", authH.logout)
			r.Get("/auth/me", authH.me)
		} else if authMode == "oidc" {
			r.Get("/auth/oidc/authorize", authH.authorizeOIDC)
			r.Get("/auth/oidc/callback", authH.callbackOIDC)
			r.Post("/auth/logout", authH.logout)
			r.Get("/auth/me", authH.me)
		}

		r.Route("/tokens", func(r chi.Router) {
			r.Post("/", tokenHandler.createToken)
			r.Get("/", tokenHandler.listTokens)
			r.Get("/{tokenId}", tokenHandler.getToken)
			r.Put("/{tokenId}", tokenHandler.updateToken)
			r.Delete("/{tokenId}", tokenHandler.deleteToken)
			r.Post("/{tokenId}/rotate-secret", tokenHandler.rotateReceiveSecret)
			r.Get("/{tokenId}/events", eventHandler.stream)
			r.Get("/{tokenId}/requests", requestHandler.listRequests)
			r.Delete("/{tokenId}/requests", requestHandler.deleteAllRequests)
			r.Get("/{tokenId}/requests/{requestId}", requestHandler.getRequest)
			r.Get("/{tokenId}/requests/{requestId}/raw", requestHandler.getRawRequest)
			r.Delete("/{tokenId}/requests/{requestId}", requestHandler.deleteRequest)

			// Grant management routes
			if authMode != "none" {
				r.Get("/{tokenId}/grants", grantH.listGrants)
				r.Post("/{tokenId}/grants", grantH.createGrant)
				r.Delete("/{tokenId}/grants/{userId}", grantH.deleteGrant)
			}
		})

		// Admin routes (require admin role)
		if authMode != "none" {
			r.Route("/admin", func(r chi.Router) {
				r.Use(auth.RequireAdmin)
				r.Get("/users", adminH.listUsers)
				r.Get("/users/{userId}", adminH.getUser)
				r.Put("/users/{userId}", adminH.updateUser)
				r.Delete("/users/{userId}", adminH.deleteUser)
			})
		}
	})

	staticFS, err := hookwatch.FrontendFS()
	if err != nil {
		panic(err)
	}

	// Webhook capture is restricted to UUID-like first path segments so SPA routes still fall through.
	r.HandleFunc("/{tokenId:[0-9a-fA-F-]{36}}", captureHandler.capture)
	r.HandleFunc("/{tokenId:[0-9a-fA-F-]{36}}/*", captureHandler.capture)
	r.Handle("/*", spaHandler(staticFS))

	return r
}

func spaHandler(staticFS fs.FS) http.Handler {
	fileServer := http.FileServerFS(staticFS)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleanPath := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if cleanPath == "" {
			cleanPath = "index.html"
		}

		if _, err := fs.Stat(staticFS, cleanPath); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		index, err := fs.ReadFile(staticFS, "index.html")
		if err != nil {
			http.Error(w, "frontend build missing index.html", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(index)
	})
}
