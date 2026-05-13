package tools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/LanthornHQ/iris/internal/browser"
)

const defaultDelayMs = 10

type TypeText struct {
	Logger *slog.Logger
	Driver browser.Driver
}

func (t *TypeText) Name() string { return "type_text" }

func (t *TypeText) Description() string {
	return `Type text into the currently focused element.

Parameters:
  - text (string, required): The text to type. Supports special keys in curly braces:
    {Enter}, {Tab}, {Escape}, {Backspace}, {Delete}, {Up}, {Down}, {Left}, {Right},
    {Home}, {End}, {PageUp}, {PageDown}, {Ctrl+A}, {Ctrl+C}, {Ctrl+V}, etc.
    Literal braces: {{ and }}.
  - delay_ms (int, optional): Delay between keystrokes in milliseconds. Default: 10.

Returns: {success: bool, image_base64, screen_width, screen_height}

Failure modes:
  - Input injection failure.`
}

func (t *TypeText) ParametersSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "Text to type; use {Enter}, {Tab}, {Ctrl+C}, etc. for special keys",
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

func (t *TypeText) Execute(ctx context.Context, args map[string]any) (any, error) {
	text, _ := args["text"].(string)
	if text == "" {
		return nil, errors.New("text is required")
	}
	delayMs := max(0, optIntArg(args, "delay_ms", defaultDelayMs))

	start := time.Now()
	t.Logger.InfoContext(ctx, "type_text starting", "text_len", len(text), "delay_ms", delayMs)

	if err := t.Driver.Type(ctx, text, delayMs); err != nil {
		return nil, fmt.Errorf("type_text failed: %w", err)
	}

	resp := TypeTextResponse{Success: true}

	if b64, w, h, ssErr := t.Driver.Screenshot(ctx); ssErr == nil {
		resp.ImageBase64 = b64
		resp.ScreenWidth = w
		resp.ScreenHeight = h
	}

	elapsed := time.Since(start)
	t.Logger.InfoContext(ctx, "type_text completed", "text_len", len(text), "elapsed_ms", elapsed.Milliseconds())

	return resp, nil
}
