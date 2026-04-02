package worker

import (
	"context"
	"encoding/json"

	"github.com/chrishaylesai/hookwatch/internal/models"
)

// PipelineContext carries mutable data through the action pipeline.
// Transform actions modify this context; forward actions read from it.
type PipelineContext struct {
	Method      string
	Body        string
	ContentType string
	Headers     map[string]string
	Query       string
	IP          string
	URL         string
}

// Result is the outcome of executing an action.
type Result struct {
	Status       string         // "success", "failed", "skipped"
	Data         map[string]any // executor-specific result data
	StopPipeline bool           // true if filter condition not met
}

// Executor executes a single action in the pipeline.
type Executor interface {
	Execute(ctx context.Context, req *models.Request, pctx *PipelineContext, config json.RawMessage) (*Result, error)
}
