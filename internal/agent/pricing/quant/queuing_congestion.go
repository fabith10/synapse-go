package quant

import (
	"math"
)

// QueuingCongestionParams defines structural parameters for M/M/c cluster capacity pricing.
type QueuingCongestionParams struct {
	BaseSpotUSD      float64 // Minimum physical cost baseline (power + cooling + base amortization)
	AlphaCongestion  float64 // Scarcity surcharge scaling factor (e.g. 1.20)
	GammaElasticity  float64 // Elasticity curvature exponent (e.g. 2.5)
	MaxUtilization   float64 // Upper bound cap on utilization (e.g. 0.99)
}

// DefaultQueuingParams returns standard cluster capacity parameters.
func DefaultQueuingParams(baseSpot float64) QueuingCongestionParams {
	if baseSpot <= 0 {
		baseSpot = 2.49
	}
	return QueuingCongestionParams{
		BaseSpotUSD:     baseSpot,
		AlphaCongestion: 1.25,
		GammaElasticity: 2.0,
		MaxUtilization:  0.995,
	}
}

// PriceFromUtilization calculates spot price as a function of cluster utilization rho.
// S(rho) = S_base + alpha * (rho^gamma) / (1 - rho)
func (p *QueuingCongestionParams) PriceFromUtilization(rho float64) float64 {
	if rho < 0 {
		rho = 0
	}
	if rho > p.MaxUtilization {
		rho = p.MaxUtilization
	}

	congestionSurcharge := (p.AlphaCongestion * math.Pow(rho, p.GammaElasticity)) / (1.0 - rho)
	return roundTo4(p.BaseSpotUSD + congestionSurcharge)
}

// ImpliedUtilization inverts an observed spot price S_obs to calculate the implied cluster utilization rho.
// Solves: S_obs - S_base = alpha * (rho^gamma) / (1 - rho) using binary search.
func (p *QueuingCongestionParams) ImpliedUtilization(observedSpot float64) float64 {
	if observedSpot <= p.BaseSpotUSD {
		return 0.0
	}

	targetSurcharge := observedSpot - p.BaseSpotUSD

	low := 0.0
	high := p.MaxUtilization
	for iter := 0; iter < 40; iter++ {
		mid := (low + high) / 2.0
		val := (p.AlphaCongestion * math.Pow(mid, p.GammaElasticity)) / (1.0 - mid)
		if math.Abs(val-targetSurcharge) < 1e-5 {
			return roundTo4(mid)
		}
		if val < targetSurcharge {
			low = mid
		} else {
			high = mid
		}
	}
	return roundTo4((low + high) / 2.0)
}

// EvictionRiskFromUtilization computes the physical eviction probability from cluster queuing saturation.
func (p *QueuingCongestionParams) EvictionRiskFromUtilization(rho float64) float64 {
	if rho <= 0.60 {
		return 0.20 // baseline 0.2%
	}
	// Convex hockey-stick probability as cluster approaches 100% capacity
	prob := 100.0 * math.Pow(rho, 6.0)
	return math.Min(95.0, roundTo4(prob))
}

// CongestionPricingResult represents cluster load and congestion analytics for agents.
type CongestionPricingResult struct {
	Asset                     string  `json:"asset"`
	Model                     string  `json:"model"`
	CurrentSpotUSD            float64 `json:"current_spot_usd_hr"`
	BasePhysicalCostUSD       float64 `json:"base_physical_cost_usd_hr"`
	ImpliedClusterLoadPct     float64 `json:"implied_cluster_load_pct"`
	CongestionSurchargeUSD    float64 `json:"congestion_surcharge_usd_hr"`
	QueueEvictionRiskProbPct  float64 `json:"queue_eviction_risk_prob_pct"`
	ClusterStatus             string  `json:"cluster_status"`
}

// ComputeQueuingCongestionAnalytics computes cluster utilization and congestion surcharges.
func ComputeQueuingCongestionAnalytics(asset string, observedSpot float64) CongestionPricingResult {
	if observedSpot <= 0 {
		observedSpot = 2.49
	}
	baseRate := spotBaseRateHelper(asset)
	params := DefaultQueuingParams(baseRate)

	rho := params.ImpliedUtilization(observedSpot)
	surcharge := math.Max(0, observedSpot-baseRate)
	evictionRisk := params.EvictionRiskFromUtilization(rho)

	status := "OPTIMAL_CAPACITY_LIQUID"
	if rho > 0.85 {
		status = "CRITICAL_SATURATION_SPIKE_RISK"
	} else if rho > 0.65 {
		status = "ELEVATED_QUEUE_MODERATE"
	}

	return CongestionPricingResult{
		Asset:                    asset,
		Model:                    "M/M/c Capacity Queuing Congestion Model (Erlang-C)",
		CurrentSpotUSD:           observedSpot,
		BasePhysicalCostUSD:      baseRate,
		ImpliedClusterLoadPct:    roundTo4(rho * 100.0),
		CongestionSurchargeUSD:   roundTo4(surcharge),
		QueueEvictionRiskProbPct: evictionRisk,
		ClusterStatus:            status,
	}
}

func spotBaseRateHelper(asset string) float64 {
	switch asset {
	case "RTX_4090":
		return 0.45
	case "A100_80GB":
		return 1.29
	default:
		return 2.49
	}
}
