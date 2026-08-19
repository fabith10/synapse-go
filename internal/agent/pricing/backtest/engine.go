package backtest

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/fabith10/synapse-go/internal/agent/pricing/quant"
)

// BacktestConfig specifies the parameters for a simulation or backtest run.
type BacktestConfig struct {
	Asset                  string    `json:"asset"`                     // e.g. "H100_SXM", "A100_80GB", "A10G_24GB"
	DatasetName            string    `json:"dataset_name,omitempty"`     // e.g. "aws_g5_xlarge", "vastai_h100_sxm", "aws_g4dn_xlarge"
	DatasetPath            string    `json:"dataset_path,omitempty"`     // Custom CSV path
	BaseSpotRateUSD        float64   `json:"base_spot_rate_usd"`        // e.g. 2.49
	SimulationDurationDays int       `json:"simulation_duration_days"`  // e.g. 30, 90
	NumMonteCarloPaths     int       `json:"num_monte_carlo_paths"`     // e.g. 1000
	ModelType              string    `json:"model_type"`                // "ou_jump_diffusion", "markov_regime_switching", "empirical_historical"
	NumWorkloadTasks       int       `json:"num_workload_tasks"`        // Number of discrete batch tasks to schedule
	StrikeRateUSD          float64   `json:"strike_rate_usd"`           // Option strike price ceiling (0 = auto 5% OTM)
	RandomSeed             int64     `json:"random_seed"`               // Seed for reproducibility (0 = time-based)
	HistoricalPricePath    []float64 `json:"historical_price_path,omitempty"`
}

// BacktestReport aggregates all quantitative performance and risk metrics across strategies.
type BacktestReport struct {
	Asset             string                        `json:"asset"`
	BaseSpotUSD       float64                       `json:"base_spot_usd"`
	DurationDays      int                           `json:"duration_days"`
	TotalHours        int                           `json:"total_hours"`
	ModelType         string                        `json:"model_type"`
	DatasetInfo       *DatasetInfo                  `json:"dataset_info,omitempty"`
	NumSimulatedPaths int                           `json:"num_simulated_paths"`
	GeneratedAt       time.Time                     `json:"generated_at"`
	Strategies        map[string]PerformanceMetrics `json:"strategies"`
	Kupiec95          KupiecTestResult              `json:"kupiec_95_test"`
	Kupiec99          KupiecTestResult              `json:"kupiec_99_test"`
	WorkloadSummary   WorkloadBacktestSummary       `json:"workload_summary"`
	SummaryNarrative  string                        `json:"summary_narrative"`
}

// WorkloadBacktestSummary summarizes the deferral optimization vs naive immediate spot execution.
type WorkloadBacktestSummary struct {
	TotalTasksSimulated int     `json:"total_tasks_simulated"`
	ImmediateCostUSD    float64 `json:"immediate_cost_usd"`
	OptimizedCostUSD    float64 `json:"optimized_cost_usd"`
	CostSavingsUSD      float64 `json:"cost_savings_usd"`
	CostSavingsPct      float64 `json:"cost_savings_pct"`
	AvgDeferralHours    float64 `json:"avg_deferral_hours"`
	SLAViolationsCount  int     `json:"sla_violations_count"`
	SLAViolationRatePct float64 `json:"sla_violation_rate_pct"`
}

// BacktestEngine runs simulation and evaluation pipelines for compute derivative models.
type BacktestEngine struct{}

// NewBacktestEngine initializes a backtest engine.
func NewBacktestEngine() *BacktestEngine {
	return &BacktestEngine{}
}

