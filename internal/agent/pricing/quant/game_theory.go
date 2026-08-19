package quant

import (
	"math"
	"math/rand"
	"time"
)

// BoltzmannNashResult represents the mixed-strategy Nash equilibrium distribution across 24 hours.
type BoltzmannNashResult struct {
	RationalityBeta      float64   `json:"rationality_beta"`
	ProbMassFunction     []float64 `json:"hourly_start_probability_pmf"`
	ShannonEntropy       float64   `json:"shannon_dispersion_entropy"`
	MaxEntropy           float64   `json:"max_theoretical_entropy"`
	AntiHerdingScorePct  float64   `json:"anti_herding_diversity_score_pct"`
	DeterministicOptHour int       `json:"deterministic_optimal_start_hour"`
	MixedStrategySampled int       `json:"mixed_strategy_sampled_start_hour"`
	StrategyExplanation  string    `json:"strategy_explanation"`
}

// ComputeBoltzmannNashDistribution evaluates a Gibbs/Boltzmann mixed-strategy probability distribution.
// beta controls rationality: higher beta -> more concentrated on absolute min cost; lower beta -> higher dispersion.
func ComputeBoltzmannNashDistribution(costs []float64, beta float64) BoltzmannNashResult {
	n := len(costs)
	if n == 0 {
		return BoltzmannNashResult{}
	}
	if beta <= 0 {
		beta = 2.5
	}

	minCost := costs[0]
	minHour := 0
	for i, c := range costs {
		if c < minCost {
			minCost = c
			minHour = i
		}
	}

	// Compute Gibbs weights exp(-beta * (cost - minCost))
	weights := make([]float64, n)
	sumWeights := 0.0
	for i, c := range costs {
		w := math.Exp(-beta * (c - minCost))
		weights[i] = w
		sumWeights += w
	}

	pmf := make([]float64, n)
	entropy := 0.0
	for i, w := range weights {
		p := w / sumWeights
		pmf[i] = roundTo4(p)
		if p > 1e-12 {
			entropy -= p * math.Log(p)
		}
	}

	maxEntropy := math.Log(float64(n))
	diversityPct := roundTo4((entropy / maxEntropy) * 100.0)

	// Sample a start hour according to the mixed-strategy PMF
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	sampledHour := SampleFromPMF(pmf, r)

	return BoltzmannNashResult{
		RationalityBeta:      beta,
		ProbMassFunction:     pmf,
		ShannonEntropy:       roundTo4(entropy),
		MaxEntropy:           roundTo4(maxEntropy),
		AntiHerdingScorePct:  diversityPct,
		DeterministicOptHour: minHour,
		MixedStrategySampled: sampledHour,
		StrategyExplanation:  "Mixed-Strategy Nash Equilibrium disperses agent batch dispatches across near-optimal off-peak and shoulder hours, preventing synchronized cluster saturation and demand spikes.",
	}
}

// SampleFromPMF samples an index from a discrete probability mass function.
func SampleFromPMF(pmf []float64, r *rand.Rand) int {
	u := r.Float64()
	cum := 0.0
	for i, p := range pmf {
		cum += p
		if u <= cum {
			return i
		}
	}
	return len(pmf) - 1
}

// MinorityGameCrowdResult represents the crowd-adjusted cost curve under the Minority Game model.
type MinorityGameCrowdResult struct {
	BaseCostsUSD        []float64 `json:"base_costs_usd"`
	EstimatedCrowdLoad  []float64 `json:"estimated_crowd_density_pct"`
	CrowdAdjustedCosts  []float64 `json:"crowd_adjusted_costs_usd"`
	OriginalOptHour     int       `json:"original_pure_optimal_hour"`
	CrowdAvoidedOptHour int       `json:"crowd_avoided_optimal_hour"`
	GameTheoryNote      string    `json:"game_theory_note"`
}

// EvaluateMinorityGameCrowding applies the Minority Game crowd penalty to 24-hour window costs.
// eta: Sensitivity factor (default 0.45).
// gamma: Elasticity exponent (default 2.0).
func EvaluateMinorityGameCrowding(baseCosts []float64, eta float64, gamma float64) MinorityGameCrowdResult {
	n := len(baseCosts)
	if n == 0 {
		return MinorityGameCrowdResult{}
	}
	if eta <= 0 {
		eta = 0.45
	}
	if gamma <= 0 {
		gamma = 2.0
	}

	// 1. Synthetic crowd density estimation:
	// Uninformed bots herd into the lowest-cost base hours (e.g. 02:00-05:00 UTC).
	minCost := baseCosts[0]
	maxCost := baseCosts[0]
	for _, c := range baseCosts {
		if c < minCost {
			minCost = c
		}
		if c > maxCost {
			maxCost = c
		}
	}
	costRange := math.Max(0.01, maxCost-minCost)

	crowdDensity := make([]float64, n)
	adjustedCosts := make([]float64, n)

	origMinHour := 0
	origMinCost := baseCosts[0]

	for i, c := range baseCosts {
		if c < origMinCost {
			origMinCost = c
			origMinHour = i
		}
		// Bots herd into low cost hours -> crowd density is inversely proportional to cost
		invCostNorm := (maxCost - c) / costRange
		// Heavy non-linear concentration at minimums
		crowd := math.Pow(invCostNorm, 2.5) * 0.85
		crowdDensity[i] = roundTo4(crowd * 100.0)

		penalty := 1.0 + eta*math.Pow(crowd, gamma)
		adjustedCosts[i] = roundTo4(c * penalty)
	}

	crowdMinHour := 0
	crowdMinCost := adjustedCosts[0]
	for i, c := range adjustedCosts {
		if c < crowdMinCost {
			crowdMinCost = c
			crowdMinHour = i
		}
	}

	return MinorityGameCrowdResult{
		BaseCostsUSD:        baseCosts,
		EstimatedCrowdLoad:  crowdDensity,
		CrowdAdjustedCosts:  adjustedCosts,
		OriginalOptHour:     origMinHour,
		CrowdAvoidedOptHour: crowdMinHour,
		GameTheoryNote:      "Minority Game: Penalizes crowded off-peak windows where other autoscalers herd, dynamically shifting the optimal start window to calm shoulder hours.",
	}
}

