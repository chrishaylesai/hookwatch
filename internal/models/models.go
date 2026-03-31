package models

import "time"

// User represents a system user.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"display_name"`
	PasswordHash *string   `json:"-"`
	OIDCProvider *string   `json:"oidc_provider,omitempty"`
	OIDCSubject  *string   `json:"oidc_subject,omitempty"`
	GlobalRole   string    `json:"global_role"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Session represents an active user session.
type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"user_agent"`
}

// Token represents a webhook endpoint.
type Token struct {
	UUID                string    `json:"uuid"`
	OwnerID             *string   `json:"owner_id,omitempty"`
	ReceiveMode         string    `json:"receive_mode"`
	ViewMode            string    `json:"view_mode"`
	ReceiveSecretHash   *string   `json:"-"`
	ReceiveSecretPrefix *string   `json:"receive_secret_prefix,omitempty"`
	DefaultStatus       int       `json:"default_status"`
	DefaultContent      string    `json:"default_content"`
	DefaultContentType  string    `json:"default_content_type"`
	MaxRequests         int       `json:"max_requests"`
	Timeout             int       `json:"timeout"`
	CORS                bool      `json:"cors"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	ExpiresAt           time.Time `json:"expires_at"`
}

// Request represents a captured webhook request.
type Request struct {
	UUID      string    `json:"uuid"`
	TokenID   string    `json:"token_id"`
	IP        string    `json:"ip"`
	Hostname  string    `json:"hostname"`
	Method    string    `json:"method"`
	UserAgent string    `json:"user_agent"`
	Content   string    `json:"content"`
	Query     string    `json:"query"`     // JSON encoded
	Headers   string    `json:"headers"`   // JSON encoded
	FormData  string    `json:"form_data"` // JSON encoded
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

// HookGrant represents access granted to a user for a specific token.
type HookGrant struct {
	ID        string    `json:"id"`
	TokenID   string    `json:"token_id"`
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	GrantedBy string    `json:"granted_by"`
	CreatedAt time.Time `json:"created_at"`
}