// RunBacktest executes a full multi-strategy backtest across the specified configuration.
func (e *BacktestEngine) RunBacktest(cfg BacktestConfig) (*BacktestReport, error) {
	var dsInfo *DatasetInfo
	if cfg.DatasetName != "" || cfg.DatasetPath != "" {
		target := cfg.DatasetName
		if target == "" {
			target = cfg.DatasetPath
		}
		prices, info, err := LoadHistoricalDataset(target)
		if err != nil {
			return nil, err
		}
		dsInfo = info
		cfg.HistoricalPricePath = prices
		cfg.ModelType = fmt.Sprintf("empirical_historical (%s)", info.ID)
		if info.Asset != "" && (cfg.Asset == "" || cfg.Asset == "H100_SXM") {
			cfg.Asset = info.Asset
		}
		if info.AverageSpotRate > 0 && (cfg.BaseSpotRateUSD <= 0 || cfg.BaseSpotRateUSD == 2.49) {
			cfg.BaseSpotRateUSD = info.AverageSpotRate
		}
	}

	if cfg.Asset == "" {
		cfg.Asset = "H100_SXM"
	}
	if cfg.BaseSpotRateUSD <= 0 {
		cfg.BaseSpotRateUSD = 2.49
	}
	if cfg.SimulationDurationDays <= 0 {
		cfg.SimulationDurationDays = 30
	}
	if cfg.NumMonteCarloPaths <= 0 {
		cfg.NumMonteCarloPaths = 1000
	}
	if cfg.NumWorkloadTasks <= 0 {
		cfg.NumWorkloadTasks = 200
	}
	if cfg.ModelType == "" {
		cfg.ModelType = "ou_jump_diffusion"
	}

	totalHours := cfg.SimulationDurationDays * 24

	// 1. Generate or load the primary price trajectory
	var pricePath []float64
	var r *rand.Rand
	if cfg.RandomSeed != 0 {
		r = rand.New(rand.NewSource(cfg.RandomSeed))
	} else {
		r = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	if len(cfg.HistoricalPricePath) >= totalHours {
		pricePath = cfg.HistoricalPricePath[:totalHours]
	} else if len(cfg.HistoricalPricePath) > 0 {
		pricePath = cfg.HistoricalPricePath
		totalHours = len(pricePath)
		cfg.SimulationDurationDays = int(math.Ceil(float64(totalHours) / 24.0))
	} else {
		switch cfg.ModelType {
		case "markov_regime_switching":
			mrsParams := quant.DefaultH100MRSParams(cfg.BaseSpotRateUSD)
			path, _ := mrsParams.SimulateMRSPath(cfg.BaseSpotRateUSD, float64(totalHours), 1, quant.RegimeNormal, r)
			pricePath = path[:totalHours]
		default: // ou_jump_diffusion
			mrjdParams := quant.DefaultH100MRJDParams(cfg.BaseSpotRateUSD)
			path := mrjdParams.SimulatePath(cfg.BaseSpotRateUSD, float64(totalHours), 1, r)
			pricePath = path[:totalHours]
		}
	}

	// 2. Evaluate Strategies
	strategies := make(map[string]PerformanceMetrics)

	// Baseline: Unhedged Spot
	spotStrat := &SpotStrategy{}
	spotCosts := spotStrat.Execute(pricePath)
	spotMetrics := ComputeSeriesMetrics(spotStrat.Name(), spotCosts, spotCosts)
	strategies[spotStrat.Name()] = spotMetrics

	// Forward Lock
	fwdStrat := &ForwardStrategy{LockedRateUSD: cfg.BaseSpotRateUSD * 1.025}
	fwdCosts := fwdStrat.Execute(pricePath)
	fwdMetrics := ComputeSeriesMetrics(fwdStrat.Name(), fwdCosts, spotCosts)
	strategies[fwdStrat.Name()] = fwdMetrics

	// Delta-Hedged Call Protection
	callStrat := &CallHedgeStrategy{
		StrikeRateUSD: cfg.StrikeRateUSD,
		Volatility:    0.35,
		RiskFreeRate:  0.045,
		ExpiryYears:   float64(totalHours) / 8760.0,
	}
	callCosts := callStrat.Execute(pricePath)
	callMetrics := ComputeSeriesMetrics(callStrat.Name(), callCosts, spotCosts)
	strategies[callStrat.Name()] = callMetrics

	// 3. Evaluate Workload Deferral Strategy
	tasks := generateSyntheticWorkloads(cfg.NumWorkloadTasks, totalHours, r)
	deferralStrat := &DeferralStrategy{}
	evaluatedTasks, immTotal, optTotal := deferralStrat.ScheduleWorkloads(tasks, pricePath)

	savingsUSD := immTotal - optTotal
	savingsPct := 0.0
	if immTotal > 0 {
		savingsPct = (savingsUSD / immTotal) * 100.0
	}

	totalDeferralHours := 0
	slaViolations := 0
	for _, t := range evaluatedTasks {
		totalDeferralHours += t.DeferredHours
		if t.SLABreached {
			slaViolations++
		}
	}

	avgDeferral := float64(totalDeferralHours) / float64(len(tasks))
	violationRate := (float64(slaViolations) / float64(len(tasks))) * 100.0

	workloadSummary := WorkloadBacktestSummary{
		TotalTasksSimulated: len(tasks),
		ImmediateCostUSD:    round(immTotal, 2),
		OptimizedCostUSD:    round(optTotal, 2),
		CostSavingsUSD:      round(savingsUSD, 2),
		CostSavingsPct:      round(savingsPct, 2),
		AvgDeferralHours:    round(avgDeferral, 2),
		SLAViolationsCount:  slaViolations,
		SLAViolationRatePct: round(violationRate, 2),
	}

	// 4. Kupiec POF Tail-Risk Coverage Tests on Spot VaR
	kupiec95 := RunKupiecPOFTest(spotCosts, spotMetrics.VaR95USD, 0.95)
	kupiec99 := RunKupiecPOFTest(spotCosts, spotMetrics.VaR99USD, 0.99)

	// 5. Generate narrative
	narrative := fmt.Sprintf(
		"Backtest over %d days (%d hours) of %s compute:\n"+
			"• Call-option delta hedging provided $%.2f savings with an empirical hedge efficiency R² of %.2f%% and reduced CVaR99 from $%.4f/hr to $%.4f/hr.\n"+
			"• Workload deferral optimization yielded %.2f%% net cost savings ($%.2f savings across %d tasks) with %d SLA breaches.\n"+
			"• Kupiec VaR95 coverage test: %s (Likelihood Ratio: %.4f vs critical 3.841).",
		cfg.SimulationDurationDays, totalHours, cfg.Asset,
		callMetrics.CostSavingsUSD, callMetrics.HedgeEfficiencyR2*100.0, spotMetrics.CVaR99USD, callMetrics.CVaR99USD,
		workloadSummary.CostSavingsPct, workloadSummary.CostSavingsUSD, workloadSummary.TotalTasksSimulated, workloadSummary.SLAViolationsCount,
		passFail(kupiec95.Passed), kupiec95.LikelihoodRatio,
	)

	return &BacktestReport{
		Asset:             cfg.Asset,
		BaseSpotUSD:       cfg.BaseSpotRateUSD,
		DurationDays:      cfg.SimulationDurationDays,
		TotalHours:        totalHours,
		ModelType:         cfg.ModelType,
		DatasetInfo:       dsInfo,
		NumSimulatedPaths: cfg.NumMonteCarloPaths,
		GeneratedAt:       time.Now().UTC(),
		Strategies:        strategies,
		Kupiec95:          kupiec95,
		Kupiec99:          kupiec99,
		WorkloadSummary:   workloadSummary,
		SummaryNarrative:  narrative,
	}, nil
}

func generateSyntheticWorkloads(numTasks int, totalHours int, r *rand.Rand) []TaskWorkload {
	tasks := make([]TaskWorkload, numTasks)
	for i := 0; i < numTasks; i++ {
		arr := r.Intn(totalHours)
		dur := 1 + r.Intn(4)     // 1 to 4 hours duration
		sla := dur + 2 + r.Intn(8) // 2 to 9 hours flexibility buffer
		tasks[i] = TaskWorkload{
			TaskID:        i + 1,
			ArrivalHour:   arr,
			DurationHours: dur,
			DeadlineHours: sla,
		}
	}
	return tasks
}

func passFail(b bool) string {
	if b {
		return "PASSED"
	}
	return "FAILED"
}
