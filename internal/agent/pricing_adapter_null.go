package agent

import (
	"context"
	"encoding/json"
	"fmt"
)

// NullPricingAdapter is the zero-configuration fallback adapter registered whenever
// no real pricing provider is wired up. It never returns a Go error; instead it
// returns a structured JSON response with status "unconfigured" so agents and
// callers can relay a clear, actionable message to the user rather than crashing.
//
// In a live environment users should replace this by setting active_provider in
// pricing_providers.json to a configured custom_http or static_json entry.
type NullPricingAdapter struct{}

func (n *NullPricingAdapter) Name() string { return "null" }

func (n *NullPricingAdapter) Query(_ context.Context, q PricingQuery) (*PricingResult, error) {
	res := map[string]interface{}{
		"status":      "unconfigured",
		"provider":    "null",
		"asset":       q.Asset,
		"market_type": q.MarketType,
		"message":     "No pricing provider is configured for this environment. Set active_provider in pricing_providers.json or the PRICING_PROVIDER environment variable to a registered provider (e.g. 'custom_http', 'static_json').",
		"docs":        "See docs/Compute_Derivatives_Pricing.md for configuration options.",
	}
	b, _ := json.Marshal(res)
	return buildResult("null", q.Asset, q.MarketType, string(b)), nil
}

// nullAdapterResponse returns the formatted "unconfigured" JSON string directly.
// Used by PricingOracleManager.ExecuteQuery when no registered adapter matches the
// active provider name and the null adapter is configured as the final fallback.
func nullAdapterResponse(activeName, asset, marketType string) string {
	res := map[string]interface{}{
		"status":      "unconfigured",
		"provider":    "null",
		"requested":   activeName,
		"asset":       asset,
		"market_type": marketType,
		"message": fmt.Sprintf(
			"Pricing provider %q is not registered. "+
				"Add it to pricing_providers.json or call RegisterAdapter() at boot. "+
				"Available providers: see GET /api/config/pricing-provider.",
			activeName,
		),
	}
	b, _ := json.Marshal(res)
	return string(b)
}
