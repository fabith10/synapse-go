package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fabith10/synapse-go/internal/agent/pricing/adapter"
	"github.com/fabith10/synapse-go/internal/agent/pricing/backtest"
	"github.com/fabith10/synapse-go/internal/agent/pricing/quant"
	"github.com/fabith10/synapse-go/internal/tools"
)

// ProviderProfile defines a pricing provider config item in pricing_providers.json.
type ProviderProfile struct {
	// Type selects the built-in adapter factory: "mock", "custom_http", "static_json".
	Type        string `json:"type"`
	Description string `json:"description"`

	// BaseURL is the root URL for http-based providers.
	BaseURL string `json:"base_url,omitempty"`

	// APIKeyEnv names the environment variable holding the API key (custom_http).
	APIKeyEnv string `json:"api_key_env,omitempty"`

	// FilePath is the local file path for static_json providers.
	FilePath string `json:"file_path,omitempty"`

	// Headers maps custom HTTP headers with environment variable expansion (e.g. "Authorization": "Bearer ${KEY}").
	Headers map[string]string `json:"headers,omitempty"`

	// Endpoints maps market type strings to per-market HTTP endpoint templates.
	Endpoints map[string]adapter.HTTPEndpointTemplate `json:"endpoints,omitempty"`

	// Mappings specifies declarative JSONPath/dot-notation extraction rules.
	Mappings adapter.DeclarativeMapping `json:"mappings,omitempty"`
}

// DeferralPolicyConfig configures when and how non-urgent high-compute tasks
// and expensive LLM queries are deferred to off-peak hours.
type DeferralPolicyConfig struct {
	Enabled          bool      `json:"enabled"`
	BypassAll        bool      `json:"bypass_all"`          // Global override to bypass all deferral checks
	CostThresholdUSD float64   `json:"cost_threshold_usd"`  // Minimum cost in USD to evaluate deferral
	TokenThreshold   int       `json:"token_threshold"`     // Minimum token count to evaluate deferral
	PeakStartHour    int       `json:"peak_start_hour_utc"` // UTC hour when peak tariff begins (0-23)
	PeakEndHour      int       `json:"peak_end_hour_utc"`   // UTC hour when peak tariff ends (0-23)
	SpotMultipliers  []float64 `json:"spot_multipliers"`     // 24 hourly spot multipliers
	VolMultipliers   []float64 `json:"vol_multipliers"`      // 24 hourly volatility multipliers
}

// PricingConfigRoot defines the structure of pricing_providers.json.
type PricingConfigRoot struct {
	ActiveProvider string                     `json:"active_provider"`
	DeferralPolicy DeferralPolicyConfig       `json:"deferral_policy"`
	Providers      map[string]ProviderProfile `json:"providers"`
}

// PricingOracleManager manages dynamic pricing provider selection and execution.
// Providers are registered as PricingProvider adapters; the manager delegates all
// query execution to the active adapter without any provider-specific logic.
type PricingOracleManager struct {
	mu         sync.RWMutex
	configPath string
	config     PricingConfigRoot
	adapters   map[string]PricingProvider
}

var (
	globalOracle     *PricingOracleManager
	globalOracleOnce sync.Once
)

// GetPricingOracleManager returns the global PricingOracleManager singleton.
func GetPricingOracleManager() *PricingOracleManager {
	globalOracleOnce.Do(func() {
		configPath := tools.FindConfigPath("pricing_providers.json")
		if configPath == "" {
			configPath = "pricing_providers.json"
		}
		globalOracle = NewPricingOracleManager(configPath)
	})
	return globalOracle
}

// SetMockServerBaseURL propagates the mock server URL into the adapter subpackage
// so the singleton mock adapter picks it up without re-creation.
func SetMockServerBaseURL(urlStr string) {
	adapter.SetMockServerBaseURL(urlStr)
}



