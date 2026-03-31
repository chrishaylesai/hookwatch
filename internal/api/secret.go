package api

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"

	"github.com/chrishaylesai/hookwatch/internal/models"
)

const (
	receiveSecretBytes  = 32
	receiveSecretPrefix = 4
)

type receiveSecretSource string

const (
	receiveSecretSourceNone   receiveSecretSource = ""
	receiveSecretSourceHeader receiveSecretSource = "header"
	receiveSecretSourceQuery  receiveSecretSource = "query"
	receiveSecretSourceBasic  receiveSecretSource = "basic"
)

func reconcileReceiveSecret(token *models.Token, previouslyPrivate bool) (*string, error) {
	if token.ReceiveMode != receiveModePrivate {
		token.ReceiveSecretHash = nil
		token.ReceiveSecretPrefix = nil
		return nil, nil
	}

	if previouslyPrivate && token.ReceiveSecretHash != nil && token.ReceiveSecretPrefix != nil {
		return nil, nil
	}

	secret, err := generateReceiveSecret()
	if err != nil {
		return nil, err
	}

	token.ReceiveSecretHash = stringPtr(hashReceiveSecret(secret))
	token.ReceiveSecretPrefix = stringPtr(secret[:receiveSecretPrefix])

	return &secret, nil
}

func rotateReceiveSecret(token *models.Token) (*string, error) {
	if token.ReceiveMode != receiveModePrivate {
		token.ReceiveSecretHash = nil
		token.ReceiveSecretPrefix = nil
		return nil, nil
	}

	secret, err := generateReceiveSecret()
	if err != nil {
		return nil, err
	}

	token.ReceiveSecretHash = stringPtr(hashReceiveSecret(secret))
	token.ReceiveSecretPrefix = stringPtr(secret[:receiveSecretPrefix])

	return &secret, nil
}

func generateReceiveSecret() (string, error) {
	buf := make([]byte, receiveSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashReceiveSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func validateReceiveSecret(token *models.Token, providedSecret string) bool {
	if token == nil || token.ReceiveSecretHash == nil || providedSecret == "" {
		return false
	}

	expected := *token.ReceiveSecretHash
	actual := hashReceiveSecret(providedSecret)
	if len(expected) != len(actual) {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}

func extractReceiveSecret(r *http.Request) (string, receiveSecretSource) {
	if secret := strings.TrimSpace(r.Header.Get("X-Hook-Secret")); secret != "" {
		return secret, receiveSecretSourceHeader
	}
	if secret := strings.TrimSpace(r.URL.Query().Get("secret")); secret != "" {
		return secret, receiveSecretSourceQuery
	}
	if _, password, ok := r.BasicAuth(); ok && strings.TrimSpace(password) != "" {
		return password, receiveSecretSourceBasic
	}

	return "", receiveSecretSourceNone
}

func sanitizedCaptureQuery(rawQuery string, isPrivate bool) string {
	if !isPrivate || strings.TrimSpace(rawQuery) == "" {
		return rawQuery
	}

	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return rawQuery
	}
	values.Del("secret")
	return values.Encode()
}

func sanitizedCaptureHeaders(header http.Header, source receiveSecretSource, isPrivate bool) http.Header {
	cloned := header.Clone()
	if !isPrivate {
		return cloned
	}

	cloned.Del("X-Hook-Secret")
	if source == receiveSecretSourceBasic {
		cloned.Del("Authorization")
	}

	return cloned
}

func stringPtr(value string) *string {
	return &value
}
