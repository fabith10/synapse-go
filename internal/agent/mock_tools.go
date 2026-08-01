package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fabith10/synapse-go/adk"
	"github.com/fabith10/synapse-go/internal/agent/pricing"
)

// Default baseURL for server mock endpoints during test/local execution.
var mockServerBaseURL = "http://localhost:8080"

// SetMockServerBaseURL configures the target URL for mock API calls.
func SetMockServerBaseURL(urlStr string) {
	mockServerBaseURL = urlStr
	pricing.SetMockServerBaseURL(urlStr)
}

// GetQueryComputePricesTool returns a tool to query spot and on-demand GPU/CPU market prices.
func GetQueryComputePricesTool() adk.Tool {
	return adk.Tool{
		Name:        "query_compute_prices",
		Description: "Queries mock compute market prices across cloud providers (RunPod, Lambda, AWS, GCP, Azure) and GPU types (H100, A100, RTX4090).",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"provider": map[string]interface{}{
					"type":        "string",
					"description": "Optional filter by provider name (e.g. 'RunPod', 'AWS', 'Lambda')",
				},
				"gpu_type": map[string]interface{}{
					"type":        "string",
					"description": "Optional filter by GPU model (e.g. 'H100', 'A100', 'RTX4090')",
				},
				"region": map[string]interface{}{
					"type":        "string",
					"description": "Optional filter by cloud region (e.g. 'us-east', 'us-west')",
				},
			},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params map[string]interface{}
			if len(args) > 0 {
				_ = json.Unmarshal(args, &params)
			}

			endpoint, _ := url.Parse(mockServerBaseURL + "/api/mock/compute/prices")
			q := endpoint.Query()
			if p, ok := params["provider"].(string); ok && p != "" {
				q.Set("provider", p)
			}
			if g, ok := params["gpu_type"].(string); ok && g != "" {
				q.Set("gpu_type", g)
			}
			if r, ok := params["region"].(string); ok && r != "" {
				q.Set("region", r)
			}
			endpoint.RawQuery = q.Encode()

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
			if err != nil {
				return "", fmt.Errorf("query_compute_prices: create request failed: %w", err)
			}

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				// Fallback offline mock response if server isn't running live
				return jsonFallbackComputePrices(params), nil
			}
			defer resp.Body.Close()

			var result map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return "", fmt.Errorf("query_compute_prices: decode response failed: %w", err)
			}

			jsonBytes, _ := json.Marshal(result)
			return string(jsonBytes), nil
		},
	}
}

func jsonFallbackComputePrices(params map[string]interface{}) string {
	provider, _ := params["provider"].(string)
	gpu, _ := params["gpu_type"].(string)
	if provider == "" {
		provider = "RunPod"
	}
	if gpu == "" {
		gpu = "NVIDIA-H100-SXM"
	}

	res := map[string]interface{}{
		"status":    "success",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"count":     1,
		"currency":  "USD",
		"prices": []map[string]interface{}{
			{
				"provider":                 provider,
				"gpu_type":                 gpu,
				"vram_gb":                  80,
				"region":                   "us-east",
				"spot_price_per_hour":      2.49,
				"on_demand_price_per_hour": 3.89,
				"availability":             "high",
			},
		},
	}
	b, _ := json.Marshal(res)
	return string(b)
}

// GetQueryForwardCurvesTool returns a tool to query futures and forward contract rates for compute.
func GetQueryForwardCurvesTool() adk.Tool {
	return adk.Tool{
		Name:        "query_forward_curves",
		Description: "Queries mock forward curves and futures pricing for compute contracts (1M, 3M, 6M, 12M tenors).",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"asset": map[string]interface{}{
					"type":        "string",
					"description": "Compute asset identifier (e.g. 'H100_SXM', 'A100_80GB', 'RTX_4090')",
				},
			},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params map[string]interface{}
			if len(args) > 0 {
				_ = json.Unmarshal(args, &params)
			}

			endpoint, _ := url.Parse(mockServerBaseURL + "/api/mock/compute/forward-curves")
			q := endpoint.Query()
			if a, ok := params["asset"].(string); ok && a != "" {
				q.Set("asset", a)
			}
			endpoint.RawQuery = q.Encode()

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
			if err != nil {
				return "", fmt.Errorf("query_forward_curves: create request failed: %w", err)
			}

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				return jsonFallbackForwardCurves(params), nil
			}
			defer resp.Body.Close()

			var result map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return "", fmt.Errorf("query_forward_curves: decode response failed: %w", err)
			}

			jsonBytes, _ := json.Marshal(result)
			return string(jsonBytes), nil
		},
	}
}