// NewPricingOracleManager initializes a manager bound to a configuration file path.
// The three built-in adapters (mock, static_json, custom_http) are registered automatically.
// Any custom_http providers defined in the config file are also registered as adapters.
func NewPricingOracleManager(configPath string) *PricingOracleManager {
	m := &PricingOracleManager{
		configPath: configPath,
		adapters:   make(map[string]PricingProvider),
		config: PricingConfigRoot{
			ActiveProvider: "mock",
			DeferralPolicy: DeferralPolicyConfig{
				Enabled:          true,
				BypassAll:        false,
				CostThresholdUSD: 1.0,
				TokenThreshold:   50000,
				PeakStartHour:    8,
				PeakEndHour:      18,
				SpotMultipliers: []float64{
					0.82, 0.80, 0.79, 0.79, 0.80, 0.83,
					0.90, 0.95, 1.08, 1.15, 1.18, 1.20,
					1.22, 1.22, 1.20, 1.18, 1.15, 1.10,
					1.05, 0.98, 0.95, 0.92, 0.88, 0.85,
				},
				VolMultipliers: []float64{
					0.32, 0.70, 0.75, 0.78, 0.72, 0.45,
					0.35, 0.32, 0.28, 0.25, 0.25, 0.26,
					0.27, 0.28, 0.28, 0.29, 0.30, 0.31,
					0.32, 0.33, 0.34, 0.33, 0.32, 0.31,
				},
			},
			Providers: map[string]ProviderProfile{
				"mock": {
					Type:        "mock",
					Description: "Internal mock compute pricing and task execution endpoints",
					BaseURL:     adapter.MockServerBaseURL,
				},
				"static_json": {
					Type:        "static_json",
					Description: "Local workspace static JSON pricing matrix",
					FilePath:    "pricing_matrix.json",
				},
			},
		},
	}
	// Register built-in adapters.
	m.adapters["null"] = &adapter.NullPricingAdapter{}
	m.adapters["mock"] = adapter.NewMockPricingAdapter("")
	m.adapters["static_json"] = adapter.NewStaticJSONAdapter("static_json", "pricing_matrix.json")
	m.adapters["openrouter"] = adapter.NewOpenRouterPricingAdapter("")
	m.adapters["vastai"] = adapter.NewVastAIPricingAdapter("")
	m.adapters["deribit"] = adapter.NewDeribitPricingAdapter("", "")
	m.adapters["free_live"] = adapter.NewFreeCompositePricingAdapter()

	// Load persisted config (may add custom_http providers).
	_ = m.LoadConfig()

	// Register any custom_http providers found in config.
	m.registerConfigAdapters()
	return m
}

func (m *PricingOracleManager) registerConfigAdapters() {
	for name, profile := range m.config.Providers {
		if _, already := m.adapters[name]; already {
			continue // built-in adapters are never overwritten by config
		}
		switch strings.ToLower(profile.Type) {
		case "openrouter":
			m.adapters[name] = adapter.NewOpenRouterPricingAdapter(profile.BaseURL)
		case "vastai":
			m.adapters[name] = adapter.NewVastAIPricingAdapter(profile.BaseURL)
		case "deribit":
			m.adapters[name] = adapter.NewDeribitPricingAdapter(profile.BaseURL, "")
		case "free_live", "composite":
			m.adapters[name] = adapter.NewFreeCompositePricingAdapter()
		case "custom_http", "http", "declarative_http":
			m.adapters[name] = newCustomHTTPAdapterFromProfile(name, profile)
		case "static_json", "file":
			m.adapters[name] = adapter.NewStaticJSONAdapter(name, profile.FilePath)
		}
	}
}

