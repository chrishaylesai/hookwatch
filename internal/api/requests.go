package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chrishaylesai/hookwatch/internal/authz"
	"github.com/chrishaylesai/hookwatch/internal/models"
	"github.com/chrishaylesai/hookwatch/internal/store"
	"github.com/go-chi/chi/v5"
)

type requestHandler struct {
	store  *store.Store
	policy *authz.Policy
}

type requestResponse struct {
	UUID      string         `json:"uuid"`
	TokenID   string         `json:"token_id"`
	IP        string         `json:"ip"`
	Hostname  string         `json:"hostname"`
	Method    string         `json:"method"`
	UserAgent string         `json:"user_agent"`
	Content   string         `json:"content"`
	Query     string         `json:"query"`
	Headers   map[string]any `json:"headers"`
	FormData  map[string]any `json:"form_data"`
	URL       string         `json:"url"`
	CreatedAt string         `json:"created_at"`
}

type requestListResponse struct {
	Data       []requestResponse `json:"data"`
	Total      int               `json:"total"`
	Page       int               `json:"page"`
	PerPage    int               `json:"per_page"`
	TotalPages int               `json:"total_pages"`
}

type requestListQuery struct {
	Page    int
	PerPage int
	Limit   int
	Offset  int
	SortBy  string
	Order   string
	Method  string
	IP      string
}

func newRequestHandler(db *store.Store, policy *authz.Policy) *requestHandler {
	return &requestHandler{store: db, policy: policy}
}

func (h *requestHandler) listRequests(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "tokenId")

	params, err := parseRequestListQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

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
	if err := refreshTokenExpiry(r.Context(), h.store, token); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to refresh token expiry")
		return
	}

	page, err := h.store.ListRequestsByToken(r.Context(), tokenID, store.RequestListParams{
		Limit:  params.Limit,
		Offset: params.Offset,
		SortBy: params.SortBy,
		Order:  params.Order,
		Method: params.Method,
		IP:     params.IP,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to list requests")
		return
	}

	data := make([]requestResponse, 0, len(page.Requests))
	for _, req := range page.Requests {
		data = append(data, toRequestResponse(req))
	}

	writeJSON(w, http.StatusOK, requestListResponse{
		Data:       data,
		Total:      page.Total,
		Page:       params.Page,
		PerPage:    params.PerPage,
		TotalPages: totalPages(page.Total, params.PerPage),
	})
}

func (h *requestHandler) getRequest(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "tokenId")
	requestID := chi.URLParam(r, "requestId")

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
	if err := refreshTokenExpiry(r.Context(), h.store, token); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to refresh token expiry")
		return
	}

	req, err := h.store.GetRequest(r.Context(), tokenID, requestID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "request not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get request")
		return
	}

	writeJSON(w, http.StatusOK, toRequestResponse(req))
}

func (h *requestHandler) getRawRequest(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "tokenId")
	requestID := chi.URLParam(r, "requestId")

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
	if err := refreshTokenExpiry(r.Context(), h.store, token); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to refresh token expiry")
		return
	}

	req, err := h.store.GetRequest(r.Context(), tokenID, requestID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "request not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get request")
		return
	}

	w.Header().Set("Content-Type", requestContentType(req.Headers))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(req.Content))
}

func (h *requestHandler) deleteRequest(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "tokenId")
	requestID := chi.URLParam(r, "requestId")

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
	if !h.policy.CanAccessToken(r.Context(), token, authz.ActionEdit) {
		writeTokenPermissionDenied(w)
		return
	}
	if err := refreshTokenExpiry(r.Context(), h.store, token); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to refresh token expiry")
		return
	}

	if err := h.store.DeleteRequest(r.Context(), tokenID, requestID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "request not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete request")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *requestHandler) deleteAllRequests(w http.ResponseWriter, r *http.Request) {
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
	if !h.policy.CanAccessToken(r.Context(), token, authz.ActionEdit) {
		writeTokenPermissionDenied(w)
		return
	}
	if err := refreshTokenExpiry(r.Context(), h.store, token); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to refresh token expiry")
		return
	}

	if err := h.store.DeleteAllRequestsByToken(r.Context(), tokenID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete requests")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func parseRequestListQuery(r *http.Request) (requestListQuery, error) {
	query := r.URL.Query()

	perPage := 50
	if raw := strings.TrimSpace(firstNonEmpty(query.Get("per_page"), query.Get("limit"))); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return requestListQuery{}, errors.New("invalid per_page")
		}
		if value < 0 {
			return requestListQuery{}, errors.New("invalid per_page")
		}
		if value == 0 {
			value = 50
		}
		if value > 100 {
			value = 100
		}
		perPage = value
	}

	page := 1
	if raw := strings.TrimSpace(query.Get("page")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return requestListQuery{}, errors.New("invalid page")
		}
		if value <= 0 {
			return requestListQuery{}, errors.New("invalid page")
		}
		page = value
	}

	offset := (page - 1) * perPage
	if raw := strings.TrimSpace(query.Get("offset")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return requestListQuery{}, errors.New("invalid offset")
		}
		if value < 0 {
			return requestListQuery{}, errors.New("invalid offset")
		}
		offset = value
		page = (offset / perPage) + 1
	}

	sortBy := strings.TrimSpace(firstNonEmpty(query.Get("sort"), query.Get("sort_by")))
	if sortBy == "" {
		sortBy = "created_at"
	}

	order := strings.TrimSpace(query.Get("order"))
	if order == "" {
		order = "desc"
	}

	return requestListQuery{
		Page:    page,
		PerPage: perPage,
		Limit:   perPage,
		Offset:  offset,
		SortBy:  sortBy,
		Order:   order,
		Method:  strings.TrimSpace(query.Get("method")),
		IP:      strings.TrimSpace(query.Get("ip")),
	}, nil
}

func toRequestResponse(req *models.Request) requestResponse {
	return requestResponse{
		UUID:      req.UUID,
		TokenID:   req.TokenID,
		IP:        req.IP,
		Hostname:  req.Hostname,
		Method:    req.Method,
		UserAgent: req.UserAgent,
		Content:   req.Content,
		Query:     req.Query,
		Headers:   decodeJSONMap(req.Headers),
		FormData:  decodeJSONMap(req.FormData),
		URL:       req.URL,
		CreatedAt: req.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func decodeJSONMap(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil || decoded == nil {
		return map[string]any{}
	}

	return decoded
}

func requestContentType(rawHeaders string) string {
	headers := decodeJSONMap(rawHeaders)
	for _, key := range []string{"Content-Type", "content-type"} {
		value, ok := headers[key]
		if !ok {
			continue
		}
		if contentType, ok := value.(string); ok && strings.TrimSpace(contentType) != "" {
			return contentType
		}
	}

	return "application/octet-stream"
}

func totalPages(total, perPage int) int {
	if total <= 0 || perPage <= 0 {
		return 0
	}
	return (total + perPage - 1) / perPage
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
