package agenttools

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

// stealthScript configures the browser environment to bypass automated bot detection heuristics.
const stealthScript = `(function() {
	// 1. Remove webdriver property from Navigator prototype & instance
	try {
		delete Object.getPrototypeOf(navigator).webdriver;
	} catch (e) {}
	try {
		delete navigator.webdriver;
	} catch (e) {}

	// 2. Normalise window.chrome
	window.chrome = {
		runtime: {},
		loadTimes: function() {},
		csi: function() {},
		app: {}
	};

	// 3. Languages
	Object.defineProperty(navigator, 'languages', { get: () => ['en-US', 'en'] });

	// 4. PluginArray normalisation & fallback
	try {
		if (!navigator.plugins || navigator.plugins.length === 0 || !(navigator.plugins instanceof PluginArray)) {
			const mockPlugin = function(name, description, filename) {
				const p = Object.create(Plugin.prototype);
				Object.defineProperties(p, {
					name: { value: name, enumerable: true },
					description: { value: description, enumerable: true },
					filename: { value: filename, enumerable: true },
					length: { value: 0, enumerable: true }
				});
				return p;
			};
			const pluginList = [
				mockPlugin('PDF Viewer', 'Portable Document Format', 'internal-pdf-viewer'),
				mockPlugin('Chrome PDF Viewer', 'Portable Document Format', 'internal-pdf-viewer'),
				mockPlugin('Chromium PDF Viewer', 'Portable Document Format', 'internal-pdf-viewer'),
				mockPlugin('Microsoft Edge PDF Viewer', 'Portable Document Format', 'internal-pdf-viewer'),
				mockPlugin('WebKit built-in PDF', 'Portable Document Format', 'internal-pdf-viewer')
			];
			const pluginArray = Object.create(PluginArray.prototype);
			pluginList.forEach((p, i) => {
				pluginArray[i] = p;
				pluginArray[p.name] = p;
			});
			Object.defineProperties(pluginArray, {
				length: { value: pluginList.length, enumerable: true },
				item: { value: function(index) { return this[index] || null; } },
				namedItem: { value: function(name) { return this[name] || null; } },
				refresh: { value: function() {} }
			});
			Object.defineProperty(Object.getPrototypeOf(navigator), 'plugins', {
				get: () => pluginArray,
				enumerable: true,
				configurable: true
			});
		}
	} catch (e) {}

	// 5. Mask WebGL vendor and renderer across WebGL1 & WebGL2
	try {
		const maskWebGL = (contextProto) => {
			if (!contextProto) return;
			const origGetParameter = contextProto.getParameter;
			contextProto.getParameter = function(parameter) {
				if (parameter === 37445) return 'Intel Inc.';
				if (parameter === 37446) return 'Intel Iris OpenGL Engine';
				return origGetParameter.apply(this, arguments);
			};
		};
		maskWebGL(window.WebGLRenderingContext && window.WebGLRenderingContext.prototype);
		maskWebGL(window.WebGL2RenderingContext && window.WebGL2RenderingContext.prototype);
	} catch (e) {}

	// 6. Notification permission
	if (window.navigator.permissions) {
		const origQuery = window.navigator.permissions.query;
		window.navigator.permissions.query = (params) => (
			params && params.name === 'notifications' ?
				Promise.resolve({ state: Notification.permission }) :
				origQuery(params)
		);
	}
})();`

type BrowserElement struct {
	Index  int    `json:"index"`
	Type   string `json:"type"`   // "link", "input", "button"
	Name   string `json:"name"`   // For form fields (inputs)
	Text   string `json:"text"`   // Display text or button label
	Target string `json:"target"` // URL for links, or input type
	FormID string `json:"form_id"` // Associated form identifier (deprecated)
}

// BrowserTab represents an open tab/page in the browser session.
type BrowserTab struct {
	Index    int       `json:"index"`
	TargetID target.ID `json:"target_id"`
	Title    string    `json:"title"`
	URL      string    `json:"url"`
	Active   bool      `json:"active"`
}

type BrowserSession struct {
	mu            sync.Mutex
	Ctx           context.Context
	Cancel        cancelFuncWrapper
	allocCancel   context.CancelFunc
	allocCtx      context.Context
	browserCtx    context.Context
	browserCancel context.CancelFunc
	activeTabID   target.ID
	tabOrder      []target.ID
	tabContexts   map[target.ID]context.Context
	tabCancels    map[target.ID]context.CancelFunc
	CurrentURL    string
	PageText      string
	Elements      []BrowserElement
	Inputs        map[int]string // Maps element index -> user filled value
	HasCaptcha    bool
	CaptchaNotice string
	lastAccess    time.Time
	Headed        bool // Explicit headed (visible window) mode
}

type cancelFuncWrapper context.CancelFunc