// newCustomHTTPAdapterFromProfile constructs a CustomHTTPAdapter from a ProviderProfile.
// Lives in package pricing so it can access ProviderProfile without creating an import cycle.
func newCustomHTTPAdapterFromProfile(name string, profile ProviderProfile) *adapter.CustomHTTPAdapter {
	baseURL := profile.BaseURL
	if envURL := os.Getenv("PRICING_ORACLE_URL"); envURL != "" {
		baseURL = envURL
	}
	endpoints := make(map[string]adapter.HTTPEndpointTemplate)
	for k, v := range profile.Endpoints {
		endpoints[k] = adapter.HTTPEndpointTemplate{
			Path:        v.Path,
			QueryParams: v.QueryParams,
		}
	}
	return adapter.NewCustomHTTPAdapter(adapter.CustomHTTPAdapterConfig{
		Name:      name,
		BaseURL:   baseURL,
		Endpoints: endpoints,
		Headers:   profile.Headers,
		APIKeyEnv: profile.APIKeyEnv,
		Mappings:  profile.Mappings,
	})
}


// LoadConfig reads the configuration file from disk if present.
func (m *PricingOracleManager) LoadConfig() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.configPath == "" {
		return nil
	}

	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return err
	}

	root := m.config
	if err := json.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("invalid pricing_providers.json: %w", err)
	}

	m.config = root
	return nil
}

// SaveConfig persists the current configuration to disk.
func (m *PricingOracleManager) SaveConfig() error {
	m.mu.RLock()
	data, err := json.MarshalIndent(m.config, "", "  ")
	path := m.configPath
	m.mu.RUnlock()

	if err != nil {
		return err
	}
	if path == "" {
		path = "pricing_providers.json"
	}
	return os.WriteFile(path, data, 0644)
}

// GetActiveProviderName returns the current active provider name, incorporating environment variable overrides.
func (m *PricingOracleManager) GetActiveProviderName() string {
	if envProv := os.Getenv("PRICING_PROVIDER"); envProv != "" {
		return strings.ToLower(envProv)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.config.ActiveProvider != "" {
		return m.config.ActiveProvider
	}
	return "mock"
}

// SetActiveProvider changes the active provider dynamically at runtime and persists to disk.
func (m *PricingOracleManager) SetActiveProvider(providerName string) error {
	m.mu.Lock()
	pName := strings.ToLower(providerName)
	if _, exists := m.config.Providers[pName]; !exists {
		m.config.Providers[pName] = ProviderProfile{
			Type:        pName,
			Description: fmt.Sprintf("Dynamic provider %s", pName),
		}
	}
	m.config.ActiveProvider = pName
	m.mu.Unlock()

	return m.SaveConfig()
}

// RegisterAdapter registers a custom PricingProvider adapter under the given name.
func (m *PricingOracleManager) RegisterAdapter(name string, p PricingProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.adapters[name] = p
}

// GetConfigRoot returns a copy of the current full pricing configuration.
func (m *PricingOracleManager) GetConfigRoot() PricingConfigRoot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rootCopy := PricingConfigRoot{
		ActiveProvider: m.GetActiveProviderName(),
		Providers:      make(map[string]ProviderProfile),
	}
	for k, v := range m.config.Providers {
		rootCopy.Providers[k] = v
	}
	return rootCopy
}

