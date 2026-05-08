package tools

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/LanthornHQ/iris/internal/browser"
)

type Screenshot struct {
	Logger *slog.Logger
	Driver browser.Driver
}

func (t *Screenshot) Name() string { return "screenshot" }

func (t *Screenshot) Description() string {
	return `Capture a screenshot of the current browser viewport.

Parameters: (none)

Returns: {image_base64: string, width: int, height: int}
  - image_base64 (string): JPEG image encoded as base64 (~85% quality).
  - width (int): Width of the captured image in pixels.
  - height (int): Height of the captured image in pixels.

The screenshot represents exactly what the browser renders — no OS desktop capture, no window management.

Failure modes:
  - Browser not running or page not loaded.`
}

func (t *Screenshot) ParametersSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *Screenshot) Execute(ctx context.Context, _ map[string]any) (any, error) {
	t.Logger.InfoContext(ctx, "screenshot starting")
	start := time.Now()

	if err := t.Driver.DrawMarks(ctx); err != nil {
		t.Logger.WarnContext(ctx, "failed to draw Set-of-Mark badges", "error", err)
	}

	b64, w, h, err := t.Driver.Screenshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("screenshot failed: %w", err)
	}

	elapsed := time.Since(start)
	t.Logger.InfoContext(ctx, "screenshot captured", "width", w, "height", h, "base64_len", len(b64), "elapsed_ms", elapsed.Milliseconds())

	return ScreenshotResponse{
		ImageBase64: b64,
		Width:       w,
		Height:      h,
	}, nil
}
