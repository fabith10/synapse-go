package agenttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fabith10/synapse-go/adk"
	"github.com/fabith10/synapse-go/internal/agent/pricing"
	"github.com/fabith10/synapse-go/internal/agent/pricing/hedge"
	"github.com/fabith10/synapse-go/internal/agent/pricing/quant"
)

// GetManageHedgeContractTool returns the Tier 1 tool for entering, holding, and managing derivative hedge contracts.
func GetManageHedgeContractTool() adk.Tool {
	return adk.Tool{
		Name: "manage_hedge_contract",
		Description: "Enters and manages active derivative hedge contracts (forwards and call options) to guarantee downside price protection on cloud GPU compute.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"action": map[string]interface{}{
					"type":        "string",
					"description": "Hedge operation: 'enter_forward' (lock compute cost at forward rate), 'buy_call_option' (cap compute rate at strike), 'list_positions' (show active hedges & savings), 'close_position'.",
				},
				"asset": map[string]interface{}{
					"type":        "string",
					"description": "Hardware asset or GPU architecture (e.g. 'H100_SXM', 'A100_80GB', 'RTX_4090').",
				},
				"forward_rate": map[string]interface{}{
					"type":        "number",
					"description": "Target locked forward price per hour (USD) when action='enter_forward'. If omitted, fetched from pricing oracle.",
				},
				"strike_rate": map[string]interface{}{
					"type":        "number",
					"description": "Option strike price ceiling per hour (USD) when action='buy_call_option'.",
				},
				"duration_hours": map[string]interface{}{
					"type":        "integer",
					"description": "Total compute duration in hours to protect under the derivative contract (e.g. 4, 8, 24). Default is 4.",
				},
				"expiry_days": map[string]interface{}{
					"type":        "number",
					"description": "Contract expiry in days (e.g. 30.0). Default is 30.",
				},
				"contract_id": map[string]interface{}{
					"type":        "string",
					"description": "Contract ID when action='close_position'.",
				},
			},
			"required": []string{"action"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				Action        string  `json:"action"`
				Asset         string  `json:"asset"`
				ForwardRate   float64 `json:"forward_rate"`
				StrikeRate    float64 `json:"strike_rate"`
				DurationHours int     `json:"duration_hours"`
				ExpiryDays    float64 `json:"expiry_days"`
				ContractID    string  `json:"contract_id"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("manage_hedge_contract: invalid args: %w", err)
			}

			ledger := hedge.GetGlobalHedgeLedger()
			if params.DurationHours <= 0 {
				params.DurationHours = 4
			}
			if params.ExpiryDays <= 0 {
				params.ExpiryDays = 30.0
			}

			action := strings.ToLower(strings.TrimSpace(params.Action))
			if action == "" || action == "manage_hedge_contract" || action == "hedge" || action == "enter" {
				if params.StrikeRate > 0 {
					action = "buy_call_option"
				} else if params.ForwardRate > 0 || params.DurationHours > 0 {
					action = "enter_forward"
				} else {
					action = "list_positions"
				}
			}

			switch action {
			case "enter_forward", "book_forward", "forward", "enter":
				if params.Asset == "" {
					params.Asset = "H100_SXM"
				}
				if params.ForwardRate <= 0 {
					// Query oracle for live forward rate
					oracleRes, err := pricing.GetPricingOracleManager().ExecuteQuery(ctx, params.Asset, "spot", "", nil)
					if err == nil {
						var parsed map[string]interface{}
						_ = json.Unmarshal([]byte(oracleRes), &parsed)
						if f, ok := parsed["forward_rate_usd"].(float64); ok && f > 0 {
							params.ForwardRate = f
						} else if s, ok := parsed["spot_price"].(float64); ok && s > 0 {
							params.ForwardRate = s * 1.05
						}
					}
					if params.ForwardRate <= 0 {
						params.ForwardRate = 2.6145
					}
				}

				contract, err := ledger.BookForward(params.Asset, params.ForwardRate, params.DurationHours, params.ExpiryDays)
				if err != nil {
					return "", fmt.Errorf("failed to book forward contract: %w", err)
				}
				resp := map[string]interface{}{
					"status":   "success",
					"message":  fmt.Sprintf("Successfully entered forward hedge contract for %s at locked rate $%.4f/hr", params.Asset, contract.StrikeRateUSD),
					"contract": contract,
				}
				b, _ := json.Marshal(resp)
				return string(b), nil

			case "buy_call_option", "buy_option", "call_option":
				if params.Asset == "" {
					params.Asset = "H100_SXM"
				}
				spotRate := 2.49
				oracleRes, err := pricing.GetPricingOracleManager().ExecuteQuery(ctx, params.Asset, "spot", "", nil)
				if err == nil {
					var parsed map[string]interface{}
					_ = json.Unmarshal([]byte(oracleRes), &parsed)
					if s, ok := parsed["spot_price"].(float64); ok && s > 0 {
						spotRate = s
					}
				}

				if params.StrikeRate <= 0 {
					params.StrikeRate = spotRate * 1.05
				}

				// Price the option premium using Black-76
				T := params.ExpiryDays / 365.0
				optRes, err := quant.Black76Price(spotRate*1.05, params.StrikeRate, T, 0.35, 0.045, quant.OptionCall)
				premiumUSD := 0.05
				if err == nil && optRes != nil {
					premiumUSD = optRes.Price * float64(params.DurationHours)
				}

				contract, err := ledger.BuyCallOption(params.Asset, params.StrikeRate, premiumUSD, params.DurationHours, params.ExpiryDays)
				if err != nil {
					return "", fmt.Errorf("failed to buy call option: %w", err)
				}
				resp := map[string]interface{}{
					"status":   "success",
					"message":  fmt.Sprintf("Successfully bought call option for %s (Strike: $%.4f/hr, Premium: $%.4f)", params.Asset, contract.StrikeRateUSD, contract.PremiumPaidUSD),
					"contract": contract,
				}
				b, _ := json.Marshal(resp)
				return string(b), nil

			case "list_positions", "list", "positions":
				positions := ledger.ListPositions()
				resp := map[string]interface{}{
					"status":         "success",
					"total_hedges":   len(positions),
					"open_positions": positions,
				}
				b, _ := json.Marshal(resp)
				return string(b), nil

			case "close_position", "close", "exercise":
				if params.ContractID == "" {
					return "", fmt.Errorf("contract_id is required to close position")
				}
				if err := ledger.ClosePosition(params.ContractID); err != nil {
					return "", err
				}
				resp := map[string]interface{}{
					"status":  "success",
					"message": fmt.Sprintf("Contract %s closed successfully", params.ContractID),
				}
				b, _ := json.Marshal(resp)
				return string(b), nil

			default:
				return "", fmt.Errorf("unknown action %q; expected 'enter_forward', 'buy_call_option', 'list_positions', or 'close_position'", params.Action)
			}
		},
	}
}