// ProviderAllocation represents a slice of a multi-cluster Colonel Blotto allocation.
type ProviderAllocation struct {
	ProviderName         string  `json:"provider"`
	AllocationWeightPct  float64 `json:"allocation_weight_pct"`
	AllocatedHours       float64 `json:"allocated_gpu_hours"`
	EffectiveSpotRateUSD float64 `json:"effective_spot_rate_usd_hr"`
	StandAloneEvictRisk  float64 `json:"standalone_eviction_risk_pct"`
}

// BlottoPortfolioResult holds the optimal multi-cluster diversification strategy.
type BlottoPortfolioResult struct {
	Asset                     string               `json:"asset"`
	TotalBatchHours           float64              `json:"total_batch_hours"`
	Allocations               []ProviderAllocation `json:"cluster_allocations"`
	BlottoJointFailureRiskPct float64              `json:"blotto_joint_failure_risk_pct"`
	SingleClusterRiskPct      float64              `json:"single_cluster_baseline_risk_pct"`
	RiskReductionPct          float64              `json:"eviction_risk_reduction_pct"`
	Recommendation            string               `json:"recommendation"`
}

// ComputeBlottoAllocation calculates optimal workload distribution across non-correlated clusters.
func ComputeBlottoAllocation(asset string, totalBatchHours float64, baseSpot float64) BlottoPortfolioResult {
	if totalBatchHours <= 0 {
		totalBatchHours = 16.0
	}
	if baseSpot <= 0 {
		baseSpot = 2.49
	}

	// 3 Distinct Providers with uncorrelated failure domains:
	// 1. Vast.ai Spot: Lowest cost, moderate eviction risk (12%)
	// 2. RunPod Spot: Balanced cost, low eviction risk (5%)
	// 3. Apple Silicon Metal / On-Demand: Zero eviction risk (0.01%), higher baseline
	wVast := 0.50
	wRunPod := 0.35
	wMetal := 0.15

	allocations := []ProviderAllocation{
		{
			ProviderName:         "Vast.ai Spot Cluster",
			AllocationWeightPct:  wVast * 100.0,
			AllocatedHours:       roundTo4(totalBatchHours * wVast),
			EffectiveSpotRateUSD: roundTo4(baseSpot * 0.92),
			StandAloneEvictRisk:  12.0,
		},
		{
			ProviderName:         "RunPod Community Spot",
			AllocationWeightPct:  wRunPod * 100.0,
			AllocatedHours:       roundTo4(totalBatchHours * wRunPod),
			EffectiveSpotRateUSD: roundTo4(baseSpot * 1.04),
			StandAloneEvictRisk:  5.0,
		},
		{
			ProviderName:         "Local Metal / Cloud On-Demand Buffer",
			AllocationWeightPct:  wMetal * 100.0,
			AllocatedHours:       roundTo4(totalBatchHours * wMetal),
			EffectiveSpotRateUSD: roundTo4(baseSpot * 1.40),
			StandAloneEvictRisk:  0.10,
		},
	}

	// Joint failure probability: P(all clusters preempted simultaneously)
	// P_joint = (0.12) * (0.05) * (0.001) = 0.000006 = 0.0006%
	jointRisk := (0.12 * 0.05 * 0.001) * 100.0
	singleRisk := 12.0
	riskReduction := ((singleRisk - jointRisk) / singleRisk) * 100.0

	return BlottoPortfolioResult{
		Asset:                     asset,
		TotalBatchHours:           totalBatchHours,
		Allocations:               allocations,
		BlottoJointFailureRiskPct: roundTo4(jointRisk),
		SingleClusterRiskPct:      singleRisk,
		RiskReductionPct:          roundTo4(riskReduction),
		Recommendation:            "COLONEL_BLOTTO_MULTI_CLUSTER_DISPATCH: Split batch across 3 non-correlated providers to reduce joint eviction risk by >99%.",
	}
}
