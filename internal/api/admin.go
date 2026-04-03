package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chrishaylesai/hookwatch/internal/models"
	"github.com/chrishaylesai/hookwatch/internal/store"
	"github.com/go-chi/chi/v5"
)

type adminHandler struct {
	store *store.Store
}

func newAdminHandler(db *store.Store) *adminHandler {
	return &adminHandler{store: db}
}

type adminUserResponse struct {
	ID           string  `json:"id"`
	Email        string  `json:"email"`
	DisplayName  string  `json:"display_name"`
	GlobalRole   string  `json:"global_role"`
	OIDCProvider *string `json:"oidc_provider,omitempty"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

type updateUserRequest struct {
	DisplayName *string `json:"display_name"`
	GlobalRole  *string `json:"global_role"`
}

func (h *adminHandler) listUsers(w http.ResponseWriter, r *http.Request) {
	limit := 50
	offset := 0

	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			limit = v
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v >= 0 {
			offset = v
		}
	}

	users, total, err := h.store.ListUsers(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}

	data := make([]adminUserResponse, 0, len(users))
	for _, u := range users {
		data = append(data, toAdminUserResponse(u))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":   data,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *adminHandler) listTokens(w http.ResponseWriter, r *http.Request) {
	params, err := parseTokenListParams(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	page, err := h.store.ListTokensForAdmin(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tokens")
		return
	}

	data := make([]tokenResponse, 0, len(page.Items))
	for _, item := range page.Items {
		data = append(data, toListedTokenResponse(item.Token, true, item.AccessRole, item.OwnerDisplay))
	}

	writeJSON(w, http.StatusOK, tokenListResponse{
		Data:   data,
		Total:  page.Total,
		Limit:  page.Limit,
		Offset: page.Offset,
	})
}

func (h *adminHandler) getUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")

	user, err := h.store.GetUser(r.Context(), userID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get user")
		return
	}

	writeJSON(w, http.StatusOK, toAdminUserResponse(user))
}

func (h *adminHandler) updateUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")

	user, err := h.store.GetUser(r.Context(), userID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get user")
		return
	}

	var req updateUserRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.DisplayName != nil {
		user.DisplayName = *req.DisplayName
	}
	if req.GlobalRole != nil {
		role := strings.ToLower(strings.TrimSpace(*req.GlobalRole))
		if role != "admin" && role != "user" {
			writeError(w, http.StatusBadRequest, "global_role must be admin or user")
			return
		}
		user.GlobalRole = role
	}

	user.UpdatedAt = time.Now().UTC()

	if err := h.store.UpdateUser(r.Context(), user); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update user")
		return
	}

	writeJSON(w, http.StatusOK, toAdminUserResponse(user))
}

func (h *adminHandler) deleteUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")

	if err := h.store.DeleteUser(r.Context(), userID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func toAdminUserResponse(u *models.User) adminUserResponse {
	return adminUserResponse{
		ID:           u.ID,
		Email:        u.Email,
		DisplayName:  u.DisplayName,
		GlobalRole:   u.GlobalRole,
		OIDCProvider: u.OIDCProvider,
		CreatedAt:    u.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:    u.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}
