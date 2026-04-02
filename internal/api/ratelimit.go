package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/chrishaylesai/hookwatch/internal/store"
	"github.com/go-chi/chi/v5"
)

const rateLimitWindow = time.Minute

func rateLimitMiddleware(db *store.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenID := chi.URLParam(r, "tokenId")
			if tokenID == "" {
				next.ServeHTTP(w, r)
				return
			}

			token, err := db.GetToken(r.Context(), tokenID)
			if err != nil || token.RateLimit <= 0 {
				next.ServeHTTP(w, r)
				return
			}

			ip := r.RemoteAddr
			if fwd := r.Header.Get("X-Real-Ip"); fwd != "" {
				ip = fwd
			}

			windowStart := time.Now().UTC().Truncate(rateLimitWindow)
			count, err := db.IncrementRateLimit(r.Context(), ip, tokenID, windowStart)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			remaining := token.RateLimit - count
			if remaining < 0 {
				remaining = 0
			}
			reset := windowStart.Add(rateLimitWindow)

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(token.RateLimit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", reset.Unix()))

			if count > token.RateLimit {
				writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
