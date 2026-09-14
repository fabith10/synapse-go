package agenttools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/fabith10/synapse-go/adk"
)

// GetBrowserNavigateTool returns a native tool to load a webpage and show structured elements.
func GetBrowserNavigateTool() adk.Tool {
	return adk.Tool{
		Name:        "browser_navigate",
		Description: "Loads the target URL inside the stateful simulated web browser and returns the text body and interactable elements.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url": map[string]interface{}{
					"type":        "string",
					"description": "The target website URL to navigate to.",
				},
				"headed": map[string]interface{}{
					"type":        "boolean",
					"description": "Optional flag to run browser in visible headed mode (window on screen) so a user can watch the session.",
				},
			},
			"required": []string{"url"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var raw map[string]interface{}
			if err := json.Unmarshal(args, &raw); err != nil {
				return "", fmt.Errorf("failed to parse navigation URL: %w", err)
			}
			targetURL := extractStringAlias(raw, "url", "target_url", "link", "address", "uri", "target", "href", "destination", "site", "webpage")
			if targetURL == "" {
				return "", fmt.Errorf("missing required parameter 'url'")
			}
			if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") &&
				!strings.HasPrefix(targetURL, "about:") && !strings.HasPrefix(targetURL, "data:") && !strings.HasPrefix(targetURL, "file://") {
				targetURL = "https://" + targetURL
			}
			sessionID, _ := ctx.Value("session_id").(string)
			session := GlobalSessionManager.GetSession(sessionID)
			if headedVal, ok := raw["headed"].(bool); ok {
				session.SetHeaded(headedVal)
			}
			return session.Navigate(targetURL)
		},
	}
}

// GetBrowserInputTool returns a native tool to input values into form fields.
func GetBrowserInputTool() adk.Tool {
	return adk.Tool{
		Name:        "browser_input",
		Description: "Enters text value into a specific numbered input field identified in the browser session.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"element_index": map[string]interface{}{
					"type":        "integer",
					"description": "The index number of the target input field (e.g. 3).",
				},
				"text_value": map[string]interface{}{
					"type":        "string",
					"description": "The text content to input into the field.",
				},
			},
			"required": []string{"element_index", "text_value"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var raw map[string]interface{}
			if err := json.Unmarshal(args, &raw); err != nil {
				return "", fmt.Errorf("failed to parse input arguments: %w", err)
			}
			idx := extractIntAlias(raw, "element_index", "element_id", "index", "id", "element", "target_element", "target_index", "target")
			textVal := extractStringAlias(raw, "text_value", "text", "value", "content", "input")
			if idx <= 0 {
				return "", fmt.Errorf("missing or invalid required parameter 'element_index' (must be a positive integer matching a numbered element)")
			}
			sessionID, _ := ctx.Value("session_id").(string)
			return GlobalSessionManager.GetSession(sessionID).Input(idx, textVal)
		},
	}
}

// GetBrowserClickTool returns a native tool to click links or buttons.
func GetBrowserClickTool() adk.Tool {
	return adk.Tool{
		Name:        "browser_click",
		Description: "Simulates clicking a link or submit button by its index number, executing navigation or form submission.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"element_index": map[string]interface{}{
					"type":        "integer",
					"description": "The index number of the target clickable element (e.g. 5).",
				},
			},
			"required": []string{"element_index"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var raw map[string]interface{}
			if err := json.Unmarshal(args, &raw); err != nil {
				return "", fmt.Errorf("failed to parse click arguments: %w", err)
			}
			idx := extractIntAlias(raw, "element_index", "element_id", "index", "id", "element", "target_element", "target_index", "target")
			if idx <= 0 {
				return "", fmt.Errorf("missing or invalid required parameter 'element_index' (must be a positive integer matching a numbered element)")
			}
			sessionID, _ := ctx.Value("session_id").(string)
			return GlobalSessionManager.GetSession(sessionID).Click(idx)
		},
	}
}

// GetBrowserScrollTool returns a native tool to scroll the current browser page.
func GetBrowserScrollTool() adk.Tool {
	return adk.Tool{
		Name:        "browser_scroll",
		Description: "Scrolls the browser page vertically by a specified pixel delta (positive = down, negative = up) to trigger lazy-loaded elements or reveal content below the fold.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"dy": map[string]interface{}{
					"type":        "integer",
					"description": "Pixel amount to scroll vertically (e.g. 500 for scrolling down half a screen, -500 for up).",
				},
			},
			"required": []string{"dy"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var raw map[string]interface{}
			if err := json.Unmarshal(args, &raw); err != nil {
				return "", fmt.Errorf("failed to parse scroll arguments: %w", err)
			}
			dy := extractIntAlias(raw, "dy", "delta", "pixels", "amount", "y", "scroll_y")
			if dy == 0 {
				dy = 500
			}
			sessionID, _ := ctx.Value("session_id").(string)
			return GlobalSessionManager.GetSession(sessionID).Scroll(dy)
		},
	}
}

