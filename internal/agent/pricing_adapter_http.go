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

// HTTPEndpointTemplate describes how a CustomHTTPAdapter resolves a URL for a given market type.
// BaseURL is the root of the remote API. Path overrides the default "/prices" for this market.
// QueryParams adds fixed query-string parameters (e.g. {"format": "json"}).
type HTTPEndpointTemplate struct {
	// Path is the URL path for this market type (e.g. "/v1/options/chain").
	// If empty the adapter uses the DefaultPath configured on the parent adapter.
	Path string `json:"path,omitempty"`

	// QueryParams are additional fixed key-value pairs appended to every request
	// for this market type (e.g. {"currency": "USD"}).
	QueryParams map[string]string `json:"query_params,omitempty"`
}

// CustomHTTPAdapterConfig configures a CustomHTTPAdapter without requiring code changes.
// It is typically populated from ProviderProfile fields or pricing_providers.json.
type CustomHTTPAdapterConfig struct {
	// Name is the unique adapter identifier (matches the provider key in pricing_providers.json).
	Name string

	// BaseURL is the root of the remote pricing API (e.g. "https://pricing.company.com").
	BaseURL string

	// DefaultPath is the fallback HTTP path used when no per-market endpoint template is defined.
	// Defaults to "/prices" if empty.
	DefaultPath string

	// Endpoints maps MarketType strings to per-market endpoint templates.
	// E.g.: {"spot": {Path: "/v1/spot"}, "options": {Path: "/v1/options/chain"}}
	// Any market type not present here falls back to DefaultPath.
	Endpoints map[string]HTTPEndpointTemplate

	// AuthHeaderName is the request header used for authentication (default: "Authorization").
	AuthHeaderName string

	// AuthHeaderPrefix is the scheme prefix for the auth value (default: "Bearer ").
	AuthHeaderPrefix string

	// APIKeyEnv is the environment variable name holding the API key.
	// If empty "PRICING_API_KEY" is used.
	APIKeyEnv string

	// Timeout for each HTTP request. Defaults to 5 seconds.
	Timeout time.Duration
}

// CustomHTTPAdapter implements PricingProvider by querying an external HTTP pricing API.
// It supports fully configurable per-market-type endpoint paths, auth schemes, and fixed
// query parameters — no code changes are required to integrate a new external provider.
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
	return &CustomHTTPAdapter{cfg: cfg}
}

// NewCustomHTTPAdapterFromProfile builds a CustomHTTPAdapter from a ProviderProfile.
// EndpointTemplates in the profile are read from the Endpoints field if present.
func NewCustomHTTPAdapterFromProfile(name string, profile ProviderProfile) *CustomHTTPAdapter {
	baseURL := profile.BaseURL
	if envURL := os.Getenv("PRICING_ORACLE_URL"); envURL != "" {
		baseURL = envURL
	}
	return NewCustomHTTPAdapter(CustomHTTPAdapterConfig{
		Name:      name,
		BaseURL:   baseURL,
		Endpoints: profile.Endpoints,
		APIKeyEnv: profile.APIKeyEnv,
	})
}

func (a *CustomHTTPAdapter) Name() string { return a.cfg.Name }

func (a *CustomHTTPAdapter) Query(ctx context.Context, q PricingQuery) (*PricingResult, error) {
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
	qv.Set("asset", q.Asset)
	qv.Set("market_type", q.MarketType)
	if q.ProviderFilter != "" {
		qv.Set("provider", q.ProviderFilter)
	}
	for k, v := range fixedParams {
		qv.Set(k, v)
	}
	endpoint.RawQuery = qv.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("custom_http adapter %q: request error: %w", a.cfg.Name, err)
	}

	apiKeyEnv := a.cfg.APIKeyEnv
	if key := os.Getenv(apiKeyEnv); key != "" {
		req.Header.Set(a.cfg.AuthHeaderName, a.cfg.AuthHeaderPrefix+key)
	}

	client := &http.Client{Timeout: a.cfg.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("custom_http adapter %q: request failed: %w", a.cfg.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("custom_http adapter %q: unexpected status %d from %s", a.cfg.Name, resp.StatusCode, endpoint.String())
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("custom_http adapter %q: decode error: %w", a.cfg.Name, err)
	}

	b, _ := json.Marshal(result)
	return buildResult(a.cfg.Name, q.Asset, q.MarketType, string(b)), nil
}

// resolveEndpoint returns the URL path and any fixed query params for the given market type.
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
