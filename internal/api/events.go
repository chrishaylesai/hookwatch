package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/chrishaylesai/hookwatch/internal/authz"
	"github.com/chrishaylesai/hookwatch/internal/hub"
	"github.com/chrishaylesai/hookwatch/internal/models"
	"github.com/chrishaylesai/hookwatch/internal/store"
	"github.com/go-chi/chi/v5"
)

var sseEventCounter atomic.Uint64

type eventHandler struct {
	store  *store.Store
	hub    *hub.Hub
	policy *authz.Policy
}

func newEventHandler(db *store.Store, eventHub *hub.Hub, policy *authz.Policy) *eventHandler {
	return &eventHandler{
		store:  db,
		hub:    eventHub,
		policy: policy,
	}
}

func (h *eventHandler) stream(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "tokenId")

	token, err := loadActiveToken(r.Context(), h.store, tokenID, false)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		if isTokenExpiredError(err) {
			writeTokenExpired(w)
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get token")
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

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := h.hub.Subscribe(tokenID)
	defer h.hub.Unsubscribe(tokenID, ch)

	_, _ = w.Write([]byte(": connected\n\n"))
	flusher.Flush()

	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			_, _ = w.Write([]byte(": ping\n\n"))
			flusher.Flush()
		case event := <-ch:
			if err := writeSSEEvent(w, event); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func publishRequestCreated(eventHub *hub.Hub, tokenID string, req *models.Request, total int) {
	payload, err := json.Marshal(map[string]any{
		"request": toRequestResponse(req),
		"total":   total,
	})
	if err != nil {
		return
	}

	eventHub.Publish(tokenID, hub.Event{
		ID:   nextSSEEventID(),
		Type: "request.created",
		Data: payload,
	})
}

func publishTokenUpdated(eventHub *hub.Hub, token *models.Token) {
	payload, err := json.Marshal(map[string]any{
		"token": toTokenResponse(token, nil),
	})
	if err != nil {
		return
	}

	eventHub.Publish(token.UUID, hub.Event{
		ID:   nextSSEEventID(),
		Type: "token.updated",
		Data: payload,
	})
}

func publishTokenDeleted(eventHub *hub.Hub, tokenID string) {
	payload, err := json.Marshal(map[string]any{
		"token_id": tokenID,
	})
	if err != nil {
		return
	}

	eventHub.Publish(tokenID, hub.Event{
		ID:   nextSSEEventID(),
		Type: "token.deleted",
		Data: payload,
	})
}

func writeSSEEvent(w http.ResponseWriter, event hub.Event) error {
	var buf bytes.Buffer
	if event.ID != "" {
		buf.WriteString("id: ")
		buf.WriteString(event.ID)
		buf.WriteByte('\n')
	}
	if event.Type != "" {
		buf.WriteString("event: ")
		buf.WriteString(event.Type)
		buf.WriteByte('\n')
	}
	if len(event.Data) > 0 {
		for _, line := range bytes.Split(event.Data, []byte{'\n'}) {
			buf.WriteString("data: ")
			buf.Write(line)
			buf.WriteByte('\n')
		}
	}
	buf.WriteByte('\n')

	_, err := w.Write(buf.Bytes())
	return err
}

func nextSSEEventID() string {
	return strconv.FormatUint(sseEventCounter.Add(1), 10)
}
