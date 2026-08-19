package quant

import (
	"math"
)

// RealOptionsValuation contains the economic valuation of algorithmic flexibilities.
type RealOptionsValuation struct {
	Asset                 string  `json:"asset"`
	ImmediateCostUSD      float64 `json:"immediate_cost_usd"`
	OptimalDeferredCostUSD float64 `json:"optimal_deferred_cost_usd"`
	OptionToDeferValueUSD float64 `json:"option_to_defer_value_usd"` // Net economic value of waiting for off-peak slot
	OptionToSwitchValueUSD float64 `json:"option_to_switch_value_usd"` // Value of multi-tier fallback flexibility
	Recommendation        string  `json:"recommendation"`
}

// EvaluateRealOptions computes the real options value of deferral and compute switching.
func EvaluateRealOptions(
	asset string,
	immediateSpotRate float64,
	offPeakSpotRate float64,
	durationHours int,
	impliedVol float64,
	onDemandRate float64,
) RealOptionsValuation {
	dur := float64(durationHours)
	if dur <= 0 {
		dur = 4.0
	}
	if impliedVol <= 0 {
		impliedVol = 0.35
	}
	if onDemandRate <= 0 {
		onDemandRate = immediateSpotRate * 1.55
	}

	immediateTotalCost := immediateSpotRate * dur
	deferredTotalCost := offPeakSpotRate * dur

	// Option to Defer: Value of waiting = Direct Cost Savings + Volatility Option Value (Black-76 style)
	directSavings := math.Max(0, immediateTotalCost-deferredTotalCost)
	// Time value of flexibility under volatility: 0.5 * sigma * sqrt(T_wait) * Strike
	timeValueFlexibility := 0.25 * impliedVol * math.Sqrt(12.0/24.0) * deferredTotalCost
	optionToDeferValue := directSavings + timeValueFlexibility

	// Option to Switch: Value of having local/spot/on-demand triage vs pure fixed on-demand
	// Spread between on-demand baseline and spot expectation
	switchSpread := math.Max(0, onDemandRate*dur-immediateTotalCost)
	optionToSwitchValue := switchSpread * (1.0 - 0.15) // less 15% frictional switching penalty

	rec := "EXECUTE_DEFERRED"
	if optionToDeferValue < 0.10 {
		rec = "EXECUTE_IMMEDIATE"
	}

	return RealOptionsValuation{
		Asset:                  asset,
		ImmediateCostUSD:       roundTo4(immediateTotalCost),
		OptimalDeferredCostUSD: roundTo4(deferredTotalCost),
		OptionToDeferValueUSD:  roundTo4(optionToDeferValue),
		OptionToSwitchValueUSD: roundTo4(optionToSwitchValue),
		Recommendation:         rec,
	}
}