// ExecuteQuery resolves the active provider adapter and delegates the query to it.
func (m *PricingOracleManager) ExecuteQuery(ctx context.Context, asset, marketType, providerFilter string, options map[string]string) (string, error) {
	mt := strings.ToLower(marketType)
	if mt == "execution_window" || mt == "optimal_window" || mt == "scheduling" {
		return m.ExecuteExecutionWindowQuery(ctx, asset, providerFilter, options)
	}
	if mt == "prompt_cost" || mt == "token_cost" || mt == "llm_cost" {
		return m.ExecutePromptCostQuery(ctx, asset, providerFilter, options)
	}
	if mt == "options" || mt == "derivatives" || mt == "greeks" {
		return m.ExecuteOptionsQuery(ctx, asset, providerFilter, options)
	}
	if mt == "risk_metrics" || mt == "var" || mt == "cvar" {
		return m.ExecuteRiskMetricsQuery(ctx, asset, providerFilter, options)
	}
	if mt == "real_options" || mt == "roa" {
		return m.ExecuteRealOptionsQuery(ctx, asset, providerFilter, options)
	}
	if mt == "lucia_schwartz" || mt == "two_factor" || mt == "hardware_decay" {
		return m.ExecuteLuciaSchwartzQuery(ctx, asset, providerFilter, options)
	}
	if mt == "regime_switching" || mt == "mrs" || mt == "hamilton" {
		return m.ExecuteRegimeSwitchingQuery(ctx, asset, providerFilter, options)
	}
	if mt == "congestion" || mt == "queuing" || mt == "cluster_load" {
		return m.ExecuteCongestionQuery(ctx, asset, providerFilter, options)
	}
	if mt == "game_theory" || mt == "nash" || mt == "minority_game" || mt == "blotto" {
		return m.ExecuteGameTheoryQuery(ctx, asset, providerFilter, options)
	}
	if mt == "backtest" || mt == "backtesting" || mt == "simulation" {
		return m.ExecuteBacktestQuery(ctx, asset, providerFilter, options)
	}

	activeName := m.GetActiveProviderName()

	m.mu.RLock()
	prov, ok := m.adapters[activeName]
	m.mu.RUnlock()

	if !ok {
		return adapter.NullAdapterResponse(activeName, asset, marketType), nil
	}

	result, err := prov.Query(ctx, PricingQuery{
		Asset:          asset,
		MarketType:     marketType,
		ProviderFilter: providerFilter,
		Options:        options,
	})
	if err != nil {
		return "", fmt.Errorf("pricing oracle [%s]: %w", activeName, err)
	}
	return result.Raw, nil
}

// ExecuteOptionsQuery evaluates European options and analytical Greeks on compute forwards using Black-76.
func (m *PricingOracleManager) ExecuteOptionsQuery(ctx context.Context, asset, providerFilter string, options map[string]string) (string, error) {
	spotRaw, _ := m.ExecuteQuery(ctx, asset, "spot", providerFilter, nil)
	var parsed map[string]interface{}
	_ = json.Unmarshal([]byte(spotRaw), &parsed)
	s0 := spotBaseRate(asset)
	if p, ok := parsed["spot_price"].(float64); ok && p > 0 {
		s0 = p
	} else if p, ok := parsed["price"].(float64); ok && p > 0 {
		s0 = p
	}

	F := s0 * 1.05
	if f, ok := parsed["forward_rate_usd"].(float64); ok && f > 0 {
		F = f
	}

	K := F
	if kStr, ok := options["strike"]; ok {
		if kVal, err := strconv.ParseFloat(kStr, 64); err == nil && kVal > 0 {
			K = kVal
		}
	}

	days := 30.0
	if dStr, ok := options["days"]; ok {
		if dVal, err := strconv.ParseFloat(dStr, 64); err == nil && dVal > 0 {
			days = dVal
		}
	}
	T := days / 365.0

	sigma := 0.35
	if v, ok := parsed["atm_vol"].(float64); ok && v > 0 {
		sigma = v
	}
	if vStr, ok := options["volatility"]; ok {
		if vVal, err := strconv.ParseFloat(vStr, 64); err == nil && vVal > 0 {
			sigma = vVal
		}
	}

	r := 0.045
	if rStr, ok := options["rate"]; ok {
		if rVal, err := strconv.ParseFloat(rStr, 64); err == nil {
			r = rVal
		}
	}

	callRes, err := quant.Black76Price(F, K, T, sigma, r, quant.OptionCall)
	if err != nil {
		return "", err
	}
	putRes, err := quant.Black76Price(F, K, T, sigma, r, quant.OptionPut)
	if err != nil {
		return "", err
	}

	resp := map[string]interface{}{
		"status":          "success",
		"asset":           asset,
		"model":           "Black-76 Commodity Option & Greeks Engine",
		"forward_rate":    F,
		"strike_price":    K,
		"expiry_days":     days,
		"implied_vol":     sigma,
		"risk_free_rate":  r,
		"call_option":     callRes,
		"put_option":      putRes,
		"put_call_parity": math.Abs((callRes.Price-putRes.Price)-math.Exp(-r*T)*(F-K)) < 1e-4,
	}

	b, _ := json.Marshal(resp)
	return string(b), nil
}

