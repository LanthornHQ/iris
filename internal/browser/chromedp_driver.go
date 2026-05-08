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
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/network"
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
	xvfb       *xvfbProcess
}

var stealthHWOptions = []int{4, 8, 16}

// buildStealthJS generates stealth override JS with randomized hardware fingerprint values
// to avoid sessions sharing an identical browser signature.
func buildStealthJS(hwConcurrency, deviceMemory int) string {
	return fmt.Sprintf(`(function() {
	// --- navigator properties ---
	Object.defineProperty(navigator, 'webdriver', {get: () => undefined});
	Object.defineProperty(navigator, 'language',  {get: () => 'en-US'});
	Object.defineProperty(navigator, 'languages', {get: () => ['en-US', 'en']});
	Object.defineProperty(navigator, 'hardwareConcurrency', {get: () => %d});
	Object.defineProperty(navigator, 'deviceMemory',        {get: () => %d});

	// Plugins: real Chrome always shows at least the PDF plugin.
	Object.defineProperty(navigator, 'plugins', {get: () => {
		var pdf = {name:'Chrome PDF Plugin',filename:'internal-pdf-viewer',description:'Portable Document Format',length:1};
		var arr = [pdf];
		arr.item = function(i) { return this[i]; };
		arr.namedItem = function(n) { for(var i=0;i<this.length;i++){if(this[i].name===n)return this[i];} return null; };
		arr.refresh = function() {};
		Object.setPrototypeOf(arr, PluginArray.prototype);
		return arr;
	}});

	// MimeTypes: match the PDF plugin above.
	Object.defineProperty(navigator, 'mimeTypes', {get: () => {
		var pdf = {type:'application/pdf',suffixes:'pdf',description:'Portable Document Format',enabledPlugin:navigator.plugins[0]};
		var arr = [pdf];
		arr.item = function(i) { return this[i]; };
		arr.namedItem = function(t) { for(var i=0;i<this.length;i++){if(this[i].type===t)return this[i];} return null; };
		Object.setPrototypeOf(arr, MimeTypeArray.prototype);
		return arr;
	}});

	// --- window dimensions: headless reports outerHeight=0 ---
	try {
		Object.defineProperty(window, 'outerWidth',  {get: () => window.innerWidth});
		Object.defineProperty(window, 'outerHeight', {get: () => window.innerHeight + 85});
	} catch(e) {}

	// --- window.chrome: detectors inspect app, runtime, csi ---
	window.chrome = {
		app: {
			isInstalled: false,
			InstallState: {DISABLED:'disabled',INSTALLED:'installed',NOT_INSTALLED:'not_installed'},
			RunningState:  {CANNOT_RUN:'cannot_run',READY_TO_RUN:'ready_to_run',RUNNING:'running'},
			getDetails:    function(){},
			getIsInstalled:function(){},
			installState:  function(){},
		},
		runtime: {
			connect:     function(){return{disconnect:function(){},postMessage:function(){},onMessage:{addListener:function(){}},onDisconnect:{addListener:function(){}}};},
			sendMessage: function(){},
			getManifest: function(){return {};},
			id:          undefined,
			OnInstalledReason: {CHROME_UPDATE:'chrome_update',INSTALL:'install',SHARED_MODULE_UPDATE:'shared_module_update',UPDATE:'update'},
			PlatformOs:        {ANDROID:'android',CROS:'cros',LINUX:'linux',MAC:'mac',OPENBSD:'openbsd',WIN:'win'},
			PlatformArch:      {ARM:'arm','ARM64':'arm64',MIPS:'mips',MIPS64:'mips64',X86_32:'x86-32',X86_64:'x86-64'},
			RequestUpdateCheckStatus: {NO_UPDATE:'no_update',THROTTLED:'throttled',UPDATE_AVAILABLE:'update_available'},
		},
		loadTimes: function(){},
		csi:       function(){return {startE:Date.now(),onloadT:Date.now(),pageT:Date.now(),tran:15};},
	};

	// --- Permissions ---
	const _origQuery = window.navigator.permissions.query;
	window.navigator.permissions.query = (parameters) => (
		parameters.name === 'notifications' ?
			Promise.resolve({state: (typeof Notification !== 'undefined' ? Notification.permission : 'default')}) :
			_origQuery(parameters)
	);

	// --- WebGL: patch both WebGL1 and WebGL2 contexts ---
	function patchWebGL(ctx) {
		if (!ctx) return;
		const orig = ctx.prototype.getParameter;
		ctx.prototype.getParameter = function(parameter) {
			if (parameter === 37445) return 'Google Inc. (NVIDIA)';
			if (parameter === 37446) return 'ANGLE (NVIDIA, NVIDIA GeForce GTX 1060, OpenGL 4.5)';
			return orig.call(this, parameter);
		};
	}
	patchWebGL(WebGLRenderingContext);
	if (typeof WebGL2RenderingContext !== 'undefined') patchWebGL(WebGL2RenderingContext);
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
			chromedp.Flag("lang", "en-US"),
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
		chromedp.Flag("lang", "en-US"),
	)
	return opts
}

func newChromedpDriver(ctx context.Context, logger *slog.Logger, cfg Config) (*chromedpDriver, error) {
	var xvfb *xvfbProcess
	if cfg.Xvfb {
		var err error
		xvfb, err = startXvfb(cfg.Width, cfg.Height)
		if err != nil {
			return nil, fmt.Errorf("start Xvfb: %w", err)
		}
		logger.InfoContext(ctx, "Xvfb started", "display", xvfb.display)
	}

	allocatorOpts := buildAllocatorOpts(cfg)

	if xvfb != nil {
		allocatorOpts = append(allocatorOpts, chromedp.Env("DISPLAY="+xvfb.display))
	} else if d := os.Getenv("DISPLAY"); d != "" && !cfg.Headless {
		allocatorOpts = append(allocatorOpts, chromedp.Env("DISPLAY="+d))
	}

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
		xvfb:       xvfb,
	}

	logger.InfoContext(ctx, "starting Chrome",
		"headless", cfg.Headless,
		"xvfb", cfg.Xvfb,
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

	var cookies []*network.CookieParam

	if len(cfg.InitialCookies) > 0 {
		for _, c := range cfg.InitialCookies {
			path := c.Path
			if path == "" {
				path = "/"
			}
			cookies = append(cookies, &network.CookieParam{
				Name:   c.Name,
				Value:  c.Value,
				Domain: c.Domain,
				Path:   path,
			})
		}
	}

	// Always inject default bypass cookies for Google and YouTube domains to prevent consent overlays
	googleDomains := []string{
		".google.com",
		".google.de",
		".google.co.uk",
		".google.fr",
		".google.it",
		".google.es",
		".google.nl",
		".google.co.jp",
		".google.ca",
		".google.com.br",
		".google.pl",
		".google.ch",
		".google.at",
		".google.be",
		".google.cz",
		".google.se",
		".google.no",
		".google.dk",
		".google.fi",
		".youtube.com",
	}
	for _, domain := range googleDomains {
		cookies = append(cookies,
			&network.CookieParam{
				Name:   "SOCS",
				Value:  "CAESHAgBEhIYNDY4NDY4NDY4NDY4NDY4NDY4GgVlbi1VUw",
				Domain: domain,
				Path:   "/",
			},
			&network.CookieParam{
				Name:   "CONSENT",
				Value:  "PENDING+999",
				Domain: domain,
				Path:   "/",
			},
		)
	}

	initActions = append(initActions, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.SetCookies(cookies).Do(ctx)
	}))

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
	actionCtx, cancel := d.withTimeout(ctx)
	defer cancel()

	parts := parseSpecialKeys(text)
	delay := time.Duration(max(0, delayMs)) * time.Millisecond

	return chromedp.Run(actionCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		for i, seg := range parts {
			if i > 0 && delay > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(delay):
				}
			}
			var opts []chromedp.KeyOption
			if seg.modifier != 0 {
				opts = append(opts, chromedp.KeyModifiers(seg.modifier))
			}
			if err := chromedp.KeyEvent(seg.keys, opts...).Do(ctx); err != nil {
				return err
			}
		}
		return nil
	}))
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
	d.xvfb.stop()
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

type keySegment struct {
	keys     string
	modifier input.Modifier
}

func parseSpecialKeys(text string) []keySegment {
	var parts []keySegment
	runes := []rune(text)
	n := len(runes)
	i := 0

	for i < n {
		if i+1 < n && runes[i] == '{' && runes[i+1] == '{' {
			parts = append(parts, keySegment{keys: "{"})
			i += 2
			continue
		}
		if i+1 < n && runes[i] == '}' && runes[i+1] == '}' {
			parts = append(parts, keySegment{keys: "}"})
			i += 2
			continue
		}

		if runes[i] == '{' {
			closeIdx := -1
			for j := i + 1; j < n; j++ {
				if runes[j] == '}' {
					closeIdx = j
					break
				}
			}
			if closeIdx != -1 {
				name := strings.ToLower(string(runes[i+1 : closeIdx]))
				if seg, ok := resolveSpecialKey(name); ok {
					parts = append(parts, seg)
					i = closeIdx + 1
					continue
				}
			}
		}

		parts = append(parts, keySegment{keys: string(runes[i])})
		i++
	}

	return parts
}

func resolveSpecialKey(name string) (keySegment, bool) {
	switch name {
	case "enter":
		return keySegment{keys: "\r"}, true
	case "tab":
		return keySegment{keys: "\t"}, true
	case "escape", "esc":
		return keySegment{keys: "\x1b"}, true
	case "backspace":
		return keySegment{keys: "\b"}, true
	case "delete":
		return keySegment{keys: "\x7f"}, true
	case "up":
		return keySegment{keys: "\u0304"}, true
	case "down":
		return keySegment{keys: "\u0301"}, true
	case "left":
		return keySegment{keys: "\u0302"}, true
	case "right":
		return keySegment{keys: "\u0303"}, true
	case "home":
		return keySegment{keys: "\u0306"}, true
	case "end":
		return keySegment{keys: "\u0305"}, true
	case "pageup":
		return keySegment{keys: "\u0308"}, true
	case "pagedown":
		return keySegment{keys: "\u0307"}, true
	case "shift+tab", "shift-tab":
		return keySegment{keys: "\t", modifier: input.ModifierShift}, true
	}
	// {Ctrl+X} / {Ctrl-X} where X is a single letter or digit
	if (strings.HasPrefix(name, "ctrl+") || strings.HasPrefix(name, "ctrl-")) && len([]rune(name)) == 6 {
		ch := strings.ToLower(string([]rune(name)[5]))
		return keySegment{keys: ch, modifier: input.ModifierCtrl}, true
	}
	return keySegment{}, false
}

