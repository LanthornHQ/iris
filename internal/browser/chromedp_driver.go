package browser

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image/jpeg"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"

	"github.com/LanthornHQ/iris/internal/imageutil"
)

const (
	defaultPollInterval = 200 * time.Millisecond
	defaultTimeout      = 30 * time.Second
	doubleClickCount    = 2
	scrollDeltaPerClick = 300
)

type chromedpDriver struct {
	mu         sync.Mutex
	browserCtx context.Context
	cancel     context.CancelFunc
	logger     *slog.Logger
	timeout    time.Duration
}

const stealthJS = `(function() {
	Object.defineProperty(navigator, 'webdriver', {get: () => undefined});
})();`

func buildAllocatorOpts(cfg Config) []chromedp.ExecAllocatorOption {
	opts := commonFlags(cfg)
	opts = append(opts,
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
	)
	if cfg.Headless {
		opts = append(opts, chromedp.Flag("headless", "new"))
	}
	return opts
}

// commonFlags returns the default allocator options.
func commonFlags(cfg Config) []chromedp.ExecAllocatorOption {
	return []chromedp.ExecAllocatorOption{
		// --- Rendering ---
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("no-sandbox", cfg.NoSandbox),
		chromedp.WindowSize(cfg.Width, cfg.Height),
		chromedp.Flag("window-position", "0,0"),
		chromedp.Flag("hide-scrollbars", true),
		chromedp.Flag("disable-smooth-scrolling", true),
		chromedp.Flag("mute-audio", true),

		// --- Suppress first-run / onboarding UI ---
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-search-engine-choice-screen", true),

		// --- Suppress permission / dialog prompts ---
		chromedp.Flag("deny-permission-prompts", true),
		chromedp.Flag("disable-notifications", true),
		chromedp.Flag("disable-hang-monitor", true),
		chromedp.Flag("disable-prompt-on-repost", true),
		chromedp.Flag("autoplay-policy", "no-user-gesture-required"),

		// --- Disable features: UI chrome, telemetry, consent popups ---
		chromedp.Flag("disable-features",
			"SearchEngineChoice,SearchEngineChoiceScreen,"+
				"FirstRunDesktopRefresh,FirstRunDesktopChoiceScreenRefresh,FirstRunDesktopRevamp,"+
				"PrivacySandboxSettings4,GpcConsent,TopicsFencingV3,ConsentBump,"+
				"OptimizationGuideModelDownloading,OptimizationHints,"+
				"OptimizationTargetPrediction,OptimizationHintsFetching,"+
				"Translate,MediaRouter,Preload"),

		// Tell Chrome to skip its own consent/privacy-sandbox dialogs.
		chromedp.Flag("enable-features", "PrivacySandboxConsentExemption"),

		// --- Network & telemetry suppression ---
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-backgrounding-occluded-windows", true),
		chromedp.Flag("disable-renderer-backgrounding", true),
		chromedp.Flag("disable-component-update", true),
		chromedp.Flag("disable-domain-reliability", true),
		chromedp.Flag("disable-crash-reporter", true),
		chromedp.Flag("disable-component-extensions-with-background-pages", true),
		chromedp.Flag("no-pings", true),

		// --- Credentials ---
		chromedp.Flag("password-store", "basic"),

		// --- Language / locale ---
		chromedp.Flag("lang", "en-US"),
	}
}