// SetHeaded configures whether the browser session runs in visible (headed) mode.
// If the session is already active with a different mode, it re-initializes cleanly.
func (b *BrowserSession) SetHeaded(headed bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.Headed == headed && b.Ctx != nil {
		return
	}
	b.Headed = headed
	for _, cancel := range b.tabCancels {
		if cancel != nil {
			cancel()
		}
	}
	b.tabContexts = nil
	b.tabCancels = nil

	if b.browserCancel != nil {
		b.browserCancel()
		b.browserCtx = nil
		b.browserCancel = nil
	}
	if b.Cancel != nil {
		context.CancelFunc(b.Cancel)()
		b.Ctx = nil
		b.Cancel = nil
	}
	if b.allocCancel != nil {
		b.allocCancel()
		b.allocCancel = nil
		b.allocCtx = nil
	}
	b.activeTabID = ""
}

// IsHeaded reports whether the session is operating in headed mode.
func (b *BrowserSession) IsHeaded() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Headed || os.Getenv("HEADED") == "true" || os.Getenv("HEADLESS") == "false" || os.Getenv("BROWSER_HEADED") == "true"
}

// Close releases the chromedp context and exec allocator, cleaning up the
// Chrome process. Both cancels must be called to avoid orphaned processes (L-4).
func (b *BrowserSession) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, cancel := range b.tabCancels {
		if cancel != nil {
			cancel()
		}
	}
	b.tabContexts = nil
	b.tabCancels = nil

	if b.browserCancel != nil {
		b.browserCancel()
		b.browserCtx = nil
		b.browserCancel = nil
	}
	if b.Cancel != nil {
		context.CancelFunc(b.Cancel)()
		b.Ctx = nil
		b.Cancel = nil
	}
	if b.allocCancel != nil {
		b.allocCancel()
		b.allocCancel = nil
		b.allocCtx = nil
	}
	b.activeTabID = ""
}

// ensureContextLocked initializes or self-heals the stealth chromedp session.
func (b *BrowserSession) ensureContextLocked() {
	if b.tabContexts == nil {
		b.tabContexts = make(map[target.ID]context.Context)
		b.tabCancels = make(map[target.ID]context.CancelFunc)
	}
	if b.Ctx != nil && b.Ctx.Err() == nil {
		return
	}
	isHeadless := true
	if b.Headed || os.Getenv("HEADED") == "true" || os.Getenv("HEADLESS") == "false" || os.Getenv("BROWSER_HEADED") == "true" {
		isHeadless = false
	}
	opts := []chromedp.ExecAllocatorOption{
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.NoSandbox,
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.UserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"),
		chromedp.WindowSize(1440, 900),
		chromedp.Flag("accept-lang", "en-US,en;q=0.9"),
		chromedp.Flag("enable-features", "NetworkService,NetworkServiceInProcess"),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("force-color-profile", "srgb"),
	}
	if isHeadless {
		opts = append(opts, chromedp.Flag("headless", "new"))
	} else {
		opts = append(opts,
			chromedp.Flag("disable-infobars", true),
			chromedp.Flag("disable-save-password-bubble", true),
			chromedp.Flag("disable-single-click-autofill", true),
		)
	}

	if proxy := os.Getenv("BROWSER_PROXY"); proxy != "" {
		opts = append(opts, chromedp.Flag("proxy-server", proxy))
	} else if proxy := os.Getenv("HTTP_PROXY"); proxy != "" {
		opts = append(opts, chromedp.Flag("proxy-server", proxy))
	}

	if userDataDir := os.Getenv("BROWSER_USER_DATA_DIR"); userDataDir != "" {
		opts = append(opts, chromedp.UserDataDir(userDataDir))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	b.allocCtx = allocCtx
	b.allocCancel = allocCancel
	b.browserCtx = browserCtx
	b.browserCancel = browserCancel

	b.Ctx = browserCtx
	b.Cancel = cancelFuncWrapper(browserCancel)

	_ = chromedp.Run(b.Ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		_, err := page.AddScriptToEvaluateOnNewDocument(stealthScript).Do(ctx)
		return err
	}))

	if c := chromedp.FromContext(b.Ctx); c != nil && c.Target != nil {
		b.activeTabID = c.Target.TargetID
		b.tabContexts[c.Target.TargetID] = b.Ctx
		b.tabCancels[c.Target.TargetID] = browserCancel
		b.tabOrder = []target.ID{c.Target.TargetID}
	}
}

// BrowserSessionManager stores thread-safe browser sessions isolated by session ID.
// ponytail: In-memory map storage with stdlib TTL eviction and Chrome process cleanup; upgrade to Redis/LRU for multi-node deployments.
type BrowserSessionManager struct {
	mu            sync.Mutex
	sessions      map[string]*BrowserSession
	defaultHeaded bool
}

// SetDefaultHeaded configures the default headed mode for newly created sessions.
func (m *BrowserSessionManager) SetDefaultHeaded(headed bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaultHeaded = headed
}

