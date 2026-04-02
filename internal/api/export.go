package api

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/chrishaylesai/hookwatch/internal/authz"
	"github.com/chrishaylesai/hookwatch/internal/models"
	"github.com/chrishaylesai/hookwatch/internal/store"
	"github.com/go-chi/chi/v5"
)

const maxExportContentLength = 10 * 1024 // 10KB content truncation for CSV

type exportHandler struct {
	store  *store.Store
	policy *authz.Policy
}

func newExportHandler(db *store.Store, policy *authz.Policy) *exportHandler {
	return &exportHandler{store: db, policy: policy}
}

func (h *exportHandler) exportCSV(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "tokenId")

	token, err := loadActiveToken(r.Context(), h.store, tokenID, false)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		if isTokenExpiredError(err) {
			writeTokenExpired(w)
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get token")
		return
	}
	if !h.policy.CanAccessToken(r.Context(), token, authz.ActionView) {
		writePrivateViewModeDenied(w)
		return
	}

	params := parseExportFilterParams(r)

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="requests-%s.csv"`, tokenID))

	writer := csv.NewWriter(w)
	defer writer.Flush()

	header := []string{"uuid", "method", "ip", "url", "hostname", "user_agent", "size", "created_at", "content", "headers", "query"}
	if err := writer.Write(header); err != nil {
		return
	}

	_ = h.store.StreamRequestsByToken(r.Context(), tokenID, params, func(req *models.Request) error {
		content := req.Content
		if len(content) > maxExportContentLength {
			content = content[:maxExportContentLength]
		}
		return writer.Write([]string{
			req.UUID,
			req.Method,
			req.IP,
			req.URL,
			req.Hostname,
			req.UserAgent,
			strconv.Itoa(req.Size),
			req.CreatedAt.UTC().Format(time.RFC3339),
			content,
			req.Headers,
			req.Query,
		})
	})
}

func (h *exportHandler) exportJSON(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "tokenId")

	token, err := loadActiveToken(r.Context(), h.store, tokenID, false)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		if isTokenExpiredError(err) {
			writeTokenExpired(w)
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get token")
		return
	}
	if !h.policy.CanAccessToken(r.Context(), token, authz.ActionView) {
		writePrivateViewModeDenied(w)
		return
	}

	params := parseExportFilterParams(r)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="requests-%s.json"`, tokenID))

	enc := json.NewEncoder(w)
	first := true
	_, _ = w.Write([]byte("["))

	_ = h.store.StreamRequestsByToken(r.Context(), tokenID, params, func(req *models.Request) error {
		if !first {
			_, _ = w.Write([]byte(","))
		}
		first = false
		return enc.Encode(toRequestResponse(req))
	})

	_, _ = w.Write([]byte("]"))
}

func parseExportFilterParams(r *http.Request) store.RequestListParams {
	query := r.URL.Query()

	params := store.RequestListParams{
		Method: query.Get("method"),
		IP:     query.Get("ip"),
		Search: query.Get("search"),
	}

	if raw := query.Get("since"); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			params.Since = t
		}
	}
	if raw := query.Get("until"); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			params.Until = t
		}
	}

	return params
}
