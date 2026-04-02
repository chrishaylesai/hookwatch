package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/chrishaylesai/hookwatch/internal/models"
)

const (
	signatureStatusUnknown = "unknown"
	signatureStatusValid   = "valid"
	signatureStatusInvalid = "invalid"
)

func evaluateRequestSignature(token *models.Token, headers http.Header, body []byte) (string, *string, *string) {
	provider := strings.ToLower(strings.TrimSpace(token.SignatureProvider))
	if provider == "" || token.SignatureSecret == nil || strings.TrimSpace(*token.SignatureSecret) == "" {
		return signatureStatusUnknown, nil, nil
	}

	providerValue := provider
	switch provider {
	case "github":
		return validateGitHubSignature(providerValue, headers, body, *token.SignatureSecret)
	case "stripe":
		return validateStripeSignature(providerValue, headers, body, *token.SignatureSecret)
	default:
		message := "unsupported signature provider"
		return signatureStatusInvalid, &providerValue, &message
	}
}

func validateGitHubSignature(provider string, headers http.Header, body []byte, secret string) (string, *string, *string) {
	headerValue := strings.TrimSpace(headers.Get("X-Hub-Signature-256"))
	if headerValue == "" {
		message := "missing X-Hub-Signature-256 header"
		return signatureStatusInvalid, &provider, &message
	}

	const prefix = "sha256="
	if !strings.HasPrefix(headerValue, prefix) {
		message := "invalid GitHub signature format"
		return signatureStatusInvalid, &provider, &message
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	received := strings.TrimPrefix(headerValue, prefix)
	if !hmac.Equal([]byte(strings.ToLower(received)), []byte(expected)) {
		message := "signature mismatch"
		return signatureStatusInvalid, &provider, &message
	}

	return signatureStatusValid, &provider, nil
}

func validateStripeSignature(provider string, headers http.Header, body []byte, secret string) (string, *string, *string) {
	headerValue := strings.TrimSpace(headers.Get("Stripe-Signature"))
	if headerValue == "" {
		message := "missing Stripe-Signature header"
		return signatureStatusInvalid, &provider, &message
	}

	var timestamp string
	var signatures []string
	for _, part := range strings.Split(headerValue, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch key {
		case "t":
			timestamp = value
		case "v1":
			signatures = append(signatures, value)
		}
	}

	if timestamp == "" || len(signatures) == 0 {
		message := "invalid Stripe signature format"
		return signatureStatusInvalid, &provider, &message
	}

	signedPayload := timestamp + "." + string(body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signedPayload))
	expected := hex.EncodeToString(mac.Sum(nil))

	for _, candidate := range signatures {
		if hmac.Equal([]byte(strings.ToLower(candidate)), []byte(expected)) {
			return signatureStatusValid, &provider, nil
		}
	}

	message := "signature mismatch"
	return signatureStatusInvalid, &provider, &message
}
