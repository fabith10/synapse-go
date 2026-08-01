package agent

import (
	"context"
	"encoding/json"
	"fmt"

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
			},
			"required": []string{"url"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				URL string `json:"url"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("failed to parse navigation URL: %w", err)
			}
			sessionID, _ := ctx.Value("session_id").(string)
			return GlobalSessionManager.GetSession(sessionID).Navigate(params.URL)
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
			var params struct {
				ElementIndex int    `json:"element_index"`
				ElementID    int    `json:"element_id"`
				TextValue    string `json:"text_value"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("failed to parse input arguments: %w", err)
			}
			idx := params.ElementIndex
			if idx == 0 && params.ElementID != 0 {
				idx = params.ElementID
			}
			sessionID, _ := ctx.Value("session_id").(string)
			return GlobalSessionManager.GetSession(sessionID).Input(idx, params.TextValue)
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
			var params struct {
				ElementIndex int `json:"element_index"`
				ElementID    int `json:"element_id"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("failed to parse click arguments: %w", err)
			}
			idx := params.ElementIndex
			if idx == 0 && params.ElementID != 0 {
				idx = params.ElementID
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
			var params struct {
				Dy int `json:"dy"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("failed to parse scroll arguments: %w", err)
			}
			if params.Dy == 0 {
				params.Dy = 500
			}
			sessionID, _ := ctx.Value("session_id").(string)
			return GlobalSessionManager.GetSession(sessionID).Scroll(params.Dy)
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
			sessionID, _ := ctx.Value("session_id").(string)
			return GlobalSessionManager.GetSession(sessionID).Screenshot(params.OutPath)
		},
	}
}