func buildCookies(initial []CookieDef, bypassGoogleConsent bool) []*network.CookieParam {
	var cookies []*network.CookieParam

	for _, c := range initial {
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

	if bypassGoogleConsent {
		// Inject default bypass cookies for Google and YouTube domains to prevent consent overlays
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
	}
	return cookies
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
		"window_size", fmt.Sprintf("%dx%d", cfg.Width, cfg.Height),
		"no_sandbox", cfg.NoSandbox,
		"timeout", timeout)

	var initActions []chromedp.Action

	initActions = append(initActions,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(stealthJS).Do(ctx)
			return err
		}),
	)

	cookies := buildCookies(cfg.InitialCookies, cfg.BypassGoogleConsent)
	if len(cookies) > 0 {
		initActions = append(initActions, chromedp.ActionFunc(func(ctx context.Context) error {
			return network.SetCookies(cookies).Do(ctx)
		}))
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
	d.mu.Lock()
	defer d.mu.Unlock()
	d.logger.InfoContext(ctx, "navigate", "url", url)
	actionCtx, cancel := d.withTimeout(ctx)
	defer cancel()
	return chromedp.Run(actionCtx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body", chromedp.ByQuery),
	)
}

func (d *chromedpDriver) Click(ctx context.Context, x, y int, button string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
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
	d.mu.Lock()
	defer d.mu.Unlock()
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
	d.mu.Lock()
	defer d.mu.Unlock()
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
	d.mu.Lock()
	defer d.mu.Unlock()
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
	d.mu.Lock()
	defer d.mu.Unlock()
	d.logger.DebugContext(ctx, "screenshot starting")
	actionCtx, cancel := d.withTimeout(ctx)
	defer cancel()

	var buf []byte
	if err := chromedp.Run(actionCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		buf, err = page.CaptureScreenshot().
			WithFormat(page.CaptureScreenshotFormatJpeg).
			WithQuality(imageutil.DefaultJPEGQuality).
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

func (d *chromedpDriver) checkMutationCount(ctx context.Context, checkJS string) (int, error) {
	actionCtx, cancel := d.withTimeout(ctx)
	defer cancel()
	var count int
	if err := chromedp.Run(actionCtx, chromedp.Evaluate(checkJS, &count)); err != nil {
		return 0, err
	}
	return count, nil
}

func (d *chromedpDriver) reinitMutationObserver(ctx context.Context, observeJS string) error {
	reinitCtx, reinitCancel := d.withTimeout(ctx)
	defer reinitCancel()
	return chromedp.Run(reinitCtx, chromedp.Evaluate(observeJS, nil))
}

func (d *chromedpDriver) pollStableIteration(
	ctx context.Context,
	start time.Time,
	observeJS string,
	checkJS string,
	pollInterval time.Duration,
	prevCount *int,
	iter *int,
) (bool, bool, int64, error) {
	select {
	case <-ctx.Done():
		return false, false, time.Since(start).Milliseconds(), ctx.Err()
	default:
	}

	count, checkErr := d.checkMutationCount(ctx, checkJS)
	if checkErr != nil {
		if errors.Is(checkErr, context.DeadlineExceeded) {
			return false, true, 0, nil
		}
		return false, false, 0, fmt.Errorf("wait_for_stable: mutation check failed: %w", checkErr)
	}

	if count == -1 {
		d.logger.DebugContext(ctx, "wait_for_stable: context/document refreshed, re-injecting mutation observer")
		if err := d.reinitMutationObserver(ctx, observeJS); err != nil {
			d.logger.DebugContext(ctx, "wait_for_stable: failed to re-inject observer", "error", err)
		}
		*prevCount = 0
		*iter = 0
		pollTimer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			pollTimer.Stop()
			return false, false, time.Since(start).Milliseconds(), ctx.Err()
		case <-pollTimer.C:
		}
		return false, true, 0, nil
	}

	*iter++

	if *iter > 1 && count == *prevCount && count >= 0 {
		elapsed := time.Since(start)
		d.logger.InfoContext(ctx, "wait_for_stable: DOM stable", "iterations", *iter, "mutation_count", count, "elapsed_ms", elapsed.Milliseconds())
		return true, false, elapsed.Milliseconds(), nil
	}
	*prevCount = count

	pollTimer := time.NewTimer(pollInterval)
	select {
	case <-ctx.Done():
		pollTimer.Stop()
		return false, false, time.Since(start).Milliseconds(), ctx.Err()
	case <-pollTimer.C:
	}

	return false, true, 0, nil
}

func (d *chromedpDriver) WaitForStable(ctx context.Context, timeoutMs int, threshold float64) (bool, int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
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
		stable, shouldContinue, elapsed, err := d.pollStableIteration(ctx, start, observeJS, checkJS, pollInterval, &prevCount, &iter)
		if err != nil {
			return false, 0, err
		}
		if stable {
			return true, elapsed, nil
		}
		if !shouldContinue {
			return false, elapsed, nil
		}
	}

	elapsed := time.Since(start)
	d.logger.InfoContext(ctx, "wait_for_stable: timeout reached", "timeout_ms", timeoutMs, "iterations", iter, "actual_ms", elapsed.Milliseconds())
	return false, elapsed.Milliseconds(), nil
}

