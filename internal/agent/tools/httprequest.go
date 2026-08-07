package agenttools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/fabith10/synapse-go/adk"
)

// GetHTTPAPIRequestTool returns a tool to execute arbitrary REST API requests (GET, POST, PUT, DELETE, PATCH).
func GetHTTPAPIRequestTool() adk.Tool {
	return adk.Tool{
		Name:        "http_api_request",
		Description: "Executes HTTP REST API calls (GET, POST, PUT, DELETE, PATCH) with custom headers, JSON body or query parameters, returning status code and response body.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url": map[string]interface{}{
					"type":        "string",
					"description": "Full target HTTP/HTTPS URL",
				},
				"method": map[string]interface{}{
					"type":        "string",
					"description": "HTTP method: GET, POST, PUT, DELETE, PATCH (defaults to GET)",
				},
				"headers": map[string]interface{}{
					"type":        "object",
					"description": "Optional HTTP request headers key-value map",
				},
				"body": map[string]interface{}{
					"type":        "string",
					"description": "Optional request body string (JSON, form data, or text)",
				},
				"timeout_seconds": map[string]interface{}{
					"type":        "integer",
					"description": "Request timeout in seconds (default 15s, max 60s)",
				},
			},
			"required": []interface{}{"url"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				URL            string            `json:"url"`
				Method         string            `json:"method"`
				Headers        map[string]string `json:"headers"`
				Body           string            `json:"body"`
				TimeoutSeconds int               `json:"timeout_seconds"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}

			if params.URL == "" {
				return "", fmt.Errorf("url parameter is required")
			}
			if !strings.HasPrefix(params.URL, "http://") && !strings.HasPrefix(params.URL, "https://") {
				params.URL = "https://" + params.URL
			}

			method := strings.ToUpper(strings.TrimSpace(params.Method))
			if method == "" {
				method = http.MethodGet
			}

			timeoutSecs := params.TimeoutSeconds
			if timeoutSecs <= 0 {
				timeoutSecs = 15
			} else if timeoutSecs > 60 {
				timeoutSecs = 60
			}

			execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
			defer cancel()

			var reqBody io.Reader
			if params.Body != "" {
				reqBody = strings.NewReader(params.Body)
			}

			req, err := http.NewRequestWithContext(execCtx, method, params.URL, reqBody)
			if err != nil {
				return "", fmt.Errorf("failed to build HTTP request: %w", err)
			}

			// Default User-Agent if not supplied
			req.Header.Set("User-Agent", "SynapseGo-Agent/1.0")

			if params.Body != "" && req.Header.Get("Content-Type") == "" {
				if strings.HasPrefix(strings.TrimSpace(params.Body), "{") || strings.HasPrefix(strings.TrimSpace(params.Body), "[") {
					req.Header.Set("Content-Type", "application/json")
				}
			}

			for k, v := range params.Headers {
				req.Header.Set(k, v)
			}

			client := &http.Client{
				Timeout: time.Duration(timeoutSecs) * time.Second,
			}

			start := time.Now()
			var resp *http.Response

			// Attempt request with 1 automatic retry on transient gateway/server failure
			for attempt := 1; attempt <= 2; attempt++ {
				if attempt > 1 && params.Body != "" {
					req.Body = io.NopCloser(strings.NewReader(params.Body))
				}
				resp, err = client.Do(req)
				if err == nil && (resp.StatusCode != http.StatusBadGateway && resp.StatusCode != http.StatusServiceUnavailable && resp.StatusCode != http.StatusGatewayTimeout) {
					break
				}
				if attempt < 2 {
					if resp != nil {
						resp.Body.Close()
					}
					time.Sleep(300 * time.Millisecond)
				}
			}
			duration := time.Since(start)

			if err != nil {
				return "", fmt.Errorf("HTTP %s to %s failed: %w", method, params.URL, err)
			}
			defer resp.Body.Close()

			// Cap response body read at 1MB to prevent memory exhaustion
			const maxReadBytes = 1024 * 1024
			lr := io.LimitReader(resp.Body, maxReadBytes)
			respBytes, err := io.ReadAll(lr)
			if err != nil {
				return "", fmt.Errorf("failed to read HTTP response body: %w", err)
			}

			respStr := string(respBytes)

			// Pretty-print JSON responses if applicable
			var jsonFormatted bytes.Buffer
			if json.Indent(&jsonFormatted, respBytes, "", "  ") == nil {
				respStr = jsonFormatted.String()
			}

			result := map[string]interface{}{
				"status_code":   resp.StatusCode,
				"status":        resp.Status,
				"duration_ms":   duration.Milliseconds(),
				"content_type":  resp.Header.Get("Content-Type"),
				"response_body": respStr,
			}

			outBytes, _ := json.MarshalIndent(result, "", "  ")
			return string(outBytes), nil
		},
	}
}
