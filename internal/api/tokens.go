package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chrishaylesai/hookwatch/internal/models"
	"github.com/chrishaylesai/hookwatch/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type tokenHandler struct {
	store *store.Store
}

type tokenPayload struct {
	DefaultStatus      *int    `json:"default_status"`
	DefaultContentType *string `json:"default_content_type"`
	DefaultBody        *string `json:"default_body"`
	Timeout            *int    `json:"timeout"`
	CORSEnabled        *bool   `json:"cors_enabled"`
	ReceiveMode        *string `json:"receive_mode"`
	ViewMode           *string `json:"view_mode"`
}

type tokenResponse struct {
	UUID                string    `json:"uuid"`
	OwnerID             *string   `json:"owner_id,omitempty"`
	ReceiveMode         string    `json:"receive_mode"`
	ViewMode            string    `json:"view_mode"`
	ReceiveSecretPrefix *string   `json:"receive_secret_prefix,omitempty"`
	DefaultStatus       int       `json:"default_status"`
	DefaultContentType  string    `json:"default_content_type"`
	DefaultBody         string    `json:"default_body"`
	Timeout             int       `json:"timeout"`
	CORSEnabled         bool      `json:"cors_enabled"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type tokenListResponse struct {
	Data   []tokenResponse `json:"data"`
	Total  int             `json:"total"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func newTokenHandler(db *store.Store) *tokenHandler {
	return &tokenHandler{store: db}
}

func (h *tokenHandler) createToken(w http.ResponseWriter, r *http.Request) {
	var payload tokenPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	now := time.Now().UTC()
	token := &models.Token{
		UUID:               uuid.NewString(),
		ReceiveMode:        "public",
		ViewMode:           "public",
		DefaultStatus:      http.StatusOK,
		DefaultContent:     "",
		DefaultContentType: "text/plain",
		Timeout:            0,
		CORS:               false,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	applyTokenPayload(token, payload)

	if err := h.store.CreateToken(r.Context(), token); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create token")
		return
	}

	writeJSON(w, http.StatusCreated, toTokenResponse(token))
}

func (h *tokenHandler) listTokens(w http.ResponseWriter, r *http.Request) {
	params, err := parseTokenListParams(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	page, err := h.store.ListTokens(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tokens")
		return
	}

	data := make([]tokenResponse, 0, len(page.Tokens))
	for _, token := range page.Tokens {
		data = append(data, toTokenResponse(token))
	}

	writeJSON(w, http.StatusOK, tokenListResponse{
		Data:   data,
		Total:  page.Total,
		Limit:  page.Limit,
		Offset: page.Offset,
	})
}

func (h *tokenHandler) getToken(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "tokenId")

	token, err := h.store.GetToken(r.Context(), tokenID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get token")
		return
	}

	writeJSON(w, http.StatusOK, toTokenResponse(token))
}

func (h *tokenHandler) updateToken(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "tokenId")

	token, err := h.store.GetToken(r.Context(), tokenID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get token")
		return
	}

	var payload tokenPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	applyTokenPayload(token, payload)
	token.UpdatedAt = time.Now().UTC()

	if err := h.store.UpdateToken(r.Context(), token); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update token")
		return
	}

	writeJSON(w, http.StatusOK, toTokenResponse(token))
}

func (h *tokenHandler) deleteToken(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "tokenId")

	if err := h.store.DeleteToken(r.Context(), tokenID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete token")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func applyTokenPayload(token *models.Token, payload tokenPayload) {
	if payload.DefaultStatus != nil {
		token.DefaultStatus = *payload.DefaultStatus
	}
	if payload.DefaultContentType != nil {
		token.DefaultContentType = *payload.DefaultContentType
	}
	if payload.DefaultBody != nil {
		token.DefaultContent = *payload.DefaultBody
	}
	if payload.Timeout != nil {
		token.Timeout = *payload.Timeout
	}
	if payload.CORSEnabled != nil {
		token.CORS = *payload.CORSEnabled
	}
	if payload.ReceiveMode != nil {
		token.ReceiveMode = *payload.ReceiveMode
	}
	if payload.ViewMode != nil {
		token.ViewMode = *payload.ViewMode
	}
}

func parseTokenListParams(r *http.Request) (store.TokenListParams, error) {
	query := r.URL.Query()

	limit := 20
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return store.TokenListParams{}, errors.New("invalid limit")
		}
		if value < 0 {
			return store.TokenListParams{}, errors.New("invalid limit")
		}
		if value == 0 {
			value = 20
		}
		if value > 100 {
			value = 100
		}
		limit = value
	}

	offset := 0
	if raw := strings.TrimSpace(query.Get("offset")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return store.TokenListParams{}, errors.New("invalid offset")
		}
		if value < 0 {
			return store.TokenListParams{}, errors.New("invalid offset")
		}
		offset = value
	}

	sortBy := strings.TrimSpace(query.Get("sort_by"))
	if sortBy == "" {
		sortBy = "created_at"
	}

	order := strings.TrimSpace(query.Get("order"))
	if order == "" {
		order = "desc"
	}

	return store.TokenListParams{
		Limit:  limit,
		Offset: offset,
		SortBy: sortBy,
		Order:  order,
	}, nil
}

func toTokenResponse(token *models.Token) tokenResponse {
	return tokenResponse{
		UUID:                token.UUID,
		OwnerID:             token.OwnerID,
		ReceiveMode:         token.ReceiveMode,
		ViewMode:            token.ViewMode,
		ReceiveSecretPrefix: token.ReceiveSecretPrefix,
		DefaultStatus:       token.DefaultStatus,
		DefaultContentType:  token.DefaultContentType,
		DefaultBody:         token.DefaultContent,
		Timeout:             token.Timeout,
		CORSEnabled:         token.CORS,
		CreatedAt:           token.CreatedAt,
		UpdatedAt:           token.UpdatedAt,
	}
}

func decodeJSON(r *http.Request, dst any) error {
	if r.Body == nil || r.Body == http.NoBody {
		return nil
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}

	var extra any
	if err := decoder.Decode(&extra); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}

	return errors.New("request body must contain a single JSON object")
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(payload); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("{\"error\":\"failed to encode response\"}\n"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
