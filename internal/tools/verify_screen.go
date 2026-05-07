package tools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/LanthornHQ/iris/internal/browser"
)

type VerifyScreen struct {
	Logger   *slog.Logger
	Driver   browser.Driver
	Verifier VerifyClient
}

func (t *VerifyScreen) Name() string { return "verify_screen" }

func (t *VerifyScreen) Description() string {
	return `Ask the vision model a yes/no question about the current browser page.

Parameters:
  - question (string, required): A yes/no question about what should be visible,
    e.g. "Does the page show a login form?" or "Is the success message displayed?".
  - image_base64 (string, optional): Pre-captured screenshot as base64.
    If provided, skips taking a new screenshot.

Returns: {answer: string, evidence: string, x1, y1, x2, y2: int}
  - answer (string): "yes" or "no".
  - evidence (string): Brief explanation from the model.
  - x1, y1, x2, y2 (int): Bounding box of supporting region in viewport pixels.
    All zeros if the model did not identify a specific region.

Preconditions:
  - IRIS_GROUNDING_URL must be set (grounding model required).

Failure modes:
  - Vision model unreachable or returns invalid response.
  - Screenshot capture fails.`
}

func (t *VerifyScreen) ParametersSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"question": map[string]any{
				"type":        "string",
				"description": "Yes/no question about the current page state",
			},
			"image_base64": map[string]any{
				"type":        "string",
				"description": "Pre-captured screenshot as base64. If provided, skips taking a new screenshot.",
			},
		},
		"required": []string{"question"},
	}
}

func (t *VerifyScreen) Execute(ctx context.Context, args map[string]any) (any, error) {
	if t.Verifier == nil {
		return nil, errors.New("verify_screen: vision model not configured; set IRIS_GROUNDING_URL to enable verify_screen")
	}

	question, _ := args["question"].(string)
	if question == "" {
		return nil, errors.New("question is required")
	}

	start := time.Now()
	t.Logger.InfoContext(ctx, "verify_screen started", "question", question)

	var screenshotB64 string
	var w, h int
	if provided, _ := args["image_base64"].(string); provided != "" {
		screenshotB64 = provided
		w, h = 0, 0
		t.Logger.InfoContext(ctx, "verify_screen: using caller-provided screenshot", "base64_len", len(provided))
	} else {
		var err error
		screenshotB64, w, h, err = t.Driver.Screenshot(ctx)
		if err != nil {
			return nil, fmt.Errorf("verify_screen: screenshot failed: %w", err)
		}
		t.Logger.InfoContext(ctx, "verify_screen: screenshot captured", "width", w, "height", h, "base64_len", len(screenshotB64))
	}

	img := Image{Base64: screenshotB64, Width: w, Height: h}
	answer, evidence, bbox, err := t.Verifier.Verify(ctx, question, img)
	if err != nil {
		return nil, fmt.Errorf("verify_screen: vision model failed: %w", err)
	}

	saveVerifyOverlay(t.Logger, screenshotB64, w, h, bbox, answer, evidence)

	elapsed := time.Since(start)
	t.Logger.InfoContext(ctx, "verify_screen completed",
		"question", question,
		"answer", answer,
		"evidence", evidence,
		"bbox", bbox,
		"elapsed_ms", elapsed.Milliseconds())

	return VerifyScreenResponse{
		Answer:   answer,
		Evidence: evidence,
		X1:       bbox.X1,
		Y1:       bbox.Y1,
		X2:       bbox.X2,
		Y2:       bbox.Y2,
	}, nil
}
