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

func TestBrowserElementIndexAliases(t *testing.T) {
	sessionID := "test_alias_session"
	session := GlobalSessionManager.GetSession(sessionID)
	defer session.Close()

	// Navigate to minimal HTML page with an input field tagged with data-agent-index="1"
	html := `data:text/html,<html><body><input data-agent-index="1" id="test-input" name="username"></body></html>`
	_, err := session.Navigate(html)
	if err != nil {
		t.Fatalf("failed to initialize session: %v", err)
	}

	session.Elements = []BrowserElement{
		{Index: 1, Type: "input", Name: "username"},
	}

	inputTool := GetBrowserInputTool()
	ctx := context.WithValue(context.Background(), "session_id", sessionID)

	// 1. Test input with "index" string
	_, err = inputTool.Execute(ctx, []byte(`{"index": "1", "value": "alice"}`))
	if err != nil {
		t.Fatalf("unexpected error with index alias: %v", err)
	}
	if session.Inputs[1] != "alice" {
		t.Errorf("expected input 1 to be 'alice', got %q", session.Inputs[1])
	}

	// 2. Test input with nested action_input
	_, err = inputTool.Execute(ctx, []byte(`{"action_input": {"element": 1, "text": "bob"}}`))
	if err != nil {
		t.Fatalf("unexpected error with nested action_input: %v", err)
	}
	if session.Inputs[1] != "bob" {
		t.Errorf("expected input 1 to be 'bob', got %q", session.Inputs[1])
	}

	// 3. Test invalid/zero index returns clear error
	_, err = inputTool.Execute(ctx, []byte(`{"index": 0, "value": "test"}`))
	if err == nil {
		t.Errorf("expected error for index 0, got nil")
	}

	// 4. Test click on index 0 returns clear error
	clickTool := GetBrowserClickTool()
	_, err = clickTool.Execute(ctx, []byte(`{"index": 0}`))
	if err == nil {
		t.Errorf("expected error for click on index 0, got nil")
	}

	// 5. Test navigate with 'href' alias
	navTool := GetBrowserNavigateTool()
	_, err = navTool.Execute(ctx, []byte(`{"href": "data:text/html,<h1>Hello</h1>"}`))
	if err != nil {
		t.Errorf("expected navigation via 'href' alias to succeed, got %v", err)
	}
}

func TestBrowserMultiTabManagement(t *testing.T) {
	sessionID := "test_multitab_session"
	session := GlobalSessionManager.GetSession(sessionID)
	defer session.Close()

	// 1. Initial page on Tab 1
	tab1HTML := `data:text/html,<html><head><title>Tab 1</title></head><body><h1>Tab 1 Content</h1></body></html>`
	_, err := session.Navigate(tab1HTML)
	if err != nil {
		t.Fatalf("failed to navigate initial tab: %v", err)
	}

	tabs, err := session.ListTabs()
	if err != nil {
		t.Fatalf("failed to list tabs: %v", err)
	}
	if len(tabs) != 1 {
		t.Fatalf("expected 1 tab initially, got %d", len(tabs))
	}
	if !tabs[0].Active {
		t.Errorf("expected tab 0 to be active")
	}

	// 2. Open Tab 2
	tab2HTML := `data:text/html,<html><head><title>Tab 2</title></head><body><h1>Tab 2 Content</h1></body></html>`
	out, err := session.NewTab(tab2HTML)
	if err != nil {
		t.Fatalf("failed to open new tab: %v", err)
	}
	if !strings.Contains(out, "Tab 2 Content") {
		t.Errorf("expected Tab 2 content in output, got: %s", out)
	}

	tabs, err = session.ListTabs()
	if err != nil {
		t.Fatalf("failed to list tabs after open: %v", err)
	}
	if len(tabs) != 2 {
		t.Fatalf("expected 2 tabs, got %d", len(tabs))
	}
	if !tabs[1].Active {
		t.Errorf("expected tab 2 (index 1) to be active after NewTab")
	}

	// 3. Switch back to Tab 1
	out, err = session.SwitchTab("1")
	if err != nil {
		t.Fatalf("failed to switch back to tab 1: %v", err)
	}
	if !strings.Contains(out, "Tab 1 Content") {
		t.Errorf("expected Tab 1 content after switch, got: %s", out)
	}

	tabs, err = session.ListTabs()
	if err != nil {
		t.Fatalf("failed to list tabs after switch: %v", err)
	}
	if !tabs[0].Active {
		t.Errorf("expected tab 1 (index 0) to be active after SwitchTab")
	}

	// 4. Test tool wrappers
	ctx := context.WithValue(context.Background(), "session_id", sessionID)
	listTool := GetBrowserListTabsTool()
	listOut, err := listTool.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("list tabs tool error: %v", err)
	}
	if !strings.Contains(listOut, "Open Browser Tabs (2 total)") {
		t.Errorf("expected 2 total tabs in list output, got: %s", listOut)
	}

	switchTool := GetBrowserSwitchTabTool()
	switchOut, err := switchTool.Execute(ctx, []byte(`{"tab": "2"}`))
	if err != nil {
		t.Fatalf("switch tab tool error: %v", err)
	}
	if !strings.Contains(switchOut, "Tab 2") {
		t.Errorf("expected switch to Tab 2, got: %s", switchOut)
	}

	closeTool := GetBrowserCloseTabTool()
	closeOut, err := closeTool.Execute(ctx, []byte(`{"tab": "2"}`))
	if err != nil {
		t.Fatalf("close tab tool error: %v", err)
	}
	if !strings.Contains(closeOut, "Closed tab [2]") {
		t.Errorf("expected confirmation of closed tab 2, got: %s", closeOut)
	}

	tabs, err = session.ListTabs()
	if err != nil {
		t.Fatalf("failed to list tabs after close: %v", err)
	}
	if len(tabs) != 1 {
		t.Errorf("expected 1 remaining tab after close, got %d", len(tabs))
	}
}




