package quant

import (
	"math"
	"math/rand"
	"sort"
	"time"
)

// MarketRegime represents the physical availability state of a compute cluster.
type MarketRegime int

const (
	RegimeNormal    MarketRegime = 0 // Liquid availability, low volatility, low eviction risk
	RegimeCongested MarketRegime = 1 // Capacity saturation / large batch training, severe volatility & spikes
)

// MRSParams defines parameters for the 2-State Markov Regime-Switching Model.
type MRSParams struct {
	// Regime 0 (Normal)
	Kappa0        float64 // Mean reversion speed (e.g. 0.80)
	Theta0        float64 // Long-term price level (e.g. 2.49)
	Sigma0        float64 // Volatility (e.g. 0.12)
	EvictionRisk0 float64 // Base eviction probability (e.g. 0.5%)

	// Regime 1 (Congested)
	Kappa1        float64 // Mean reversion speed (e.g. 2.20)
	Theta1        float64 // Congestion price level (e.g. 5.50)
	Sigma1        float64 // Volatility (e.g. 0.85)
	EvictionRisk1 float64 // Congestion eviction probability (e.g. 28.0%)

	// Transition intensities per hour
	Lambda01 float64 // Rate of moving Normal -> Congested (e.g. 0.03/hr)
	Lambda10 float64 // Rate of returning Congested -> Normal (e.g. 0.18/hr, mean duration ~5.5 hrs)
}

// DefaultH100MRSParams returns calibrated 2-state regime parameters for H100 GPU compute.
func DefaultH100MRSParams(baseSpot float64) MRSParams {
	if baseSpot <= 0 {
		baseSpot = 2.49
	}
	return MRSParams{
		Kappa0:        0.80,
		Theta0:        baseSpot,
		Sigma0:        0.12,
		EvictionRisk0: 0.50, // 0.5% eviction risk in calm regime

		Kappa1:        2.20,
		Theta1:        baseSpot * 2.20,
		Sigma1:        0.85,
		EvictionRisk1: 28.0, // 28% eviction risk during congestion

		Lambda01: 0.035, // ~1 congestion event every 28 hours
		Lambda10: 0.20,  // congestion clears in average ~5 hours
	}
}

// FilterRegimeProbabilities uses Bayesian posterior filtering to determine the probability
// of being in RegimeNormal vs RegimeCongested given the currently observed spot price.
func (p *MRSParams) FilterRegimeProbabilities(observedSpot float64) (probNormal float64, probCongested float64) {
	// Stationary prior probabilities
	piCongested := p.Lambda01 / (p.Lambda01 + p.Lambda10)
	piNormal := 1.0 - piCongested

	// Likelihood densities under Gaussian assumption around regime means
	pdfNormal := StandardNormalPDF((observedSpot - p.Theta0) / (p.Theta0 * p.Sigma0))
	pdfCongested := StandardNormalPDF((observedSpot - p.Theta1) / (p.Theta1 * p.Sigma1))

	numNormal := pdfNormal * piNormal
	numCongested := pdfCongested * piCongested
	denom := numNormal + numCongested

	if denom <= 0 {
		return 0.90, 0.10
	}

	pNorm := numNormal / denom
	pCong := numCongested / denom
	return roundTo4(pNorm), roundTo4(pCong)
}

// SimulateMRSPath generates a spot price trajectory governed by Markov regime transitions.
func (p *MRSParams) SimulateMRSPath(s0 float64, totalHours float64, stepsPerHour int, initialRegime MarketRegime, r *rand.Rand) ([]float64, []MarketRegime) {
	if stepsPerHour <= 0 {
		stepsPerHour = 1
	}
	numSteps := int(totalHours * float64(stepsPerHour))
	dt := 1.0 / float64(stepsPerHour)
	sqrtDt := math.Sqrt(dt)

	prices := make([]float64, numSteps+1)
	regimes := make([]MarketRegime, numSteps+1)

	prices[0] = s0
	regimes[0] = initialRegime

	currentS := s0
	currentRegime := initialRegime

	for step := 1; step <= numSteps; step++ {
		// 1. Regime transition step
		if currentRegime == RegimeNormal {
			if r.Float64() < p.Lambda01*dt {
				currentRegime = RegimeCongested
			}
		} else {
			if r.Float64() < p.Lambda10*dt {
				currentRegime = RegimeNormal
			}
		}

		// 2. Regime-dependent price dynamics
		kappa := p.Kappa0
		theta := p.Theta0
		sigma := p.Sigma0
		if currentRegime == RegimeCongested {
			kappa = p.Kappa1
			theta = p.Theta1
			sigma = p.Sigma1
		}

		decay := math.Exp(-kappa * dt)
		driftedS := currentS*decay + theta*(1.0-decay)
		diffusion := sigma * currentS * sqrtDt * r.NormFloat64()

		currentS = math.Max(0.01, driftedS+diffusion)
		prices[step] = currentS
		regimes[step] = currentRegime
	}

	return prices, regimes
}

