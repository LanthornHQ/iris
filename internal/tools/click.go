package tools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/LanthornHQ/iris/internal/browser"
)

type Click struct {
	Logger          *slog.Logger
	Driver          browser.Driver
	AnnotateDefault bool
}

func (t *Click) Name() string { return "click" }

func (t *Click) Description() string {
	return `Click at specific viewport coordinates or a Set-of-Mark (SoM) element ID in the browser.

Parameters:
  - element_id (string, optional): The ID of the Set-of-Mark (SoM) badge on the element to click. If specified, x and y are ignored.
  - x (int, optional): X coordinate in viewport pixels. Required if element_id is not specified.
  - y (int, optional): Y coordinate in viewport pixels. Required if element_id is not specified.
  - button (string, optional): Mouse button. One of "left" (default), "right", "middle".
    Use "double" for a double-click.
  - annotate (boolean, optional): If true, a red dot marks the click position on the returned screenshot.
    Defaults to the server-wide default (determined by IRIS_ANNOTATE_CLICKS).

Returns: {success: bool, image_base64: string, width: int, height: int}
  - success: true if the click was injected successfully.
  - image_base64: post-click screenshot (base64 JPEG).
  - width/height: dimensions of the screenshot.

Coordinate system: Viewport pixels. (0,0) is the top-left corner of the page.
The agent should get coordinates from a prior screenshot + grounding call, or use the Set-of-Mark element_id.

Failure modes:
  - Coordinates out of viewport bounds.
  - Specified element_id not found on screen.
  - Browser not responding.`
}

func (t *Click) ParametersSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"x": map[string]any{
				"type":        "number",
				"description": "X coordinate in viewport pixels. Required if element_id is not specified.",
			},
			"y": map[string]any{
				"type":        "number",
				"description": "Y coordinate in viewport pixels. Required if element_id is not specified.",
			},
			"element_id": map[string]any{
				"type":        "string",
				"description": "Optional Set-of-Mark (SoM) badge ID (e.g. A, B, AA) to click instead of explicit coordinates",
			},
			"button": map[string]any{
				"type":        "string",
				"enum":        []string{"left", "right", "middle", "double"},
				"description": "Mouse button: left (default), right, middle, double",
				"default":     "left",
			},
			"annotate": map[string]any{
				"type":        "boolean",
				"description": "If true, draw a red dot at the click coordinates on the returned screenshot",
			},
		},
	}
}

func (t *Click) resolveCoordinates(ctx context.Context, args map[string]any) (int, int, error) {
	if elementID, ok := args["element_id"]; ok {
		var sid string
		switch v := elementID.(type) {
		case string:
			sid = v
		case float64:
			sid = strconv.Itoa(int(v))
		default:
			return 0, 0, fmt.Errorf("element_id must be a string, got %T", elementID)
		}
		if sid == "" {
			return 0, 0, errors.New("element_id must be a non-empty string")
		}
		x, y, err := t.Driver.GetElementCoords(ctx, sid)
		if err != nil {
			return 0, 0, fmt.Errorf("locating element %s: %w", sid, err)
		}
		t.Logger.InfoContext(ctx, "resolved element_id to coordinates", "element_id", sid, "x", x, "y", y)
		return x, y, nil
	}

	x, err := intArg(args, "x")
	if err != nil {
		return 0, 0, err
	}
	y, err := intArg(args, "y")
	if err != nil {
		return 0, 0, err
	}
	return x, y, nil
}

func (t *Click) Execute(ctx context.Context, args map[string]any) (any, error) {
	x, y, err := t.resolveCoordinates(ctx, args)
	if err != nil {
		return nil, err
	}

	button, _ := args["button"].(string)
	if button == "" {
		button = "left"
	}

	annotate := t.AnnotateDefault
	if val, ok := args["annotate"].(bool); ok {
		annotate = val
	}

	t.Logger.InfoContext(ctx, "click starting", "x", x, "y", y, "button", button, "annotate", annotate)
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

	if annotate {
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
