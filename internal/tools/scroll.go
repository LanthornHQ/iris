package tools

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/LanthornHQ/iris/internal/browser"
)

const defaultScrollClicks = 3

type Scroll struct {
	Logger *slog.Logger
	Driver browser.Driver
}

func (t *Scroll) Name() string { return "scroll" }

func (t *Scroll) Description() string {
	return `Scroll the browser page up or down.

Parameters:
  - direction (string, optional): "down" (default) or "up".
  - clicks (int, optional): Number of scroll steps. Default: 3.

Returns: {success: bool, image_base64: string, screen_width: int, screen_height: int}
  - image_base64: screenshot after scrolling (base64 JPEG).

The scroll amount per step is approximately 300px (one viewport-height step).`
}

func (t *Scroll) ParametersSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"direction": map[string]any{
				"type":        "string",
				"enum":        []string{"down", "up"},
				"description": "Scroll direction: down (default) or up",
				"default":     "down",
			},
			"clicks": map[string]any{
				"type":        "integer",
				"description": "Number of scroll steps (default: 3)",
				"default":     defaultScrollClicks,
			},
		},
	}
}

func (t *Scroll) Execute(ctx context.Context, args map[string]any) (any, error) {
	direction := "down"
	if d, ok := args["direction"].(string); ok && d != "" {
		direction = d
	}
	clicks := optIntArg(args, "clicks", defaultScrollClicks)

	t.Logger.InfoContext(ctx, "scroll starting", "direction", direction, "clicks", clicks)
	start := time.Now()

	if err := t.Driver.Scroll(ctx, direction, clicks); err != nil {
		return nil, fmt.Errorf("scroll failed: %w", err)
	}

	resp := ScrollResponse{Success: true}

	if b64, w, h, ssErr := t.Driver.Screenshot(ctx); ssErr == nil {
		resp.ImageBase64 = b64
		resp.ScreenWidth = w
		resp.ScreenHeight = h
	}

	elapsed := time.Since(start)
	t.Logger.InfoContext(ctx, "scroll completed", "direction", direction, "clicks", clicks, "elapsed_ms", elapsed.Milliseconds())

	return resp, nil
}
