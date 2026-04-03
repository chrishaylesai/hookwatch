package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chrishaylesai/hookwatch/internal/auth"
	"github.com/chrishaylesai/hookwatch/internal/authz"
	"github.com/chrishaylesai/hookwatch/internal/hub"
	"github.com/chrishaylesai/hookwatch/internal/models"
	"github.com/chrishaylesai/hookwatch/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const maxResponseTimeoutSeconds = 10

type tokenHandler struct {
	store    *store.Store
	eventHub *hub.Hub
	authMode string
	policy   *authz.Policy
}

type tokenPayload struct {
	DefaultStatus      *int    `json:"default_status"`
	DefaultContentType *string `json:"default_content_type"`
	DefaultContent     *string `json:"default_content"`
	LegacyDefaultBody  *string `json:"default_body"`
	MaxRequests        *int    `json:"max_requests"`
	Timeout            *int    `json:"timeout"`
	CORS               *bool   `json:"cors"`
	LegacyCORSEnabled  *bool   `json:"cors_enabled"`
	ReceiveMode        *string `json:"receive_mode"`
	ViewMode           *string `json:"view_mode"`
	Persistent         *bool   `json:"persistent"`
	RateLimit          *int    `json:"rate_limit"`
	SignatureProvider  *string `json:"signature_provider"`
	SignatureSecret    *string `json:"signature_secret"`
}

