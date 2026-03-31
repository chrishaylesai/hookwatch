package api

import (
	"net/http"
	"strings"

	"github.com/chrishaylesai/hookwatch/internal/models"
)

const (
	authModeNone = "none"

	receiveModePublic  = "public"
	receiveModePrivate = "private"

	viewModePublic  = "public"
	viewModePrivate = "private"
)

func normalizedAuthMode(mode string) string {
	if strings.TrimSpace(mode) == "" {
		return authModeNone
	}
	return strings.ToLower(strings.TrimSpace(mode))
}

func validateAndNormalizeTokenAccess(token *models.Token, authMode string) error {
	switch token.ReceiveMode {
	case receiveModePublic, receiveModePrivate:
	default:
		return modeValidationError("receive_mode must be public or private")
	}

	switch token.ViewMode {
	case viewModePublic, viewModePrivate:
	default:
		return modeValidationError("view_mode must be public or private")
	}

	if normalizedAuthMode(authMode) == authModeNone {
		token.ViewMode = viewModePublic
	}

	return nil
}

func canViewToken(token *models.Token) bool {
	return token.ViewMode != viewModePrivate
}

func canCaptureWebhook(token *models.Token) bool {
	return token.ReceiveMode != receiveModePrivate
}

func writePrivateViewModeDenied(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, "token not found")
}

func writeReceiveModeUnauthorized(w http.ResponseWriter) {
	writeError(w, http.StatusUnauthorized, "private hook requires a secret")
}

type modeValidationError string

func (e modeValidationError) Error() string {
	return string(e)
}
