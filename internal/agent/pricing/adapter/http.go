package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/fabith10/synapse-go/internal/agent/pricing/types"
)

// HTTPEndpointTemplate describes how a CustomHTTPAdapter resolves a URL for a given market type.
type HTTPEndpointTemplate struct {
	Path        string            `json:"path,omitempty"`
	QueryParams map[string]string `json:"query_params,omitempty"`
}

// CustomHTTPAdapterConfig configures a CustomHTTPAdapter.
type CustomHTTPAdapterConfig struct {
	Name             string
	BaseURL          string
	DefaultPath      string
	Endpoints        map[string]HTTPEndpointTemplate
	AuthHeaderName   string
	AuthHeaderPrefix string
	APIKeyEnv        string
	Timeout          time.Duration
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
