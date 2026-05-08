package tools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

type Sleep struct {
	Logger *slog.Logger
}

func (t *Sleep) Name() string { return "sleep" }

func (t *Sleep) Description() string {
	return `Unconditionally sleep for a specific duration in milliseconds.

Parameters:
  - duration_ms (int, required): Number of milliseconds to sleep.

Returns: {success: bool}

Use this when you explicitly need to wait for a fixed amount of time.
Prefer wait_for_stable when waiting for page content to settle.

Failure modes:
  - Context cancellation (returns early).`
}

func (t *Sleep) ParametersSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"duration_ms": map[string]any{
				"type":        "integer",
				"description": "Sleep duration in milliseconds",
			},
		},
		"required": []string{"duration_ms"},
	}
}

const maxSleepMs = 60_000

func (t *Sleep) Execute(ctx context.Context, args map[string]any) (any, error) {
	durationMs, err := intArg(args, "duration_ms")
	if err != nil {
		return nil, err
	}
	if durationMs < 0 {
		return nil, errors.New("duration_ms must be non-negative")
	}
	if durationMs > maxSleepMs {
		return nil, fmt.Errorf("duration_ms %d exceeds maximum of %d", durationMs, maxSleepMs)
	}

	t.Logger.InfoContext(ctx, "sleep starting", "duration_ms", durationMs)

	select {
	case <-ctx.Done():
		t.Logger.InfoContext(ctx, "sleep cancelled")
		return nil, ctx.Err()
	case <-time.After(time.Duration(durationMs) * time.Millisecond):
	}

	t.Logger.InfoContext(ctx, "sleep completed", "duration_ms", durationMs)
	return SleepResponse{Success: true}, nil
}
