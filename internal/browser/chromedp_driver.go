package browser

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image/jpeg"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

const (
	defaultJPEGQuality  = 85
	defaultPollInterval = 200 * time.Millisecond
	defaultTimeout      = 30 * time.Second
	doubleClickCount    = 2
	scrollDeltaPerClick = 300
)

type chromedpDriver struct {
	browserCtx context.Context
	cancel     context.CancelFunc
	logger     *slog.Logger
	timeout    time.Duration
}

var stealthHWOptions = []int{4, 8, 16}

// buildStealthJS generates stealth override JS with randomized hardware fingerprint values
// to avoid sessions sharing an identical browser signature.
func buildStealthJS(hwConcurrency, deviceMemory int) string {
	return fmt.Sprintf(`(function() {
	Object.defineProperty(navigator, 'webdriver', {get: () => undefined});
	Object.defineProperty(navigator, 'languages', {get: () => ['en-US', 'en']});
	Object.defineProperty(navigator, 'plugins', {get: () => {
		var arr = [1, 2, 3, 4, 5];
		arr.item = function(i) { return this[i]; };
		arr.namedItem = function(name) { return null; };
		arr.refresh = function() {};
		Object.setPrototypeOf(arr, PluginArray.prototype);
		return arr;
	}});
	Object.defineProperty(navigator, 'hardwareConcurrency', {get: () => %d});
	Object.defineProperty(navigator, 'deviceMemory', {get: () => %d});
	window.chrome = {runtime: {}, loadTimes: function(){}, csi: function(){} };
	const originalQuery = window.navigator.permissions.query;
	window.navigator.permissions.query = (parameters) => (
		parameters.name === 'notifications' ?
			Promise.resolve({state: (typeof Notification !== 'undefined' ? Notification.permission : 'default')}) :
			originalQuery(parameters)
	);
	const getParameter = WebGLRenderingContext.prototype.getParameter;
	WebGLRenderingContext.prototype.getParameter = function(parameter) {
		if (parameter === 37445) return 'Google Inc. (NVIDIA)';
		if (parameter === 37446) return 'ANGLE (NVIDIA, NVIDIA GeForce GTX 1060, OpenGL 4.5)';
		return getParameter.call(this, parameter);
	};
	const origAttachShadow = Element.prototype.attachShadow;
	Element.prototype.attachShadow = function() {
		return origAttachShadow.apply(this, arguments);
	};
})();`, hwConcurrency, deviceMemory)
}

func buildAllocatorOpts(cfg Config) []chromedp.ExecAllocatorOption {
	if cfg.Stealth {
		opts := []chromedp.ExecAllocatorOption{
			chromedp.Flag("disable-gpu", true),
			chromedp.Flag("no-sandbox", cfg.NoSandbox),
			chromedp.Flag("disable-dev-shm-usage", true),
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
			chromedp.Flag("disable-blink-features", "AutomationControlled"),
			chromedp.Flag("user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"),
		}
		if cfg.Headless {
			opts = append(opts, chromedp.Flag("headless", "new"))
		}
		return opts
	}

	opts := chromedp.DefaultExecAllocatorOptions[:]
	if cfg.Headless {
		opts = append(opts, chromedp.Headless)
	}
	opts = append(opts,
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
		chromedp.Flag("user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"),
	)
	return opts
}