func jsonFallbackForwardCurves(params map[string]interface{}) string {
	asset, _ := params["asset"].(string)
	if asset == "" {
		asset = "H100_SXM"
	}

	res := map[string]interface{}{
		"status":          "success",
		"asset":           asset,
		"base_spot_price": 2.49,
		"currency":        "USD",
		"as_of":           time.Now().UTC().Format(time.RFC3339),
		"curve": []map[string]interface{}{
			{"tenor": "1M", "contract": asset + "-2026-08", "forward_price_usd": 2.54, "open_interest": 1240},
			{"tenor": "3M", "contract": asset + "-2026-10", "forward_price_usd": 2.62, "open_interest": 3400},
			{"tenor": "6M", "contract": asset + "-2027-01", "forward_price_usd": 2.75, "open_interest": 5120},
		},
	}
	b, _ := json.Marshal(res)
	return string(b)
}

// GetQueryOptionsChainTool returns a tool to inspect call/put option chains for compute contracts.
func GetQueryOptionsChainTool() adk.Tool {
	return adk.Tool{
		Name:        "query_options_chain",
		Description: "Queries option chains (calls, puts, strikes, implied volatility, Greeks) for compute contracts.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"asset": map[string]interface{}{
					"type":        "string",
					"description": "Compute asset identifier (e.g. 'H100_SXM', 'A100_80GB')",
				},
				"expiration": map[string]interface{}{
					"type":        "string",
					"description": "Expiration duration (e.g. '30d', '60d', '90d')",
				},
			},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params map[string]interface{}
			if len(args) > 0 {
				_ = json.Unmarshal(args, &params)
			}

			endpoint, _ := url.Parse(mockServerBaseURL + "/api/mock/compute/options")
			q := endpoint.Query()
			if a, ok := params["asset"].(string); ok && a != "" {
				q.Set("asset", a)
			}
			if e, ok := params["expiration"].(string); ok && e != "" {
				q.Set("expiration", e)
			}
			endpoint.RawQuery = q.Encode()

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
			if err != nil {
				return "", fmt.Errorf("query_options_chain: create request failed: %w", err)
			}

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				return jsonFallbackOptionsChain(params), nil
			}
			defer resp.Body.Close()

			var result map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return "", fmt.Errorf("query_options_chain: decode response failed: %w", err)
			}

			jsonBytes, _ := json.Marshal(result)
			return string(jsonBytes), nil
		},
	}
}

func jsonFallbackOptionsChain(params map[string]interface{}) string {
	asset, _ := params["asset"].(string)
	exp, _ := params["expiration"].(string)
	if asset == "" {
		asset = "H100_SXM"
	}
	if exp == "" {
		exp = "30d"
	}

	res := map[string]interface{}{
		"status":          "success",
		"asset":           asset,
		"expiration":      exp,
		"underlying_spot": 2.50,
		"options": []map[string]interface{}{
			{"type": "call", "strike": 2.50, "bid": 0.12, "ask": 0.15, "implied_volatility": 0.35, "delta": 0.52},
			{"type": "put", "strike": 2.50, "bid": 0.10, "ask": 0.13, "implied_volatility": 0.37, "delta": -0.48},
		},
	}
	b, _ := json.Marshal(res)
	return string(b)
}