func (d *chromedpDriver) Title(ctx context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.logger.DebugContext(ctx, "get title starting")
	actionCtx, cancel := d.withTimeout(ctx)
	defer cancel()

	var title string
	if err := chromedp.Run(actionCtx, chromedp.Title(&title)); err != nil {
		return "", fmt.Errorf("failed to get title: %w", err)
	}
	return title, nil
}

const somMarkJS = `(function() {
	function cleanWindow(win) {
		let doc;
		try {
			doc = win.document;
			if (!doc) return;
		} catch (e) {
			return;
		}
		doc.querySelectorAll('.iris-som-mark').forEach(e => e.remove());
		
		function cleanSubtree(root) {
			root.querySelectorAll('[data-iris-id]').forEach(el => el.removeAttribute('data-iris-id'));
			root.querySelectorAll('*').forEach(el => {
				if (el.shadowRoot) {
					cleanSubtree(el.shadowRoot);
				}
			});
		}
		cleanSubtree(doc);

		try {
			const iframes = doc.querySelectorAll('iframe');
			iframes.forEach(iframe => {
				try {
					if (iframe.contentWindow) {
						cleanWindow(iframe.contentWindow);
					}
				} catch (e) {}
			});
		} catch (e) {}
	}
	cleanWindow(window);

	try {
		window.top.document.querySelectorAll('.iris-som-mark').forEach(e => e.remove());
	} catch (e) {}

	let idCounter = 1;
	const elementsList = [];
	const placedBadges = [];

	function deepQuery(root, selector) {
		const results = Array.from(root.querySelectorAll(selector));
		root.querySelectorAll('*').forEach(el => {
			if (el.shadowRoot) {
				results.push(...deepQuery(el.shadowRoot, selector));
			}
		});
		return results;
	}

	function processWindow(win, iframeOffsetTop, iframeOffsetLeft) {
		if (idCounter > 200) return;

		let doc;
		try {
			doc = win.document;
			if (!doc) return;
		} catch (e) {
			return;
		}

		// 1. Pass 1: Standard interactive query
		const standardSelector = 'button, a, input, select, textarea, [role="button"], [role="link"], [role="tab"], [role="menuitem"], [role="switch"], [role="checkbox"], [contenteditable="true"], [tabindex]:not([tabindex="-1"])';
		const standard = deepQuery(doc, standardSelector);

		const candidates = [];
		const marked = new Set();
		standard.forEach(el => {
			if (!marked.has(el)) {
				marked.add(el);
				candidates.push({el: el, isStandardInteractive: true});
			}
		});

		// 2. Pass 2: Custom pointer candidates from common clickable tags only to avoid layout thrashing
		if (candidates.length < 200) {
			const pointerSelector = 'div, span, li, svg, img';
			const pointerCandidates = deepQuery(doc, pointerSelector);
			pointerCandidates.forEach(el => {
				if (!marked.has(el)) {
					try {
						const style = win.getComputedStyle(el);
						if (style && style.cursor === 'pointer') {
							marked.add(el);
							candidates.push({el: el, isStandardInteractive: false, style: style});
						}
					} catch (e) {}
				}
			});
		}

		candidates.forEach(cand => {
			if (idCounter > 200) return;

			const el = cand.el;
			const rect = el.getBoundingClientRect();
			const style = cand.style || win.getComputedStyle(el);

			const isVisible = rect.width > 0 && rect.height > 0 && 
			                  style.visibility !== 'hidden' && 
			                  style.opacity !== '0' && 
			                  style.display !== 'none';

			if (!isVisible) return;

			const top = rect.top + iframeOffsetTop;
			const left = rect.left + iframeOffsetLeft;
			const bottom = rect.bottom + iframeOffsetTop;
			const right = rect.right + iframeOffsetLeft;

			const inViewport = top < window.innerHeight && bottom > 0 && 
			                   left < window.innerWidth && right > 0;

			if (!inViewport) return;

			el.setAttribute('data-iris-id', idCounter);

			const text = (el.innerText || el.textContent || el.value || el.getAttribute('aria-label') || el.getAttribute('title') || '').trim().substring(0, 200);
			const ariaLabel = el.getAttribute('aria-label') || el.getAttribute('placeholder') || el.getAttribute('title') || '';
			const role = el.getAttribute('role') || el.tagName.toLowerCase();
			const typeAttr = el.getAttribute('type') || '';

			elementsList.push({
				id: idCounter,
				tag: el.tagName.toLowerCase(),
				text: text,
				aria_label: ariaLabel,
				role: role,
				type: typeAttr,
				bounds: {
					x: Math.round(left),
					y: Math.round(top),
					width: Math.round(rect.width),
					height: Math.round(rect.height)
				}
			});

			const isSmall = rect.width < 32 || rect.height < 32;
			const offset = isSmall ? 15 : 10;

			let isTop = false;
			let markParent = doc.body;
			try {
				if (window.top && window.top.document) {
					markParent = window.top.document.body;
					isTop = true;
				}
			} catch (e) {}

			let badgeTop = Math.max(0, (isTop ? top : rect.top) - offset);
			let badgeLeft = Math.max(0, (isTop ? left : rect.left) - offset);

			// Badge Collision Avoidance
			let attempts = 0;
			while (attempts < 5) {
				const overlaps = placedBadges.some(b => 
					!(badgeLeft + 25 < b.left || badgeLeft > b.left + b.width ||
					  badgeTop + 18 < b.top || badgeTop > b.top + b.height)
				);
				if (!overlaps) break;
				badgeLeft += 15;
				badgeTop += 12;
				attempts++;
			}
			placedBadges.push({left: badgeLeft, top: badgeTop, width: 20, height: 16});

			const mark = doc.createElement('div');
			mark.className = 'iris-som-mark';
			mark.innerText = idCounter;
			mark.style.cssText = 'position:fixed; top:'+badgeTop+'px; left:'+badgeLeft+'px; background:yellow; color:black; font-weight:bold; font-size:13px; padding:2px 4px; border:1px solid black; z-index:2147483647; pointer-events:none; border-radius:3px; box-shadow: 0 2px 4px rgba(0,0,0,0.2); line-height: 1.1;';
			
			try {
				markParent.appendChild(mark);
			} catch (e) {
				doc.body.appendChild(mark);
			}
			idCounter++;
		});

		try {
			const iframes = doc.querySelectorAll('iframe');
			iframes.forEach(iframe => {
				const iframeRect = iframe.getBoundingClientRect();
				const iframeStyle = win.getComputedStyle(iframe);
				const iframeVisible = iframeRect.width > 0 && iframeRect.height > 0 && 
				                      iframeStyle.visibility !== 'hidden' && 
				                      iframeStyle.display !== 'none';

				if (iframeVisible) {
					let winContent = null;
					try {
						winContent = iframe.contentWindow;
					} catch (e) {}
					if (winContent) {
						processWindow(winContent, iframeOffsetTop + iframeRect.top, iframeOffsetLeft + iframeRect.left);
					}
				}
			});
		} catch (e) {}
	}

	processWindow(window, 0, 0);
	return elementsList;
})()`

