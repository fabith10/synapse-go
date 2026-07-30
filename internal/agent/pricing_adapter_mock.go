package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// MockPricingAdapter implements PricingProvider by dispatching to the framework's
// internal mock HTTP API (/api/mock/compute/*). It requires no external connectivity.
type MockPricingAdapter struct {
	baseURL string
}

// NewMockPricingAdapter creates a new mock adapter pointed at the given base URL.
// If baseURL is empty the global mockServerBaseURL variable is resolved dynamically.
func NewMockPricingAdapter(baseURL string) *MockPricingAdapter {
	return &MockPricingAdapter{baseURL: baseURL}
}

func (a *MockPricingAdapter) Name() string { return "mock" }

func (a *MockPricingAdapter) Query(ctx context.Context, q PricingQuery) (*PricingResult, error) {
	asset := q.Asset
	if asset == "" {
		asset = "H100_SXM"
	}

	baseURL := a.baseURL
	if baseURL == "" {
		baseURL = mockServerBaseURL
	}
	if envURL := os.Getenv("PRICING_ORACLE_URL"); envURL != "" {
		baseURL = envURL
	}

	path, params := a.resolveEndpoint(asset, q.MarketType, q.ProviderFilter, q.Options)
	endpoint, err := url.Parse(baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("mock adapter: bad URL: %w", err)
	}
	q2 := endpoint.Query()
	for k, v := range params {
		q2.Set(k, v)
	}
	endpoint.RawQuery = q2.Encode()

	raw, err := a.fetchJSON(ctx, endpoint.String())
	if err != nil {
		// Return a minimal inline fallback rather than a hard error, so agents
		// are never blocked by a temporarily unavailable mock server.
		raw = a.fallback(asset, q.MarketType, q.Options)
	}

	return buildResult("mock", asset, q.MarketType, raw), nil
}

// resolveEndpoint maps a market type to the correct mock API path and query params.
// Options are passed through directly as query string parameters for applicable endpoints.
func (a *MockPricingAdapter) resolveEndpoint(asset, marketType, providerFilter string, _ map[string]string) (string, map[string]string) {
	params := map[string]string{"asset": asset}
	mt := strings.ToLower(marketType)
	switch {
	case mt == "futures" || mt == "forward" || mt == "forwards":
		return "/api/mock/compute/forward-curves", params
	case mt == "options":
		return "/api/mock/compute/options", params
	case mt == "vol_surface" || mt == "volatility_surface" || mt == "vol-surface":
		return "/api/mock/compute/vol-surface", params
	default: // "spot", "on_demand", or empty
		if providerFilter != "" {
			params["provider"] = providerFilter
		}
		params["gpu_type"] = asset
		delete(params, "asset")
		return "/api/mock/compute/prices", params
	}
}

// fetchJSON performs a GET request and returns the raw response body.
func (a *MockPricingAdapter) fetchJSON(ctx context.Context, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	b, _ := json.Marshal(result)
	return string(b), nil
}

// fallback returns a minimal inline JSON response when the mock server is unreachable.
func (a *MockPricingAdapter) fallback(asset, marketType string, _ map[string]string) string {
	spot := spotBaseRate(asset)
	switch strings.ToLower(marketType) {
	case "futures", "forward", "forwards":
		return fmt.Sprintf(`{"status":"success","provider":"mock","asset":%q,"market":"futures","spot_index_usd":%.2f,"30d_forward_contract_rate_usd":%.4f}`,
			asset, spot, spot*1.025)
	case "options":
		return fmt.Sprintf(`{"status":"success","provider":"mock","asset":%q,"market":"options","underlying_spot":%.2f,"call_strike_105":%.2f,"option_premium":%.4f,"implied_volatility":0.35}`,
			asset, spot, spot*1.05, spot*0.018)
	case "vol_surface", "volatility_surface", "vol-surface":
		return fmt.Sprintf(`{"status":"success","provider":"mock","asset":%q,"market":"vol_surface","underlying_spot":%.2f,"atm_vol_30d":0.380,"atm_vol_90d":0.335,"atm_vol_180d":0.310,"model":"SVI-inspired smile"}`,
			asset, spot)
	default:
		fwd := spot * 1.025
		opt := spot * 0.018
		return fmt.Sprintf(`{"status":"synchronized","provider":"mock","asset":%q,"spot_gpu_rate_per_hour_usd":"$%.4f","30d_forward_contract_rate":"$%.4f","option_premium_call_105":"$%.4f"}`,
			asset, spot, fwd, opt)
	}
}

// spotBaseRate returns a direct USD/hour baseline for a given asset string.
func spotBaseRate(asset string) float64 {
	up := strings.ToUpper(asset)
	switch {
	case strings.Contains(up, "A100"):
		return 1.29
	case strings.Contains(up, "4090"):
		return 0.45
	default:
		return 2.49
	}
}
