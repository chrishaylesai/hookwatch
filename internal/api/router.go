package api

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/chrishaylesai/hookwatch"
	"github.com/chrishaylesai/hookwatch/internal/hub"
	"github.com/chrishaylesai/hookwatch/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter creates the HTTP router with all routes.
func NewRouter(db *store.Store, eventHub *hub.Hub) http.Handler {
	r := chi.NewRouter()
	tokenHandler := newTokenHandler(db)

	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// API routes
	r.Route("/api", func(r chi.Router) {
		r.Route("/tokens", func(r chi.Router) {
			r.Post("/", tokenHandler.createToken)
			r.Get("/", tokenHandler.listTokens)
			r.Get("/{tokenId}", tokenHandler.getToken)
			r.Put("/{tokenId}", tokenHandler.updateToken)
			r.Delete("/{tokenId}", tokenHandler.deleteToken)
		})
	})

	staticFS, err := hookwatch.FrontendFS()
	if err != nil {
		panic(err)
	}

	// Webhook capture - catch-all at root
	// r.HandleFunc("/{tokenId}", captureHandler)
	// r.HandleFunc("/{tokenId}/*", captureHandler)
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