// IsDefaultHeaded returns whether new sessions default to headed mode.
func (m *BrowserSessionManager) IsDefaultHeaded() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.defaultHeaded
}

func (m *BrowserSessionManager) GetSession(sessionID string) *BrowserSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	if sessionID == "" {
		sessionID = "default"
	}

	// Prune sessions idle for >15 minutes or if total active sessions exceed 20
	now := time.Now()
	for id, s := range m.sessions {
		if id != sessionID && (now.Sub(s.lastAccess) > 15*time.Minute || len(m.sessions) > 20) {
			s.Close()
			delete(m.sessions, id)
		}
	}

	s, ok := m.sessions[sessionID]
	if !ok {
		s = &BrowserSession{
			Inputs: make(map[int]string),
			Headed: m.defaultHeaded,
		}
		m.sessions[sessionID] = s
	}
	s.lastAccess = now
	return s
}

var GlobalSessionManager = &BrowserSessionManager{
	sessions: make(map[string]*BrowserSession),
}

// GlobalBrowser is the active simulated browser session.
var GlobalBrowser = &BrowserSession{
	Inputs: make(map[int]string),
}

// Navigate loads the URL using chromedp, executes dynamic script, and extracts page state.
func (b *BrowserSession) Navigate(urlStr string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	parsed, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" {
		parsed.Scheme = "http"
	}

	b.ensureContextLocked()

	err = chromedp.Run(b.Ctx,
		chromedp.Navigate(parsed.String()),
		chromedp.Sleep(time.Duration(300+rand.Intn(200))*time.Millisecond),
	)
	if err != nil {
		return "", fmt.Errorf("chromedp navigate: %w", err)
	}

	b.updateStateLocked()
	return b.renderViewLocked(), nil
}

// Input sets the text value of an input field element in headless Chrome.
func (b *BrowserSession) Input(index int, val string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.Ctx == nil {
		return "", fmt.Errorf("no active browser session; call navigate first")
	}

	var targetEl *BrowserElement
	for i := range b.Elements {
		if b.Elements[i].Index == index {
			targetEl = &b.Elements[i]
			break
		}
	}

	if targetEl == nil {
		return "", fmt.Errorf("element with index %d not found", index)
	}

	sel := fmt.Sprintf("[data-agent-index='%d']", index)
	opCtx, opCancel := context.WithTimeout(b.Ctx, 8*time.Second)
	defer opCancel()

	err := chromedp.Run(opCtx,
		chromedp.Focus(sel, chromedp.ByQuery),
		chromedp.Evaluate(fmt.Sprintf(`document.querySelector("%s").value = ""; document.querySelector("%s").dispatchEvent(new Event('input', { bubbles: true }));`, sel, sel), nil),
	)
	if err != nil {
		return "", fmt.Errorf("chromedp input focus: %w", err)
	}

	// Humanized typing per character with randomized inter-key delay (30ms - 75ms)
	for _, ch := range val {
		if opCtx.Err() != nil {
			return "", fmt.Errorf("chromedp typing timed out: %w", opCtx.Err())
		}
		_ = chromedp.Run(opCtx, chromedp.SendKeys(sel, string(ch), chromedp.ByQuery))
		time.Sleep(time.Duration(30+rand.Intn(45)) * time.Millisecond)
	}

	// Trigger input & change events for frameworks (React, Vue, Angular)
	_ = chromedp.Run(opCtx,
		chromedp.Evaluate(fmt.Sprintf(`document.querySelector("%s").dispatchEvent(new Event('change', { bubbles: true }));`, sel), nil),
		chromedp.Sleep(time.Duration(150+rand.Intn(150))*time.Millisecond),
	)

	b.Inputs[index] = val
	b.updateStateLocked()

	return fmt.Sprintf("Set input [%d] (%s) to %q\n\nCurrent Page:\n%s", index, targetEl.Name, val, b.renderViewLocked()), nil
}

