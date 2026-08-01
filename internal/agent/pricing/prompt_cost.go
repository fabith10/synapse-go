package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

// ExecutePromptCostQuery calculates prompt execution cost for open-weight models based on token volumes and GPU spot rates.
func (m *PricingOracleManager) ExecutePromptCostQuery(ctx context.Context, asset, providerFilter string, options map[string]string) (string, error) {
	inputTokens := 10000
	if inStr, ok := options["input_tokens"]; ok {
		if val, err := strconv.Atoi(inStr); err == nil && val > 0 {
			inputTokens = val
		}
	}
	outputTokens := 2000
	if outStr, ok := options["output_tokens"]; ok {
		if val, err := strconv.Atoi(outStr); err == nil && val > 0 {
			outputTokens = val
		}
	}

	activeName := m.GetActiveProviderName()
	m.mu.RLock()
	prov, ok := m.adapters[activeName]
	m.mu.RUnlock()

	var inputRate, outputRate, baseSpot float64 = 0.55, 0.75, 1.85
	if ok {
		res, err := prov.Query(ctx, PricingQuery{
			Asset:          asset,
			MarketType:     "spot",
			ProviderFilter: providerFilter,
		})
		if err == nil {
			var rawData map[string]interface{}
			if json.Unmarshal([]byte(res.Raw), &rawData) == nil {
				if models, ok := rawData["models"].(map[string]interface{}); ok {
					if modelData, ok := models[asset].(map[string]interface{}); ok {
						if ir, ok := modelData["input_cost_per_1m"].(float64); ok {
							inputRate = ir
						}
						if or, ok := modelData["output_cost_per_1m"].(float64); ok {
							outputRate = or
						}
						if bs, ok := modelData["base_spot_per_hour"].(float64); ok {
							baseSpot = bs
						}
					}
				}
			}
			if extractedSpot, err := extractSpotPrice(res.Raw, asset); err == nil && extractedSpot > 0 {
				baseSpot = extractedSpot
			}
		}
	}

	tokenCostUSD := roundTo4((float64(inputTokens)/1000000.0)*inputRate + (float64(outputTokens)/1000000.0)*outputRate)

	// Evaluate execution window optimization
	windowJSON, _ := m.ExecuteExecutionWindowQuery(ctx, asset, providerFilter, options)
	var windowMap map[string]interface{}
	_ = json.Unmarshal([]byte(windowJSON), &windowMap)
	optimalWindow := windowMap["optimal_window"]

	resPayload := map[string]interface{}{
		"status":                    "success",
		"provider":                  activeName,
		"model_asset":               asset,
		"input_tokens":              inputTokens,
		"output_tokens":             outputTokens,
		"input_rate_per_1m_usd":     inputRate,
		"output_rate_per_1m_usd":    outputRate,
		"estimated_prompt_cost_usd": tokenCostUSD,
		"gpu_base_spot_per_hour":    baseSpot,
		"optimal_window":            optimalWindow,
		"message":                   fmt.Sprintf("Prompt run for %s (%d in / %d out) estimated at $%0.4f USD. Off-peak window provides optimal scheduling.", asset, inputTokens, outputTokens, tokenCostUSD),
	}

	b, _ := json.Marshal(resPayload)
	return string(b), nil
}
