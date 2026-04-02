package api

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/chrishaylesai/hookwatch/internal/auth"
	"github.com/chrishaylesai/hookwatch/internal/models"
)

type authHandler struct {
	authService auth.Authenticator
	authMode    string
}

func newAuthHandler(authService auth.Authenticator, authMode string) *authHandler {
	return &authHandler{
		authService: authService,
		authMode:    authMode,
	}
}

type registerRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userResponse struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	GlobalRole  string `json:"global_role"`
	CreatedAt   string `json:"created_at"`
}

func (h *authHandler) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	if req.DisplayName == "" {
		req.DisplayName = req.Email
	}

	user, err := h.authService.Register(r.Context(), req.Email, req.DisplayName, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrEmailTaken):
			writeError(w, http.StatusConflict, "email already registered")
		case errors.Is(err, auth.ErrRegistrationClosed):
			writeError(w, http.StatusForbidden, "registration is not allowed")
		case errors.Is(err, auth.ErrWeakPassword):
			writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		default:
			writeError(w, http.StatusInternalServerError, "failed to register")
		}
		return
	}

	writeJSON(w, http.StatusCreated, toUserResponse(user))
}

func (h *authHandler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	ip := r.RemoteAddr
	userAgent := r.UserAgent()

	user, session, err := h.authService.Login(r.Context(), req.Email, req.Password, ip, userAgent)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "invalid email or password")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to log in")
		return
	}

	auth.SetSessionCookie(w, session.ID)
	writeJSON(w, http.StatusOK, toUserResponse(user))
}

func (h *authHandler) logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err == nil && cookie.Value != "" {
		_ = h.authService.Logout(r.Context(), cookie.Value)
	}

	auth.ClearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *authHandler) me(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	writeJSON(w, http.StatusOK, toUserResponse(user))
}

func (h *authHandler) authInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"auth_mode": h.authMode,
	})
}

func (h *authHandler) authorizeOIDC(w http.ResponseWriter, r *http.Request) {
	flow, err := h.authService.StartOIDCAuth(baseURLFromRequest(r), r.URL.Query().Get("redirect"))
	if err != nil {
		if errors.Is(err, auth.ErrUnsupportedAuthMode) {
			http.NotFound(w, r)
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to start oidc login")
		return
	}

	auth.SetOIDCFlowCookies(w, flow)
	http.Redirect(w, r, flow.URL, http.StatusFound)
}

func (h *authHandler) callbackOIDC(w http.ResponseWriter, r *http.Request) {
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if state == "" || code == "" {
		writeError(w, http.StatusBadRequest, "missing oidc callback parameters")
		return
	}

	expectedState := cookieValue(r, auth.OIDCStateCookieName)
	expectedNonce := cookieValue(r, auth.OIDCNonceCookieName)
	redirectPath := cookieValue(r, auth.OIDCRedirectCookieName)
	redirectPath = sanitizeRedirectPath(redirectPath)
	auth.ClearOIDCFlowCookies(w)

	if expectedState == "" || expectedNonce == "" || state != expectedState {
		redirectOIDCError(w, r, redirectPath, "oidc_failed")
		return
	}

	_, session, err := h.authService.CompleteOIDCAuth(
		r.Context(),
		baseURLFromRequest(r),
		code,
		expectedNonce,
		r.RemoteAddr,
		r.UserAgent(),
	)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrOIDCEmailRequired):
			redirectOIDCError(w, r, redirectPath, "email_required")
		case errors.Is(err, auth.ErrOIDCAccountConflict):
			redirectOIDCError(w, r, redirectPath, "account_conflict")
		default:
			redirectOIDCError(w, r, redirectPath, "oidc_failed")
		}
		return
	}

	auth.SetSessionCookie(w, session.ID)
	http.Redirect(w, r, redirectPath, http.StatusFound)
}

func toUserResponse(u *models.User) userResponse {
	return userResponse{
		ID:          u.ID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		GlobalRole:  u.GlobalRole,
		CreatedAt:   u.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func redirectOIDCError(w http.ResponseWriter, r *http.Request, redirectPath, code string) {
	values := url.Values{}
	values.Set("error", code)
	if redirectPath != "/" {
		values.Set("redirect", redirectPath)
	}

	http.Redirect(w, r, "/login?"+values.Encode(), http.StatusFound)
}

func cookieValue(r *http.Request, name string) string {
	cookie, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

func sanitizeRedirectPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw[0] != '/' {
		return "/"
	}
	if len(raw) > 1 && raw[1] == '/' {
		return "/"
	}
	return raw
}

func baseURLFromRequest(r *http.Request) string {
	scheme := "http"
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]); forwarded != "" {
		scheme = forwarded
	} else if r.TLS != nil {
		scheme = "https"
	}

	host := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0])
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}

	return scheme + "://" + host
}
