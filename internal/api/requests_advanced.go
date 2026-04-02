package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/chrishaylesai/hookwatch/internal/authz"
	"github.com/chrishaylesai/hookwatch/internal/models"
	"github.com/chrishaylesai/hookwatch/internal/store"
	"github.com/go-chi/chi/v5"
)

const (
	maxReplayResponseBody = 4096
	replayTimeout         = 30 * time.Second
)

type replayRequestPayload struct {
	URL               string            `json:"url"`
	PreserveHeaders   *bool             `json:"preserve_headers"`
	AdditionalHeaders map[string]string `json:"additional_headers"`
}

type replayResponse struct {
	StatusCode int               `json:"status"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	DurationMs int64             `json:"duration_ms"`
	URL        string            `json:"url"`
}

type requestDiffResponse struct {
	LeftRequestID  string               `json:"left_request_id"`
	RightRequestID string               `json:"right_request_id"`
	Sections       []requestDiffSection `json:"sections"`
}

type requestDiffSection struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Format  string `json:"format"`
	Left    string `json:"left"`
	Right   string `json:"right"`
	Changed bool   `json:"changed"`
}

func (h *requestHandler) replayRequest(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "tokenId")
	requestID := chi.URLParam(r, "requestId")

	token, err := loadActiveToken(r.Context(), h.store, tokenID, false)
	if err != nil {
		writeTokenLoadError(w, err)
		return
	}
	if !h.policy.CanAccessToken(r.Context(), token, authz.ActionEdit) {
		writeTokenPermissionDenied(w)
		return
	}
	if err := refreshTokenExpiry(r.Context(), h.store, token); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to refresh token expiry")
		return
	}

	req, err := h.store.GetRequest(r.Context(), tokenID, requestID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "request not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get request")
		return
	}

	var payload replayRequestPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	targetURL, err := buildReplayURL(payload.URL, req.Query)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	preserveHeaders := true
	if payload.PreserveHeaders != nil {
		preserveHeaders = *payload.PreserveHeaders
	}

	client := &http.Client{Timeout: replayTimeout}
	ctx, cancel := context.WithTimeout(r.Context(), replayTimeout)
	defer cancel()

	outbound, err := http.NewRequestWithContext(ctx, req.Method, targetURL, strings.NewReader(req.Content))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid replay request")
		return
	}
	if preserveHeaders {
		applyReplayHeaders(outbound.Header, req.Headers)
	}
	for key, value := range payload.AdditionalHeaders {
		if strings.TrimSpace(key) == "" {
			continue
		}
		outbound.Header.Set(key, value)
	}
	if requestContentType(req.Headers) != "" && outbound.Header.Get("Content-Type") == "" {
		outbound.Header.Set("Content-Type", requestContentType(req.Headers))
	}

	start := time.Now()
	resp, err := client.Do(outbound)
	if err != nil {
		writeError(w, http.StatusBadGateway, "replay request failed")
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxReplayResponseBody))
	duration := time.Since(start)

	writeJSON(w, http.StatusOK, replayResponse{
		StatusCode: resp.StatusCode,
		Headers:    flattenResponseHeaders(resp.Header),
		Body:       string(body),
		DurationMs: duration.Milliseconds(),
		URL:        targetURL,
	})
}

func (h *requestHandler) diffRequests(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "tokenId")
	leftID := strings.TrimSpace(r.URL.Query().Get("left"))
	rightID := strings.TrimSpace(r.URL.Query().Get("right"))
	if leftID == "" || rightID == "" {
		writeError(w, http.StatusBadRequest, "left and right request IDs are required")
		return
	}

	token, err := loadActiveToken(r.Context(), h.store, tokenID, false)
	if err != nil {
		writeTokenLoadError(w, err)
		return
	}
	if !h.policy.CanAccessToken(r.Context(), token, authz.ActionView) {
		writePrivateViewModeDenied(w)
		return
	}
	if err := refreshTokenExpiry(r.Context(), h.store, token); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to refresh token expiry")
		return
	}

	leftReq, err := h.store.GetRequest(r.Context(), tokenID, leftID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "left request not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get left request")
		return
	}
	rightReq, err := h.store.GetRequest(r.Context(), tokenID, rightID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "right request not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get right request")
		return
	}

	writeJSON(w, http.StatusOK, requestDiffResponse{
		LeftRequestID:  leftReq.UUID,
		RightRequestID: rightReq.UUID,
		Sections: []requestDiffSection{
			buildDiffSection("method", "Method", "text", leftReq.Method, rightReq.Method),
			buildDiffSection("url", "URL", "text", leftReq.URL, rightReq.URL),
			buildDiffSection("query", "Query", "json", normalizeQueryForDiff(leftReq.Query), normalizeQueryForDiff(rightReq.Query)),
			buildDiffSection("headers", "Headers", "json", normalizeJSONForDiff(leftReq.Headers), normalizeJSONForDiff(rightReq.Headers)),
			buildDiffSection("form_data", "Form data", "json", normalizeJSONForDiff(leftReq.FormData), normalizeJSONForDiff(rightReq.FormData)),
			buildDiffSection("body", "Body", detectBodyFormat(leftReq, rightReq), normalizeBodyForDiff(leftReq), normalizeBodyForDiff(rightReq)),
			buildDiffSection("ip", "IP address", "text", leftReq.IP, rightReq.IP),
			buildDiffSection("user_agent", "User agent", "text", leftReq.UserAgent, rightReq.UserAgent),
			buildDiffSection("received_at", "Received", "text", leftReq.CreatedAt.UTC().Format(time.RFC3339), rightReq.CreatedAt.UTC().Format(time.RFC3339)),
		},
	})
}

func (h *requestHandler) getOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "tokenId")

	token, err := loadActiveToken(r.Context(), h.store, tokenID, false)
	if err != nil {
		writeTokenLoadError(w, err)
		return
	}
	if !h.policy.CanAccessToken(r.Context(), token, authz.ActionView) {
		writePrivateViewModeDenied(w)
		return
	}
	if err := refreshTokenExpiry(r.Context(), h.store, token); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to refresh token expiry")
		return
	}

	paths := map[string]map[string]any{}
	if err := h.store.StreamRequestsByToken(r.Context(), tokenID, store.RequestListParams{}, func(req *models.Request) error {
		path := normalizedOpenAPIPath(tokenID, req.URL)
		method := strings.ToLower(req.Method)
		pathItem := paths[path]
		if pathItem == nil {
			pathItem = map[string]any{}
			paths[path] = pathItem
		}
		if _, exists := pathItem[method]; exists {
			return nil
		}
		pathItem[method] = buildOpenAPIOperation(path, req)
		return nil
	}); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to generate OpenAPI spec")
		return
	}

	spec := map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":   fmt.Sprintf("HookWatch generated spec for %s", tokenID),
			"version": "1.0.0",
		},
		"paths": paths,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s-openapi.json\"", tokenID))
	writeJSON(w, http.StatusOK, spec)
}

func writeTokenLoadError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "token not found")
		return
	}
	if isTokenExpiredError(err) {
		writeTokenExpired(w)
		return
	}
	writeError(w, http.StatusInternalServerError, "failed to get token")
}

func buildReplayURL(rawTargetURL, originalQuery string) (string, error) {
	targetURL := strings.TrimSpace(rawTargetURL)
	if targetURL == "" {
		return "", errors.New("url is required")
	}

	parsed, err := url.Parse(targetURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("url must be an absolute http or https URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("url must use http or https")
	}

	if strings.TrimSpace(originalQuery) != "" {
		if strings.TrimSpace(parsed.RawQuery) == "" {
			parsed.RawQuery = originalQuery
		} else {
			parsed.RawQuery = parsed.RawQuery + "&" + originalQuery
		}
	}

	return parsed.String(), nil
}

func applyReplayHeaders(header http.Header, rawHeaders string) {
	const replayContentTypeFallback = "Content-Type"

	for key, value := range decodeJSONMap(rawHeaders) {
		if isReplayBlockedHeader(key) {
			continue
		}

		switch typed := value.(type) {
		case string:
			header.Set(key, typed)
		case []any:
			for _, item := range typed {
				header.Add(key, fmt.Sprint(item))
			}
		}
	}

	if header.Get(replayContentTypeFallback) == "" {
		if contentType := requestContentType(rawHeaders); contentType != "" {
			header.Set(replayContentTypeFallback, contentType)
		}
	}
}

func isReplayBlockedHeader(key string) bool {
	blocked := []string{
		"connection",
		"content-length",
		"host",
		"keep-alive",
		"proxy-authenticate",
		"proxy-authorization",
		"te",
		"trailer",
		"transfer-encoding",
		"upgrade",
	}
	return slices.Contains(blocked, strings.ToLower(strings.TrimSpace(key)))
}

func flattenResponseHeaders(header http.Header) map[string]string {
	result := make(map[string]string, len(header))
	for key, values := range header {
		result[key] = strings.Join(values, ", ")
	}
	return result
}

func buildDiffSection(key, label, format, left, right string) requestDiffSection {
	return requestDiffSection{
		Key:     key,
		Label:   label,
		Format:  format,
		Left:    left,
		Right:   right,
		Changed: left != right,
	}
}

func normalizeQueryForDiff(rawQuery string) string {
	if strings.TrimSpace(rawQuery) == "" {
		return ""
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return rawQuery
	}
	normalized := map[string]any{}
	for key, items := range values {
		if len(items) == 1 {
			normalized[key] = items[0]
		} else {
			normalized[key] = items
		}
	}
	encoded, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return rawQuery
	}
	return string(encoded)
}

func normalizeJSONForDiff(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return raw
	}
	encoded, err := json.MarshalIndent(decoded, "", "  ")
	if err != nil {
		return raw
	}
	return string(encoded)
}

func normalizeBodyForDiff(req *models.Request) string {
	if strings.Contains(strings.ToLower(requestContentType(req.Headers)), "json") {
		return normalizeJSONForDiff(req.Content)
	}
	return req.Content
}

func detectBodyFormat(left, right *models.Request) string {
	leftJSON := strings.Contains(strings.ToLower(requestContentType(left.Headers)), "json")
	rightJSON := strings.Contains(strings.ToLower(requestContentType(right.Headers)), "json")
	if leftJSON || rightJSON {
		return "json"
	}
	return "text"
}

func normalizedOpenAPIPath(tokenID, rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Path == "" {
		return "/"
	}

	prefix := "/" + tokenID
	switch {
	case parsed.Path == prefix:
		return "/"
	case strings.HasPrefix(parsed.Path, prefix+"/"):
		return parsed.Path[len(prefix):]
	case parsed.Path == "":
		return "/"
	default:
		return parsed.Path
	}
}

func buildOpenAPIOperation(path string, req *models.Request) map[string]any {
	operation := map[string]any{
		"summary":   fmt.Sprintf("Captured %s %s", strings.ToUpper(req.Method), path),
		"responses": map[string]any{"200": map[string]any{"description": "Captured request replay target response"}},
	}

	if params := buildOpenAPIQueryParameters(req.Query); len(params) > 0 {
		operation["parameters"] = params
	}

	if requestBody := buildOpenAPIRequestBody(req); requestBody != nil {
		operation["requestBody"] = requestBody
	}

	return operation
}

func buildOpenAPIQueryParameters(rawQuery string) []map[string]any {
	if strings.TrimSpace(rawQuery) == "" {
		return nil
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return nil
	}

	params := make([]map[string]any, 0, len(values))
	for key := range values {
		params = append(params, map[string]any{
			"name":     key,
			"in":       "query",
			"required": false,
			"schema":   map[string]any{"type": "string"},
		})
	}
	slices.SortFunc(params, func(a, b map[string]any) int {
		return strings.Compare(a["name"].(string), b["name"].(string))
	})
	return params
}

func buildOpenAPIRequestBody(req *models.Request) map[string]any {
	if strings.TrimSpace(req.Content) == "" && strings.TrimSpace(req.FormData) == "" {
		return nil
	}

	contentType := requestContentType(req.Headers)
	if strings.Contains(strings.ToLower(contentType), "json") {
		var decoded any
		if err := json.Unmarshal([]byte(req.Content), &decoded); err == nil {
			return map[string]any{
				"required": true,
				"content": map[string]any{
					"application/json": map[string]any{
						"schema": inferOpenAPISchema(decoded),
					},
				},
			}
		}
	}

	formData := decodeJSONMap(req.FormData)
	if len(formData) > 0 {
		return map[string]any{
			"required": true,
			"content": map[string]any{
				"application/x-www-form-urlencoded": map[string]any{
					"schema": inferOpenAPISchema(formData),
				},
			},
		}
	}

	return map[string]any{
		"required": true,
		"content": map[string]any{
			firstNonEmpty(contentType, "text/plain"): map[string]any{
				"schema": map[string]any{"type": "string"},
			},
		},
	}
}

func inferOpenAPISchema(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		properties := map[string]any{}
		required := make([]string, 0, len(typed))
		for key, item := range typed {
			properties[key] = inferOpenAPISchema(item)
			required = append(required, key)
		}
		slices.Sort(required)
		return map[string]any{
			"type":       "object",
			"properties": properties,
			"required":   required,
		}
	case []any:
		schema := map[string]any{"type": "array"}
		if len(typed) > 0 {
			schema["items"] = inferOpenAPISchema(typed[0])
		} else {
			schema["items"] = map[string]any{}
		}
		return schema
	case bool:
		return map[string]any{"type": "boolean"}
	case float64:
		if typed == float64(int64(typed)) {
			return map[string]any{"type": "integer"}
		}
		return map[string]any{"type": "number"}
	case nil:
		return map[string]any{"nullable": true}
	default:
		return map[string]any{"type": "string"}
	}
}
