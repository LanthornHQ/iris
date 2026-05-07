package browser

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"log/slog"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

type chromedpDriver struct {
	browserCtx context.Context
	cancel     context.CancelFunc
	logger     *slog.Logger
	timeout    time.Duration
}

func newChromedpDriver(ctx context.Context, logger *slog.Logger, cfg Config) (*chromedpDriver, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", cfg.Headless),
		chromedp.Flag("no-sandbox", cfg.NoSandbox),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.WindowSize(cfg.Width, cfg.Height),
		chromedp.Flag("hide-scrollbars", true),
		chromedp.Flag("mute-audio", true),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-notifications", true),
		chromedp.Flag("disable-backgrounding-occluded-windows", true),
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-renderer-backgrounding", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-component-update", true),
		chromedp.Flag("disable-domain-reliability", true),
		chromedp.Flag("disable-crash-reporter", true),
		chromedp.Flag("password-store", "basic"),
	)

	if cfg.ChromePath != "" {
		opts = append(opts, chromedp.ExecPath(cfg.ChromePath))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)

	browserCtx, browserCancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(func(s string, i ...interface{}) {
		logger.Debug("chromedp", "msg", fmt.Sprintf(s, i...))
	}))

	timeout := 30 * time.Second
	if cfg.TimeoutMs > 0 {
		timeout = time.Duration(cfg.TimeoutMs) * time.Millisecond
	}

	d := &chromedpDriver{
		browserCtx: browserCtx,
		cancel:     func() { browserCancel(); allocCancel() },
		logger:     logger,
		timeout:    timeout,
	}

	logger.InfoContext(ctx, "starting headless Chrome",
		"headless", cfg.Headless,
		"window_size", fmt.Sprintf("%dx%d", cfg.Width, cfg.Height),
		"no_sandbox", cfg.NoSandbox,
		"timeout", timeout)

	if err := chromedp.Run(browserCtx,
		chromedp.Navigate("about:blank"),
	); err != nil {
		browserCancel()
		allocCancel()
		return nil, fmt.Errorf("failed to start Chrome: %w", err)
	}

	logger.InfoContext(ctx, "headless Chrome started successfully")
	return d, nil
}

func (d *chromedpDriver) Navigate(ctx context.Context, url string) error {
	d.logger.InfoContext(ctx, "navigate", "url", url)
	actionCtx, cancel := d.withTimeout(ctx)
	defer cancel()
	return chromedp.Run(actionCtx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body", chromedp.ByQuery),
	)
}

func (d *chromedpDriver) Click(ctx context.Context, x, y int) error {
	d.logger.InfoContext(ctx, "click", "x", x, "y", y)
	actionCtx, cancel := d.withTimeout(ctx)
	defer cancel()
	return chromedp.Run(actionCtx,
		mouseMove(x, y),
		mousePress(x, y, 1),
		mouseRelease(x, y, 1),
	)
}

func (d *chromedpDriver) DoubleClick(ctx context.Context, x, y int) error {
	d.logger.InfoContext(ctx, "double_click", "x", x, "y", y)
	actionCtx, cancel := d.withTimeout(ctx)
	defer cancel()
	return chromedp.Run(actionCtx,
		mouseMove(x, y),
		mousePress(x, y, 2),
		mouseRelease(x, y, 2),
	)
}

func (d *chromedpDriver) Type(ctx context.Context, text string, delayMs int) error {
	d.logger.InfoContext(ctx, "type_text", "text_len", len(text), "delay_ms", delayMs)
	actionCtx, cancel := d.withTimeout(ctx)
	defer cancel()

	return chromedp.Run(actionCtx,
		chromedp.SendKeys("document", text, chromedp.ByJSPath),
	)
}

func (d *chromedpDriver) Scroll(ctx context.Context, direction string, clicks int) error {
	d.logger.InfoContext(ctx, "scroll", "direction", direction, "clicks", clicks)
	actionCtx, cancel := d.withTimeout(ctx)
	defer cancel()

	var delta int
	switch direction {
	case "up":
		delta = -clicks * 300
	default:
		delta = clicks * 300
	}

	js := fmt.Sprintf("window.scrollBy(0, %d)", delta)
	return chromedp.Run(actionCtx,
		chromedp.Evaluate(js, nil),
	)
}

