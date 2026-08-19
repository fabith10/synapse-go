package agenttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/fabith10/synapse-go/adk"
	"github.com/fabith10/synapse-go/internal/agent/pricing"
)

// GetPricingOracleTool is an alias for GetQueryPricingOracleTool for backward compatibility.
func GetPricingOracleTool() adk.Tool {
	return GetQueryPricingOracleTool()
}

// GetQueryPricingOracleTool returns the tool to query real-time oracle pricing data (spot, futures, options, vol surface, backtest).
func GetQueryPricingOracleTool() adk.Tool {
	return adk.Tool{
		Name:        "query_pricing_oracle",
		Description: "Queries the central pricing oracle for GPU spot rates, compute forward curves, options pricing, implied volatility surface, game theory models, or backtests.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"asset_or_instance": map[string]interface{}{
					"type":        "string",
					"description": "Target compute asset, GPU model, or ticker symbol (e.g. 'H100_SXM', 'A100_80GB', 'RTX_4090', 'g5.xlarge').",
				},
				"market_type": map[string]interface{}{
					"type":        "string",
					"description": "Market classification: 'spot', 'on_demand', 'futures' (or 'forward'), 'options', 'risk_metrics' (VaR/CVaR), 'real_options', 'lucia_schwartz', 'regime_switching', 'congestion', 'game_theory', 'execution_window', or 'backtest'.",
				},
				"provider": map[string]interface{}{
					"type":        "string",
					"description": "Optional cloud provider filter (e.g. 'RunPod', 'AWS', 'Lambda', 'GCP').",
				},
				"duration_hours": map[string]interface{}{
					"type":        "integer",
					"description": "Job duration in hours (1-23). Used when market_type='execution_window' or risk analysis.",
				},
				"cost_mode": map[string]interface{}{
					"type":        "string",
					"description": "Optimization objective: 'spot' to minimize raw spot rate, or 'hedged_spot'.",
				},
				"forward_rate": map[string]interface{}{
					"type":        "number",
					"description": "Ceiling price per hour (USD) to use in 'hedged_spot' cost calculations.",
				},
				"days": map[string]interface{}{
					"type":        "integer",
					"description": "Simulation horizon in days (e.g. 14, 30, 90) when market_type='backtest'.",
				},
				"model": map[string]interface{}{
					"type":        "string",
					"description": "Simulation stochastic model: 'ou_jump_diffusion' or 'markov_regime_switching'.",
				},
				"dataset": map[string]interface{}{
					"type":        "string",
					"description": "Empirical historical dataset ID ('aws_g5_xlarge', 'aws_g4dn_xlarge', 'aws_p3_2xlarge', 'vastai_h100_sxm', 'vastai_a100_80gb', 'vastai_rtx4090') or path to custom CSV.",
				},
				"format": map[string]interface{}{
					"type":        "string",
					"description": "Output formatting: 'markdown' (default) or 'json'.",
				},
			},
			"required": []string{"asset_or_instance"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				Asset         string  `json:"asset_or_instance"`
				MarketType    string  `json:"market_type"`
				Provider      string  `json:"provider"`
				DurationHours int     `json:"duration_hours"`
				CostMode      string  `json:"cost_mode"`
				ForwardRate   float64 `json:"forward_rate"`
				Days          int     `json:"days"`
				Model         string  `json:"model"`
				Dataset       string  `json:"dataset"`
				DatasetPath   string  `json:"dataset_path"`
				Format        string  `json:"format"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("query_pricing_oracle: invalid args: %w", err)
			}

			opts := make(map[string]string)
			if params.DurationHours > 0 {
				opts["duration_hours"] = strconv.Itoa(params.DurationHours)
			}
			if params.CostMode != "" {
				opts["cost_mode"] = params.CostMode
			}
			if params.ForwardRate > 0 {
				opts["forward_rate"] = fmt.Sprintf("%.4f", params.ForwardRate)
			}
			if params.Days > 0 {
				opts["days"] = strconv.Itoa(params.Days)
			}
			if params.Model != "" {
				opts["model"] = params.Model
			}
			if params.Dataset != "" {
				opts["dataset"] = params.Dataset
			}
			if params.DatasetPath != "" {
				opts["dataset_path"] = params.DatasetPath
			}
			if params.Format != "" {
				opts["format"] = params.Format
			}

			return pricing.GetPricingOracleManager().ExecuteQuery(ctx, params.Asset, params.MarketType, params.Provider, opts)
		},
	}
}

// GetRunPricingBacktestTool returns the tool to execute multi-strategy quantitative backtests across compute models.
func GetRunPricingBacktestTool() adk.Tool {
	return adk.Tool{
		Name:        "run_pricing_backtest",
		Description: "Executes a rigorous quantitative backtest comparing Unhedged Spot, Forward Lock, Delta-Hedged Call Protection, and Workload Deferral across empirical historical spot traces or stochastic simulations.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"dataset": map[string]interface{}{
					"type":        "string",
					"description": "Historical empirical dataset ID: 'aws_g5_xlarge' (A10G), 'aws_g4dn_xlarge' (T4), 'aws_p3_2xlarge' (V100), 'vastai_h100_sxm' (H100), 'vastai_a100_80gb' (A100), 'vastai_rtx4090' (RTX 4090), or custom CSV path.",
				},
				"asset": map[string]interface{}{
					"type":        "string",
					"description": "Hardware asset or GPU model (e.g. 'H100_SXM', 'A100_80GB', 'A10G_24GB'). Default is 'H100_SXM'.",
				},
				"days": map[string]interface{}{
					"type":        "integer",
					"description": "Simulation horizon in days (e.g. 14, 30, 90). Default is 30.",
				},
				"model": map[string]interface{}{
					"type":        "string",
					"description": "Stochastic model (when not using historical dataset): 'ou_jump_diffusion' or 'markov_regime_switching'.",
				},
				"workload_tasks": map[string]interface{}{
					"type":        "integer",
					"description": "Number of discrete batch tasks to simulate for deferral scheduling (e.g. 50, 100, 200). Default is 100.",
				},
				"strike_rate": map[string]interface{}{
					"type":        "number",
					"description": "Option strike price cap (USD/hr) for the Delta-Hedged Call Protection strategy. If 0, auto-calculated 5% OTM.",
				},
				"format": map[string]interface{}{
					"type":        "string",
					"description": "Output format: 'markdown' (default human-readable report) or 'json'.",
				},
			},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				Dataset       string  `json:"dataset"`
				DatasetPath   string  `json:"dataset_path"`
				Asset         string  `json:"asset"`
				Days          int     `json:"days"`
				Model         string  `json:"model"`
				WorkloadTasks int     `json:"workload_tasks"`
				StrikeRate    float64 `json:"strike_rate"`
				Format        string  `json:"format"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("run_pricing_backtest: invalid args: %w", err)
			}

			opts := make(map[string]string)
			if params.Dataset != "" {
				opts["dataset"] = params.Dataset
			}
			if params.DatasetPath != "" {
				opts["dataset_path"] = params.DatasetPath
			}
			if params.Days > 0 {
				opts["days"] = strconv.Itoa(params.Days)
			}
			if params.Model != "" {
				opts["model"] = params.Model
			}
			if params.WorkloadTasks > 0 {
				opts["tasks"] = strconv.Itoa(params.WorkloadTasks)
			}
			if params.StrikeRate > 0 {
				opts["strike_rate"] = fmt.Sprintf("%.4f", params.StrikeRate)
			}
			if params.Format != "" {
				opts["format"] = params.Format
			}

			asset := params.Asset
			if asset == "" && params.Dataset == "" {
				asset = "H100_SXM"
			}

			return pricing.GetPricingOracleManager().ExecuteBacktestQuery(ctx, asset, "", opts)
		},
	}
}