type tokenResponse struct {
	UUID                string    `json:"uuid"`
	OwnerID             *string   `json:"owner_id,omitempty"`
	ReceiveMode         string    `json:"receive_mode"`
	ReceiveSecret       *string   `json:"receive_secret,omitempty"`
	ViewMode            string    `json:"view_mode"`
	Persistent          bool      `json:"persistent"`
	ReceiveSecretPrefix *string   `json:"receive_secret_prefix,omitempty"`
	SignatureProvider   string    `json:"signature_provider,omitempty"`
	SignatureConfigured bool      `json:"signature_secret_configured"`
	DefaultStatus       int       `json:"default_status"`
	DefaultContentType  string    `json:"default_content_type"`
	DefaultContent      string    `json:"default_content"`
	MaxRequests         int       `json:"max_requests"`
	Timeout             int       `json:"timeout"`
	CORS                bool      `json:"cors"`
	RateLimit           int       `json:"rate_limit"`
	CanDelete           bool      `json:"can_delete"`
	AccessRole          string    `json:"access_role,omitempty"`
	OwnerDisplay        string    `json:"owner_display,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	ExpiresAt           time.Time `json:"expires_at"`
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

func newTokenHandler(db *store.Store, eventHub *hub.Hub, authMode string, policy *authz.Policy) *tokenHandler {
	return &tokenHandler{
		store:    db,
		eventHub: eventHub,
		authMode: normalizedAuthMode(authMode),
		policy:   policy,
	}
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
		Persistent:         false,
		SignatureProvider:  "",
		DefaultStatus:      http.StatusOK,
		DefaultContent:     "",
		DefaultContentType: "text/plain",
		MaxRequests:        store.DefaultMaxRequests,
		Timeout:            0,
		CORS:               false,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	applyTokenPayload(token, payload)
	if user := auth.UserFromContext(r.Context()); user != nil {
		token.OwnerID = &user.ID
	}
	if err := validateAndNormalizeTokenAccess(token, h.authMode); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.validateAdvancedTokenConfig(r.Context(), token, payload.Persistent != nil); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateTokenConfig(token); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	receiveSecret, err := reconcileReceiveSecret(token, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate receive secret")
		return
	}

	if err := h.store.CreateToken(r.Context(), token); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create token")
		return
	}

	writeJSON(w, http.StatusCreated, toTokenResponse(token, receiveSecret))
}

func (h *tokenHandler) listTokens(w http.ResponseWriter, r *http.Request) {
	params, err := parseTokenListParams(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if h.authMode == authModeNone {
		page, listErr := h.store.ListTokens(r.Context(), params)
		if listErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to list tokens")
			return
		}

		data := make([]tokenResponse, 0, len(page.Tokens))
		for _, token := range page.Tokens {
			data = append(data, toListedTokenResponse(token, true, "", ""))
		}

		writeJSON(w, http.StatusOK, tokenListResponse{
			Data:   data,
			Total:  page.Total,
			Limit:  page.Limit,
			Offset: page.Offset,
		})
		return
	}

	user := auth.UserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	page, err := h.store.ListTokensForUser(r.Context(), user.ID, params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tokens")
		return
	}

	data := make([]tokenResponse, 0, len(page.Items))
	for _, item := range page.Items {
		canDelete := item.Token.OwnerID != nil && *item.Token.OwnerID == user.ID
		data = append(data, toListedTokenResponse(item.Token, canDelete, item.AccessRole, item.OwnerDisplay))
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

	writeJSON(w, http.StatusOK, toTokenResponse(token, nil))
}

func (h *tokenHandler) updateToken(w http.ResponseWriter, r *http.Request) {
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

	var payload tokenPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	previouslyPrivate := token.ReceiveMode == receiveModePrivate
	applyTokenPayload(token, payload)
	if err := validateAndNormalizeTokenAccess(token, h.authMode); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.validateAdvancedTokenConfig(r.Context(), token, payload.Persistent != nil); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateTokenConfig(token); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	receiveSecret, err := reconcileReceiveSecret(token, previouslyPrivate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate receive secret")
		return
	}
	now := timeNow().UTC()
	token.UpdatedAt = now
	if !token.Persistent {
		token.ExpiresAt = now.Add(store.DefaultTokenTTL)
	} else {
		token.ExpiresAt = time.Time{}
	}

	if err := h.store.UpdateToken(r.Context(), token); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update token")
		return
	}

	publishTokenUpdated(h.eventHub, token)
	writeJSON(w, http.StatusOK, toTokenResponse(token, receiveSecret))
}

func (h *tokenHandler) deleteToken(w http.ResponseWriter, r *http.Request) {
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
	if !h.policy.CanAccessToken(r.Context(), token, authz.ActionDelete) {
		writeTokenPermissionDenied(w)
		return
	}

	if err := h.store.DeleteToken(r.Context(), tokenID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete token")
		return
	}

	publishTokenDeleted(h.eventHub, tokenID)
	w.WriteHeader(http.StatusNoContent)
}

func (h *tokenHandler) rotateReceiveSecret(w http.ResponseWriter, r *http.Request) {
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

	if token.ReceiveMode != receiveModePrivate {
		writeError(w, http.StatusBadRequest, "receive_mode must be private")
		return
	}

	receiveSecret, err := rotateReceiveSecret(token)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to rotate receive secret")
		return
	}

	now := timeNow().UTC()
	token.UpdatedAt = now
	if !token.Persistent {
		token.ExpiresAt = now.Add(store.DefaultTokenTTL)
	}
	if err := h.store.UpdateToken(r.Context(), token); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update token")
		return
	}

	publishTokenUpdated(h.eventHub, token)
	writeJSON(w, http.StatusOK, map[string]any{
		"receive_secret":        receiveSecret,
		"receive_secret_prefix": token.ReceiveSecretPrefix,
	})
}

func applyTokenPayload(token *models.Token, payload tokenPayload) {
	if payload.DefaultStatus != nil {
		token.DefaultStatus = *payload.DefaultStatus
	}
	if payload.DefaultContentType != nil {
		token.DefaultContentType = *payload.DefaultContentType
	}
	if payload.DefaultContent != nil {
		token.DefaultContent = *payload.DefaultContent
	} else if payload.LegacyDefaultBody != nil {
		token.DefaultContent = *payload.LegacyDefaultBody
	}
	if payload.Timeout != nil {
		token.Timeout = *payload.Timeout
	}
	if payload.MaxRequests != nil {
		token.MaxRequests = *payload.MaxRequests
	}
	if payload.CORS != nil {
		token.CORS = *payload.CORS
	} else if payload.LegacyCORSEnabled != nil {
		token.CORS = *payload.LegacyCORSEnabled
	}
	if payload.ReceiveMode != nil {
		token.ReceiveMode = *payload.ReceiveMode
	}
	if payload.ViewMode != nil {
		token.ViewMode = *payload.ViewMode
	}
	if payload.Persistent != nil {
		token.Persistent = *payload.Persistent
	}
	if payload.RateLimit != nil {
		token.RateLimit = *payload.RateLimit
	}
	if payload.SignatureProvider != nil {
		token.SignatureProvider = strings.TrimSpace(*payload.SignatureProvider)
	}
	if payload.SignatureSecret != nil {
		secret := strings.TrimSpace(*payload.SignatureSecret)
		if secret == "" {
			token.SignatureSecret = nil
		} else {
			token.SignatureSecret = &secret
		}
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

func toTokenResponse(token *models.Token, receiveSecret *string) tokenResponse {
	return tokenResponse{
		UUID:                token.UUID,
		OwnerID:             token.OwnerID,
		ReceiveMode:         token.ReceiveMode,
		ReceiveSecret:       receiveSecret,
		ViewMode:            token.ViewMode,
		Persistent:          token.Persistent,
		ReceiveSecretPrefix: token.ReceiveSecretPrefix,
		SignatureProvider:   token.SignatureProvider,
		SignatureConfigured: token.SignatureSecret != nil && strings.TrimSpace(*token.SignatureSecret) != "",
		DefaultStatus:       token.DefaultStatus,
		DefaultContentType:  token.DefaultContentType,
		DefaultContent:      token.DefaultContent,
		MaxRequests:         token.MaxRequests,
		Timeout:             token.Timeout,
		CORS:                token.CORS,
		RateLimit:           token.RateLimit,
		CreatedAt:           token.CreatedAt,
		UpdatedAt:           token.UpdatedAt,
		ExpiresAt:           token.ExpiresAt,
	}
}

func toListedTokenResponse(token *models.Token, canDelete bool, accessRole, ownerDisplay string) tokenResponse {
	resp := toTokenResponse(token, nil)
	resp.CanDelete = canDelete
	resp.AccessRole = accessRole
	resp.OwnerDisplay = ownerDisplay
	return resp
}

func validateTokenConfig(token *models.Token) error {
	if token.DefaultStatus < 100 || token.DefaultStatus > 999 {
		return errors.New("default_status must be between 100 and 999")
	}
	if strings.TrimSpace(token.DefaultContentType) == "" {
		return errors.New("default_content_type must not be empty")
	}
	if token.MaxRequests < 1 {
		return errors.New("max_requests must be at least 1")
	}
	if token.Timeout < 0 || token.Timeout > maxResponseTimeoutSeconds {
		return errors.New("timeout must be between 0 and 10")
	}
	return nil
}

func (h *tokenHandler) validateAdvancedTokenConfig(ctx context.Context, token *models.Token, persistenceChanged bool) error {
	token.SignatureProvider = strings.ToLower(strings.TrimSpace(token.SignatureProvider))

	switch token.SignatureProvider {
	case "", "github", "stripe":
	default:
		return errors.New("signature_provider must be empty, github, or stripe")
	}

	if token.SignatureProvider == "" {
		token.SignatureSecret = nil
	} else if token.SignatureSecret == nil || strings.TrimSpace(*token.SignatureSecret) == "" {
		return errors.New("signature_secret is required when signature_provider is set")
	}

	if token.Persistent {
		if h.authMode == authModeNone {
			return errors.New("persistent tokens require authentication")
		}
		if token.OwnerID == nil {
			return errors.New("persistent tokens require an authenticated owner")
		}
	}

	if persistenceChanged && !authz.IsOwner(ctx, token) && !authz.IsAdmin(ctx) {
		return errors.New("only the owner or an admin can change persistence")
	}

	return nil
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
