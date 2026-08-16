package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/fabith10/synapse-go/internal/agent/pricing/types"
)

// HTTPEndpointTemplate describes how a CustomHTTPAdapter resolves a URL for a given market type.
type HTTPEndpointTemplate struct {
	Path        string            `json:"path,omitempty"`
	QueryParams map[string]string `json:"query_params,omitempty"`
}

// DeclarativeMapping defines how arbitrary JSON fields from a third-party API are mapped to standard pricing values.
type DeclarativeMapping struct {
	// SpotRatePath is the dot-notation path to extract spot price (e.g., "price", "rates.{{asset}}", "offers.0.dph_total").
	SpotRatePath string `json:"spot_rate_path,omitempty"`

	// InputTokenRatePath is the dot-notation path to extract prompt token cost (e.g., "data.0.pricing.prompt").
	InputTokenRatePath string `json:"input_token_rate_path,omitempty"`

	// OutputTokenRatePath is the dot-notation path to extract completion token cost (e.g., "data.0.pricing.completion").
	OutputTokenRatePath string `json:"output_token_rate_path,omitempty"`

	// ForwardRatePath is the dot-notation path to extract forward/futures rate (e.g., "futures.30d_mark_price").
	ForwardRatePath string `json:"forward_rate_path,omitempty"`

	// ImpliedVolPath is the dot-notation path to extract implied volatility (e.g., "volatility.atm_vol").
	ImpliedVolPath string `json:"implied_vol_path,omitempty"`

	// TokenRateMultiplier scales token rates if returned per 1 token instead of per 1,000,000 tokens (e.g., 1000000.0).
	TokenRateMultiplier float64 `json:"token_rate_multiplier,omitempty"`

	// ArrayMatchKey specifies a field to match against the requested asset when searching an array (e.g., "id", "name", "gpu_name").
	ArrayMatchKey string `json:"array_match_key,omitempty"`

	// StaticMatrix is an optional baseline map of asset -> spot price fallback.
	StaticMatrix map[string]float64 `json:"static_matrix,omitempty"`
}

// CustomHTTPAdapterConfig configures a CustomHTTPAdapter.
type CustomHTTPAdapterConfig struct {
	Name             string
	BaseURL          string
	DefaultPath      string
	Endpoints        map[string]HTTPEndpointTemplate
	Headers          map[string]string // Custom HTTP headers with ${ENV_VAR} expansion
	AuthHeaderName   string
	AuthHeaderPrefix string
	APIKeyEnv        string
	Timeout          time.Duration
	Mappings         DeclarativeMapping
}

// CustomHTTPAdapter implements PricingProvider by querying an external HTTP pricing API.
type CustomHTTPAdapter struct {
	cfg CustomHTTPAdapterConfig
}