// Click simulates clicking a link or button in headless Chrome.
func (b *BrowserSession) Click(index int) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.Ctx == nil {
		return "", fmt.Errorf("no active browser session; call navigate first")
	}

	var targetEl *BrowserElement
	for i := range b.Elements {
		if b.Elements[i].Index == index {
			targetEl = &b.Elements[i]
			break
		}
	}

	if targetEl == nil {
		return "", fmt.Errorf("element with index %d not found", index)
	}

	sel := fmt.Sprintf("[data-agent-index='%d']", index)

	// Smoothly scroll target element into view & dispatch hover events prior to click
	scrollHoverScript := fmt.Sprintf(`(function() {
		let el = document.querySelector("%s");
		if (el) {
			el.scrollIntoView({ behavior: 'smooth', block: 'center' });
			el.dispatchEvent(new MouseEvent('mouseover', { bubbles: true }));
			el.dispatchEvent(new MouseEvent('mouseenter', { bubbles: true }));
		}
	})()`, sel)

	opCtx, opCancel := context.WithTimeout(b.Ctx, 8*time.Second)
	defer opCancel()

	// Snapshot existing targets before click
	var beforeIDs map[target.ID]bool
	if allTargets, err := chromedp.Targets(b.Ctx); err == nil {
		beforeIDs = make(map[target.ID]bool, len(allTargets))
		for _, t := range allTargets {
			if t.Type == "page" {
				beforeIDs[t.TargetID] = true
			}
		}
	}

	err := chromedp.Run(opCtx,
		chromedp.Evaluate(scrollHoverScript, nil),
		chromedp.Sleep(time.Duration(150+rand.Intn(150))*time.Millisecond),
		chromedp.Click(sel, chromedp.ByQuery),
		chromedp.Sleep(time.Duration(450+rand.Intn(300))*time.Millisecond), // SPA transition delay
	)
	if err != nil {
		return "", fmt.Errorf("chromedp click: %w", err)
	}

	newTabNotice := ""
	if beforeIDs != nil {
		if afterTargets, err := chromedp.Targets(b.Ctx); err == nil {
			for _, t := range afterTargets {
				if t.Type == "page" && !beforeIDs[t.TargetID] {
					tabCtx, tabCancel := chromedp.NewContext(b.browserCtx, chromedp.WithTargetID(t.TargetID))
					b.tabContexts[t.TargetID] = tabCtx
					b.tabCancels[t.TargetID] = tabCancel
					_ = chromedp.Run(tabCtx, chromedp.ActionFunc(func(ctx context.Context) error {
						_, err := page.AddScriptToEvaluateOnNewDocument(stealthScript).Do(ctx)
						return err
					}))
					_ = chromedp.Run(tabCtx, target.ActivateTarget(t.TargetID))
					b.Ctx = tabCtx
					b.Cancel = cancelFuncWrapper(tabCancel)
					b.activeTabID = t.TargetID
					newTabNotice = fmt.Sprintf("[Notice: Click opened a new tab %q (%s). Automatically switched focus to it.]\n\n", t.Title, t.URL)
					break
				}
			}
		}
	}

	b.updateStateLocked()
	return newTabNotice + b.renderViewLocked(), nil
}

// Scroll scrolls the page by dy pixels (positive = down, negative = up) and
// returns the updated page state. Useful for lazy-loaded content and
// infinite-scroll pages that only render elements after scrolling.
func (b *BrowserSession) Scroll(dy int) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.Ctx == nil {
		return "", fmt.Errorf("no active browser session; call navigate first")
	}

	opCtx, opCancel := context.WithTimeout(b.Ctx, 5*time.Second)
	defer opCancel()

	script := fmt.Sprintf("window.scrollBy(0, %d);", dy)
	err := chromedp.Run(opCtx,
		chromedp.Evaluate(script, nil),
		chromedp.Sleep(400*time.Millisecond),
	)
	if err != nil {
		return "", fmt.Errorf("chromedp scroll: %w", err)
	}

	b.updateStateLocked()
	return b.renderViewLocked(), nil
}

// WaitFor polls until the given CSS selector is present and visible, or until
// the timeout elapses. Use this before extracting data from dynamically rendered
// content (charts, tables loaded via XHR, SPA route changes).
func (b *BrowserSession) WaitFor(selector string, timeoutSec int) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.Ctx == nil {
		return "", fmt.Errorf("no active browser session; call navigate first")
	}

	if timeoutSec <= 0 || timeoutSec > 30 {
		timeoutSec = 10
	}

	ctx, cancel := context.WithTimeout(b.Ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	err := chromedp.Run(ctx, chromedp.WaitVisible(selector, chromedp.ByQuery))
	if err != nil {
		return fmt.Sprintf("WaitFor: selector %q not visible within %ds: %v", selector, timeoutSec, err), nil
	}

	b.updateStateLocked()
	return fmt.Sprintf("Element %q is now visible.\n\n%s", selector, b.renderViewLocked()), nil
}

// ExtractJS executes an arbitrary JavaScript expression in the current page
// context and returns its result as a string. Use this to:
//   - Extract data from tables, charts, or JavaScript state objects
//   - Read values not visible in the truncated page text
//   - Interact with site-specific JS APIs (e.g. window.__NEXT_DATA__)
//
// Examples:
//
//	document.querySelector('.price-value').innerText
//	JSON.stringify(window.__REDUX_STATE__.prices)
//	document.querySelectorAll('table tr td:nth-child(2)')
//	  .map(e => e.innerText).join(',') // extract 2nd column
func (b *BrowserSession) ExtractJS(expression string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.Ctx == nil {
		return "", fmt.Errorf("no active browser session; call navigate first")
	}

	// Wrap in String() to handle non-string return values gracefully.
	wrapped := fmt.Sprintf(`(function(){ try { return String(%s); } catch(e){ return 'JS error: ' + e.message; } })()`, expression)

	var result string
	err := chromedp.Run(b.Ctx, chromedp.Evaluate(wrapped, &result))
	if err != nil {
		return "", fmt.Errorf("chromedp ExtractJS: %w", err)
	}
	return result, nil
}

