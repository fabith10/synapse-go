package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

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
	prov, ok := m.adapters[activeName]
	configPolicy := m.config.DeferralPolicy
	m.mu.RUnlock()

	if !ok {
		return nullAdapterResponse(activeName, asset, "execution_window"), nil
	}

	// 1. Fetch raw spot price from the active provider adapter
	spotRes, err := prov.Query(ctx, PricingQuery{
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
	atmVol := 0.0
	if costMode == "risk_adjusted" {
		volRes, err := prov.Query(ctx, PricingQuery{
			Asset:          asset,
			MarketType:     "vol_surface",
			ProviderFilter: providerFilter,
		})
		if err == nil {
			atmVol = extractATMVol(volRes.Raw)
		}
		// If vol_surface returned 0 or failed, fallback to querying options market endpoint
		if atmVol <= 0 {
			optRes, err := prov.Query(ctx, PricingQuery{
				Asset:          asset,
				MarketType:     "options",
				ProviderFilter: providerFilter,
			})
			if err == nil {
				atmVol = extractIVFromOptions(optRes.Raw, baseSpot)
			}
		}
		// Fallback to default IV baseline if options query also yielded no IV
		if atmVol <= 0 {
			atmVol = 0.35
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
