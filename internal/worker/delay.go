package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/chrishaylesai/hookwatch/internal/models"
)

const maxDelayMs = 30000

// DelayExecutor pauses the pipeline for a configured duration.
type DelayExecutor struct{}

func (e *DelayExecutor) Execute(ctx context.Context, req *models.Request, pctx *PipelineContext, config json.RawMessage) (*Result, error) {
	var cfg models.DelayConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("parse delay config: %w", err)
	}

	duration := cfg.DurationMs
	if duration > maxDelayMs {
		duration = maxDelayMs
	}

	timer := time.NewTimer(time.Duration(duration) * time.Millisecond)
	defer timer.Stop()

	select {
	case <-timer.C:
		return &Result{
			Status: "success",
			Data: map[string]any{
				"delayed_ms": duration,
			},
		}, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("delay cancelled: %w", ctx.Err())
	}
}
