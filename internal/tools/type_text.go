package tools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/LanthornHQ/iris/internal/browser"
)

const (
	defaultDelayMs     = 10
	centerDivisor      = 2
	focusWaitTimeoutMs = 150
)

type TypeText struct {
	Logger    *slog.Logger
	Driver    browser.Driver
	Grounding GroundingClient
}

func (t *TypeText) Name() string { return "type_text" }

func (t *TypeText) Description() string {
	return `Type text into the currently focused element, or into a specific element found by description.

When 'description' is provided, this tool takes a screenshot, uses the vision grounding
model to locate the element, clicks it to focus, waits briefly, then types the text.
When 'description' is omitted, types into whatever currently has focus.

Parameters:
  - text (string, required): The text to type. Supports special keys in curly braces:
    {Enter}, {Tab}, {Escape}, {Backspace}, {Delete}, {Up}, {Down}, {Left}, {Right},
    {Home}, {End}, {PageUp}, {PageDown}, {Ctrl+A}, {Ctrl+C}, {Ctrl+V}, etc.
    Literal braces: {{ and }}.
  - description (string, optional): Natural-language description of the target element
    to click before typing, e.g. "the search box". Uses vision grounding.
  - delay_ms (int, optional): Delay between keystrokes in milliseconds. Default: 10.

Returns: {success: bool, ui_changed: bool, image_base64, screen_width, screen_height}
  When description is provided, also returns: {x, y, width, height, method, confidence}

Failure modes:
  - Input injection failure.
  - When description provided: element not found or click failure.`
}

func (t *TypeText) ParametersSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "Text to type; use {Enter}, {Tab}, {Ctrl+C}, etc. for special keys",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Natural-language description of the element to click before typing. Omit to type into focused element.",
			},
			"delay_ms": map[string]any{
				"type":        "integer",
				"description": "Delay between keystrokes in milliseconds (default: 10)",
				"default":     defaultDelayMs,
			},
		},
		"required": []string{"text"},
	}
}

//nolint:gocognit // Grounding-then-type flow is inherently branching
func (t *TypeText) Execute(ctx context.Context, args map[string]any) (any, error) {
	text, _ := args["text"].(string)
	if text == "" {
		return nil, errors.New("text is required")
	}
	description, _ := args["description"].(string)
	delayMs := optIntArg(args, "delay_ms", defaultDelayMs)
	if delayMs < 0 {
		delayMs = 0
	}

	start := time.Now()
	t.Logger.InfoContext(ctx, "type_text starting", "text_len", len(text), "description", description, "delay_ms", delayMs)

	resp := TypeTextResponse{Success: true}

	if description != "" {
		if t.Grounding == nil {
			return nil, errors.New("type_text: grounding model not configured; set IRIS_GROUNDING_URL to enable description-based typing")
		}

		b64, w, h, ssErr := t.Driver.Screenshot(ctx)
		if ssErr != nil {
			return nil, fmt.Errorf("type_text: screenshot for grounding failed: %w", ssErr)
		}

		img := Image{Base64: b64, Width: w, Height: h}
		bbox, gErr := t.Grounding.Ground(ctx, description, "type", img)
		if gErr != nil {
			return nil, fmt.Errorf("type_text: grounding failed: %w", gErr)
		}

		cx := (bbox.X1 + bbox.X2) / centerDivisor
		cy := (bbox.Y1 + bbox.Y2) / centerDivisor
		bw := bbox.X2 - bbox.X1
		bh := bbox.Y2 - bbox.Y1

		resp.X = cx
		resp.Y = cy
		resp.Width = bw
		resp.Height = bh
		resp.Method = "vision"
		resp.Confidence = ConfidenceVision

		if err := t.Driver.Click(ctx, cx, cy); err != nil {
			return nil, fmt.Errorf("type_text: click on element failed: %w", err)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(focusWaitTimeoutMs * time.Millisecond):
		}
	}

	if err := t.Driver.Type(ctx, text, delayMs); err != nil {
		return nil, fmt.Errorf("type_text failed: %w", err)
	}

	if b64, w, h, ssErr := t.Driver.Screenshot(ctx); ssErr == nil {
		resp.ImageBase64 = b64
		resp.ScreenWidth = w
		resp.ScreenHeight = h

		if os.Getenv("IRIS_ANNOTATE_CLICKS") == "1" && resp.X != 0 && resp.Y != 0 {
			if annotated, annErr := drawClickDot(b64, resp.X, resp.Y, w, h); annErr == nil {
				resp.ImageBase64 = annotated
			}
		}
	}

	elapsed := time.Since(start)
	t.Logger.InfoContext(ctx, "type_text completed", "text_len", len(text), "description", description, "elapsed_ms", elapsed.Milliseconds())

	return resp, nil
}