func newChromedpDriver(ctx context.Context, logger *slog.Logger, cfg Config) (*chromedpDriver, error) {
	allocatorOpts := buildAllocatorOpts(cfg)

	if cfg.ChromePath != "" {
		allocatorOpts = append(allocatorOpts, chromedp.ExecPath(cfg.ChromePath))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, allocatorOpts...)

	browserCtx, browserCancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(func(s string, i ...any) {
		logger.Debug("chromedp", "msg", fmt.Sprintf(s, i...))
	}))

	timeout := defaultTimeout
	if cfg.TimeoutMs > 0 {
		timeout = time.Duration(cfg.TimeoutMs) * time.Millisecond
	}

	d := &chromedpDriver{
		browserCtx: browserCtx,
		cancel:     func() { browserCancel(); allocCancel() },
		logger:     logger,
		timeout:    timeout,
	}

	logger.InfoContext(ctx, "starting Chrome",
		"headless", cfg.Headless,
		"stealth", cfg.Stealth,
		"window_size", fmt.Sprintf("%dx%d", cfg.Width, cfg.Height),
		"no_sandbox", cfg.NoSandbox,
		"timeout", timeout)

	var initActions []chromedp.Action

	if cfg.Stealth {
		hwConcurrency := stealthHWOptions[rand.IntN(len(stealthHWOptions))] //nolint:gosec // non-security randomisation of browser fingerprint values
		deviceMemory := stealthHWOptions[rand.IntN(len(stealthHWOptions))]  //nolint:gosec // non-security randomisation of browser fingerprint values
		stealthJS := buildStealthJS(hwConcurrency, deviceMemory)
		initActions = append(initActions,
			chromedp.ActionFunc(func(ctx context.Context) error {
				_, err := page.AddScriptToEvaluateOnNewDocument(stealthJS).Do(ctx)
				return err
			}),
		)
	}

	initActions = append(initActions, chromedp.Navigate("about:blank"))

	if err := chromedp.Run(browserCtx, initActions...); err != nil {
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

func (d *chromedpDriver) Click(ctx context.Context, x, y int, button string) error {
	d.logger.InfoContext(ctx, "click", "x", x, "y", y, "button", button)
	actionCtx, cancel := d.withTimeout(ctx)
	defer cancel()
	btn := mouseButton(button)
	return chromedp.Run(actionCtx,
		mouseMove(x, y),
		mousePress(x, y, btn, 1),
		mouseRelease(x, y, btn, 1),
	)
}

func (d *chromedpDriver) DoubleClick(ctx context.Context, x, y int) error {
	d.logger.InfoContext(ctx, "double_click", "x", x, "y", y)
	actionCtx, cancel := d.withTimeout(ctx)
	defer cancel()
	return chromedp.Run(actionCtx,
		mouseMove(x, y),
		mousePress(x, y, input.Left, doubleClickCount),
		mouseRelease(x, y, input.Left, doubleClickCount),
	)
}

func (d *chromedpDriver) Type(ctx context.Context, text string, delayMs int) error {
	d.logger.InfoContext(ctx, "type_text", "text_len", len(text), "delay_ms", delayMs)

	if delayMs <= 0 {
		actionCtx, cancel := d.withTimeout(ctx)
		defer cancel()
		return chromedp.Run(actionCtx, chromedp.SendKeys("document", text, chromedp.ByJSPath))
	}

	delay := time.Duration(delayMs) * time.Millisecond
	for _, ch := range text {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		actionCtx, cancel := d.withTimeout(ctx)
		err := chromedp.Run(actionCtx, chromedp.SendKeys("document", string(ch), chromedp.ByJSPath))
		cancel()
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil
}

func (d *chromedpDriver) Scroll(ctx context.Context, direction string, clicks int) error {
	d.logger.InfoContext(ctx, "scroll", "direction", direction, "clicks", clicks)
	actionCtx, cancel := d.withTimeout(ctx)
	defer cancel()

	var delta int
	switch direction {
	case "up":
		delta = -clicks * scrollDeltaPerClick
	default:
		delta = clicks * scrollDeltaPerClick
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
	if err := chromedp.Run(actionCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		buf, err = page.CaptureScreenshot().
			WithFormat(page.CaptureScreenshotFormatJpeg).
			WithQuality(defaultJPEGQuality).
			Do(ctx)
		return err
	})); err != nil {
		return "", 0, 0, fmt.Errorf("screenshot failed: %w", err)
	}

	cfg, err := jpeg.DecodeConfig(bytes.NewReader(buf))
	if err != nil {
		return "", 0, 0, fmt.Errorf("screenshot: jpeg decode config failed: %w", err)
	}

	w := cfg.Width
	h := cfg.Height
	encoded := base64.StdEncoding.EncodeToString(buf)
	d.logger.DebugContext(ctx, "screenshot captured", "width", w, "height", h, "base64_len", len(encoded))
	return encoded, w, h, nil
}

func (d *chromedpDriver) WaitForStable(ctx context.Context, timeoutMs int, threshold float64) (bool, int64, error) {
	d.logger.InfoContext(ctx, "wait_for_stable starting", "timeout_ms", timeoutMs, "threshold", threshold)
	start := time.Now()
	deadline := start.Add(time.Duration(timeoutMs) * time.Millisecond)

	const observeJS = `
(function() {
	const obs = {count: 0};
	const observer = new MutationObserver(() => { obs.count++; });
	observer.observe(document, {childList: true, subtree: true, attributes: true, characterData: true});
	window.__waitStableObs = obs;
	window.__waitStableMO = observer;
})();
`

	const checkJS = `(function() { return window.__waitStableObs ? window.__waitStableObs.count : -1; })()`
	const cleanupJS = `(function() { if (window.__waitStableMO) { window.__waitStableMO.disconnect(); delete window.__waitStableMO; delete window.__waitStableObs; } })()`

	actionCtx, cancel := d.withTimeout(ctx)
	if err := chromedp.Run(actionCtx,
		chromedp.Evaluate(observeJS, nil),
	); err != nil {
		cancel()
		return false, 0, fmt.Errorf("wait_for_stable: mutation observer setup failed: %w", err)
	}
	cancel()

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(d.browserCtx, d.timeout)
		_ = chromedp.Run(cleanupCtx, chromedp.Evaluate(cleanupJS, nil))
		cleanupCancel()
	}()

	pollInterval := defaultPollInterval
	iter := 0
	var prevCount int

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false, time.Since(start).Milliseconds(), ctx.Err()
		default:
		}

		actionCtx, cancel := d.withTimeout(ctx)
		var count int
		checkErr := chromedp.Run(actionCtx, chromedp.Evaluate(checkJS, &count))
		cancel()
		if checkErr != nil {
			if errors.Is(checkErr, context.DeadlineExceeded) {
				continue
			}
			return false, 0, fmt.Errorf("wait_for_stable: mutation check failed: %w", checkErr)
		}
		iter++

		if iter > 1 && count == prevCount {
			elapsed := time.Since(start)
			d.logger.InfoContext(ctx, "wait_for_stable: DOM stable", "iterations", iter, "mutation_count", count, "elapsed_ms", elapsed.Milliseconds())
			return true, elapsed.Milliseconds(), nil
		}
		prevCount = count

		pollTimer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			pollTimer.Stop()
			return false, time.Since(start).Milliseconds(), ctx.Err()
		case <-pollTimer.C:
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
	// Must derive from browserCtx so chromedp can locate its CDP session via context values.
	// context.AfterFunc propagates caller cancellation without spawning a persistent goroutine.
	tctx, cancel := context.WithTimeout(d.browserCtx, d.timeout)
	stop := context.AfterFunc(ctx, cancel)
	return tctx, func() { stop(); cancel() }
}

func mouseButton(button string) input.MouseButton {
	switch button {
	case "right":
		return input.Right
	case "middle":
		return input.Middle
	default:
		return input.Left
	}
}

func mouseMove(x, y int) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		return input.DispatchMouseEvent(input.MouseMoved, float64(x), float64(y)).Do(ctx)
	})
}

func mousePress(x, y int, btn input.MouseButton, clickCount int) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		return input.DispatchMouseEvent(input.MousePressed, float64(x), float64(y)).WithButton(btn).WithClickCount(int64(clickCount)).Do(ctx)
	})
}

func mouseRelease(x, y int, btn input.MouseButton, clickCount int) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		return input.DispatchMouseEvent(input.MouseReleased, float64(x), float64(y)).WithButton(btn).WithClickCount(int64(clickCount)).Do(ctx)
	})
}
