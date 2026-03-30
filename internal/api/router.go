package api

import (
	"net/http"

	"github.com/chrishaylesai/hookwatch/internal/hub"
	"github.com/chrishaylesai/hookwatch/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter creates the HTTP router with all routes.
func NewRouter(db *store.Store, eventHub *hub.Hub) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// API routes
	r.Route("/api", func(r chi.Router) {
		r.Route("/tokens", func(r chi.Router) {
			// Token CRUD - to be implemented
		})
	})

	// Webhook capture - catch-all at root
	// r.HandleFunc("/{tokenId}", captureHandler)
	// r.HandleFunc("/{tokenId}/*", captureHandler)

	return r
}
