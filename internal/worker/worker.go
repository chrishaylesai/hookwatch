package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/chrishaylesai/hookwatch/internal/hub"
	"github.com/chrishaylesai/hookwatch/internal/models"
	"github.com/chrishaylesai/hookwatch/internal/store"
	"github.com/google/uuid"
)

// Worker executes action pipelines asynchronously after webhook capture.
type Worker struct {
	store  *store.Store
	hub    *hub.Hub
	client *http.Client
	sem    chan struct{}
	logger *slog.Logger
}

// New creates a new Worker with the given concurrency limit.
func New(db *store.Store, eventHub *hub.Hub, concurrency int, logger *slog.Logger) *Worker {
	if concurrency <= 0 {
		concurrency = 50
	}
	return &Worker{
		store:  db,
		hub:    eventHub,
		client: &http.Client{Timeout: 30 * time.Second},
		sem:    make(chan struct{}, concurrency),
		logger: logger,
	}
}

// ExecuteActions loads and runs the action pipeline for a token after a request is captured.
// This is called as a goroutine from the capture handler.
func (w *Worker) ExecuteActions(tokenID string, request *models.Request) {
	w.sem <- struct{}{}
	defer func() { <-w.sem }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	actions, err := w.store.ListActionsByToken(ctx, tokenID)
	if err != nil {
		w.logger.Error("failed to load actions", "token_id", tokenID, "error", err)
		return
	}
	if len(actions) == 0 {
		return
	}

	// Filter to enabled actions only
	var enabled []*models.Action
	for _, a := range actions {
		if a.Enabled {
			enabled = append(enabled, a)
		}
	}
	if len(enabled) == 0 {
		return
	}

	// Create pending log entries for all actions
	logs := make([]*models.ActionLog, len(enabled))
	for i, a := range enabled {
		logs[i] = &models.ActionLog{
			UUID:      uuid.NewString(),
			ActionID:  a.UUID,
			RequestID: request.UUID,
			Status:    "pending",
			Result:    "{}",
		}
		if err := w.store.CreateActionLog(ctx, logs[i]); err != nil {
			w.logger.Error("failed to create action log", "action_id", a.UUID, "error", err)
			return
		}
	}

	// Build initial pipeline context from the captured request
	pctx := &PipelineContext{
		Method:      request.Method,
		Body:        request.Content,
		ContentType: extractContentType(request.Headers),
		Headers:     extractHeadersMap(request.Headers),
		Query:       request.Query,
		IP:          request.IP,
		URL:         request.URL,
	}

	// Execute each action sequentially
	for i, action := range enabled {
		log := logs[i]

		log.Status = "running"
		log.StartedAt = time.Now().UTC()
		_ = w.store.UpdateActionLog(ctx, log)

		executor := w.executorFor(action.Type)
		if executor == nil {
			log.Status = "failed"
			log.Result = `{"error": "unknown action type"}`
			log.CompletedAt = time.Now().UTC()
			_ = w.store.UpdateActionLog(ctx, log)
			w.publishActionCompleted(tokenID, log)
			continue
		}

		result, err := executor.Execute(ctx, request, pctx, json.RawMessage(action.Config))
		now := time.Now().UTC()
		log.CompletedAt = now

		if err != nil {
			log.Status = "failed"
			resultData := map[string]any{"error": err.Error()}
			if encoded, e := json.Marshal(resultData); e == nil {
				log.Result = string(encoded)
			}
		} else {
			log.Status = result.Status
			if result.Data != nil {
				if encoded, e := json.Marshal(result.Data); e == nil {
					log.Result = string(encoded)
				}
			}
		}

		_ = w.store.UpdateActionLog(ctx, log)
		w.publishActionCompleted(tokenID, log)

		// If the action signals pipeline stop (filter didn't match), skip remaining
		if result != nil && result.StopPipeline {
			for j := i + 1; j < len(enabled); j++ {
				skipLog := logs[j]
				skipLog.Status = "skipped"
				skipLog.CompletedAt = now
				_ = w.store.UpdateActionLog(ctx, skipLog)
				w.publishActionCompleted(tokenID, skipLog)
			}
			break
		}
	}
}

func (w *Worker) executorFor(actionType string) Executor {
	switch actionType {
	case "forward":
		return &ForwardExecutor{client: w.client}
	case "filter":
		return &FilterExecutor{}
	case "delay":
		return &DelayExecutor{}
	case "transform":
		return &TransformExecutor{}
	default:
		return nil
	}
}

func (w *Worker) publishActionCompleted(tokenID string, log *models.ActionLog) {
	data, err := json.Marshal(map[string]any{"action_log": log})
	if err != nil {
		return
	}
	w.hub.Publish(tokenID, hub.Event{
		Type: "action.completed",
		Data: data,
	})
}

func extractContentType(headersJSON string) string {
	var headers map[string]any
	if err := json.Unmarshal([]byte(headersJSON), &headers); err != nil {
		return ""
	}
	for _, key := range []string{"Content-Type", "content-type"} {
		if v, ok := headers[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

func extractHeadersMap(headersJSON string) map[string]string {
	var raw map[string]any
	if err := json.Unmarshal([]byte(headersJSON), &raw); err != nil {
		return nil
	}
	result := make(map[string]string, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}
	return result
}