func (d *chromedpDriver) DrawMarks(ctx context.Context) ([]SomElement, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.logger.InfoContext(ctx, "drawing Set-of-Mark badges")

	var res []SomElement
	actionCtx, cancel := d.withTimeout(ctx)
	defer cancel()
	if err := chromedp.Run(actionCtx, chromedp.Evaluate(somMarkJS, &res)); err != nil {
		return nil, fmt.Errorf("drawing marks: %w", err)
	}
	return res, nil
}

func (d *chromedpDriver) GetElementCoords(ctx context.Context, id int) (int, int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.logger.InfoContext(ctx, "getting element coordinates", "element_id", id)

	getCoordsJS := fmt.Sprintf(`(function() {
		function findElement(win, iframeOffsetTop, iframeOffsetLeft) {
			let doc;
			try {
				doc = win.document;
				if (!doc) return null;
			} catch (e) {
				return null;
			}

			function searchSubtree(root) {
				const el = root.querySelector('[data-iris-id="%d"]');
				if (el) return el;
				
				const all = root.querySelectorAll('*');
				for (let i = 0; i < all.length; i++) {
					const child = all[i];
					if (child.shadowRoot) {
						const found = searchSubtree(child.shadowRoot);
						if (found) return found;
					}
				}
				return null;
			}

			const targetEl = searchSubtree(doc);
			if (targetEl) {
				const rect = targetEl.getBoundingClientRect();
				return {
					x: rect.left + rect.width/2 + iframeOffsetLeft,
					y: rect.top + rect.height/2 + iframeOffsetTop
				};
			}

			try {
				const iframes = doc.querySelectorAll('iframe');
				for (let i = 0; i < iframes.length; i++) {
					const iframe = iframes[i];
					const iframeRect = iframe.getBoundingClientRect();
					let winContent = null;
					try {
						winContent = iframe.contentWindow;
					} catch (e) {}
					if (winContent) {
						const res = findElement(winContent, iframeOffsetTop + iframeRect.top, iframeOffsetLeft + iframeRect.left);
						if (res) return res;
					}
				}
			} catch (e) {
				// Ignore inaccessible frames
			}

			return null;
		}

		return findElement(window, 0, 0);
	})()`, id)

	var res map[string]float64
	actionCtx, cancel := d.withTimeout(ctx)
	defer cancel()

	if err := chromedp.Run(actionCtx, chromedp.Evaluate(getCoordsJS, &res)); err != nil {
		return 0, 0, fmt.Errorf("evaluating element coords: %w", err)
	}
	if res == nil {
		return 0, 0, fmt.Errorf("element %d not found on screen (may be inside cross-origin iframe)", id)
	}
	return int(math.Round(res["x"])), int(math.Round(res["y"])), nil
}

