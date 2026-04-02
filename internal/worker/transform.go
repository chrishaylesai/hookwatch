package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chrishaylesai/hookwatch/internal/models"
)

// TransformExecutor mutates the pipeline context for downstream actions.
// It does NOT affect the already-sent HTTP response.
type TransformExecutor struct{}

func (e *TransformExecutor) Execute(ctx context.Context, req *models.Request, pctx *PipelineContext, config json.RawMessage) (*Result, error) {
	var cfg models.TransformConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("parse transform config: %w", err)
	}

	changes := map[string]any{}

	if cfg.Body != nil {
		pctx.Body = *cfg.Body
		changes["body"] = true
	}
	if cfg.ContentType != nil {
		pctx.ContentType = *cfg.ContentType
		changes["content_type"] = *cfg.ContentType
	}
	if cfg.Status != nil {
		changes["status"] = *cfg.Status
	}

	return &Result{
		Status: "success",
		Data:   changes,
	}, nil
}
