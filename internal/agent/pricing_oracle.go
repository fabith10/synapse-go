package agent

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

	// Endpoints maps market type strings to per-market HTTP endpoint templates.
	// Used by custom_http providers to support APIs with different URL structures
	// per market (e.g. {"options": {"path": "/v1/options/chain"}}).
	Endpoints map[string]HTTPEndpointTemplate `json:"endpoints,omitempty"`
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
		configPath := findConfigPath("pricing_providers.json")
		if configPath == "" {
			configPath = "pricing_providers.json"
		}
		globalOracle = NewPricingOracleManager(configPath)
	})
	return globalOracle
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
					BaseURL:     mockServerBaseURL,
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
	// The null adapter is always present as the final no-op fallback;
	// it returns a structured "unconfigured" response instead of a Go error.
	m.adapters["null"] = &NullPricingAdapter{}
	m.adapters["mock"] = NewMockPricingAdapter("")
	m.adapters["static_json"] = NewStaticJSONAdapter("static_json", "pricing_matrix.json")

	// Load persisted config (may add custom_http providers).
	_ = m.LoadConfig()

	// Register any custom_http providers found in config.
	m.registerConfigAdapters()
	return m
}

// registerConfigAdapters creates adapters for all custom_http entries in the config.
// Called after LoadConfig so that user-defined providers in pricing_providers.json are honoured.
func (m *PricingOracleManager) registerConfigAdapters() {
	for name, profile := range m.config.Providers {
		if _, already := m.adapters[name]; already {
			continue // built-in adapters are never overwritten by config
		}
		switch strings.ToLower(profile.Type) {
		case "custom_http", "http":
			m.adapters[name] = NewCustomHTTPAdapterFromProfile(name, profile)
		case "static_json", "file":
			m.adapters[name] = NewStaticJSONAdapter(name, profile.FilePath)
		}
		// Unknown / legacy types (e.g. "coingecko") are intentionally skipped;
		// activating them will return a clear "no adapter registered" error.
	}
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
// This is the extension point for new providers — no changes to the manager are required.
// If an adapter with the same name already exists it is replaced.
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
// All provider-specific logic lives in the adapter; this method is provider-agnostic.
// If no adapter is registered for the active provider, a structured "unconfigured"
// JSON response is returned (not a Go error) so agents can relay a clear message.
func (m *PricingOracleManager) ExecuteQuery(ctx context.Context, asset, marketType, providerFilter string, options map[string]string) (string, error) {
	// Intercept execution_window queries to execute sliding window cost minimization
	// dynamically in Go rather than delegating to mock server endpoints.
	mt := strings.ToLower(marketType)
	if mt == "execution_window" || mt == "optimal_window" || mt == "scheduling" {
		return m.ExecuteExecutionWindowQuery(ctx, asset, providerFilter, options)
	}
	if mt == "prompt_cost" || mt == "token_cost" || mt == "llm_cost" {
		return m.ExecutePromptCostQuery(ctx, asset, providerFilter, options)
	}

	activeName := m.GetActiveProviderName()

	m.mu.RLock()
	adapter, ok := m.adapters[activeName]
	m.mu.RUnlock()

	if !ok {
		// No adapter registered — return a graceful "unconfigured" response.
		// This avoids hard task failures in environments where no pricing
		// provider has been wired up yet.
		return nullAdapterResponse(activeName, asset, marketType), nil
	}

	result, err := adapter.Query(ctx, PricingQuery{
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

// buildResult is a shared helper that constructs a PricingResult from a raw JSON string.
func buildResult(provider, asset, marketType, raw string) *PricingResult {
	var data map[string]interface{}
	_ = json.Unmarshal([]byte(raw), &data)
	return &PricingResult{
		Provider:   provider,
		Asset:      asset,
		MarketType: marketType,
		Raw:        raw,
		Data:       data,
	}
}

// NOTE: all execute*Query methods have been removed.
// Provider-specific logic now lives in the adapter implementations:
//   internal/agent/pricing_adapter_mock.go   → MockPricingAdapter
//   internal/agent/pricing_adapter_static.go → StaticJSONAdapter
//   internal/agent/pricing_adapter_http.go   → CustomHTTPAdapter

// ShouldDefer evaluates whether a task or query exceeds the configured deferral
// thresholds and should be deferred because we are currently in peak hours.
// If it should be deferred, it returns true and a descriptive reason.
func (m *PricingOracleManager) ShouldDefer(estimatedCost float64, tokenCount int, forceImmediate bool) (bool, string) {
	// 1. Force urgent environment override
	if os.Getenv("FORCE_URGENT") == "true" || os.Getenv("DEFERRAL_BYPASS") == "true" {
		return false, "global environment override (FORCE_URGENT/DEFERRAL_BYPASS)"
	}

	m.mu.RLock()
	policy := m.config.DeferralPolicy
	m.mu.RUnlock()

	// 2. Policy disabled or global config bypass
	if !policy.Enabled {
		return false, "deferral policy disabled"
	}
	if policy.BypassAll || forceImmediate {
		return false, "forced immediate or global config bypass active"
	}

	// 3. Check workload weight thresholds
	costExceeded := policy.CostThresholdUSD > 0 && estimatedCost >= policy.CostThresholdUSD
	tokensExceeded := policy.TokenThreshold > 0 && tokenCount >= policy.TokenThreshold

	if !costExceeded && !tokensExceeded {
		return false, "workload under cost and token thresholds"
	}

	// 4. Peak hours check (UTC)
	now := time.Now().UTC()
	currentHour := now.Hour()

	isPeak := false
	if policy.PeakStartHour <= policy.PeakEndHour {
		isPeak = currentHour >= policy.PeakStartHour && currentHour < policy.PeakEndHour
	} else {
		// Wraps midnight (e.g. peak is 22:00 to 06:00)
		isPeak = currentHour >= policy.PeakStartHour || currentHour < policy.PeakEndHour
	}

	if !isPeak {
		return false, fmt.Sprintf("current hour %02d:00 UTC is off-peak", currentHour)
	}

	reason := fmt.Sprintf("Peak hours active (%02d:00-%02d:00 UTC) and workload exceeds thresholds (cost: $%.2f vs $%.2f, tokens: %d vs %d)",
		policy.PeakStartHour, policy.PeakEndHour, estimatedCost, policy.CostThresholdUSD, tokenCount, policy.TokenThreshold)
	return true, reason
}

// ExecutionWindowSlot represents one candidate execution window in the 24-hour cycle.
type ExecutionWindowSlot struct {
	StartHour      int     `json:"start_hour"`         // 0-23 (UTC)
	EndHour        int     `json:"end_hour"`           // start + duration (may wrap)
	TotalCostUSD   float64 `json:"total_cost_usd"`
	AvgHourlyRate  float64 `json:"avg_hourly_rate_usd"`
	SavingsVsWorst float64 `json:"savings_vs_worst_pct"` // % cheaper than worst window
	Label          string  `json:"label"`               // e.g. "off-peak", "peak", "shoulder"
}

// ExecuteExecutionWindowQuery performs the 24-hour sliding window cost optimization in-engine.
// It queries the active provider's raw spot rates and vol surface, projects the 24-hour diurnal cycle,
// applies hedging/risk adjustments, and returns a compiled optimal window schedule.
func (m *PricingOracleManager) ExecuteExecutionWindowQuery(ctx context.Context, asset, providerFilter string, options map[string]string) (string, error) {
	activeName := m.GetActiveProviderName()

	m.mu.RLock()
	adapter, ok := m.adapters[activeName]
	configPolicy := m.config.DeferralPolicy
	m.mu.RUnlock()

	if !ok {
		return nullAdapterResponse(activeName, asset, "execution_window"), nil
	}

	// 1. Fetch raw spot price from the active provider adapter
	spotRes, err := adapter.Query(ctx, PricingQuery{
		Asset:          asset,
		MarketType:     "spot",
		ProviderFilter: providerFilter,
	})
	if err != nil {
		return "", fmt.Errorf("failed to fetch spot price for window optimization: %w", err)
	}
	baseSpot, err := extractSpotPrice(spotRes.Raw, asset)
	if err != nil {
		baseSpot = spotBaseRate(asset)
	}

	costMode := strings.ToLower(options["cost_mode"])
	if costMode != "hedged_spot" && costMode != "risk_adjusted" {
		costMode = "spot"
	}

	// 2. Fetch ATM Implied Volatility if costMode is risk_adjusted
	atmVol := 0.35
	if costMode == "risk_adjusted" {
		volRes, err := adapter.Query(ctx, PricingQuery{
			Asset:          asset,
			MarketType:     "vol_surface",
			ProviderFilter: providerFilter,
		})
		if err == nil {
			atmVol = extractATMVol(volRes.Raw)
		}
	}

	durationHours := 4
	if dStr, ok := options["duration_hours"]; ok {
		if d, err := strconv.Atoi(dStr); err == nil && d >= 1 && d <= 23 {
			durationHours = d
		}
	}

	forwardRate := 0.0
	if frStr, ok := options["forward_rate"]; ok {
		if fr, err := strconv.ParseFloat(frStr, 64); err == nil && fr > 0 {
			forwardRate = fr
		}
	}
	if costMode == "hedged_spot" && forwardRate == 0.0 {
		forwardRate = roundTo4(baseSpot * 1.05)
	}

	// 3. Load multipliers from config (falls back to defaults if not 24 items)
	spotMults := configPolicy.SpotMultipliers
	if len(spotMults) != 24 {
		spotMults = []float64{
			0.82, 0.80, 0.79, 0.79, 0.80, 0.83,
			0.90, 0.95, 1.08, 1.15, 1.18, 1.20,
			1.22, 1.22, 1.20, 1.18, 1.15, 1.10,
			1.05, 0.98, 0.95, 0.92, 0.88, 0.85,
		}
	}
	volMults := configPolicy.VolMultipliers
	if len(volMults) != 24 {
		volMults = []float64{
			0.32, 0.70, 0.75, 0.78, 0.72, 0.45,
			0.35, 0.32, 0.28, 0.25, 0.25, 0.26,
			0.27, 0.28, 0.28, 0.29, 0.30, 0.31,
			0.32, 0.33, 0.34, 0.33, 0.32, 0.31,
		}
	}

	// 4. Project spot hourly rates
	hourlyProfile := make([]float64, 24)
	for i := 0; i < 24; i++ {
		hourlyProfile[i] = roundTo4(baseSpot * spotMults[i])
	}

	// 5. Apply cost mode modifiers
	effectiveProfile := make([]float64, 24)
	for i := 0; i < 24; i++ {
		rate := hourlyProfile[i]
		switch costMode {
		case "hedged_spot":
			if rate > forwardRate {
				effectiveProfile[i] = forwardRate
			} else {
				effectiveProfile[i] = rate
			}
		case "risk_adjusted":
			iv := volMults[i] * (atmVol / 0.35)
			const kRisk = 1.0
			effectiveProfile[i] = roundTo4(rate * (1.0 + iv*kRisk))
		default:
			effectiveProfile[i] = rate
		}
	}

	// 6. Sliding window calculations
	windows := make([]ExecutionWindowSlot, 24)
	for start := 0; start < 24; start++ {
		total := 0.0
		for h := 0; h < durationHours; h++ {
			total += effectiveProfile[(start+h)%24]
		}
		avg := total / float64(durationHours)
		endHour := (start + durationHours) % 24
		windows[start] = ExecutionWindowSlot{
			StartHour:      start,
			EndHour:        endHour,
			TotalCostUSD:   roundTo4(total),
			AvgHourlyRate:  roundTo4(avg),
			Label:          labelHour(start),
		}
	}

	// Find min and max cost windows
	minCost, maxCost := windows[0].TotalCostUSD, windows[0].TotalCostUSD
	optimalIdx := 0
	for i, w := range windows {
		if w.TotalCostUSD < minCost {
			minCost = w.TotalCostUSD
			optimalIdx = i
		}
		if w.TotalCostUSD > maxCost {
			maxCost = w.TotalCostUSD
		}
	}

	// Back-fill savings percentage
	costRange := maxCost - minCost
	for i := range windows {
		if costRange > 0 {
			windows[i].SavingsVsWorst = roundTo4((maxCost - windows[i].TotalCostUSD) / maxCost * 100.0)
		}
	}

	optimal := windows[optimalIdx]
	savingVsWorstAbs := maxCost - minCost

	respData := map[string]interface{}{
		"status":                  "success",
		"asset":                   asset,
		"duration_hours":          durationHours,
		"cost_mode":               costMode,
		"currency":                "USD",
		"as_of":                   time.Now().UTC().Format(time.RFC3339),
		"algorithm":               "sliding_window_minimum_24h",
		"optimal_window":          optimal,
		"worst_window":            windows[(optimalIdx+12)%24],
		"absolute_saving_usd":     roundTo4(savingVsWorstAbs),
		"saving_pct_vs_worst":     roundTo4(optimal.SavingsVsWorst),
		"all_windows":             windows,
		"hourly_profile_usd_hr":   hourlyProfile,
	}

	switch costMode {
	case "hedged_spot":
		respData["forward_rate_usd"] = forwardRate
		respData["effective_profile_usd_hr"] = effectiveProfile
	case "risk_adjusted":
		respData["volatility_profile_pct"] = volMults
		respData["effective_profile_usd_hr"] = effectiveProfile
	}

	b, err := json.Marshal(respData)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

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
	adapter, ok := m.adapters[activeName]
	m.mu.RUnlock()

	var inputRate, outputRate, baseSpot float64 = 0.55, 0.75, 1.85
	if ok {
		res, err := adapter.Query(ctx, PricingQuery{
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

// extractSpotPrice extracts a base spot rate from various provider JSON formats.
func extractSpotPrice(rawJSON, asset string) (float64, error) {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &m); err != nil {
		return 0, err
	}
	// Case 1: {"prices": [{"gpu_type": "...", "spot_price_per_hour": 2.49}]}
	if prices, ok := m["prices"].([]interface{}); ok && len(prices) > 0 {
		for _, pItem := range prices {
			if pMap, ok := pItem.(map[string]interface{}); ok {
				if sph, ok := pMap["spot_price_per_hour"].(float64); ok {
					return sph, nil
				}
			}
		}
	}
	// Case 2: {"models": {"llama-3-70b": {"base_spot_per_hour": 1.85}}}
	if models, ok := m["models"].(map[string]interface{}); ok {
		if modelData, ok := models[asset].(map[string]interface{}); ok {
			if sph, ok := modelData["base_spot_per_hour"].(float64); ok {
				return sph, nil
			}
		}
	}
	// Case 3: {"matrix": {"llama-3-70b": 1.85}}
	if matrix, ok := m["matrix"].(map[string]interface{}); ok {
		if sph, ok := matrix[asset].(float64); ok {
			return sph, nil
		}
	}
	// Case 4: {"llama-3-70b": 1.85}
	if price, ok := m[asset].(float64); ok {
		return price, nil
	}
	// Case 5: {"price": 1.23}
	if price, ok := m["price"].(float64); ok {
		return price, nil
	}
	return 0, fmt.Errorf("could not parse spot price for %s", asset)
}

// extractATMVol extracts ATM implied volatility from vol surface outputs.
func extractATMVol(rawJSON string) float64 {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &m); err != nil {
		return 0.35
	}
	// Case 1: {"vol_surface": [{"tenor": "30d", "atm_vol": 0.385}]}
	if vs, ok := m["vol_surface"].([]interface{}); ok && len(vs) > 0 {
		for _, item := range vs {
			if vMap, ok := item.(map[string]interface{}); ok {
				if tenor, _ := vMap["tenor"].(string); tenor == "30d" || tenor == "30-day" {
					if atm, ok := vMap["atm_vol"].(float64); ok {
						return atm
					}
				}
			}
		}
		if vMap, ok := vs[0].(map[string]interface{}); ok {
			if atm, ok := vMap["atm_vol"].(float64); ok {
				return atm
			}
		}
	}
	// Case 2: {"atm_vol_30d": 0.380}
	if atm, ok := m["atm_vol_30d"].(float64); ok {
		return atm
	}
	return 0.35
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