func (d *chromedpDriver) Screenshot(ctx context.Context) (string, int, int, error) {
	d.logger.DebugContext(ctx, "screenshot starting")
	actionCtx, cancel := d.withTimeout(ctx)
	defer cancel()

	var buf []byte
	if err := chromedp.Run(actionCtx,
		chromedp.FullScreenshot(&buf, 100),
	); err != nil {
		return "", 0, 0, fmt.Errorf("screenshot failed: %w", err)
	}

	var jpgBuf bytes.Buffer
	img, _, err := image.Decode(bytes.NewReader(buf))
	if err != nil {
		return "", 0, 0, fmt.Errorf("screenshot: image decode failed: %w", err)
	}

	if err := jpeg.Encode(&jpgBuf, img, &jpeg.Options{Quality: 85}); err != nil {
		return "", 0, 0, fmt.Errorf("jpeg encode failed: %w", err)
	}

	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	encoded := base64.StdEncoding.EncodeToString(jpgBuf.Bytes())
	d.logger.DebugContext(ctx, "screenshot captured", "width", w, "height", h, "base64_len", len(encoded))
	return encoded, w, h, nil
}

func (d *chromedpDriver) WaitForStable(ctx context.Context, timeoutMs int, threshold float64) (bool, int64, error) {
	d.logger.InfoContext(ctx, "wait_for_stable starting", "timeout_ms", timeoutMs, "threshold", threshold)
	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	pollInterval := 200 * time.Millisecond
	start := time.Now()

	var prevB64 string
	iter := 0

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false, time.Since(start).Milliseconds(), ctx.Err()
		default:
		}

		actionCtx, cancel := d.withTimeout(ctx)
		b64, _, _, err := d.Screenshot(actionCtx)
		cancel()
		if err != nil {
			return false, 0, fmt.Errorf("wait_for_stable: screenshot failed: %w", err)
		}
		iter++

		if prevB64 != "" {
			if b64 == prevB64 {
				elapsed := time.Since(start)
				d.logger.InfoContext(ctx, "wait_for_stable: screen stable (identical screenshots)", "iterations", iter, "elapsed_ms", elapsed.Milliseconds())
				return true, elapsed.Milliseconds(), nil
			}
		}
		prevB64 = b64

		select {
		case <-ctx.Done():
			return false, time.Since(start).Milliseconds(), ctx.Err()
		case <-time.After(pollInterval):
		}
	}

	elapsed := time.Since(start)
	d.logger.InfoContext(ctx, "wait_for_stable: timeout reached", "timeout_ms", timeoutMs, "iterations", iter, "actual_ms", elapsed.Milliseconds())
	return false, elapsed.Milliseconds(), nil
}

func (d *chromedpDriver) Title(ctx context.Context) (string, error) {
	d.logger.DebugContext(ctx, "get title starting")
	actionCtx, cancel := d.withTimeout(ctx)
	defer cancel()

	var title string
	if err := chromedp.Run(actionCtx, chromedp.Title(&title)); err != nil {
		return "", fmt.Errorf("failed to get title: %w", err)
	}
	return title, nil
}

func (d *chromedpDriver) Close() error {
	d.logger.Info("closing browser")
	d.cancel()
	return nil
}

func (d *chromedpDriver) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	actionCtx, cancel := context.WithTimeout(d.browserCtx, d.timeout)
	if ctx.Done() != nil {
		go func() {
			select {
			case <-ctx.Done():
				cancel()
			case <-actionCtx.Done():
			}
		}()
	}
	return actionCtx, cancel
}

func mouseMove(x, y int) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		return input.DispatchMouseEvent(input.MouseMoved, float64(x), float64(y)).Do(ctx)
	})
}

func mousePress(x, y, clickCount int) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		return input.DispatchMouseEvent(input.MousePressed, float64(x), float64(y)).WithButton(input.Left).WithClickCount(int64(clickCount)).Do(ctx)
	})
}

func mouseRelease(x, y, clickCount int) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		return input.DispatchMouseEvent(input.MouseReleased, float64(x), float64(y)).WithButton(input.Left).WithClickCount(int64(clickCount)).Do(ctx)
	})
}