// NewCustomHTTPAdapter creates a new adapter from the given config.
func NewCustomHTTPAdapter(cfg CustomHTTPAdapterConfig) *CustomHTTPAdapter {
	if cfg.DefaultPath == "" {
		cfg.DefaultPath = "/prices"
	}
	if cfg.AuthHeaderName == "" {
		cfg.AuthHeaderName = "Authorization"
	}
	if cfg.AuthHeaderPrefix == "" {
		cfg.AuthHeaderPrefix = "Bearer "
	}
	if cfg.APIKeyEnv == "" {
		cfg.APIKeyEnv = "PRICING_API_KEY"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.Endpoints == nil {
		cfg.Endpoints = map[string]HTTPEndpointTemplate{}
	}
	if cfg.Headers == nil {
		cfg.Headers = map[string]string{}
	}
	return &CustomHTTPAdapter{cfg: cfg}
}

func (a *CustomHTTPAdapter) Name() string { return a.cfg.Name }

func (a *CustomHTTPAdapter) Query(ctx context.Context, q types.PricingQuery) (*types.PricingResult, error) {
	baseURL := a.cfg.BaseURL
	if envURL := os.Getenv("PRICING_ORACLE_URL"); envURL != "" {
		baseURL = envURL
	}
	if baseURL == "" {
		return nil, fmt.Errorf("custom_http adapter %q: base_url is not configured (set base_url in pricing_providers.json or PRICING_ORACLE_URL env)", a.cfg.Name)
	}

	path, fixedParams := a.resolveEndpoint(q.MarketType)
	endpoint, err := url.Parse(baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("custom_http adapter %q: bad URL: %w", a.cfg.Name, err)
	}

	qv := endpoint.Query()
	if !a.hasDeclarativeMappings() {
		qv.Set("asset", q.Asset)
		qv.Set("market_type", q.MarketType)
		if q.ProviderFilter != "" {
			qv.Set("provider", q.ProviderFilter)
		}
	}
	for k, v := range fixedParams {
		qv.Set(k, strings.ReplaceAll(v, "{{asset}}", q.Asset))
	}
	endpoint.RawQuery = qv.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		if a.hasDeclarativeMappings() {
			normalizedJSON := a.normalizeDeclarativeResponse(nil, q)
			return BuildResult(a.cfg.Name, q.Asset, q.MarketType, normalizedJSON), nil
		}
		return nil, fmt.Errorf("custom_http adapter %q: request error: %w", a.cfg.Name, err)
	}

	// Apply configured custom headers with environment variable expansion
	for k, v := range a.cfg.Headers {
		req.Header.Set(k, expandEnvVars(v))
	}

	// Standard API key header
	apiKeyEnv := a.cfg.APIKeyEnv
	if key := os.Getenv(apiKeyEnv); key != "" && req.Header.Get(a.cfg.AuthHeaderName) == "" {
		req.Header.Set(a.cfg.AuthHeaderName, a.cfg.AuthHeaderPrefix+key)
	}

	client := &http.Client{Timeout: a.cfg.Timeout}
	resp, err := client.Do(req)
	if err != nil || (resp != nil && (resp.StatusCode < 200 || resp.StatusCode >= 300)) {
		if a.hasDeclarativeMappings() {
			if resp != nil {
				resp.Body.Close()
			}
			normalizedJSON := a.normalizeDeclarativeResponse(nil, q)
			return BuildResult(a.cfg.Name, q.Asset, q.MarketType, normalizedJSON), nil
		}
		if err != nil {
			return nil, fmt.Errorf("custom_http adapter %q: request failed: %w", a.cfg.Name, err)
		}
		return nil, fmt.Errorf("custom_http adapter %q: unexpected status %d from %s", a.cfg.Name, resp.StatusCode, endpoint.String())
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("custom_http adapter %q: read body error: %w", a.cfg.Name, err)
	}

	var rawData interface{}
	if err := json.Unmarshal(bodyBytes, &rawData); err != nil {
		return nil, fmt.Errorf("custom_http adapter %q: decode error: %w", a.cfg.Name, err)
	}

	// If declarative extraction mappings are defined, extract and normalize into standard PricingResult
	if a.hasDeclarativeMappings() {
		normalizedJSON := a.normalizeDeclarativeResponse(rawData, q)
		return BuildResult(a.cfg.Name, q.Asset, q.MarketType, normalizedJSON), nil
	}

	// Default fallback: return verbatim parsed JSON
	b, _ := json.Marshal(rawData)
	return BuildResult(a.cfg.Name, q.Asset, q.MarketType, string(b)), nil
}

func (a *CustomHTTPAdapter) resolveEndpoint(marketType string) (string, map[string]string) {
	key := strings.ToLower(marketType)
	if tmpl, ok := a.cfg.Endpoints[key]; ok {
		path := tmpl.Path
		if path == "" {
			path = a.cfg.DefaultPath
		}
		return path, tmpl.QueryParams
	}
	return a.cfg.DefaultPath, nil
}

func (a *CustomHTTPAdapter) hasDeclarativeMappings() bool {
	m := a.cfg.Mappings
	return m.SpotRatePath != "" || m.InputTokenRatePath != "" || m.OutputTokenRatePath != "" ||
		m.ForwardRatePath != "" || m.ImpliedVolPath != "" || len(m.StaticMatrix) > 0
}

func (a *CustomHTTPAdapter) normalizeDeclarativeResponse(rawData interface{}, q types.PricingQuery) string {
	m := a.cfg.Mappings
	asset := q.Asset
	if asset == "" {
		asset = "H100_SXM"
	}

	spotRate := 0.0
	if m.SpotRatePath != "" {
		spotRate = ExtractFloatFromJSONPath(rawData, m.SpotRatePath, asset, m.ArrayMatchKey)
	}
	if spotRate <= 0 && len(m.StaticMatrix) > 0 {
		if val, ok := m.StaticMatrix[asset]; ok {
			spotRate = val
		}
	}
	if spotRate <= 0 {
		spotRate = 1.25 // Default fallback baseline
	}

	tokenMultiplier := m.TokenRateMultiplier
	if tokenMultiplier <= 0 {
		tokenMultiplier = 1.0
	}

	inputCost1M := 0.0
	if m.InputTokenRatePath != "" {
		rawIn := ExtractFloatFromJSONPath(rawData, m.InputTokenRatePath, asset, m.ArrayMatchKey)
		inputCost1M = math.Round(rawIn*tokenMultiplier*10000.0) / 10000.0
	}

	outputCost1M := 0.0
	if m.OutputTokenRatePath != "" {
		rawOut := ExtractFloatFromJSONPath(rawData, m.OutputTokenRatePath, asset, m.ArrayMatchKey)
		outputCost1M = math.Round(rawOut*tokenMultiplier*10000.0) / 10000.0
	}

	forwardRate := 0.0
	if m.ForwardRatePath != "" {
		forwardRate = ExtractFloatFromJSONPath(rawData, m.ForwardRatePath, asset, m.ArrayMatchKey)
	}
	if forwardRate <= 0 {
		forwardRate = math.Round(spotRate*1.05*10000.0) / 10000.0
	}

	atmVol := 0.35
	if m.ImpliedVolPath != "" {
		if v := ExtractFloatFromJSONPath(rawData, m.ImpliedVolPath, asset, m.ArrayMatchKey); v > 0 {
			atmVol = v
		}
	}

	matrix := map[string]interface{}{
		asset: spotRate,
	}
	for k, v := range m.StaticMatrix {
		matrix[k] = v
	}

	respMap := map[string]interface{}{
		"status":                        "success",
		"provider":                      a.cfg.Name,
		"asset":                         asset,
		"market":                        q.MarketType,
		"spot_gpu_rate_per_hour_usd":    fmt.Sprintf("$%.4f", spotRate),
		"spot_price":                    spotRate,
		"price":                         spotRate,
		"input_cost_per_1m":             inputCost1M,
		"output_cost_per_1m":            outputCost1M,
		"forward_rate_usd":              forwardRate,
		"30d_forward_contract_rate_usd": forwardRate,
		"atm_vol":                       atmVol,
		"atm_vol_30d":                   atmVol,
		"matrix":                        matrix,
		"models": map[string]interface{}{
			asset: map[string]interface{}{
				"input_cost_per_1m":  inputCost1M,
				"output_cost_per_1m": outputCost1M,
				"base_spot_per_hour": spotRate,
			},
		},
	}

	b, _ := json.Marshal(respMap)
	return string(b)
}