// Screenshot captures the current page as a PNG and saves it to outPath.
// Returns the saved file path on success. Use this for visual debugging or
// Human-in-the-Loop review of the current browser state.
func (b *BrowserSession) Screenshot(outPath string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.Ctx == nil {
		return "", fmt.Errorf("no active browser session; call navigate first")
	}
	if outPath == "" {
		outPath = fmt.Sprintf("/tmp/browser_screenshot_%d.png", time.Now().UnixMilli())
	}

	var buf []byte
	err := chromedp.Run(b.Ctx, chromedp.FullScreenshot(&buf, 90))
	if err != nil {
		return "", fmt.Errorf("chromedp screenshot: %w", err)
	}
	if dir := filepath.Dir(outPath); dir != "." && dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}
	if err := os.WriteFile(outPath, buf, 0644); err != nil {
		return "", fmt.Errorf("screenshot write: %w", err)
	}
	return fmt.Sprintf("Screenshot saved: %s (%d bytes)", outPath, len(buf)), nil
}

// Back navigates backwards in browser history.
func (b *BrowserSession) Back() (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ensureContextLocked()

	err := chromedp.Run(b.Ctx,
		chromedp.NavigateBack(),
		chromedp.Sleep(400*time.Millisecond),
	)
	if err != nil {
		return "", fmt.Errorf("chromedp back: %w", err)
	}
	b.updateStateLocked()
	return b.renderViewLocked(), nil
}

// Reload refreshes the active webpage.
func (b *BrowserSession) Reload() (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ensureContextLocked()

	err := chromedp.Run(b.Ctx,
		chromedp.Reload(),
		chromedp.Sleep(400*time.Millisecond),
	)
	if err != nil {
		return "", fmt.Errorf("chromedp reload: %w", err)
	}
	b.updateStateLocked()
	return b.renderViewLocked(), nil
}

// SaveCookies exports the active browser session cookies as JSON.
func (b *BrowserSession) SaveCookies() (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ensureContextLocked()

	var cookies []*network.Cookie
	err := chromedp.Run(b.Ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		cookies, err = network.GetCookies().Do(ctx)
		return err
	}))
	if err != nil {
		return "", fmt.Errorf("chromedp get cookies: %w", err)
	}

	outBytes, err := json.MarshalIndent(cookies, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal cookies: %w", err)
	}
	return string(outBytes), nil
}

// LoadCookies imports cookie JSON data into the active browser session.
func (b *BrowserSession) LoadCookies(cookiesJSON string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ensureContextLocked()

	var cookies []*network.CookieParam
	if err := json.Unmarshal([]byte(cookiesJSON), &cookies); err != nil {
		return "", fmt.Errorf("failed to parse cookie JSON array: %w", err)
	}

	err := chromedp.Run(b.Ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.SetCookies(cookies).Do(ctx)
	}))
	if err != nil {
		return "", fmt.Errorf("chromedp set cookies: %w", err)
	}
	return fmt.Sprintf("Successfully imported %d cookies into browser session.", len(cookies)), nil
}

