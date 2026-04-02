package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/chrishaylesai/hookwatch/internal/models"
)

// FilterExecutor evaluates a condition against the captured request.
// If the condition is not met, it stops the pipeline.
type FilterExecutor struct{}

func (e *FilterExecutor) Execute(ctx context.Context, req *models.Request, pctx *PipelineContext, config json.RawMessage) (*Result, error) {
	var cfg models.FilterConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("parse filter config: %w", err)
	}

	fieldValue, fieldExists := resolveField(cfg.Field, req, pctx)
	matched := evaluate(cfg.Operator, fieldValue, cfg.Value, fieldExists)

	if cfg.Negate {
		matched = !matched
	}

	if matched {
		return &Result{
			Status: "success",
			Data: map[string]any{
				"matched": true,
				"field":   cfg.Field,
			},
		}, nil
	}

	return &Result{
		Status:       "skipped",
		StopPipeline: true,
		Data: map[string]any{
			"matched": false,
			"field":   cfg.Field,
		},
	}, nil
}

func resolveField(field string, req *models.Request, pctx *PipelineContext) (string, bool) {
	switch field {
	case "method":
		return req.Method, true
	case "ip":
		return req.IP, true
	case "content":
		return pctx.Body, pctx.Body != ""
	default:
		if strings.HasPrefix(field, "header.") {
			name := strings.TrimPrefix(field, "header.")
			if v, ok := pctx.Headers[name]; ok {
				return v, true
			}
			// Try case-insensitive match
			lower := strings.ToLower(name)
			for k, v := range pctx.Headers {
				if strings.ToLower(k) == lower {
					return v, true
				}
			}
			return "", false
		}
		if strings.HasPrefix(field, "query.") {
			name := strings.TrimPrefix(field, "query.")
			return resolveQueryParam(name, req.Query)
		}
		return "", false
	}
}

func resolveQueryParam(name, queryJSON string) (string, bool) {
	var params map[string]any
	if err := json.Unmarshal([]byte(queryJSON), &params); err != nil {
		return "", false
	}
	v, ok := params[name]
	if !ok {
		return "", false
	}
	switch val := v.(type) {
	case string:
		return val, true
	default:
		encoded, _ := json.Marshal(val)
		return string(encoded), true
	}
}

func evaluate(operator, fieldValue, testValue string, fieldExists bool) bool {
	switch operator {
	case "equals":
		return fieldValue == testValue
	case "contains":
		return strings.Contains(fieldValue, testValue)
	case "matches":
		re, err := regexp.Compile(testValue)
		if err != nil {
			return false
		}
		return re.MatchString(fieldValue)
	case "exists":
		return fieldExists
	default:
		return false
	}
}
