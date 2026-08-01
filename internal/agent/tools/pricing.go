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

// GetQueryPricingOracleTool returns the tool to query real-time oracle pricing data (spot, futures, options, vol surface).
func GetQueryPricingOracleTool() adk.Tool {
	return adk.Tool{
		Name:        "query_pricing_oracle",
		Description: "Queries the central pricing oracle for GPU spot rates, compute forward curves, options pricing, implied volatility surface, or cloud instance pricing.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"asset_or_instance": map[string]interface{}{
					"type":        "string",
					"description": "Target compute asset, GPU model, or ticker symbol (e.g. 'H100_SXM', 'A100_80GB', 'RTX_4090', 'g5.xlarge').",
				},
				"market_type": map[string]interface{}{
					"type":        "string",
					"description": "Market classification: 'spot', 'on_demand', 'futures' (or 'forward'), 'options', 'vol_surface' (implied volatility surface), or 'execution_window'.",
				},
				"provider": map[string]interface{}{
					"type":        "string",
					"description": "Optional cloud provider filter (e.g. 'RunPod', 'AWS', 'Lambda', 'GCP').",
				},
				"duration_hours": map[string]interface{}{
					"type":        "integer",
					"description": "Job duration in hours (1-23). Used only when market_type='execution_window'.",
				},
				"cost_mode": map[string]interface{}{
					"type":        "string",
					"description": "Optimization objective: 'spot' to minimize raw spot rate, or 'hedged_spot'.",
				},
				"forward_rate": map[string]interface{}{
					"type":        "number",
					"description": "Ceiling price per hour (USD) to use in 'hedged_spot' cost calculations.",
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

			return pricing.GetPricingOracleManager().ExecuteQuery(ctx, params.Asset, params.MarketType, params.Provider, opts)
		},
	}
}
