package adapter

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fabith10/synapse-go/internal/agent/pricing/types"
)

// NullPricingAdapter is the zero-configuration fallback adapter registered whenever
// no real pricing provider is wired up.
type NullPricingAdapter struct{}

func (n *NullPricingAdapter) Name() string { return "null" }

func (n *NullPricingAdapter) Query(_ context.Context, q types.PricingQuery) (*types.PricingResult, error) {
	res := map[string]interface{}{
		"status":      "unconfigured",
		"provider":    "null",
		"asset":       q.Asset,
		"market_type": q.MarketType,
		"message":     "No pricing provider is configured for this environment. Set active_provider in pricing_providers.json or the PRICING_PROVIDER environment variable to a registered provider (e.g. 'custom_http', 'static_json').",
		"docs":        "See docs/Compute_Derivatives_Pricing.md for configuration options.",
	}
	b, _ := json.Marshal(res)
	return BuildResult("null", q.Asset, q.MarketType, string(b)), nil
}

// NullAdapterResponse returns the formatted "unconfigured" JSON string directly.
func NullAdapterResponse(activeName, asset, marketType string) string {
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

// BuildResult constructs a PricingResult from a raw JSON string.
func BuildResult(provider, asset, marketType, raw string) *types.PricingResult {
	var data map[string]interface{}
	_ = json.Unmarshal([]byte(raw), &data)
	return &types.PricingResult{
		Provider:   provider,
		Asset:      asset,
		MarketType: marketType,
		Raw:        raw,
		Data:       data,
	}
}