// ExecuteRiskMetricsQuery evaluates Monte Carlo Value-at-Risk (VaR) and Conditional VaR (Expected Shortfall).
func (m *PricingOracleManager) ExecuteRiskMetricsQuery(ctx context.Context, asset, providerFilter string, options map[string]string) (string, error) {
	spotRaw, _ := m.ExecuteQuery(ctx, asset, "spot", providerFilter, nil)
	var parsed map[string]interface{}
	_ = json.Unmarshal([]byte(spotRaw), &parsed)
	s0 := spotBaseRate(asset)
	if p, ok := parsed["spot_price"].(float64); ok && p > 0 {
		s0 = p
	}

	startHour := 0
	if shStr, ok := options["start_hour"]; ok {
		if shVal, err := strconv.Atoi(shStr); err == nil {
			startHour = shVal % 24
		}
	}

	duration := 4
	if durStr, ok := options["duration"]; ok {
		if durVal, err := strconv.Atoi(durStr); err == nil && durVal > 0 {
			duration = durVal
		}
	}

	costMode := "spot"
	if cm, ok := options["cost_mode"]; ok && cm != "" {
		costMode = cm
	}

	forwardCap := s0 * 1.05
	profile := quant.ComputeExecutionWindowRisk(s0, startHour, duration, costMode, forwardCap, 3000)

	resp := map[string]interface{}{
		"status":       "success",
		"asset":        asset,
		"model":        "Ornstein-Uhlenbeck Jump-Diffusion Monte Carlo",
		"risk_profile": profile,
	}
	b, _ := json.Marshal(resp)
	return string(b), nil
}

// ExecuteRealOptionsQuery evaluates the economic value of compute deferral and multi-tier switching.
func (m *PricingOracleManager) ExecuteRealOptionsQuery(ctx context.Context, asset, providerFilter string, options map[string]string) (string, error) {
	spotRaw, _ := m.ExecuteQuery(ctx, asset, "spot", providerFilter, nil)
	var parsed map[string]interface{}
	_ = json.Unmarshal([]byte(spotRaw), &parsed)
	s0 := spotBaseRate(asset)
	if p, ok := parsed["spot_price"].(float64); ok && p > 0 {
		s0 = p
	}

	duration := 4
	if durStr, ok := options["duration"]; ok {
		if durVal, err := strconv.Atoi(durStr); err == nil && durVal > 0 {
			duration = durVal
		}
	}

	vol := 0.35
	if v, ok := parsed["atm_vol"].(float64); ok && v > 0 {
		vol = v
	}

	roa := quant.EvaluateRealOptions(asset, s0, s0*0.75, duration, vol, s0*1.55)
	resp := map[string]interface{}{
		"status":       "success",
		"asset":        asset,
		"model":        "Real Options Analysis (ROA) Flexibilities",
		"real_options": roa,
	}
	b, _ := json.Marshal(resp)
	return string(b), nil
}

// ExecuteLuciaSchwartzQuery computes the Two-Factor forward curve incorporating diurnal mean-reversion and Moore's Law deflation.
func (m *PricingOracleManager) ExecuteLuciaSchwartzQuery(ctx context.Context, asset, providerFilter string, options map[string]string) (string, error) {
	spotRaw, _ := m.ExecuteQuery(ctx, asset, "spot", providerFilter, nil)
	var parsed map[string]interface{}
	_ = json.Unmarshal([]byte(spotRaw), &parsed)
	s0 := spotBaseRate(asset)
	if p, ok := parsed["spot_price"].(float64); ok && p > 0 {
		s0 = p
	}

	curve := quant.ComputeLuciaSchwartzCurve(asset, s0)
	resp := map[string]interface{}{
		"status":                  "success",
		"asset":                   asset,
		"model":                   "Lucia-Schwartz Two-Factor Model",
		"two_factor_term_curve":   curve,
	}
	b, _ := json.Marshal(resp)
	return string(b), nil
}