// updateStateLocked parses elements and page visible text using Javascript inside Chrome.
func (b *BrowserSession) updateStateLocked() {
	if b.Ctx == nil {
		return
	}

	var elementsJSON string
	jsScript := `(function() {
		function cleanText(str) {
			if (!str) return "";
			return str.replace(/<[^>]*>/g, " ").replace(/\s+/g, " ").trim();
		}

		function isCookieNoise(el) {
			let cur = el;
			while (cur && cur !== document.body) {
				let id = (cur.id || "").toLowerCase();
				let cls = (cur.className || "").toString().toLowerCase();
				if (id.includes("onetrust") || id.includes("cookie") || id.includes("consent") || id.includes("privacy-policy") ||
				    cls.includes("onetrust") || cls.includes("cookie") || cls.includes("consent") || cls.includes("banner")) {
					return true;
				}
				cur = cur.parentElement;
			}
			return false;
		}

		// Auto-dismiss blocking cookie consent banners (e.g. Google #L2AGLb, OneTrust, etc.)
		try {
			const consentSelectors = [
				'#L2AGLb', // Google EU "Alle akzeptieren" / "Accept all"
				'button#introAgreeButton',
				'button[id*="accept"]',
				'button[id*="agree"]',
				'button[data-testid*="agree"]',
				'button[data-testid*="accept"]'
			];
			for (const sel of consentSelectors) {
				const btn = document.querySelector(sel);
				if (btn && btn.offsetParent !== null && typeof btn.click === 'function') {
					btn.click();
					break;
				}
			}
		} catch (e) {}

		let items = [];
		let idx = 1;
		let allEls = document.querySelectorAll('a, button, input, select, textarea');
		for (let el of allEls) {
			if (items.length >= 100) break; // Cap at 100 clean elements max

			let tagName = el.tagName.toLowerCase();
			let type = tagName;
			if (tagName === 'input') {
				type = el.getAttribute('type') || 'text';
			}

			if (type === 'hidden') continue;
			if (isCookieNoise(el)) continue;

			// Skip zero-dimension non-visible elements
			if (el.offsetWidth === 0 && el.offsetHeight === 0 && !el.getClientRects().length) continue;

			el.setAttribute('data-agent-index', idx);

			let name = el.getAttribute('name') || el.getAttribute('id') || el.getAttribute('placeholder') || '';
			let text = el.innerText || el.value || el.getAttribute('value') || '';
			let target = el.getAttribute('href') || el.getAttribute('action') || '';

			let normType = "input";
			if (tagName === 'a') {
				normType = "link";
			} else if (tagName === 'button' || type === 'submit' || type === 'button') {
				normType = "button";
			}

			items.push({
				index: idx,
				type: normType,
				name: name,
				text: cleanText(text).substring(0, 40),
				target: target
			});
			idx++;
		}

		let bodyClone = document.body.cloneNode(true);
		let removeScripts = bodyClone.querySelectorAll('script, style, noscript, svg, nav, footer');
		for (let s of removeScripts) {
			s.remove();
		}

		return JSON.stringify({
			text: cleanText(bodyClone.innerText).substring(0, 2500),
			elements: items
		});
	})();`

	_ = chromedp.Run(b.Ctx,
		chromedp.Evaluate(jsScript, &elementsJSON),
		chromedp.Location(&b.CurrentURL),
	)

	if elementsJSON != "" {
		type pageState struct {
			Elements []BrowserElement `json:"elements"`
			Text     string           `json:"text"`
		}

		var ps pageState
		if err := json.Unmarshal([]byte(elementsJSON), &ps); err == nil {
			b.Elements = ps.Elements
			b.PageText = ps.Text

			lowerText := strings.ToLower(b.PageText)
			lowerURL := strings.ToLower(b.CurrentURL)
			if strings.Contains(lowerURL, "/sorry/index") ||
				strings.Contains(lowerText, "unusual traffic") ||
				strings.Contains(lowerText, "not a robot") ||
				strings.Contains(lowerText, "g-recaptcha") ||
				strings.Contains(lowerText, "hcaptcha") ||
				strings.Contains(lowerText, "cf-turnstile") ||
				strings.Contains(lowerText, "recaptcha") ||
				strings.Contains(lowerText, "bot verification") ||
				strings.Contains(lowerText, "verify you are human") ||
				strings.Contains(lowerText, "please verify you are human") ||
				strings.Contains(lowerText, "security check") ||
				strings.Contains(lowerText, "attention required") {
				b.HasCaptcha = true
				b.CaptchaNotice = "[CAPTCHA DETECTED] A bot verification or CAPTCHA prompt was detected on the page. Human intervention required!"
			} else {
				b.HasCaptcha = false
				b.CaptchaNotice = ""
			}
		}
	}
}

// RenderView formats the text output showing page text and interactables.
func (b *BrowserSession) RenderView() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.renderViewLocked()
}

func (b *BrowserSession) renderViewLocked() string {
	var sb strings.Builder
	if b.HasCaptcha {
		sb.WriteString("⚠️  [CAPTCHA DETECTED] Human-in-the-Loop review required! Solve CAPTCHA in open browser window.\n")
	}
	sb.WriteString(fmt.Sprintf("URL: %s\n", b.CurrentURL))
	sb.WriteString("--------------------------------------------------------------------------------\n")
	sb.WriteString(b.PageText)
	sb.WriteString("\n--------------------------------------------------------------------------------\n")
	sb.WriteString("Interactable Elements:\n")

	hasInteractables := false
	for _, el := range b.Elements {
		hasInteractables = true
		switch el.Type {
		case "link":
			sb.WriteString(fmt.Sprintf("  [%d] Link: %q (href: %s)\n", el.Index, el.Text, el.Target))
		case "input":
			valStr := ""
			if v, ok := b.Inputs[el.Index]; ok && v != "" {
				valStr = fmt.Sprintf(" [Value: %q]", v)
			}
			sb.WriteString(fmt.Sprintf("  [%d] Input field (name: %q, type: %q)%s\n", el.Index, el.Name, el.Target, valStr))
		case "button":
			sb.WriteString(fmt.Sprintf("  [%d] Button: %q (type: %s)\n", el.Index, el.Text, el.Target))
		}
	}

	if !hasInteractables {
		sb.WriteString("  (None found)\n")
	}

	return sb.String()
}

// ListTabs returns all open browser tabs (pages) with their index, title, URL, and active status.
func (b *BrowserSession) ListTabs() ([]BrowserTab, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.listTabsLocked()
}

