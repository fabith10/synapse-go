package backtest

import (
	"math"
	"sort"
)

// PerformanceMetrics captures statistical, financial, and tail-risk performance.
type PerformanceMetrics struct {
	StrategyName       string  `json:"strategy_name"`
	TotalCostUSD       float64 `json:"total_cost_usd"`
	AverageHourlyCost  float64 `json:"average_hourly_cost_usd"`
	CostStdDev         float64 `json:"cost_std_dev_usd"`
	MinHourlyCost      float64 `json:"min_hourly_cost_usd"`
	MaxHourlyCost      float64 `json:"max_hourly_cost_usd"`
	MaxDrawdownUSD     float64 `json:"max_drawdown_usd"`
	VaR95USD           float64 `json:"var_95_usd"`
	VaR99USD           float64 `json:"var_99_usd"`
	CVaR95USD          float64 `json:"cvar_95_usd"`
	CVaR99USD          float64 `json:"cvar_99_usd"`
	CostSavingsUSD     float64 `json:"cost_savings_usd"`
	CostSavingsPct     float64 `json:"cost_savings_pct"`
	HedgeEfficiencyR2  float64 `json:"hedge_efficiency_r2"` // 1 - Var(Strat)/Var(Spot)
	TotalHours         int     `json:"total_hours"`
	SpikesEncountered  int     `json:"spikes_encountered"`
	SLAViolationsCount int     `json:"sla_violations_count"`
	SLAViolationRatePct float64 `json:"sla_violation_rate_pct"`
}

// KupiecTestResult represents the result of the Kupiec POF coverage test.
type KupiecTestResult struct {
	ConfidenceLevel   float64 `json:"confidence_level"`     // e.g. 0.95 or 0.99
	ExpectedExceedPct float64 `json:"expected_exceed_pct"`  // 5% or 1%
	SampleSize        int     `json:"sample_size"`
	ObservedFailures  int     `json:"observed_failures"`
	ObservedFailurePct float64 `json:"observed_failure_pct"`
	LikelihoodRatio   float64 `json:"likelihood_ratio"`
	CriticalValueChi2 float64 `json:"critical_value_chi2"` // 3.841 for 95% confidence chi-squared df=1
	Passed            bool    `json:"passed"`
}