var envVarRegex = regexp.MustCompile(`\$\{([a-zA-Z0-9_]+)\}`)

func expandEnvVars(s string) string {
	return envVarRegex.ReplaceAllStringFunc(s, func(match string) string {
		varName := match[2 : len(match)-1]
		return os.Getenv(varName)
	})
}

// ExtractFloatFromJSONPath traverses arbitrary JSON data using dot/bracket paths.
// Supports:
//   - Dot notation: "rates.H100"
//   - Array index: "offers.0.dph_total" or "offers[0].dph_total"
//   - Placeholder: "{{asset}}" replaced with asset or normalized variants
//   - Array element matching by arrayMatchKey (e.g. searching data for element with id == asset)
func ExtractFloatFromJSONPath(root interface{}, path, asset, arrayMatchKey string) float64 {
	if root == nil || path == "" {
		return 0
	}

	resolvedPath := strings.ReplaceAll(path, "{{asset}}", asset)
	// Standardize array bracket notation: "offers[0]" -> "offers.0"
	resolvedPath = strings.ReplaceAll(resolvedPath, "[", ".")
	resolvedPath = strings.ReplaceAll(resolvedPath, "]", "")
	parts := strings.Split(resolvedPath, ".")

	current := root
	for i, part := range parts {
		if current == nil {
			return 0
		}
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		switch node := current.(type) {
		case map[string]interface{}:
			// Direct key match
			if val, ok := node[part]; ok {
				current = val
			} else {
				// Case-insensitive key match
				found := false
				partLower := strings.ToLower(part)
				for k, v := range node {
					if strings.ToLower(k) == partLower {
						current = v
						found = true
						break
					}
				}
				if !found {
					return 0
				}
			}

		case []interface{}:
			// Numeric array index (e.g. "0")
			if idx, err := strconv.Atoi(part); err == nil && idx >= 0 && idx < len(node) {
				current = node[idx]
			} else if arrayMatchKey != "" {
				// Search array items matching key
				var matchedItem interface{}
				assetNorm := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(asset, "-", ""), "_", ""))
				for _, item := range node {
					if itemMap, ok := item.(map[string]interface{}); ok {
						if rawVal, ok := itemMap[arrayMatchKey]; ok {
							valStr := fmt.Sprintf("%v", rawVal)
							valNorm := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(valStr, "-", ""), "_", ""))
							if strings.Contains(valNorm, assetNorm) || strings.Contains(assetNorm, valNorm) {
								matchedItem = item
								break
							}
						}
					}
				}
				if matchedItem != nil {
					// Apply current part to matched item
					if itemMap, ok := matchedItem.(map[string]interface{}); ok {
						if val, ok := itemMap[part]; ok {
							current = val
						} else {
							// Continue traversal from matched item
							remainingParts := parts[i:]
							subPath := strings.Join(remainingParts, ".")
							return ExtractFloatFromJSONPath(matchedItem, subPath, asset, "")
						}
					} else {
						current = matchedItem
					}
				} else if len(node) > 0 {
					current = node[0] // fallback to first item
				} else {
					return 0
				}
			} else if len(node) > 0 {
				current = node[0]
			} else {
				return 0
			}

		default:
			return coerceToFloat(current)
		}
	}

	return coerceToFloat(current)
}

func coerceToFloat(val interface{}) float64 {
	switch v := val.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		clean := strings.TrimSpace(v)
		clean = strings.TrimPrefix(clean, "$")
		if f, err := strconv.ParseFloat(clean, 64); err == nil {
			return f
		}
	}
	return 0
}
