package agenttools

import (
	"context"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/fabith10/synapse-go/adk"
)

func TestBrowserEnhancements(t *testing.T) {
	session := &BrowserSession{
		Inputs: make(map[int]string),
	}

	// 1. Test self-healing ensureContextLocked
	session.ensureContextLocked()
	if session.Ctx == nil {
		t.Fatalf("expected session.Ctx to be initialized")
	}

	// 2. Test Close cleanup
	session.Close()
	if session.Ctx != nil {
		t.Errorf("expected session.Ctx to be nil after Close")
	}

	// 3. Test re-initialization after Close (self-healing recovery)
	session.ensureContextLocked()
	if session.Ctx == nil {
		t.Fatalf("expected session.Ctx to self-heal after Close")
	}

	// Clean up after test
	session.Close()
}

func TestBrowserToolsConstructors(t *testing.T) {
	tools := []func() adk.Tool{
		GetBrowserBackTool,
		GetBrowserReloadTool,
		GetBrowserSaveCookiesTool,
		GetBrowserLoadCookiesTool,
	}

	for _, fn := range tools {
		tool := fn()
		if tool.Name == "" {
			t.Errorf("expected non-empty tool name")
		}
		if tool.Tier != adk.TierNative {
			t.Errorf("expected TierNative for %s", tool.Name)
		}
	}
}

func TestBrowserStealthEvaluation(t *testing.T) {
	session := &BrowserSession{
		Inputs: make(map[int]string),
	}
	session.ensureContextLocked()
	defer session.Close()

	if session.Ctx == nil {
		t.Fatalf("expected session.Ctx to be initialized")
	}

	var hasWebdriver, pluginsType bool
	var webdriverStr, pluginsLen, pluginsFirst, webglVendor string

	err := chromedp.Run(session.Ctx,
		chromedp.Navigate("about:blank"),
		chromedp.Evaluate(`'webdriver' in navigator`, &hasWebdriver),
		chromedp.Evaluate(`String(navigator.webdriver)`, &webdriverStr),
		chromedp.Evaluate(`String(navigator.plugins.length)`, &pluginsLen),
		chromedp.Evaluate(`navigator.plugins instanceof PluginArray`, &pluginsType),
		chromedp.Evaluate(`String(navigator.plugins[0])`, &pluginsFirst),
		chromedp.Evaluate(`(() => {
			const c = document.createElement('canvas');
			const gl = c.getContext('webgl');
			if (!gl) return 'no webgl';
			const ext = gl.getExtension('WEBGL_debug_renderer_info');
			return ext ? gl.getParameter(ext.UNMASKED_VENDOR_WEBGL) : 'no ext';
		})()`, &webglVendor),
	)
	if err != nil {
		t.Fatalf("chromedp evaluation failed: %v", err)
	}

	if hasWebdriver {
		t.Errorf("expected 'webdriver' in navigator to be false, got true")
	}
	if webdriverStr != "undefined" && webdriverStr != "" {
		t.Errorf("expected navigator.webdriver to be undefined, got %s", webdriverStr)
	}
	if !pluginsType {
		t.Errorf("expected navigator.plugins instanceof PluginArray to be true")
	}
	if pluginsLen == "0" || pluginsLen == "" {
		t.Errorf("expected non-zero plugins length, got %s", pluginsLen)
	}
	if pluginsFirst != "[object Plugin]" {
		t.Errorf("expected plugins[0].toString() == '[object Plugin]', got %s", pluginsFirst)
	}
	if webglVendor != "Intel Inc." {
		t.Errorf("expected masked WebGL vendor 'Intel Inc.', got %s", webglVendor)
	}
}

func TestBrowserHeadedConfiguration(t *testing.T) {
	session := &BrowserSession{
		Inputs: make(map[int]string),
	}
	defer session.Close()

	if session.Headed {
		t.Errorf("expected default Headed to be false")
	}

	session.SetHeaded(true)
	if !session.Headed || !session.IsHeaded() {
		t.Errorf("expected Headed to be true after SetHeaded(true)")
	}

	// Test tool configure execution
	cfgTool := GetBrowserConfigureTool()
	resp, err := cfgTool.Execute(context.Background(), []byte(`{"headed": false}`))
	if err != nil {
		t.Fatalf("unexpected error configuring browser: %v", err)
	}
	if !strings.Contains(resp, "HEADLESS") {
		t.Errorf("expected headless confirmation, got: %s", resp)
	}

	resp, err = cfgTool.Execute(context.Background(), []byte(`{"headed": true}`))
	if err != nil {
		t.Fatalf("unexpected error configuring browser: %v", err)
	}
	if !strings.Contains(resp, "HEADED") {
		t.Errorf("expected headed confirmation, got: %s", resp)
	}

	// Reset default
	GlobalSessionManager.SetDefaultHeaded(false)
	if GlobalSessionManager.IsDefaultHeaded() {
		t.Errorf("expected default headed to be false")
	}
}

func TestBrowserNavigateHeadedOption(t *testing.T) {
	navTool := GetBrowserNavigateTool()
	ctx := context.WithValue(context.Background(), "session_id", "test_headed_session")
	defer GlobalSessionManager.GetSession("test_headed_session").Close()

	out, err := navTool.Execute(ctx, []byte(`{"url": "about:blank", "headed": true}`))
	if err != nil {
		t.Fatalf("unexpected error navigating in headed mode: %v", err)
	}
	if !GlobalSessionManager.GetSession("test_headed_session").IsHeaded() {
		t.Errorf("expected session to be marked headed")
	}
	if !strings.Contains(out, "about:blank") {
		t.Errorf("expected URL in output, got: %s", out)
	}
}



