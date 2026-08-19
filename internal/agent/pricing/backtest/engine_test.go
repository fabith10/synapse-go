package backtest_test

import (
	"strings"
	"testing"

	"github.com/fabith10/synapse-go/internal/agent/pricing/backtest"
)

func TestBacktestEngine_OUJumpDiffusion(t *testing.T) {
	engine := backtest.NewBacktestEngine()
	cfg := backtest.BacktestConfig{
		Asset:                  "H100_SXM",
		BaseSpotRateUSD:        2.49,
		SimulationDurationDays: 30,
		NumMonteCarloPaths:     500,
		ModelType:              "ou_jump_diffusion",
		NumWorkloadTasks:       100,
		RandomSeed:             42,
	}

	report, err := engine.RunBacktest(cfg)
	if err != nil {
		t.Fatalf("failed to run backtest: %v", err)
	}

	if report.Asset != "H100_SXM" {
		t.Errorf("expected asset H100_SXM, got %s", report.Asset)
	}
	if report.TotalHours != 720 {
		t.Errorf("expected 720 total hours (30d * 24h), got %d", report.TotalHours)
	}

	// 1. Check Strategies
	spot, hasSpot := report.Strategies["Unhedged Spot"]
	if !hasSpot || spot.TotalCostUSD <= 0 {
		t.Errorf("expected valid spot metrics, got %+v", spot)
	}

	call, hasCall := report.Strategies["Delta-Hedged Call Protection"]
	if !hasCall || call.TotalCostUSD <= 0 {
		t.Errorf("expected valid call hedge metrics, got %+v", call)
	}

	// Delta hedging must reduce tail risk (CVaR99 should be less than or equal to unhedged spot max tail)
	if call.CVaR99USD > spot.MaxHourlyCost {
		t.Errorf("expected delta hedging to protect extreme tail, got call CVaR99=%f vs spot Max=%f", call.CVaR99USD, spot.MaxHourlyCost)
	}

	// 2. Check Workload Deferral
	if report.WorkloadSummary.CostSavingsPct <= 0 {
		t.Errorf("expected positive cost savings from workload deferral, got %.2f%%", report.WorkloadSummary.CostSavingsPct)
	}
	if report.WorkloadSummary.SLAViolationsCount != 0 {
		t.Errorf("expected 0 SLA violations, got %d", report.WorkloadSummary.SLAViolationsCount)
	}

	// 3. Check Kupiec POF Test
	if report.Kupiec95.SampleSize != 720 {
		t.Errorf("expected sample size 720, got %d", report.Kupiec95.SampleSize)
	}
	if !report.Kupiec95.Passed {
		t.Logf("Kupiec 95%% test LR: %f (critical %f)", report.Kupiec95.LikelihoodRatio, report.Kupiec95.CriticalValueChi2)
	}

	// 4. Check Markdown formatting
	md := backtest.FormatMarkdownReport(report)
	if !strings.Contains(md, "# Compute Pricing Oracle Backtest Report: H100_SXM") {
		t.Errorf("markdown report missing header")
	}
	if !strings.Contains(md, "Unhedged Spot") {
		t.Errorf("markdown report missing Unhedged Spot table row")
	}
}

func TestBacktestEngine_MarkovRegimeSwitching(t *testing.T) {
	engine := backtest.NewBacktestEngine()
	cfg := backtest.BacktestConfig{
		Asset:                  "A100_80GB",
		BaseSpotRateUSD:        1.29,
		SimulationDurationDays: 14,
		ModelType:              "markov_regime_switching",
		NumWorkloadTasks:       50,
		RandomSeed:             123,
	}

	report, err := engine.RunBacktest(cfg)
	if err != nil {
		t.Fatalf("failed to run MRS backtest: %v", err)
	}

	if report.TotalHours != 336 {
		t.Errorf("expected 336 hours (14d * 24h), got %d", report.TotalHours)
	}
	if len(report.Strategies) < 3 {
		t.Errorf("expected at least 3 strategies evaluated, got %d", len(report.Strategies))
	}
}

func TestBacktestEngine_HistoricalReplay(t *testing.T) {
	engine := backtest.NewBacktestEngine()

	// Deterministic 48-hour price path with a spike
	customPath := make([]float64, 48)
	for i := 0; i < 48; i++ {
		customPath[i] = 2.50
	}
	// Spike at hour 12-14
	customPath[12] = 5.00
	customPath[13] = 6.00
	customPath[14] = 4.50

	cfg := backtest.BacktestConfig{
		Asset:               "Custom_GPU",
		BaseSpotRateUSD:     2.50,
		HistoricalPricePath: customPath,
		NumWorkloadTasks:    20,
		RandomSeed:          99,
	}

	report, err := engine.RunBacktest(cfg)
	if err != nil {
		t.Fatalf("failed to run historical replay backtest: %v", err)
	}

	if report.TotalHours != 48 {
		t.Errorf("expected 48 hours, got %d", report.TotalHours)
	}

	spot := report.Strategies["Unhedged Spot"]
	if spot.MaxHourlyCost != 6.00 {
		t.Errorf("expected spot max hourly cost to be 6.00, got %f", spot.MaxHourlyCost)
	}

	jsonStr, err := report.ToJSON()
	if err != nil || len(jsonStr) == 0 {
		t.Fatalf("failed to serialize backtest report to JSON: %v", err)
	}
}

func TestKupiecPOFTest_Accuracy(t *testing.T) {
	// Sample of 1000 observations where exactly 50 exceed the 95% VaR threshold
	costs := make([]float64, 1000)
	for i := 0; i < 950; i++ {
		costs[i] = 2.0
	}
	for i := 950; i < 1000; i++ {
		costs[i] = 10.0
	}

	res := backtest.RunKupiecPOFTest(costs, 5.0, 0.95)
	if !res.Passed {
		t.Errorf("expected exact 5%% failure rate to pass Kupiec test, got LR=%f", res.LikelihoodRatio)
	}
	if res.ObservedFailures != 50 {
		t.Errorf("expected 50 observed failures, got %d", res.ObservedFailures)
	}
}
