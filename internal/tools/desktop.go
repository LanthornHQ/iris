package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image/png"
	"log/slog"
	"os"
	"os/exec"
	"time"
)

// DesktopScreenshot captures the full Xvfb display using scrot, including
// browser chrome and any overlapping UI (e.g. password manager popups).
type DesktopScreenshot struct {
	Logger *slog.Logger
}

func (t *DesktopScreenshot) Name() string { return "desktop_screenshot" }

func (t *DesktopScreenshot) Description() string {
	return `Capture a full desktop screenshot of the Xvfb display (browser chrome + all overlays).
Use this when you need to see what is actually on screen, including popups, modals, or
OS-level overlays that may be blocking clicks. Unlike 'screenshot', this captures the
entire display — not just the browser viewport.
Returns: {image_base64: string, width: int, height: int}`
}

func (t *DesktopScreenshot) ParametersSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *DesktopScreenshot) Execute(ctx context.Context, _ map[string]any) (any, error) {
	t.Logger.InfoContext(ctx, "desktop screenshot starting")
	start := time.Now()

	path := fmt.Sprintf("/tmp/iris-desktop-%d.png", start.UnixMilli())

	cmd := exec.CommandContext(ctx, "scrot", "--silent", path)
	cmd.Env = append(os.Environ(), "DISPLAY=:99")
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("scrot failed: %w: %s", err, out)
	}
	defer os.Remove(path)

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading desktop screenshot: %w", err)
	}

	pngCfg, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decoding desktop screenshot config: %w", err)
	}

	b64 := base64.StdEncoding.EncodeToString(raw)
	elapsed := time.Since(start)
	t.Logger.InfoContext(ctx, "desktop screenshot captured",
		"bytes", len(raw), "elapsed_ms", elapsed.Milliseconds())

	return map[string]any{
		"image_base64": b64,
		"width":        pngCfg.Width,
		"height":       pngCfg.Height,
	}, nil
}