func (d *chromedpDriver) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
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

type keySegment struct {
	keys     string
	modifier input.Modifier
}

func tryParseSpecial(runes []rune, i int) (keySegment, int, bool) {
	n := len(runes)
	if runes[i] != '{' {
		return keySegment{}, i, false
	}
	if i+1 < n && runes[i+1] == '{' {
		const braceEscapeLen = 2
		return keySegment{keys: "{"}, i + braceEscapeLen, true
	}
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
			return seg, closeIdx + 1, true
		}
	}
	return keySegment{}, i, false
}

func tryParseDoubleBrace(runes []rune, i int) ([]keySegment, int, bool) {
	n := len(runes)
	const doubleBraceCharLen = 2
	if i+1 >= n || runes[i] != '{' || runes[i+1] != '{' {
		return nil, i, false
	}

	closeIdx := -1
	for j := i + doubleBraceCharLen; j+1 < n; j++ {
		if runes[j] == '}' && runes[j+1] == '}' {
			closeIdx = j
			break
		}
	}

	if closeIdx == -1 {
		return nil, i, false
	}

	var segments []keySegment
	segments = append(segments, keySegment{keys: "{"})
	content := runes[i+doubleBraceCharLen : closeIdx]
	for _, r := range content {
		segments = append(segments, keySegment{keys: string(r)})
	}
	segments = append(segments, keySegment{keys: "}"})
	return segments, closeIdx + doubleBraceCharLen, true
}