// ExecuteRegimeSwitchingQuery computes Markov Regime-Switching risk and Bayesian state probabilities.
func (m *PricingOracleManager) ExecuteRegimeSwitchingQuery(ctx context.Context, asset, providerFilter string, options map[string]string) (string, error) {
	spotRaw, _ := m.ExecuteQuery(ctx, asset, "spot", providerFilter, nil)
	var parsed map[string]interface{}
	_ = json.Unmarshal([]byte(spotRaw), &parsed)
	s0 := spotBaseRate(asset)
	if p, ok := parsed["spot_price"].(float64); ok && p > 0 {
		s0 = p
	}

	duration := 4
	if durStr, ok := options["duration"]; ok {
		if durVal, err := strconv.Atoi(durStr); err == nil && durVal > 0 {
			duration = durVal
		}
	}

	mrsProfile := quant.EvaluateRegimeSwitchingRisk(asset, s0, duration, 2000)
	resp := map[string]interface{}{
		"status":             "success",
		"asset":              asset,
		"model":              "Markov Regime-Switching Model (MRS / Hamilton)",
		"regime_risk_report": mrsProfile,
	}
	b, _ := json.Marshal(resp)
	return string(b), nil
}

// ExecuteCongestionQuery computes M/M/c cluster capacity load, congestion surcharges, and queue eviction risk.
func (m *PricingOracleManager) ExecuteCongestionQuery(ctx context.Context, asset, providerFilter string, options map[string]string) (string, error) {
	spotRaw, _ := m.ExecuteQuery(ctx, asset, "spot", providerFilter, nil)
	var parsed map[string]interface{}
	_ = json.Unmarshal([]byte(spotRaw), &parsed)
	s0 := spotBaseRate(asset)
	if p, ok := parsed["spot_price"].(float64); ok && p > 0 {
		s0 = p
	}

	congestion := quant.ComputeQueuingCongestionAnalytics(asset, s0)
	resp := map[string]interface{}{
		"status":             "success",
		"asset":              asset,
		"model":              "M/M/c Queuing Capacity Congestion Model",
		"congestion_report":  congestion,
	}
	b, _ := json.Marshal(resp)
	return string(b), nil
}

// ExecuteGameTheoryQuery evaluates Boltzmann Nash mixed strategies, Minority Game crowd penalties, and Colonel Blotto allocation.
func (m *PricingOracleManager) ExecuteGameTheoryQuery(ctx context.Context, asset, providerFilter string, options map[string]string) (string, error) {
	spotRaw, _ := m.ExecuteQuery(ctx, asset, "spot", providerFilter, nil)
	var parsed map[string]interface{}
	_ = json.Unmarshal([]byte(spotRaw), &parsed)
	s0 := spotBaseRate(asset)
	if p, ok := parsed["spot_price"].(float64); ok && p > 0 {
		s0 = p
	}

	duration := 4
	if durStr, ok := options["duration"]; ok {
		if durVal, err := strconv.Atoi(durStr); err == nil && durVal > 0 {
			duration = durVal
		}
	}

	// 24-hour tariff profile
	baseProfile := []float64{
		s0 * 0.82, s0 * 0.80, s0 * 0.79, s0 * 0.79, s0 * 0.80, s0 * 0.83,
		s0 * 0.90, s0 * 0.95, s0 * 1.08, s0 * 1.15, s0 * 1.18, s0 * 1.20,
		s0 * 1.22, s0 * 1.22, s0 * 1.20, s0 * 1.18, s0 * 1.15, s0 * 1.10,
		s0 * 1.05, s0 * 0.98, s0 * 0.95, s0 * 0.92, s0 * 0.88, s0 * 0.85,
	}

	windowCosts := make([]float64, 24)
	for start := 0; start < 24; start++ {
		tot := 0.0
		for h := 0; h < duration; h++ {
			tot += baseProfile[(start+h)%24]
		}
		windowCosts[start] = roundTo4(tot)
	}

	boltzmann := quant.ComputeBoltzmannNashDistribution(windowCosts, 2.5)
	minority := quant.EvaluateMinorityGameCrowding(windowCosts, 0.45, 2.0)
	blotto := quant.ComputeBlottoAllocation(asset, float64(duration), s0)

	resp := map[string]interface{}{
		"status":                      "success",
		"asset":                       asset,
		"model":                       "Game-Theoretic Anti-Herding & Multi-Cluster Suite",
		"boltzmann_nash_distribution": boltzmann,
		"minority_game_crowd_penalty": minority,
		"blotto_portfolio_allocation": blotto,
	}
	b, _ := json.Marshal(resp)
	return string(b), nil
}