// NewTab opens a new tab, navigates to initialURL, brings it to the front, and updates state.
func (b *BrowserSession) NewTab(initialURL string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.newTabLocked(initialURL)
}

// SwitchTab activates and focuses the specified tab by 1-based index, target ID, or title/URL substring.
func (b *BrowserSession) SwitchTab(identifier string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.switchTabLocked(identifier)
}

// CloseTab closes the specified tab (or the active tab if empty) and switches focus to a remaining tab.
func (b *BrowserSession) CloseTab(identifier string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closeTabLocked(identifier)
}

func (b *BrowserSession) listTabsLocked() ([]BrowserTab, error) {
	b.ensureContextLocked()

	targets, err := chromedp.Targets(b.Ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve browser targets: %w", err)
	}

	if b.activeTabID == "" {
		if c := chromedp.FromContext(b.Ctx); c != nil && c.Target != nil {
			b.activeTabID = c.Target.TargetID
		}
	}

	targetMap := make(map[target.ID]*target.Info)
	for _, t := range targets {
		if t.Type == "page" {
			targetMap[t.TargetID] = t
		}
	}

	// Reconcile tabOrder: preserve existing stable tab order and append newly discovered targets
	var validOrder []target.ID
	for _, id := range b.tabOrder {
		if _, exists := targetMap[id]; exists {
			validOrder = append(validOrder, id)
		}
	}
	for _, t := range targets {
		if t.Type == "page" {
			alreadyInOrder := false
			for _, id := range validOrder {
				if id == t.TargetID {
					alreadyInOrder = true
					break
				}
			}
			if !alreadyInOrder {
				validOrder = append(validOrder, t.TargetID)
			}
		}
	}
	b.tabOrder = validOrder

	var tabs []BrowserTab
	for idx, id := range b.tabOrder {
		t := targetMap[id]
		if t == nil {
			continue
		}
		isActive := false
		if b.activeTabID != "" && t.TargetID == b.activeTabID {
			isActive = true
		} else if b.activeTabID == "" && idx == 0 {
			isActive = true
			b.activeTabID = t.TargetID
		}
		tabs = append(tabs, BrowserTab{
			Index:    idx + 1,
			TargetID: t.TargetID,
			Title:    t.Title,
			URL:      t.URL,
			Active:   isActive,
		})
	}
	return tabs, nil
}

func (b *BrowserSession) newTabLocked(initialURL string) (string, error) {
	b.ensureContextLocked()

	if initialURL == "" {
		initialURL = "about:blank"
	}

	var newTargetID target.ID
	err := chromedp.Run(b.Ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		id, err := target.CreateTarget(initialURL).Do(ctx)
		if err != nil {
			return err
		}
		newTargetID = id
		return nil
	}))
	if err != nil {
		return "", fmt.Errorf("failed to create new tab: %w", err)
	}

	b.tabOrder = append(b.tabOrder, newTargetID)

	tabCtx, tabCancel := chromedp.NewContext(b.browserCtx, chromedp.WithTargetID(newTargetID))
	b.tabContexts[newTargetID] = tabCtx
	b.tabCancels[newTargetID] = tabCancel

	_ = chromedp.Run(tabCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		_, err := page.AddScriptToEvaluateOnNewDocument(stealthScript).Do(ctx)
		return err
	}))

	_ = chromedp.Run(tabCtx, target.ActivateTarget(newTargetID))

	b.Ctx = tabCtx
	b.Cancel = cancelFuncWrapper(tabCancel)
	b.activeTabID = newTargetID

	b.updateStateLocked()
	return fmt.Sprintf("Opened new tab (Target: %s, URL: %s) and activated.\n\nCurrent Page:\n%s", newTargetID, initialURL, b.renderViewLocked()), nil
}