func tryParseClosingDoubleBrace(runes []rune, i int) (keySegment, int, bool) {
	n := len(runes)
	const doubleBraceCharLen = 2
	if i+1 < n && runes[i] == '}' && runes[i+1] == '}' {
		return keySegment{keys: "}"}, i + doubleBraceCharLen, true
	}
	return keySegment{}, i, false
}

func parseSpecialKeys(text string) []keySegment {
	var parts []keySegment
	runes := []rune(text)
	n := len(runes)
	i := 0

	for i < n {
		if segs, nextIdx, ok := tryParseDoubleBrace(runes, i); ok {
			parts = append(parts, segs...)
			i = nextIdx
			continue
		}

		if seg, nextIdx, ok := tryParseClosingDoubleBrace(runes, i); ok {
			parts = append(parts, seg)
			i = nextIdx
			continue
		}

		if seg, nextIdx, ok := tryParseSpecial(runes, i); ok {
			parts = append(parts, seg)
			i = nextIdx
			continue
		}

		parts = append(parts, keySegment{keys: string(runes[i])})
		i++
	}

	return parts
}

func lookupBaseKey(key string, mod input.Modifier) (keySegment, bool) {
	switch key {
	case "enter":
		return keySegment{keys: "\r", modifier: mod}, true
	case "tab":
		return keySegment{keys: "\t", modifier: mod}, true
	case "escape", "esc":
		return keySegment{keys: "\x1b", modifier: mod}, true
	case "backspace":
		return keySegment{keys: "\b", modifier: mod}, true
	case "delete":
		return keySegment{keys: "\x7f", modifier: mod}, true
	case "up":
		return keySegment{keys: kb.ArrowUp, modifier: mod}, true
	case "down":
		return keySegment{keys: kb.ArrowDown, modifier: mod}, true
	case "left":
		return keySegment{keys: kb.ArrowLeft, modifier: mod}, true
	case "right":
		return keySegment{keys: kb.ArrowRight, modifier: mod}, true
	case "home":
		return keySegment{keys: kb.Home, modifier: mod}, true
	case "end":
		return keySegment{keys: kb.End, modifier: mod}, true
	case "pageup":
		return keySegment{keys: kb.PageUp, modifier: mod}, true
	case "pagedown":
		return keySegment{keys: kb.PageDown, modifier: mod}, true
	}
	return keySegment{}, false
}

func resolveSpecialKey(name string) (keySegment, bool) {
	normalized := strings.ReplaceAll(name, "-", "+")
	parts := strings.Split(normalized, "+")

	if len(parts) == 0 {
		return keySegment{}, false
	}

	// If it's a single part, just look it up in our base map
	if len(parts) == 1 {
		return lookupBaseKey(parts[0], 0)
	}

	// If it has multiple parts (modifiers + key)
	var mod input.Modifier
	keyPart := ""

	for i, part := range parts {
		if i == len(parts)-1 {
			keyPart = part
			break
		}
		switch part {
		case "ctrl", "control":
			mod |= input.ModifierCtrl
		case "shift":
			mod |= input.ModifierShift
		case "alt":
			mod |= input.ModifierAlt
		case "meta", "command", "cmd", "win":
			mod |= input.ModifierMeta
		default:
			return keySegment{}, false
		}
	}

	// Resolve the last part using our base key table
	seg, ok := lookupBaseKey(keyPart, mod)
	if ok {
		return seg, true
	}

	// If the keyPart is a single character, we can treat it as a literal single-character key segment
	if len([]rune(keyPart)) == 1 {
		return keySegment{keys: keyPart, modifier: mod}, true
	}

	return keySegment{}, false
}