// ExecuteBacktestQuery executes a simulation/backtest of pricing oracle models across configurable days and scenarios.
func (m *PricingOracleManager) ExecuteBacktestQuery(ctx context.Context, asset, providerFilter string, options map[string]string) (string, error) {
	spotRaw, _ := m.ExecuteQuery(ctx, asset, "spot", providerFilter, nil)
	var parsed map[string]interface{}
	_ = json.Unmarshal([]byte(spotRaw), &parsed)
	s0 := spotBaseRate(asset)
	if p, ok := parsed["spot_price"].(float64); ok && p > 0 {
		s0 = p
	}

	days := 30
	if dStr, ok := options["days"]; ok {
		if dVal, err := strconv.Atoi(dStr); err == nil && dVal > 0 {
			days = dVal
		}
	} else if dStr, ok := options["duration_days"]; ok {
		if dVal, err := strconv.Atoi(dStr); err == nil && dVal > 0 {
			days = dVal
		}
	}

	modelType := "ou_jump_diffusion"
	if mStr, ok := options["model"]; ok && mStr != "" {
		modelType = mStr
	} else if mStr, ok := options["model_type"]; ok && mStr != "" {
		modelType = mStr
	}

	tasksCount := 100
	if tStr, ok := options["tasks"]; ok {
		if tVal, err := strconv.Atoi(tStr); err == nil && tVal > 0 {
			tasksCount = tVal
		}
	}

	strike := 0.0
	if sStr, ok := options["strike_rate"]; ok {
		if sVal, err := strconv.ParseFloat(sStr, 64); err == nil && sVal > 0 {
			strike = sVal
		}
	}

	dataset := ""
	if ds, ok := options["dataset"]; ok && ds != "" {
		dataset = ds
	} else if ds, ok := options["dataset_name"]; ok && ds != "" {
		dataset = ds
	}

	datasetPath := ""
	if dp, ok := options["dataset_path"]; ok && dp != "" {
		datasetPath = dp
	}

	engine := backtest.NewBacktestEngine()
	report, err := engine.RunBacktest(backtest.BacktestConfig{
		Asset:                  asset,
		DatasetName:            dataset,
		DatasetPath:            datasetPath,
		BaseSpotRateUSD:        s0,
		SimulationDurationDays: days,
		NumMonteCarloPaths:     1000,
		ModelType:              modelType,
		NumWorkloadTasks:       tasksCount,
		StrikeRateUSD:          strike,
	})
	if err != nil {
		return "", fmt.Errorf("backtest execution failed: %w", err)
	}

	format := strings.ToLower(options["format"])
	if format == "json" {
		return report.ToJSON()
	}

	return backtest.FormatMarkdownReport(report), nil
}

