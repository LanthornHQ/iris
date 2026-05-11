package tools

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
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

Behavioral Guidance & Set-of-Mark (SoM):
- Set-of-Mark (SoM) is an advanced visual grounding mechanism. When active (som=true), it overlays high-visibility yellow badges with alphanumeric IDs over all interactive elements on the page.
- Calling agents can subsequently use these IDs directly in interaction tools (such as click with element_id) to bypass raw coordinate estimation entirely.
- SoM badges are dynamic and temporary: they are wiped and redrawn fresh on each screenshot call to reflect the latest interactive elements without accumulating visual noise as the DOM mutates.
- Use som=false to obtain a raw, unannotated screenshot of the webpage when you need a pristine view or are performing pure visual inspection.
- The returned 'som_applied' boolean field indicates whether badges were successfully drawn. If false, the model should fall back to raw coordinate-based interaction.

Page Tree (AXTree):
- The response includes a 'page_tree' field with a compact text representation of the page structure.
- Each line shows: element_id] role "label" (e.g., 'A] button "Submit"')
- Use page_tree for precise element identification; it is cheaper than re-screenshoting just to see what changed.

Coordinate System & Scaling:
- The coordinate system is based on standard CSS viewport pixels. (0,0) is the top-left corner of the page.
- Image coordinates map 1:1 to CSS pixels (not device-dependent Retina physical pixels), meaning that interaction coordinates predicted directly from the screenshot scale correctly across different display setups.`
}

func (t *Screenshot) ParametersSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"som": map[string]any{
				"type":        "boolean",
				"description": "If true (default), draw Set-of-Mark (SoM) badges and return element metadata.",
				"default":     true,
			},
		},
		"required": []string{},
	}
}

func (t *Screenshot) Execute(ctx context.Context, args map[string]any) (any, error) {
	t.Logger.InfoContext(ctx, "screenshot starting")
	start := time.Now()

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("screenshot aborted: %w", err)
	}

	som := true
	explicitSom := false
	if val, ok := args["som"].(bool); ok {
		som = val
		explicitSom = true
	}

	// 1. Extract page tree BEFORE drawing badges — this avoids badge elements
	//    polluting the accessibility tree, and ensures page_tree IDs are assigned
	//    before badges overwrite data-iris-id attributes.
	var pageTree string
	pageTreeLineCount := 0
	if tree, err := t.Driver.PageTree(ctx); err != nil {
		t.Logger.WarnContext(ctx, "page tree extraction failed, continuing without", "error", err)
	} else {
		pageTree = tree
		pageTreeLineCount = len(strings.Split(tree, "\n"))
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("screenshot aborted before capture: %w", err)
	}

	// 2. Draw SoM badges AFTER page tree extraction — badges will be visible in the screenshot.
	var somElems []browser.SomElement
	somApplied := false
	if som {
		if res, err := t.Driver.DrawMarks(ctx); err != nil {
			if explicitSom {
				return nil, fmt.Errorf("som requested but failed to draw marks: %w", err)
			}
			t.Logger.WarnContext(ctx, "failed to draw Set-of-Mark badges", "error", err)
		} else {
			somElems = res
			somApplied = true
		}
	}

	if somApplied && pageTreeLineCount < 5 && len(somElems) > 20 {
		t.Logger.WarnContext(ctx, "sparse page tree: accessibility tree may be incomplete",
			"page_tree_lines", pageTreeLineCount, "som_elements", len(somElems))
	}

	// 3. Capture screenshot with badges visible.
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
		SoMApplied:  somApplied,
		Elements:    somElems,
		PageTree:    pageTree,
	}, nil
}
