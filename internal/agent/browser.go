package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

type BrowserElement struct {
	Index  int    `json:"index"`
	Type   string `json:"type"`   // "link", "input", "button"
	Name   string `json:"name"`   // For form fields (inputs)
	Text   string `json:"text"`   // Display text or button label
	Target string `json:"target"` // URL for links, or input type
	FormID string `json:"form_id"` // Associated form identifier (deprecated)
}

type BrowserSession struct {
	mu            sync.Mutex
	Ctx           context.Context
	Cancel        cancelFuncWrapper
	// Security (L-4): track the allocator cancel separately so both
	// the chromedp context and its exec allocator are cleaned up on Close.
	allocCancel   context.CancelFunc
	CurrentURL    string
	PageText      string
	Elements      []BrowserElement
	Inputs        map[int]string // Maps element index -> user filled value
	HasCaptcha    bool
	CaptchaNotice string
}

type cancelFuncWrapper context.CancelFunc

// Close releases the chromedp context and exec allocator, cleaning up the
// Chrome process. Both cancels must be called to avoid orphaned processes (L-4).
func (b *BrowserSession) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.Cancel != nil {
		context.CancelFunc(b.Cancel)()
		b.Ctx = nil
		b.Cancel = nil
	}
	if b.allocCancel != nil {
		b.allocCancel()
		b.allocCancel = nil
	}
}

// BrowserSessionManager stores thread-safe browser sessions isolated by session ID.
type BrowserSessionManager struct {
	mu       sync.Mutex
	sessions map[string]*BrowserSession
}

func (m *BrowserSessionManager) GetSession(sessionID string) *BrowserSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	if sessionID == "" {
		sessionID = "default"
	}
	s, ok := m.sessions[sessionID]
	if !ok {
		s = &BrowserSession{
			Inputs: make(map[int]string),
		}
		m.sessions[sessionID] = s
	}
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

	// Initialize stealth chromedp session if not exists
	if b.Ctx == nil {
		isHeadless := true
		if os.Getenv("HEADED") == "true" || os.Getenv("HEADLESS") == "false" || os.Getenv("BROWSER_HEADED") == "true" {
			isHeadless = false
		}
		opts := append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.NoSandbox,
			chromedp.Flag("disable-gpu", true),
			chromedp.Flag("headless", isHeadless),
			// Stealth flags: mask automation flags & set realistic viewport/User-Agent
			chromedp.Flag("disable-blink-features", "AutomationControlled"),
			chromedp.UserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"),
			chromedp.WindowSize(1440, 900),
			chromedp.Flag("accept-lang", "en-US,en;q=0.9"),
		)

		if proxy := os.Getenv("BROWSER_PROXY"); proxy != "" {
			opts = append(opts, chromedp.Flag("proxy-server", proxy))
		} else if proxy := os.Getenv("HTTP_PROXY"); proxy != "" {
			opts = append(opts, chromedp.Flag("proxy-server", proxy))
		}

		if userDataDir := os.Getenv("BROWSER_USER_DATA_DIR"); userDataDir != "" {
			opts = append(opts, chromedp.UserDataDir(userDataDir))
		}

		allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
		ctx, cancel := chromedp.NewContext(allocCtx)
		b.Ctx = ctx
		b.Cancel = cancelFuncWrapper(cancel)
		b.allocCancel = allocCancel // Security (L-4): stored so Close() can release allocator

		// Inject CDP stealth overrides before page load to hide automated browser signatures
		const stealthScript = `(function() {
			Object.defineProperty(navigator, 'webdriver', { get: () => undefined });
			window.chrome = { runtime: {}, loadTimes: function() {}, csi: function() {}, app: {} };
			Object.defineProperty(navigator, 'languages', { get: () => ['en-US', 'en'] });
			Object.defineProperty(navigator, 'plugins', { get: () => [1, 2, 3, 4, 5] });
			const getParameter = WebGLRenderingContext.prototype.getParameter;
			WebGLRenderingContext.prototype.getParameter = function(parameter) {
				if (parameter === 37445) return 'Intel Inc.';
				if (parameter === 37446) return 'Intel Iris OpenGL Engine';
				return getParameter.apply(this, arguments);
			};
			if (window.navigator.permissions) {
				const origQuery = window.navigator.permissions.query;
				window.navigator.permissions.query = (params) => (
					params && params.name === 'notifications' ?
						Promise.resolve({ state: Notification.permission }) :
						origQuery(params)
				);
			}
		})();`

		_ = chromedp.Run(b.Ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(stealthScript).Do(ctx)
			return err
		}))
	}

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
	err := chromedp.Run(b.Ctx,
		chromedp.Focus(sel, chromedp.ByQuery),
		chromedp.Evaluate(fmt.Sprintf(`document.querySelector("%s").value = ""; document.querySelector("%s").dispatchEvent(new Event('input', { bubbles: true }));`, sel, sel), nil),
	)
	if err != nil {
		return "", fmt.Errorf("chromedp input focus: %w", err)
	}

	// Humanized typing per character with randomized inter-key delay (30ms - 75ms)
	for _, ch := range val {
		_ = chromedp.Run(b.Ctx, chromedp.SendKeys(sel, string(ch), chromedp.ByQuery))
		time.Sleep(time.Duration(30+rand.Intn(45)) * time.Millisecond)
	}

	// Trigger input & change events for frameworks (React, Vue, Angular)
	_ = chromedp.Run(b.Ctx,
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

	err := chromedp.Run(b.Ctx,
		chromedp.Evaluate(scrollHoverScript, nil),
		chromedp.Sleep(time.Duration(150+rand.Intn(150))*time.Millisecond),
		chromedp.Click(sel, chromedp.ByQuery),
		chromedp.Sleep(time.Duration(450+rand.Intn(300))*time.Millisecond), // SPA transition delay
	)
	if err != nil {
		return "", fmt.Errorf("chromedp click: %w", err)
	}

	b.updateStateLocked()
	return b.renderViewLocked(), nil
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

	script := fmt.Sprintf("window.scrollBy(0, %d);", dy)
	err := chromedp.Run(b.Ctx,
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

		let visibleText = document.body.innerText || "";
		visibleText = visibleText.replace(/\s+/g, " ").trim();

		return JSON.stringify({
			elements: items,
			text: visibleText.substring(0, 12000)
		});
	})()`

	err := chromedp.Run(b.Ctx,
		chromedp.Evaluate(jsScript, &elementsJSON),
		chromedp.Location(&b.CurrentURL),
	)
	if err == nil {
		type pageState struct {
			Elements []BrowserElement `json:"elements"`
			Text     string           `json:"text"`
		}

		var ps pageState
		if err := json.Unmarshal([]byte(elementsJSON), &ps); err == nil {
			b.Elements = ps.Elements
			b.PageText = ps.Text

			lowerText := strings.ToLower(b.PageText)
			if strings.Contains(lowerText, "g-recaptcha") ||
				strings.Contains(lowerText, "hcaptcha") ||
				strings.Contains(lowerText, "cf-turnstile") ||
				strings.Contains(lowerText, "recaptcha") ||
				strings.Contains(lowerText, "bot verification") ||
				strings.Contains(lowerText, "verify you are human") ||
				strings.Contains(lowerText, "please verify you are human") {
				b.HasCaptcha = true
				b.CaptchaNotice = "[CAPTCHA DETECTED] A CAPTCHA or bot verification prompt was detected on the page. Human intervention required!"
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
