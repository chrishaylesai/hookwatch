package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/chrishaylesai/hookwatch/internal/models"
)

const (
	defaultForwardTimeout = 10 * time.Second
	maxForwardTimeout     = 30 * time.Second
	maxResponseBodyCapture = 4096
)

// ForwardExecutor sends an HTTP request to a configured URL.
type ForwardExecutor struct {
	client *http.Client
}

func (e *ForwardExecutor) Execute(ctx context.Context, req *models.Request, pctx *PipelineContext, config json.RawMessage) (*Result, error) {
	var cfg models.ForwardConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("parse forward config: %w", err)
	}

	timeout := defaultForwardTimeout
	if cfg.Timeout > 0 {
		timeout = time.Duration(cfg.Timeout) * time.Second
		if timeout > maxForwardTimeout {
			timeout = maxForwardTimeout
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	method := pctx.Method
	if cfg.Method != "" {
		method = strings.ToUpper(cfg.Method)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, cfg.URL, strings.NewReader(pctx.Body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Copy pipeline headers
	for k, v := range pctx.Headers {
		httpReq.Header.Set(k, v)
	}
	// Override with action-configured headers
	for k, v := range cfg.Headers {
		httpReq.Header.Set(k, v)
	}
	if pctx.ContentType != "" && httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", pctx.ContentType)
	}

	start := time.Now()
	resp, err := e.client.Do(httpReq)
	duration := time.Since(start)

	if err != nil {
		return nil, fmt.Errorf("forward request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyCapture))

	return &Result{
		Status: "success",
		Data: map[string]any{
			"status_code": resp.StatusCode,
			"body":        string(body),
			"duration_ms": duration.Milliseconds(),
			"url":         cfg.URL,
		},
	}, nil
}
