package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	"github.com/chrishaylesai/hookwatch/internal/hub"
	"github.com/chrishaylesai/hookwatch/internal/models"
	"github.com/chrishaylesai/hookwatch/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type captureHandler struct {
	store    *store.Store
	eventHub *hub.Hub
}

var captureCORSHeaders = map[string]string{
	"Access-Control-Allow-Origin":   "*",
	"Access-Control-Allow-Methods":  "GET, PUT, PATCH, POST, OPTIONS",
	"Access-Control-Allow-Headers":  "DNT,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Range",
	"Access-Control-Expose-Headers": "Content-Length,Content-Range,X-Request-Id,X-Token-Id",
}

func newCaptureHandler(db *store.Store, eventHub *hub.Hub) *captureHandler {
	return &captureHandler{
		store:    db,
		eventHub: eventHub,
	}
}

func (h *captureHandler) capture(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "tokenId")
	if _, err := uuid.Parse(tokenID); err != nil {
		http.NotFound(w, r)
		return
	}

	token, err := loadActiveToken(r.Context(), h.store, tokenID, false)
	if err != nil {
		if err == store.ErrNotFound {
			http.NotFound(w, r)
			return
		}
		if isTokenExpiredError(err) {
			writeTokenExpired(w)
			return
		}
		http.Error(w, "failed to load token", http.StatusInternalServerError)
		return
	}

	secret, secretSource := extractReceiveSecret(r)
	if token.ReceiveMode == receiveModePrivate && !validateReceiveSecret(token, secret) {
		writeReceiveModeUnauthorized(w)
		return
	}
	if err := refreshTokenExpiry(r.Context(), h.store, token); err != nil {
		http.Error(w, "failed to refresh token expiry", http.StatusInternalServerError)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	sanitizedHeaders := sanitizedCaptureHeaders(r.Header, secretSource, token.ReceiveMode == receiveModePrivate)
	sanitizedQuery := sanitizedCaptureQuery(r.URL.RawQuery, token.ReceiveMode == receiveModePrivate)

	headersJSON, err := encodeHeaders(sanitizedHeaders)
	if err != nil {
		http.Error(w, "failed to encode request headers", http.StatusInternalServerError)
		return
	}

	formJSON, err := encodeFormData(r.Header.Get("Content-Type"), body)
	if err != nil {
		http.Error(w, "failed to parse form data", http.StatusBadRequest)
		return
	}

	reqID := uuid.NewString()
	now := time.Now().UTC()
	record := &models.Request{
		UUID:      reqID,
		TokenID:   tokenID,
		IP:        remoteIP(r.RemoteAddr),
		Hostname:  r.Host,
		Method:    r.Method,
		UserAgent: r.UserAgent(),
		Content:   string(body),
		Query:     sanitizedQuery,
		Headers:   headersJSON,
		FormData:  formJSON,
		URL:       absoluteURL(r, sanitizedQuery),
		CreatedAt: now,
	}

	if err := h.store.CreateRequest(r.Context(), record); err != nil {
		if err == store.ErrQuotaExceeded {
			writeRequestQuotaExceeded(w)
			return
		}
		http.Error(w, "failed to store captured request", http.StatusInternalServerError)
		return
	}

	total, err := h.store.CountRequestsByToken(r.Context(), tokenID)
	if err == nil {
		publishRequestCreated(h.eventHub, tokenID, record, total)
	}

	if token.Timeout > 0 {
		timer := time.NewTimer(time.Duration(token.Timeout) * time.Second)
		defer timer.Stop()

		select {
		case <-timer.C:
		case <-r.Context().Done():
			return
		}
	}

	if token.CORS {
		applyCaptureCORSHeaders(w.Header())
	}

	w.Header().Set("X-Request-Id", reqID)
	w.Header().Set("X-Token-Id", tokenID)
	w.Header().Set("Content-Type", token.DefaultContentType)
	w.WriteHeader(token.DefaultStatus)
	_, _ = w.Write([]byte(token.DefaultContent))
}

func encodeHeaders(header http.Header) (string, error) {
	values := make(map[string]any, len(header))
	for key, items := range header {
		switch len(items) {
		case 0:
			values[key] = ""
		case 1:
			values[key] = items[0]
		default:
			cloned := make([]string, len(items))
			copy(cloned, items)
			values[key] = cloned
		}
	}

	return marshalMap(values)
}

func encodeFormData(contentType string, body []byte) (string, error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil && strings.TrimSpace(contentType) != "" {
		return "", fmt.Errorf("parse media type: %w", err)
	}

	switch mediaType {
	case "application/x-www-form-urlencoded":
		values, err := url.ParseQuery(string(body))
		if err != nil {
			return "", fmt.Errorf("parse form-urlencoded: %w", err)
		}
		return encodeURLValues(values)
	case "multipart/form-data":
		reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		form, err := reader.ReadForm(32 << 20)
		if err != nil {
			return "", fmt.Errorf("parse multipart form: %w", err)
		}
		defer form.RemoveAll()

		encoded := make(map[string]any, len(form.Value)+len(form.File))
		for key, values := range form.Value {
			encoded[key] = normalizeStringValues(values)
		}
		for key, files := range form.File {
			encodedFiles := make([]map[string]any, 0, len(files))
			for _, file := range files {
				encodedFiles = append(encodedFiles, map[string]any{
					"filename": file.Filename,
					"size":     file.Size,
					"header":   flattenMIMEHeader(file.Header),
				})
			}
			encoded[key] = encodedFiles
		}
		return marshalMap(encoded)
	default:
		return "{}", nil
	}
}

func encodeURLValues(values url.Values) (string, error) {
	encoded := make(map[string]any, len(values))
	for key, items := range values {
		encoded[key] = normalizeStringValues(items)
	}
	return marshalMap(encoded)
}

func normalizeStringValues(values []string) any {
	if len(values) == 1 {
		return values[0]
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func flattenMIMEHeader(header textproto.MIMEHeader) map[string]any {
	flattened := make(map[string]any, len(header))
	for key, items := range header {
		flattened[key] = normalizeStringValues(items)
	}
	return flattened
}

func marshalMap(data map[string]any) (string, error) {
	if data == nil {
		data = map[string]any{}
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func absoluteURL(r *http.Request, rawQuery string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	cloned := *r.URL
	cloned.RawQuery = rawQuery
	return scheme + "://" + r.Host + cloned.RequestURI()
}

func remoteIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}
	return remoteAddr
}

func applyCaptureCORSHeaders(header http.Header) {
	for key, value := range captureCORSHeaders {
		header.Set(key, value)
	}
}