// buildResult is a shared helper that constructs a PricingResult from a raw JSON string.
func buildResult(provider, asset, marketType, raw string) *PricingResult {
	return adapter.BuildResult(provider, asset, marketType, raw)
}

// nullAdapterResponse returns the formatted JSON for an unconfigured/missing provider.
func nullAdapterResponse(activeName, asset, marketType string) string {
	return adapter.NullAdapterResponse(activeName, asset, marketType)
}

// spotBaseRate returns a default spot price for well-known GPU assets.
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

// ShouldDefer evaluates whether a task or query exceeds the configured deferral
// thresholds and should be deferred because we are currently in peak hours.
func (m *PricingOracleManager) ShouldDefer(estimatedCost float64, tokenCount int, forceImmediate bool) (bool, string) {
	if os.Getenv("FORCE_URGENT") == "true" || os.Getenv("DEFERRAL_BYPASS") == "true" {
		return false, "global environment override (FORCE_URGENT/DEFERRAL_BYPASS)"
	}

	m.mu.RLock()
	policy := m.config.DeferralPolicy
	m.mu.RUnlock()

	if !policy.Enabled {
		return false, "deferral policy disabled"
	}
	if policy.BypassAll || forceImmediate {
		return false, "forced immediate or global config bypass active"
	}

	costExceeded := policy.CostThresholdUSD > 0 && estimatedCost >= policy.CostThresholdUSD
	tokensExceeded := policy.TokenThreshold > 0 && tokenCount >= policy.TokenThreshold

	if !costExceeded && !tokensExceeded {
		return false, "workload under cost and token thresholds"
	}

	now := time.Now().UTC()
	currentHour := now.Hour()

	isPeak := false
	if policy.PeakStartHour <= policy.PeakEndHour {
		isPeak = currentHour >= policy.PeakStartHour && currentHour < policy.PeakEndHour
	} else {
		isPeak = currentHour >= policy.PeakStartHour || currentHour < policy.PeakEndHour
	}

	if !isPeak {
		return false, fmt.Sprintf("current hour %02d:00 UTC is off-peak", currentHour)
	}

	reason := fmt.Sprintf("Peak hours active (%02d:00-%02d:00 UTC) and workload exceeds thresholds (cost: $%.2f vs $%.2f, tokens: %d vs %d)",
		policy.PeakStartHour, policy.PeakEndHour, estimatedCost, policy.CostThresholdUSD, tokenCount, policy.TokenThreshold)
	return true, reason
}

// extractSpotPrice extracts a base spot rate from various provider JSON formats.
func extractSpotPrice(rawJSON, asset string) (float64, error) {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &m); err != nil {
		return 0, err
	}
	if prices, ok := m["prices"].([]interface{}); ok && len(prices) > 0 {
		for _, pItem := range prices {
			if pMap, ok := pItem.(map[string]interface{}); ok {
				if sph, ok := pMap["spot_price_per_hour"].(float64); ok {
					return sph, nil
				}
			}
		}
	}
	if models, ok := m["models"].(map[string]interface{}); ok {
		if modelData, ok := models[asset].(map[string]interface{}); ok {
			if sph, ok := modelData["base_spot_per_hour"].(float64); ok {
				return sph, nil
			}
		}
	}
	if matrix, ok := m["matrix"].(map[string]interface{}); ok {
		if sph, ok := matrix[asset].(float64); ok {
			return sph, nil
		}
	}
	if price, ok := m[asset].(float64); ok {
		return price, nil
	}
	if price, ok := m["price"].(float64); ok {
		return price, nil
	}
	return 0, fmt.Errorf("could not parse spot price for %s", asset)
}


func roundTo4(val float64) float64 {
	return math.Round(val*10000.0) / 10000.0
}

func labelHour(startHour int) string {
	switch {
	case startHour >= 8 && startHour < 18:
		return "peak"
	case (startHour >= 6 && startHour < 8) || (startHour >= 18 && startHour < 22):
		return "shoulder"
	default:
		return "off-peak"
	}
}