// MRSRiskProfile holds the regime-switching risk metrics.
type MRSRiskProfile struct {
	Asset                  string  `json:"asset"`
	CurrentSpotUSD         float64 `json:"current_spot_usd_hr"`
	ProbNormalRegimePct    float64 `json:"prob_normal_regime_pct"`
	ProbCongestedRegimePct float64 `json:"prob_congested_regime_pct"`
	CurrentRegimeLabel     string  `json:"current_regime_label"`
	ExpectedWindowCostUSD  float64 `json:"expected_window_cost_usd"`
	VaR95USD               float64 `json:"var_95_usd"`
	CVaR95USD              float64 `json:"cvar_95_usd"`
	EvictionRiskProbPct    float64 `json:"eviction_risk_prob_pct"`
	Recommendation         string  `json:"recommendation"`
}

// EvaluateRegimeSwitchingRisk runs Monte Carlo under the Markov Regime-Switching engine.
func EvaluateRegimeSwitchingRisk(asset string, s0 float64, durationHours int, numPaths int) MRSRiskProfile {
	if s0 <= 0 {
		s0 = 2.49
	}
	if durationHours <= 0 {
		durationHours = 4
	}
	if numPaths <= 0 {
		numPaths = 3000
	}

	params := DefaultH100MRSParams(s0)
	pNorm, pCong := params.FilterRegimeProbabilities(s0)

	initRegime := RegimeNormal
	regimeLabel := "NORMAL_LIQUID"
	if pCong > 0.40 {
		initRegime = RegimeCongested
		regimeLabel = "CLUSTER_CONGESTION"
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	costs := make([]float64, numPaths)
	evictionCount := 0
	sumCost := 0.0

	for i := 0; i < numPaths; i++ {
		prices, regimes := params.SimulateMRSPath(s0, float64(durationHours), 1, initRegime, r)
		wCost := 0.0
		evicted := false

		for h := 0; h < durationHours; h++ {
			wCost += prices[h]
			// Check regime-derived eviction
			risk := params.EvictionRisk0
			if regimes[h] == RegimeCongested {
				risk = params.EvictionRisk1
			}
			if r.Float64()*100.0 < risk {
				evicted = true
			}
		}

		costs[i] = wCost
		sumCost += wCost
		if evicted {
			evictionCount++
		}
	}

	sort.Float64s(costs)
	mean := sumCost / float64(numPaths)
	idx95 := int(float64(numPaths) * 0.95)
	if idx95 >= numPaths {
		idx95 = numPaths - 1
	}
	var95 := costs[idx95]

	cvarSum := 0.0
	for i := idx95; i < numPaths; i++ {
		cvarSum += costs[i]
	}
	cvar95 := cvarSum / float64(numPaths-idx95)

	evictionProb := (float64(evictionCount) / float64(numPaths)) * 100.0

	rec := "EXECUTE_SPOT_SAFE"
	if pCong > 0.40 || evictionProb > 15.0 {
		rec = "HEDGE_REQUIRED_OR_DEFER"
	}

	return MRSRiskProfile{
		Asset:                  asset,
		CurrentSpotUSD:         s0,
		ProbNormalRegimePct:    pNorm * 100.0,
		ProbCongestedRegimePct: pCong * 100.0,
		CurrentRegimeLabel:     regimeLabel,
		ExpectedWindowCostUSD:  roundTo4(mean),
		VaR95USD:               roundTo4(var95),
		CVaR95USD:              roundTo4(cvar95),
		EvictionRiskProbPct:    roundTo4(evictionProb),
		Recommendation:         rec,
	}
}
