package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/chrishaylesai/hookwatch/internal/auth"
	"github.com/chrishaylesai/hookwatch/internal/authz"
	"github.com/chrishaylesai/hookwatch/internal/models"
	"github.com/chrishaylesai/hookwatch/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type grantHandler struct {
	store  *store.Store
	policy *authz.Policy
}

func newGrantHandler(db *store.Store, policy *authz.Policy) *grantHandler {
	return &grantHandler{store: db, policy: policy}
}

type createGrantRequest struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
}

type grantResponse struct {
	ID        string `json:"id"`
	TokenID   string `json:"token_id"`
	UserID    string `json:"user_id"`
	Role      string `json:"role"`
	GrantedBy string `json:"granted_by"`
	CreatedAt string `json:"created_at"`
}

func (h *grantHandler) listGrants(w http.ResponseWriter, r *http.Request) {
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

	if !h.policy.CanManageGrants(r.Context(), token) {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	grants, err := h.store.ListHookGrants(r.Context(), tokenID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list grants")
		return
	}

	data := make([]grantResponse, 0, len(grants))
	for _, g := range grants {
		data = append(data, toGrantResponse(g))
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

func (h *grantHandler) createGrant(w http.ResponseWriter, r *http.Request) {
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

	if !h.policy.CanManageGrants(r.Context(), token) {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	var req createGrantRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Validate role
	req.Role = strings.ToLower(strings.TrimSpace(req.Role))
	if req.Role == "" {
		req.Role = authz.RoleViewer
	}
	if req.Role != authz.RoleViewer && req.Role != authz.RoleEditor {
		writeError(w, http.StatusBadRequest, "role must be viewer or editor")
		return
	}

	// Resolve user by ID or email
	var targetUserID string
	if req.UserID != "" {
		targetUserID = req.UserID
		// Verify user exists
		if _, err := h.store.GetUser(r.Context(), targetUserID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "user not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to look up user")
			return
		}
	} else if req.Email != "" {
		user, err := h.store.GetUserByEmail(r.Context(), strings.TrimSpace(req.Email))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "user not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to look up user")
			return
		}
		targetUserID = user.ID
	} else {
		writeError(w, http.StatusBadRequest, "user_id or email is required")
		return
	}

	// Don't grant to the owner
	if token.OwnerID != nil && *token.OwnerID == targetUserID {
		writeError(w, http.StatusBadRequest, "cannot grant access to token owner")
		return
	}

	currentUser := auth.UserFromContext(r.Context())
	grantedBy := ""
	if currentUser != nil {
		grantedBy = currentUser.ID
	}

	grant := &models.HookGrant{
		ID:        uuid.NewString(),
		TokenID:   tokenID,
		UserID:    targetUserID,
		Role:      req.Role,
		GrantedBy: grantedBy,
		CreatedAt: time.Now().UTC(),
	}

	if err := h.store.CreateHookGrant(r.Context(), grant); err != nil {
		// UNIQUE constraint violation means grant already exists
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusConflict, "grant already exists for this user")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create grant")
		return
	}

	writeJSON(w, http.StatusCreated, toGrantResponse(grant))
}

func (h *grantHandler) deleteGrant(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "tokenId")
	userID := chi.URLParam(r, "userId")

	token, err := h.store.GetToken(r.Context(), tokenID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get token")
		return
	}

	if !h.policy.CanManageGrants(r.Context(), token) {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	if err := h.store.DeleteHookGrantByTokenAndUser(r.Context(), tokenID, userID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "grant not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete grant")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func toGrantResponse(g *models.HookGrant) grantResponse {
	return grantResponse{
		ID:        g.ID,
		TokenID:   g.TokenID,
		UserID:    g.UserID,
		Role:      g.Role,
		GrantedBy: g.GrantedBy,
		CreatedAt: g.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}