func (b *BrowserSession) switchTabLocked(identifier string) (string, error) {
	b.ensureContextLocked()

	tabs, err := b.listTabsLocked()
	if err != nil {
		return "", err
	}
	if len(tabs) == 0 {
		return "", fmt.Errorf("no open tabs available")
	}

	var selectedTab *BrowserTab
	trimmed := strings.TrimSpace(identifier)

	// 1. Match by numeric index (e.g. "1", "2")
	if n, err := strconv.Atoi(trimmed); err == nil && n >= 1 && n <= len(tabs) {
		selectedTab = &tabs[n-1]
	}

	// 2. Match by target ID
	if selectedTab == nil {
		for i := range tabs {
			if string(tabs[i].TargetID) == trimmed || strings.HasPrefix(string(tabs[i].TargetID), trimmed) {
				selectedTab = &tabs[i]
				break
			}
		}
	}

	// 3. Match by Title or URL substring
	if selectedTab == nil && trimmed != "" {
		lower := strings.ToLower(trimmed)
		for i := range tabs {
			if strings.Contains(strings.ToLower(tabs[i].Title), lower) || strings.Contains(strings.ToLower(tabs[i].URL), lower) {
				selectedTab = &tabs[i]
				break
			}
		}
	}

	if selectedTab == nil {
		var available []string
		for _, t := range tabs {
			available = append(available, fmt.Sprintf("[%d] %q (%s)", t.Index, t.Title, t.URL))
		}
		return "", fmt.Errorf("tab %q not found. Available tabs:\n%s", identifier, strings.Join(available, "\n"))
	}

	targetID := selectedTab.TargetID

	_ = chromedp.Run(b.Ctx, target.ActivateTarget(targetID))

	tabCtx, ok := b.tabContexts[targetID]
	if !ok || tabCtx.Err() != nil {
		var tabCancel context.CancelFunc
		tabCtx, tabCancel = chromedp.NewContext(b.browserCtx, chromedp.WithTargetID(targetID))
		b.tabContexts[targetID] = tabCtx
		b.tabCancels[targetID] = tabCancel
		_ = chromedp.Run(tabCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(stealthScript).Do(ctx)
			return err
		}))
	}

	b.Ctx = tabCtx
	b.Cancel = cancelFuncWrapper(b.tabCancels[targetID])
	b.activeTabID = targetID

	b.updateStateLocked()
	return fmt.Sprintf("Switched to Tab [%d] %q (%s)\n\nCurrent Page:\n%s", selectedTab.Index, selectedTab.Title, selectedTab.URL, b.renderViewLocked()), nil
}

func (b *BrowserSession) closeTabLocked(identifier string) (string, error) {
	b.ensureContextLocked()

	tabs, err := b.listTabsLocked()
	if err != nil {
		return "", err
	}
	if len(tabs) == 0 {
		return "", fmt.Errorf("no open tabs to close")
	}

	var targetToClose *BrowserTab
	trimmed := strings.TrimSpace(identifier)

	if trimmed == "" {
		for i := range tabs {
			if tabs[i].Active {
				targetToClose = &tabs[i]
				break
			}
		}
		if targetToClose == nil && len(tabs) > 0 {
			targetToClose = &tabs[0]
		}
	} else {
		if n, err := strconv.Atoi(trimmed); err == nil && n >= 1 && n <= len(tabs) {
			targetToClose = &tabs[n-1]
		} else {
			for i := range tabs {
				if string(tabs[i].TargetID) == trimmed || strings.HasPrefix(string(tabs[i].TargetID), trimmed) {
					targetToClose = &tabs[i]
					break
				}
			}
		}
	}

	if targetToClose == nil {
		return "", fmt.Errorf("tab %q not found to close", identifier)
	}

	if cancel, ok := b.tabCancels[targetToClose.TargetID]; ok && cancel != nil {
		cancel()
	}
	delete(b.tabContexts, targetToClose.TargetID)
	delete(b.tabCancels, targetToClose.TargetID)

	var newOrder []target.ID
	for _, id := range b.tabOrder {
		if id != targetToClose.TargetID {
			newOrder = append(newOrder, id)
		}
	}
	b.tabOrder = newOrder

	_ = chromedp.Run(b.Ctx, target.CloseTarget(targetToClose.TargetID))

	remaining, _ := b.listTabsLocked()
	if len(remaining) == 0 {
		return b.newTabLocked("about:blank")
	}

	newActive := remaining[0]
	targetID := newActive.TargetID
	_ = chromedp.Run(b.browserCtx, target.ActivateTarget(targetID))

	tabCtx, ok := b.tabContexts[targetID]
	if !ok || tabCtx.Err() != nil {
		var tabCancel context.CancelFunc
		tabCtx, tabCancel = chromedp.NewContext(b.browserCtx, chromedp.WithTargetID(targetID))
		b.tabContexts[targetID] = tabCtx
		b.tabCancels[targetID] = tabCancel
	}
	b.Ctx = tabCtx
	b.Cancel = cancelFuncWrapper(b.tabCancels[targetID])
	b.activeTabID = targetID

	b.updateStateLocked()
	return fmt.Sprintf("Closed tab [%d] %q. Switched to Tab [%d] %q (%s)\n\nCurrent Page:\n%s",
		targetToClose.Index, targetToClose.Title, newActive.Index, newActive.Title, newActive.URL, b.renderViewLocked()), nil
}

// FormatTabsSummary formats the tab list into a clean, human-readable summary.
func FormatTabsSummary(tabs []BrowserTab) string {
	if len(tabs) == 0 {
		return "No open browser tabs."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Open Browser Tabs (%d total):\n", len(tabs)))
	for _, t := range tabs {
		status := ""
		prefix := "  "
		if t.Active {
			status = " [ACTIVE]"
			prefix = "* "
		}
		title := t.Title
		if title == "" {
			title = "(Untitled)"
		}
		sb.WriteString(fmt.Sprintf("%s[%d] %s (%s)%s\n", prefix, t.Index, title, t.URL, status))
	}
	return sb.String()
}
