package backtest

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FormatMarkdownReport formats a BacktestReport into a clean, GitHub Flavored Markdown document with tables.
func FormatMarkdownReport(r *BacktestReport) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Compute Pricing Oracle Backtest Report: %s\n\n", r.Asset))
	if r.DatasetInfo != nil {
		sb.WriteString(fmt.Sprintf("**Empirical Dataset:** `%s` (%s / %s) | On-Demand Baseline: `$%.4f/hr` | Avg Spot: `$%.4f/hr`\n\n",
			r.DatasetInfo.Name, r.DatasetInfo.Provider, r.DatasetInfo.Region, r.DatasetInfo.OnDemandRate, r.DatasetInfo.AverageSpotRate))
		sb.WriteString(fmt.Sprintf("**Simulation Parameters:** %d Days (%d Total Hours) | Model: `%s` | Generated: `%s`\n\n",
			r.DurationDays, r.TotalHours, r.ModelType, r.GeneratedAt.Format("2006-01-02 15:04:05 UTC")))
	} else {
		sb.WriteString(fmt.Sprintf("**Simulation Parameters:** %d Days (%d Total Hours) | Base Spot: `$%.2f/hr` | Model: `%s` | Generated: `%s`\n\n",
			r.DurationDays, r.TotalHours, r.BaseSpotUSD, r.ModelType, r.GeneratedAt.Format("2006-01-02 15:04:05 UTC")))
	}

	sb.WriteString("## 1. Strategy Financial Performance & Tail-Risk Comparison\n\n")
	sb.WriteString("| Strategy | Total Cost ($) | Avg Cost ($/hr) | Volatility ($\\sigma$) | VaR 95 ($) | CVaR 99 ($) | Max Cost ($) | Savings ($) | Savings (%) | Hedge Eff ($R^2$) |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |\n")

	// Print strategies in consistent order: Spot, Forward Lock, Delta-Hedged Call Protection
	order := []string{"Unhedged Spot", "Forward Lock", "Delta-Hedged Call Protection"}
	for _, name := range order {
		if m, exists := r.Strategies[name]; exists {
			sb.WriteString(fmt.Sprintf("| **%s** | $%.2f | $%.4f | $%.4f | $%.4f | $%.4f | $%.4f | $%.2f | %.2f%% | %.2f%% |\n",
				m.StrategyName, m.TotalCostUSD, m.AverageHourlyCost, m.CostStdDev, m.VaR95USD, m.CVaR99USD, m.MaxHourlyCost,
				m.CostSavingsUSD, m.CostSavingsPct, m.HedgeEfficiencyR2*100.0))
		}
	}

	sb.WriteString("\n## 2. Workload Deferral & Execution Window Optimization\n\n")
	sb.WriteString("| Metric | Realized Value |\n")
	sb.WriteString("| :--- | :--- |\n")
	sb.WriteString(fmt.Sprintf("| **Total Tasks Simulated** | %d discrete compute workloads |\n", r.WorkloadSummary.TotalTasksSimulated))
	sb.WriteString(fmt.Sprintf("| **Immediate Spot Cost** | $%.2f |\n", r.WorkloadSummary.ImmediateCostUSD))
	sb.WriteString(fmt.Sprintf("| **Optimized Deferral Cost** | $%.2f |\n", r.WorkloadSummary.OptimizedCostUSD))
	sb.WriteString(fmt.Sprintf("| **Net Cost Reduction** | **$%.2f (%.2f%%)** |\n", r.WorkloadSummary.CostSavingsUSD, r.WorkloadSummary.CostSavingsPct))
	sb.WriteString(fmt.Sprintf("| **Average Deferral Window** | %.2f hours |\n", r.WorkloadSummary.AvgDeferralHours))
	sb.WriteString(fmt.Sprintf("| **SLA Deadline Compliance** | **%d Breaches (%.2f%% SLA Failure)** |\n", r.WorkloadSummary.SLAViolationsCount, r.WorkloadSummary.SLAViolationRatePct))

	sb.WriteString("\n## 3. Kupiec Value-at-Risk (VaR) Calibration Coverage Test\n\n")
	sb.WriteString("| VaR Confidence | Target Failure % | Observed Failures | Observed Failure % | Likelihood Ratio ($LR$) | Critical $\\chi^2(1)$ | Test Result |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- | :--- |\n")
	sb.WriteString(fmt.Sprintf("| **95%% VaR** | %.1f%% | %d / %d | %.2f%% | %.4f | %.3f | **%s** |\n",
		r.Kupiec95.ExpectedExceedPct, r.Kupiec95.ObservedFailures, r.Kupiec95.SampleSize, r.Kupiec95.ObservedFailurePct,
		r.Kupiec95.LikelihoodRatio, r.Kupiec95.CriticalValueChi2, passFailStatus(r.Kupiec95.Passed)))
	sb.WriteString(fmt.Sprintf("| **99%% VaR** | %.1f%% | %d / %d | %.2f%% | %.4f | %.3f | **%s** |\n",
		r.Kupiec99.ExpectedExceedPct, r.Kupiec99.ObservedFailures, r.Kupiec99.SampleSize, r.Kupiec99.ObservedFailurePct,
		r.Kupiec99.LikelihoodRatio, r.Kupiec99.CriticalValueChi2, passFailStatus(r.Kupiec99.Passed)))

	sb.WriteString("\n## 4. Quantitative Summary\n\n")
	sb.WriteString(r.SummaryNarrative + "\n")

	return sb.String()
}

// ToJSON serializes the BacktestReport into formatted JSON.
func (r *BacktestReport) ToJSON() (string, error) {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func passFailStatus(passed bool) string {
	if passed {
		return "PASSED (Well-Calibrated)"
	}
	return "REJECTED (Miscalibrated)"
}
