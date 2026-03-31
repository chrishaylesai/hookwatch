package authz

import (
	"context"

	"github.com/chrishaylesai/hookwatch/internal/auth"
	"github.com/chrishaylesai/hookwatch/internal/models"
	"github.com/chrishaylesai/hookwatch/internal/store"
)

const (
	RoleViewer = "viewer"
	RoleEditor = "editor"

	GlobalRoleAdmin = "admin"
	GlobalRoleUser  = "user"
)

// Action represents an operation on a hook.
type Action string

const (
	ActionView   Action = "view"
	ActionEdit   Action = "edit"
	ActionDelete Action = "delete"
	ActionGrant  Action = "grant"
)

// Policy checks authorization for hook access.
type Policy struct {
	store    *store.Store
	authMode string
}

// NewPolicy creates a new authorization policy.
func NewPolicy(db *store.Store, authMode string) *Policy {
	return &Policy{store: db, authMode: authMode}
}

// CanAccessToken checks if the current user can perform the given action on the token.
// When auth is disabled (mode=none), all actions are allowed.
func (p *Policy) CanAccessToken(ctx context.Context, token *models.Token, action Action) bool {
	if p.authMode == "none" {
		return true
	}

	user := auth.UserFromContext(ctx)

	// Public view tokens can be viewed by anyone (including unauthenticated)
	if action == ActionView && token.ViewMode == "public" {
		return true
	}

	// Everything else requires authentication
	if user == nil {
		return false
	}

	// Admins can do everything
	if user.GlobalRole == GlobalRoleAdmin {
		return true
	}

	// Owners can do everything with their own tokens
	if token.OwnerID != nil && *token.OwnerID == user.ID {
		return true
	}

	// Check hook grants for non-owners
	grant, err := p.store.GetHookGrant(ctx, token.UUID, user.ID)
	if err != nil {
		return false
	}

	switch action {
	case ActionView:
		return grant.Role == RoleViewer || grant.Role == RoleEditor
	case ActionEdit:
		return grant.Role == RoleEditor
	case ActionDelete:
		// Only owner or admin can delete
		return false
	case ActionGrant:
		// Only owner or admin can manage grants
		return false
	default:
		return false
	}
}

// IsAdmin checks if the user in context is an admin.
func IsAdmin(ctx context.Context) bool {
	user := auth.UserFromContext(ctx)
	return user != nil && user.GlobalRole == GlobalRoleAdmin
}

// IsOwner checks if the user in context owns the given token.
func IsOwner(ctx context.Context, token *models.Token) bool {
	user := auth.UserFromContext(ctx)
	if user == nil || token.OwnerID == nil {
		return false
	}
	return *token.OwnerID == user.ID
}

// CanManageGrants checks if the user can add/remove grants on a token.
func (p *Policy) CanManageGrants(ctx context.Context, token *models.Token) bool {
	if p.authMode == "none" {
		return true
	}
	return IsAdmin(ctx) || IsOwner(ctx, token)
}
