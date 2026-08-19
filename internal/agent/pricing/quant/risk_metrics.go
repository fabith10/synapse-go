package quant

import (
	"math"
	"math/rand"
	"sort"
	"time"
)

// ExecutionWindowRiskProfile contains institutional risk and tail-loss metrics for a compute window.
type ExecutionWindowRiskProfile struct {
	StartHour           int     `json:"start_hour"`
	DurationHours       int     `json:"duration_hours"`
	CostMode            string  `json:"cost_mode"`
	ExpectedCostUSD     float64 `json:"expected_cost_usd"`
	StdDeviationUSD     float64 `json:"std_deviation_usd"`
	VaR95USD            float64 `json:"var_95_usd"`     // 95% Value-at-Risk (Worst cost with 95% confidence)
	VaR99USD            float64 `json:"var_99_usd"`     // 99% Value-at-Risk
	CVaR95USD           float64 `json:"cvar_95_usd"`    // 95% Conditional VaR / Expected Shortfall (Average cost during 5% worst spikes)
	CVaR99USD           float64 `json:"cvar_99_usd"`    // 99% Conditional VaR
	MaxSampledCostUSD   float64 `json:"max_sampled_cost_usd"`
	EvictionRiskProbPct float64 `json:"eviction_risk_prob_pct"` // Estimated probability of eviction/spike during window
	NumSimulatedPaths   int     `json:"num_simulated_paths"`
}

// ComputeExecutionWindowRisk runs a full Monte Carlo risk simulation ($N$ paths) for a compute allocation window.
func ComputeExecutionWindowRisk(
	s0 float64,
	startHour int,
	durationHours int,
	costMode string,
	forwardCap float64,
	numPaths int,
) ExecutionWindowRiskProfile {
	if numPaths <= 0 {
		numPaths = 10000
	}
	if durationHours <= 0 {
		durationHours = 4
	}

	params := DefaultH100MRJDParams(s0)
	costs := make([]float64, numPaths)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	evictionOccurredCount := 0
	sumCost := 0.0

	for p := 0; p < numPaths; p++ {
		path := params.SimulatePath(s0, float64(24+durationHours), 1, r)

		windowCost := 0.0
		spikeInWindow := false

		for h := 0; h < durationHours; h++ {
			slot := (startHour + h) % 24
			rate := path[slot]

			// Detect if an extreme jump occurred
			if rate > s0*1.5 {
				spikeInWindow = true
			}

			// Apply cost mode
			if costMode == "hedged_spot" && forwardCap > 0 {
				rate = math.Min(rate, forwardCap)
			}

			windowCost += rate
		}

		costs[p] = windowCost
		sumCost += windowCost
		if spikeInWindow {
			evictionOccurredCount++
		}
	}

	sort.Float64s(costs)

	mean := sumCost / float64(numPaths)

	// Variance
	varSum := 0.0
	for _, c := range costs {
		diff := c - mean
		varSum += diff * diff
	}
	stdDev := math.Sqrt(varSum / float64(numPaths))

	idx95 := int(float64(numPaths) * 0.95)
	idx99 := int(float64(numPaths) * 0.99)
	if idx95 >= numPaths {
		idx95 = numPaths - 1
	}
	if idx99 >= numPaths {
		idx99 = numPaths - 1
	}

	var95 := costs[idx95]
	var99 := costs[idx99]

	// Compute CVaR (Expected Shortfall) = average of tail costs >= VaR
	cvar95Sum := 0.0
	cvar95Count := 0
	for i := idx95; i < numPaths; i++ {
		cvar95Sum += costs[i]
		cvar95Count++
	}
	cvar95 := var95
	if cvar95Count > 0 {
		cvar95 = cvar95Sum / float64(cvar95Count)
	}

	cvar99Sum := 0.0
	cvar99Count := 0
	for i := idx99; i < numPaths; i++ {
		cvar99Sum += costs[i]
		cvar99Count++
	}
	cvar99 := var99
	if cvar99Count > 0 {
		cvar99 = cvar99Sum / float64(cvar99Count)
	}

	return ExecutionWindowRiskProfile{
		StartHour:           startHour,
		DurationHours:       durationHours,
		CostMode:            costMode,
		ExpectedCostUSD:     roundTo4(mean),
		StdDeviationUSD:     roundTo4(stdDev),
		VaR95USD:            roundTo4(var95),
		VaR99USD:            roundTo4(var99),
		CVaR95USD:           roundTo4(cvar95),
		CVaR99USD:           roundTo4(cvar99),
		MaxSampledCostUSD:   roundTo4(costs[numPaths-1]),
		EvictionRiskProbPct: roundTo4((float64(evictionOccurredCount) / float64(numPaths)) * 100.0),
		NumSimulatedPaths:   numPaths,
	}
}

func roundTo4(v float64) float64 {
	return math.Round(v*10000.0) / 10000.0
}
