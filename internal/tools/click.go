package tools

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/LanthornHQ/iris/internal/browser"
)

type Click struct {
	Logger *slog.Logger
	Driver browser.Driver
}

func (t *Click) Name() string { return "click" }

func (t *Click) Description() string {
	return `Click at specific viewport coordinates in the browser.

Parameters:
  - x (int, required): X coordinate in viewport pixels.
  - y (int, required): Y coordinate in viewport pixels.
  - button (string, optional): Mouse button. One of "left" (default), "right", "middle".
    Use "double" for a double-click.

Returns: {success: bool, image_base64: string, width: int, height: int}
  - success: true if the click was injected successfully.
  - image_base64: post-click screenshot (base64 JPEG). When IRIS_ANNOTATE_CLICKS=1,
    a red dot marks the click position.
  - width/height: dimensions of the screenshot.

Coordinate system: Viewport pixels. (0,0) is the top-left corner of the page.
The agent should get coordinates from a prior screenshot + grounding call.

Failure modes:
  - Coordinates out of viewport bounds.
  - Browser not responding.`
}

func (t *Click) ParametersSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"x": map[string]any{
				"type":        "number",
				"description": "X coordinate in viewport pixels",
			},
			"y": map[string]any{
				"type":        "number",
				"description": "Y coordinate in viewport pixels",
			},
			"button": map[string]any{
				"type":        "string",
				"enum":        []string{"left", "right", "middle", "double"},
				"description": "Mouse button: left (default), right, middle, double",
				"default":     "left",
			},
		},
		"required": []string{"x", "y"},
	}
}

func (t *Click) Execute(ctx context.Context, args map[string]any) (any, error) {
	x, err := intArg(args, "x")
	if err != nil {
		return nil, err
	}
	y, err := intArg(args, "y")
	if err != nil {
		return nil, err
	}
	button, _ := args["button"].(string)
	if button == "" {
		button = "left"
	}

	t.Logger.InfoContext(ctx, "click starting", "x", x, "y", y, "button", button)
	start := time.Now()

	isDoubleClick := button == "double"

	if isDoubleClick {
		if err := t.Driver.DoubleClick(ctx, x, y); err != nil {
			return nil, fmt.Errorf("click failed: %w", err)
		}
	} else {
		if err := t.Driver.Click(ctx, x, y, button); err != nil {
			return nil, fmt.Errorf("click failed: %w", err)
		}
	}

	resp := ClickResponse{Success: true}

	b64, w, h, ssErr := t.Driver.Screenshot(ctx)
	if ssErr != nil {
		t.Logger.DebugContext(ctx, "click: post-click screenshot failed", "error", ssErr)
		return resp, nil
	}

	resp.ImageBase64 = b64
	resp.Width = w
	resp.Height = h

	if os.Getenv("IRIS_ANNOTATE_CLICKS") == "1" {
		annotated, annErr := drawClickDot(b64, x, y, w, h)
		if annErr == nil {
			resp.ImageBase64 = annotated
		} else {
			t.Logger.DebugContext(ctx, "click: dot overlay failed, returning raw screenshot", "error", annErr)
		}
	}

	elapsed := time.Since(start)
	t.Logger.InfoContext(ctx, "click completed", "x", x, "y", y, "button", button, "elapsed_ms", elapsed.Milliseconds())

	return resp, nil
}
