package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/chrishaylesai/hookwatch/internal/authz"
	"github.com/chrishaylesai/hookwatch/internal/models"
	"github.com/chrishaylesai/hookwatch/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type actionHandler struct {
	store  *store.Store
	policy *authz.Policy
}

type actionResponse struct {
	UUID      string          `json:"uuid"`
	TokenID   string          `json:"token_id"`
	Type      string          `json:"type"`
	Config    json.RawMessage `json:"config"`
	SortOrder int             `json:"sort_order"`
	Enabled   bool            `json:"enabled"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}

type actionListResponse struct {
	Data []actionResponse `json:"data"`
}

type createActionPayload struct {
	Type    string          `json:"type"`
	Config  json.RawMessage `json:"config"`
	Enabled *bool           `json:"enabled"`
}

type updateActionPayload struct {
	Type    *string          `json:"type"`
	Config  *json.RawMessage `json:"config"`
	Enabled *bool            `json:"enabled"`
}

type reorderPayload struct {
	ActionIDs []string `json:"action_ids"`
}

func newActionHandler(db *store.Store, policy *authz.Policy) *actionHandler {
	return &actionHandler{store: db, policy: policy}
}

func (h *actionHandler) listActions(w http.ResponseWriter, r *http.Request) {
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

	actions, err := h.store.ListActionsByToken(r.Context(), tokenID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list actions")
		return
	}

	data := make([]actionResponse, 0, len(actions))
	for _, a := range actions {
		data = append(data, toActionResponse(a))
	}

	writeJSON(w, http.StatusOK, actionListResponse{Data: data})
}

func (h *actionHandler) createAction(w http.ResponseWriter, r *http.Request) {
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

	var payload createActionPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if err := validateActionType(payload.Type); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateActionConfig(payload.Type, payload.Config); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	sortOrder, err := h.store.NextActionSortOrder(r.Context(), tokenID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to determine sort order")
		return
	}

	enabled := true
	if payload.Enabled != nil {
		enabled = *payload.Enabled
	}

	now := time.Now().UTC()
	action := &models.Action{
		UUID:      uuid.NewString(),
		TokenID:   tokenID,
		Type:      payload.Type,
		Config:    string(payload.Config),
		SortOrder: sortOrder,
		Enabled:   enabled,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := h.store.CreateAction(r.Context(), action); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create action")
		return
	}

	writeJSON(w, http.StatusCreated, toActionResponse(action))
}

func (h *actionHandler) updateAction(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "tokenId")
	actionID := chi.URLParam(r, "actionId")

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

	action, err := h.store.GetAction(r.Context(), tokenID, actionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "action not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get action")
		return
	}

	var payload updateActionPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if payload.Type != nil {
		if err := validateActionType(*payload.Type); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		action.Type = *payload.Type
	}
	if payload.Config != nil {
		if err := validateActionConfig(action.Type, *payload.Config); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		action.Config = string(*payload.Config)
	}
	if payload.Enabled != nil {
		action.Enabled = *payload.Enabled
	}

	action.UpdatedAt = time.Now().UTC()

	if err := h.store.UpdateAction(r.Context(), action); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update action")
		return
	}

	writeJSON(w, http.StatusOK, toActionResponse(action))
}

func (h *actionHandler) deleteAction(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "tokenId")
	actionID := chi.URLParam(r, "actionId")

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

	if err := h.store.DeleteAction(r.Context(), tokenID, actionID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "action not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete action")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *actionHandler) reorderActions(w http.ResponseWriter, r *http.Request) {
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

	var payload reorderPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(payload.ActionIDs) == 0 {
		writeError(w, http.StatusBadRequest, "action_ids must not be empty")
		return
	}

	if err := h.store.ReorderActions(r.Context(), tokenID, payload.ActionIDs); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reorder actions")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func toActionResponse(a *models.Action) actionResponse {
	return actionResponse{
		UUID:      a.UUID,
		TokenID:   a.TokenID,
		Type:      a.Type,
		Config:    json.RawMessage(a.Config),
		SortOrder: a.SortOrder,
		Enabled:   a.Enabled,
		CreatedAt: a.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: a.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

var validActionTypes = map[string]bool{
	"forward":   true,
	"filter":    true,
	"delay":     true,
	"transform": true,
}

func validateActionType(t string) error {
	if !validActionTypes[t] {
		return errors.New("type must be one of: forward, filter, delay, transform")
	}
	return nil
}

func validateActionConfig(actionType string, config json.RawMessage) error {
	if len(config) == 0 {
		return errors.New("config is required")
	}

	switch actionType {
	case "forward":
		var cfg models.ForwardConfig
		if err := json.Unmarshal(config, &cfg); err != nil {
			return errors.New("invalid forward config")
		}
		if strings.TrimSpace(cfg.URL) == "" {
			return errors.New("forward config: url is required")
		}
		parsed, err := url.Parse(cfg.URL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return errors.New("forward config: url must be a valid http or https URL")
		}
		if cfg.Timeout < 0 || cfg.Timeout > 30 {
			return errors.New("forward config: timeout must be between 0 and 30")
		}

	case "filter":
		var cfg models.FilterConfig
		if err := json.Unmarshal(config, &cfg); err != nil {
			return errors.New("invalid filter config")
		}
		if strings.TrimSpace(cfg.Field) == "" {
			return errors.New("filter config: field is required")
		}
		validFields := map[string]bool{"method": true, "ip": true, "content": true}
		if !validFields[cfg.Field] && !strings.HasPrefix(cfg.Field, "header.") && !strings.HasPrefix(cfg.Field, "query.") {
			return errors.New("filter config: field must be method, ip, content, header.<name>, or query.<name>")
		}
		validOps := map[string]bool{"equals": true, "contains": true, "matches": true, "exists": true}
		if !validOps[cfg.Operator] {
			return errors.New("filter config: operator must be one of: equals, contains, matches, exists")
		}
		if cfg.Operator == "matches" {
			if _, err := regexp.Compile(cfg.Value); err != nil {
				return errors.New("filter config: value is not a valid regular expression")
			}
		}

	case "delay":
		var cfg models.DelayConfig
		if err := json.Unmarshal(config, &cfg); err != nil {
			return errors.New("invalid delay config")
		}
		if cfg.DurationMs < 100 || cfg.DurationMs > 30000 {
			return errors.New("delay config: duration_ms must be between 100 and 30000")
		}

	case "transform":
		var cfg models.TransformConfig
		if err := json.Unmarshal(config, &cfg); err != nil {
			return errors.New("invalid transform config")
		}
		if cfg.Status == nil && cfg.ContentType == nil && cfg.Body == nil {
			return errors.New("transform config: at least one of status, content_type, or body is required")
		}
		if cfg.Status != nil && (*cfg.Status < 100 || *cfg.Status > 999) {
			return errors.New("transform config: status must be between 100 and 999")
		}
	}

	return nil
}