// ComputeSeriesMetrics calculates the complete quantitative metrics for an hourly cost series.
func ComputeSeriesMetrics(name string, costs []float64, baselineCosts []float64) PerformanceMetrics {
	n := len(costs)
	if n == 0 {
		return PerformanceMetrics{StrategyName: name}
	}

	sum := 0.0
	minCost := costs[0]
	maxCost := costs[0]
	sortedCosts := make([]float64, n)

	for i, c := range costs {
		sum += c
		if c < minCost {
			minCost = c
		}
		if c > maxCost {
			maxCost = c
		}
		sortedCosts[i] = c
	}

	mean := sum / float64(n)

	// Variance and StdDev
	varSum := 0.0
	for _, c := range costs {
		diff := c - mean
		varSum += diff * diff
	}
	variance := varSum / float64(n)
	stdDev := math.Sqrt(variance)

	// Max Drawdown (cumulative cost peak vs trough)
	peak := 0.0
	maxDrawdown := 0.0
	cum := 0.0
	for _, c := range costs {
		cum += c
		if cum > peak {
			peak = cum
		}
		dd := peak - cum
		if dd > maxDrawdown {
			maxDrawdown = dd
		}
	}

	// VaR & CVaR (Tail Risk)
	sort.Float64s(sortedCosts)
	idx95 := int(float64(n) * 0.95)
	idx99 := int(float64(n) * 0.99)
	if idx95 >= n {
		idx95 = n - 1
	}
	if idx99 >= n {
		idx99 = n - 1
	}

	var95 := sortedCosts[idx95]
	var99 := sortedCosts[idx99]

	cvar95Sum := 0.0
	cvar95Count := 0
	for i := idx95; i < n; i++ {
		cvar95Sum += sortedCosts[i]
		cvar95Count++
	}
	cvar95 := var95
	if cvar95Count > 0 {
		cvar95 = cvar95Sum / float64(cvar95Count)
	}

	cvar99Sum := 0.0
	cvar99Count := 0
	for i := idx99; i < n; i++ {
		cvar99Sum += sortedCosts[i]
		cvar99Count++
	}
	cvar99 := var99
	if cvar99Count > 0 {
		cvar99 = cvar99Sum / float64(cvar99Count)
	}

	// Comparison against baseline
	savingsUSD := 0.0
	savingsPct := 0.0
	hedgeEffR2 := 0.0

	if len(baselineCosts) == n {
		baselineSum := 0.0
		for _, b := range baselineCosts {
			baselineSum += b
		}
		savingsUSD = baselineSum - sum
		if baselineSum > 0 {
			savingsPct = (savingsUSD / baselineSum) * 100.0
		}

		// Baseline variance
		baseMean := baselineSum / float64(n)
		baseVarSum := 0.0
		for _, b := range baselineCosts {
			d := b - baseMean
			baseVarSum += d * d
		}
		baseVariance := baseVarSum / float64(n)
		if baseVariance > 0 {
			hedgeEffR2 = math.Max(0, 1.0-(variance/baseVariance))
		}
	}

	return PerformanceMetrics{
		StrategyName:      name,
		TotalCostUSD:      round(sum, 2),
		AverageHourlyCost: round(mean, 4),
		CostStdDev:        round(stdDev, 4),
		MinHourlyCost:     round(minCost, 4),
		MaxHourlyCost:     round(maxCost, 4),
		MaxDrawdownUSD:    round(maxDrawdown, 2),
		VaR95USD:          round(var95, 4),
		VaR99USD:          round(var99, 4),
		CVaR95USD:         round(cvar95, 4),
		CVaR99USD:         round(cvar99, 4),
		CostSavingsUSD:    round(savingsUSD, 2),
		CostSavingsPct:    round(savingsPct, 2),
		HedgeEfficiencyR2: round(hedgeEffR2, 4),
		TotalHours:        n,
	}
}

// RunKupiecPOFTest executes the Kupiec Proportion-of-Failures Likelihood Ratio test.
// Null hypothesis H0: The true failure rate equals alpha (1 - confidenceLevel).
func RunKupiecPOFTest(actualCosts []float64, varThreshold float64, confidenceLevel float64) KupiecTestResult {
	n := len(actualCosts)
	if n == 0 {
		return KupiecTestResult{ConfidenceLevel: confidenceLevel, Passed: false}
	}

	alpha := 1.0 - confidenceLevel
	expectedExceedPct := alpha * 100.0
	failures := 0

	for _, c := range actualCosts {
		if c > varThreshold {
			failures++
		}
	}

	observedFailurePct := (float64(failures) / float64(n)) * 100.0
	p := alpha
	x := float64(failures)
	N := float64(n)

	// Critical value for Chi-Square distribution with 1 degree of freedom at 95% confidence level is 3.841
	criticalVal := 3.841
	lr := 0.0

	if x == 0 {
		// LR = -2 * ln((1-p)^N)
		lr = -2.0 * N * math.Log(1.0-p)
	} else if x == N {
		lr = -2.0 * N * math.Log(p)
	} else {
		piHat := x / N
		term1 := (N - x) * math.Log((1.0-p)/(1.0-piHat))
		term2 := x * math.Log(p/piHat)
		lr = -2.0 * (term1 + term2)
	}

	if math.IsNaN(lr) || lr < 0 {
		lr = 0.0
	}

	passed := lr <= criticalVal

	return KupiecTestResult{
		ConfidenceLevel:    confidenceLevel,
		ExpectedExceedPct:  round(expectedExceedPct, 2),
		SampleSize:         n,
		ObservedFailures:   failures,
		ObservedFailurePct: round(observedFailurePct, 2),
		LikelihoodRatio:    round(lr, 4),
		CriticalValueChi2:  criticalVal,
		Passed:             passed,
	}
}

func round(val float64, decimals int) float64 {
	pow := math.Pow(10, float64(decimals))
	return math.Round(val*pow) / pow
}
