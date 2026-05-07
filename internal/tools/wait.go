package tools

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/LanthornHQ/iris/internal/browser"
)

const maxWaitTimeoutMs = 3_600_000

type WaitForStable struct {
	Logger *slog.Logger
	Driver browser.Driver
}

func (t *WaitForStable) Name() string { return "wait_for_stable" }

func (t *WaitForStable) Description() string {
	return `Poll the browser viewport until the page is stable (no visual changes) or timeout.

Parameters:
  - timeout_ms (int, optional): Maximum time to wait in milliseconds. Default: 5000.

Returns: {stable: bool, elapsed_ms: int}
  - stable (bool): True if the page became stable before timeout.
  - elapsed_ms (int): How many milliseconds were spent waiting.

Use this between actions to ensure page loads, animations, or transitions have settled.

Algorithm: Takes consecutive screenshots at 200ms intervals. If two consecutive
screenshots are identical, the page is considered stable.

Failure modes:
  - Screenshot capture failure.
  - Timeout (returns stable=false, not an error).`
}

func (t *WaitForStable) ParametersSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"timeout_ms": map[string]any{
				"type":        "integer",
				"description": "Maximum wait time in milliseconds (default: 5000)",
				"default":     5000,
			},
		},
	}
}

func (t *WaitForStable) Execute(ctx context.Context, args map[string]any) (any, error) {
	timeoutMs := optIntArg(args, "timeout_ms", 5000)
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}
	if timeoutMs > maxWaitTimeoutMs {
		return nil, fmt.Errorf("timeout_ms %d exceeds maximum of %d", timeoutMs, maxWaitTimeoutMs)
	}

	t.Logger.InfoContext(ctx, "wait_for_stable starting", "timeout_ms", timeoutMs)

	stable, elapsedMs, err := t.Driver.WaitForStable(ctx, timeoutMs, 0.01)
	if err != nil {
		return nil, fmt.Errorf("wait_for_stable: %w", err)
	}

	t.Logger.InfoContext(ctx, "wait_for_stable completed", "stable", stable, "elapsed_ms", elapsedMs)
	return WaitForStableResponse{
		Stable:    stable,
		ElapsedMs: elapsedMs,
	}, nil
}