// GetSubmitMockTaskTool returns a tool to submit asynchronous tasks to the mock task server.
func GetSubmitMockTaskTool() adk.Tool {
	return adk.Tool{
		Name:        "submit_mock_task",
		Description: "Submits a compute or data processing job to the mock task server for end-to-end testing.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"task_type": map[string]interface{}{
					"type":        "string",
					"description": "Category of task (e.g. 'compute_optimization', 'data_pipeline', 'model_training')",
				},
				"payload": map[string]interface{}{
					"type":        "object",
					"description": "Payload parameters for the task execution",
				},
				"priority": map[string]interface{}{
					"type":        "string",
					"description": "Task priority ('high', 'medium', 'low')",
				},
				"force_immediate": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, bypasses the cost-optimization deferral checks and executes the task immediately.",
				},
				"deferred_until": map[string]interface{}{
					"type":        "string",
					"description": "Optional UTC timestamp or target start hour (e.g. '03:00') to schedule execution for later.",
				},
				"estimated_cost": map[string]interface{}{
					"type":        "number",
					"description": "Estimated USD cost of the job (used to evaluate deferral thresholds).",
				},
			},
			"required": []string{"task_type"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				TaskType       string                 `json:"task_type"`
				Payload        map[string]interface{} `json:"payload"`
				Priority       string                 `json:"priority"`
				ForceImmediate bool                   `json:"force_immediate"`
				DeferredUntil  string                 `json:"deferred_until"`
				EstimatedCost  float64                `json:"estimated_cost"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("submit_mock_task: invalid arguments: %w", err)
			}

			// If priority is high, treat as force_immediate
			forceImm := params.ForceImmediate || strings.ToLower(params.Priority) == "high"

			// If the user already specified a deferral schedule, bypass checks
			if params.DeferredUntil == "" {
				shouldDefer, reason := pricing.GetPricingOracleManager().ShouldDefer(params.EstimatedCost, 0, forceImm)
				if shouldDefer {
					// Retrieve optimal window from the pricing oracle to include in recommendation
					optimalStartHour := 3
					savingsPct := 35.0
					optCost := params.EstimatedCost * 0.79 // rough off-peak multiplier fallback

					oracleRes, err := pricing.GetPricingOracleManager().ExecuteQuery(ctx, "H100_SXM", "execution_window", "", map[string]string{
						"duration_hours": "4",
					})
					if err == nil {
						var oMap map[string]interface{}
						if json.Unmarshal([]byte(oracleRes), &oMap) == nil {
							if ow, ok := oMap["optimal_window"].(map[string]interface{}); ok {
								if sh, ok := ow["start_hour"].(float64); ok {
									optimalStartHour = int(sh)
								}
								if tc, ok := ow["total_cost_usd"].(float64); ok {
									optCost = tc
								}
							}
							if sv, ok := oMap["saving_pct_vs_worst"].(float64); ok {
								savingsPct = sv
							}
						}
					}

					res := map[string]interface{}{
						"status":                     "deferred_recommendation",
						"message":                    fmt.Sprintf("Deferral recommended: %s. To execute anyway, set force_immediate=true or priority=high.", reason),
						"optimal_start_hour_utc":     optimalStartHour,
						"savings_pct_vs_worst":       savingsPct,
						"estimated_offpeak_cost_usd": optCost,
						"deferred":                   true,
					}
					b, _ := json.Marshal(res)
					return string(b), nil
				}
			}

			jsonPayload, _ := json.Marshal(params)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, mockServerBaseURL+"/api/mock/tasks", bytes.NewBuffer(jsonPayload))
			if err != nil {
				return "", fmt.Errorf("submit_mock_task: create request failed: %w", err)
			}
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				// Fallback offline creation simulation
				resStatus := "created"
				if params.DeferredUntil != "" {
					resStatus = "scheduled"
				}
				res := map[string]interface{}{
					"status":  resStatus,
					"message": "Task queued (offline mock mode)",
					"task": map[string]interface{}{
						"id":             fmt.Sprintf("mock-task-offline-%d", time.Now().UnixNano()%10000),
						"task_type":      params.TaskType,
						"status":         resStatus,
						"deferred_until": params.DeferredUntil,
					},
				}
				b, _ := json.Marshal(res)
				return string(b), nil
			}
			defer resp.Body.Close()

			var result map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return "", fmt.Errorf("submit_mock_task: decode response failed: %w", err)
			}

			jsonBytes, _ := json.Marshal(result)
			return string(jsonBytes), nil
		},
	}
}

// GetCheckMockTaskTool returns a tool to poll/check status of submitted mock tasks.
func GetCheckMockTaskTool() adk.Tool {
	return adk.Tool{
		Name:        "check_mock_task",
		Description: "Checks the current status, progress percentage, and results of a submitted mock task.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"task_id": map[string]interface{}{
					"type":        "string",
					"description": "ID of the mock task to inspect",
				},
				"status_filter": map[string]interface{}{
					"type":        "string",
					"description": "Optional status filter when listing all tasks ('queued', 'running', 'completed')",
				},
			},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params map[string]interface{}
			if len(args) > 0 {
				_ = json.Unmarshal(args, &params)
			}

			taskID, _ := params["task_id"].(string)
			statusFilter, _ := params["status_filter"].(string)

			targetURL := mockServerBaseURL + "/api/mock/tasks"
			if taskID != "" {
				targetURL += "/" + taskID
			} else if statusFilter != "" {
				targetURL += "?status=" + statusFilter
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
			if err != nil {
				return "", fmt.Errorf("check_mock_task: create request failed: %w", err)
			}

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				// Fallback mock check
				res := map[string]interface{}{
					"status": "success",
					"task": map[string]interface{}{
						"id":               taskID,
						"status":           "completed",
						"progress_percent": 100,
						"result": map[string]interface{}{
							"output": "Simulated task completion (offline mode)",
						},
					},
				}
				b, _ := json.Marshal(res)
				return string(b), nil
			}
			defer resp.Body.Close()

			var result map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return "", fmt.Errorf("check_mock_task: decode response failed: %w", err)
			}

			jsonBytes, _ := json.Marshal(result)
			return string(jsonBytes), nil
		},
	}
}