// GetBrowserWaitForTool returns a native tool to wait for a CSS selector in the browser page.
func GetBrowserWaitForTool() adk.Tool {
	return adk.Tool{
		Name:        "browser_wait",
		Description: "Waits until a specific CSS selector becomes visible on the current page or a timeout elapses. Crucial for SPAs and XHR-loaded dynamic content.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"selector": map[string]interface{}{
					"type":        "string",
					"description": "CSS selector to wait for (e.g. 'table.data-results', '#price-value', '.article-body').",
				},
				"timeout_sec": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum seconds to wait (default 10, max 30).",
				},
			},
			"required": []string{"selector"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				Selector   string `json:"selector"`
				TimeoutSec int    `json:"timeout_sec"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("failed to parse wait arguments: %w", err)
			}
			sessionID, _ := ctx.Value("session_id").(string)
			return GlobalSessionManager.GetSession(sessionID).WaitFor(params.Selector, params.TimeoutSec)
		},
	}
}

// GetBrowserExtractJSTool returns a native tool to execute JavaScript and extract custom values.
func GetBrowserExtractJSTool() adk.Tool {
	return adk.Tool{
		Name:        "browser_extract_js",
		Description: "Executes a custom JavaScript expression in the browser context and returns its stringified evaluation result.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"expression": map[string]interface{}{
					"type":        "string",
					"description": "JavaScript expression to evaluate (e.g. 'document.querySelector(\".price\").innerText').",
				},
			},
			"required": []string{"expression"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				Expression string `json:"expression"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("failed to parse extract_js arguments: %w", err)
			}
			sessionID, _ := ctx.Value("session_id").(string)
			return GlobalSessionManager.GetSession(sessionID).ExtractJS(params.Expression)
		},
	}
}

// GetBrowserScreenshotTool returns a native tool to capture a screenshot of the browser page.
func GetBrowserScreenshotTool() adk.Tool {
	return adk.Tool{
		Name:        "browser_screenshot",
		Description: "Captures a full PNG screenshot of the current browser page and saves it to a specified or auto-generated local file path.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"out_path": map[string]interface{}{
					"type":        "string",
					"description": "Optional output path to save the screenshot PNG file (e.g. '/tmp/page.png').",
				},
			},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				OutPath string `json:"out_path"`
			}
			if len(args) > 0 {
				_ = json.Unmarshal(args, &params)
			}
			// Default to working directory instead of /tmp
			if params.OutPath == "" {
				if rootVal, ok := ctx.Value(WorkspaceRootKey).(string); ok && rootVal != "" {
					params.OutPath = filepath.Join(rootVal, fmt.Sprintf("browser_screenshot_%d.png", time.Now().UnixMilli()))
				}
			} else if !filepath.IsAbs(params.OutPath) {
				if rootVal, ok := ctx.Value(WorkspaceRootKey).(string); ok && rootVal != "" {
					params.OutPath = filepath.Join(rootVal, params.OutPath)
				}
			}
			sessionID, _ := ctx.Value("session_id").(string)
			return GlobalSessionManager.GetSession(sessionID).Screenshot(params.OutPath)
		},
	}
}

// GetBrowserBackTool returns a native tool to navigate back in browser history.
func GetBrowserBackTool() adk.Tool {
	return adk.Tool{
		Name:        "browser_back",
		Description: "Navigates backwards to the previous page in browser history.",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			sessionID, _ := ctx.Value("session_id").(string)
			return GlobalSessionManager.GetSession(sessionID).Back()
		},
	}
}

// GetBrowserReloadTool returns a native tool to refresh the active webpage.
func GetBrowserReloadTool() adk.Tool {
	return adk.Tool{
		Name:        "browser_reload",
		Description: "Refreshes/reloads the current browser page.",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			sessionID, _ := ctx.Value("session_id").(string)
			return GlobalSessionManager.GetSession(sessionID).Reload()
		},
	}
}

// GetBrowserSaveCookiesTool returns a native tool to export browser cookies as JSON.
func GetBrowserSaveCookiesTool() adk.Tool {
	return adk.Tool{
		Name:        "browser_save_cookies",
		Description: "Exports active browser session cookies to a JSON string for session authentication persistence.",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			sessionID, _ := ctx.Value("session_id").(string)
			return GlobalSessionManager.GetSession(sessionID).SaveCookies()
		},
	}
}

// GetBrowserLoadCookiesTool returns a native tool to import cookie JSON data into the browser.
func GetBrowserLoadCookiesTool() adk.Tool {
	return adk.Tool{
		Name:        "browser_load_cookies",
		Description: "Imports cookie JSON string into active browser session to restore authenticated state.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"cookies_json": map[string]interface{}{
					"type":        "string",
					"description": "JSON array string containing exported browser cookies.",
				},
			},
			"required": []string{"cookies_json"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				CookiesJSON string `json:"cookies_json"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("failed to parse cookies_json: %w", err)
			}
			sessionID, _ := ctx.Value("session_id").(string)
			return GlobalSessionManager.GetSession(sessionID).LoadCookies(params.CookiesJSON)
		},
	}
}

// GetBrowserConfigureTool returns a native tool to configure browser runtime settings (e.g. headed vs headless).
func GetBrowserConfigureTool() adk.Tool {
	return adk.Tool{
		Name:        "browser_configure",
		Description: "Configures browser runtime settings, such as toggling between visible headed window mode and headless background mode.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"headed": map[string]interface{}{
					"type":        "boolean",
					"description": "Set to true to show a visible graphical browser window on screen, or false for headless background execution.",
				},
			},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				Headed *bool `json:"headed"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("failed to parse configure arguments: %w", err)
			}
			sessionID, _ := ctx.Value("session_id").(string)
			session := GlobalSessionManager.GetSession(sessionID)
			if params.Headed != nil {
				session.SetHeaded(*params.Headed)
				if *params.Headed {
					return "Browser session switched to HEADED mode (visible window on screen).", nil
				}
				return "Browser session switched to HEADLESS mode (background execution).", nil
			}
			return "No configuration changes applied.", nil
		},
	}
}

// GetBrowserListTabsTool returns a native tool to list all open browser tabs/windows.
func GetBrowserListTabsTool() adk.Tool {
	return adk.Tool{
		Name:        "browser_list_tabs",
		Description: "Lists all currently open browser tabs/windows with their index, title, URL, and active status.",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			sessionID, _ := ctx.Value("session_id").(string)
			tabs, err := GlobalSessionManager.GetSession(sessionID).ListTabs()
			if err != nil {
				return "", err
			}
			return FormatTabsSummary(tabs), nil
		},
	}
}

// GetBrowserNewTabTool returns a native tool to open a new tab in the browser.
func GetBrowserNewTabTool() adk.Tool {
	return adk.Tool{
		Name:        "browser_new_tab",
		Description: "Opens a new browser tab with the optional target URL, brings it to the front, and renders its interactables.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url": map[string]interface{}{
					"type":        "string",
					"description": "Optional initial URL to load in the new tab (defaults to 'about:blank').",
				},
			},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var raw map[string]interface{}
			if len(args) > 0 {
				_ = json.Unmarshal(args, &raw)
			}
			targetURL := extractStringAlias(raw, "url", "target_url", "link", "address", "uri", "target", "href")
			if targetURL != "" && !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") &&
				!strings.HasPrefix(targetURL, "about:") && !strings.HasPrefix(targetURL, "data:") && !strings.HasPrefix(targetURL, "file://") {
				targetURL = "https://" + targetURL
			}
			sessionID, _ := ctx.Value("session_id").(string)
			return GlobalSessionManager.GetSession(sessionID).NewTab(targetURL)
		},
	}
}

// GetBrowserSwitchTabTool returns a native tool to switch to a specific browser tab.
func GetBrowserSwitchTabTool() adk.Tool {
	return adk.Tool{
		Name:        "browser_switch_tab",
		Description: "Switches focus to an open browser tab by 1-based index (e.g. 1, 2) or target ID, brings it to the foreground in headed mode, and renders its page state.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"tab": map[string]interface{}{
					"type":        "string",
					"description": "Tab number (e.g. '1', '2'), target ID, or title/URL keyword.",
				},
			},
			"required": []string{"tab"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var raw map[string]interface{}
			if err := json.Unmarshal(args, &raw); err != nil {
				return "", fmt.Errorf("failed to parse switch tab arguments: %w", err)
			}
			targetTab := extractStringAlias(raw, "tab", "tab_index", "index", "target_id", "id", "name", "title")
			if targetTab == "" {
				if idx := extractIntAlias(raw, "tab", "tab_index", "index", "id"); idx > 0 {
					targetTab = fmt.Sprintf("%d", idx)
				}
			}
			if targetTab == "" {
				return "", fmt.Errorf("missing required parameter 'tab' (tab index e.g. 1 or 2)")
			}
			sessionID, _ := ctx.Value("session_id").(string)
			return GlobalSessionManager.GetSession(sessionID).SwitchTab(targetTab)
		},
	}
}

// GetBrowserCloseTabTool returns a native tool to close an open browser tab.
func GetBrowserCloseTabTool() adk.Tool {
	return adk.Tool{
		Name:        "browser_close_tab",
		Description: "Closes the specified browser tab (or the current active tab if unspecified) and switches focus to a remaining open tab.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"tab": map[string]interface{}{
					"type":        "string",
					"description": "Optional tab number (e.g. '2') or target ID to close. Closes active tab if omitted.",
				},
			},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var raw map[string]interface{}
			if len(args) > 0 {
				_ = json.Unmarshal(args, &raw)
			}
			targetTab := extractStringAlias(raw, "tab", "tab_index", "index", "target_id", "id")
			if targetTab == "" {
				if idx := extractIntAlias(raw, "tab", "tab_index", "index", "id"); idx > 0 {
					targetTab = fmt.Sprintf("%d", idx)
				}
			}
			sessionID, _ := ctx.Value("session_id").(string)
			return GlobalSessionManager.GetSession(sessionID).CloseTab(targetTab)
		},
	}
}


