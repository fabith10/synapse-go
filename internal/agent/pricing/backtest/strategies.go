package backtest

import (
	"math"

	"github.com/fabith10/synapse-go/internal/agent/pricing/quant"
)

// Strategy represents a compute pricing execution or hedging strategy.
type Strategy interface {
	Name() string
	// Execute evaluates the strategy over an hourly price path and returns the realized hourly cost series.
	Execute(pricePath []float64) []float64
}

// SpotStrategy executes purely on the open spot market without hedging.
type SpotStrategy struct{}

func (s *SpotStrategy) Name() string { return "Unhedged Spot" }

func (s *SpotStrategy) Execute(pricePath []float64) []float64 {
	out := make([]float64, len(pricePath))
	copy(out, pricePath)
	return out
}

// ForwardStrategy locks all compute hours at a fixed forward rate.
type ForwardStrategy struct {
	LockedRateUSD float64
}

func (s *ForwardStrategy) Name() string { return "Forward Lock" }

func (s *ForwardStrategy) Execute(pricePath []float64) []float64 {
	n := len(pricePath)
	out := make([]float64, n)
	rate := s.LockedRateUSD
	if rate <= 0 && n > 0 {
		rate = pricePath[0] * 1.025 // Default 2.5% cost-of-carry
	}
	for i := 0; i < n; i++ {
		out[i] = rate
	}
	return out
}

// CallHedgeStrategy protects spot compute with a European Call Option (Black-76 model).
// Caps spot price at strike K while paying an upfront option premium.
type CallHedgeStrategy struct {
	StrikeRateUSD float64
	Volatility    float64
	RiskFreeRate  float64
	ExpiryYears   float64
}

func (s *CallHedgeStrategy) Name() string { return "Delta-Hedged Call Protection" }

func (s *CallHedgeStrategy) Execute(pricePath []float64) []float64 {
	n := len(pricePath)
	if n == 0 {
		return nil
	}

	s0 := pricePath[0]
	strike := s.StrikeRateUSD
	if strike <= 0 {
		strike = s0 * 1.05 // 5% OTM strike ceiling
	}

	vol := s.Volatility
	if vol <= 0 {
		vol = 0.35
	}
	r := s.RiskFreeRate
	if r <= 0 {
		r = 0.045
	}
	tExp := s.ExpiryYears
	if tExp <= 0 {
		tExp = float64(n) / 8760.0 // Path duration as fraction of a year
		if tExp <= 0 {
			tExp = 30.0 / 365.0
		}
	}

	// Compute Black-76 Call Option Premium per hour
	forwardPrice := s0 * math.Exp(r*tExp)
	optRes, err := quant.Black76Price(forwardPrice, strike, tExp, vol, r, quant.OptionCall)
	hourlyPremium := 0.0
	if err == nil && optRes != nil {
		hourlyPremium = optRes.Price
	}

	out := make([]float64, n)
	for i, spot := range pricePath {
		// Effective cost = min(spot, strike) + amortized premium
		effectiveSpot := math.Min(spot, strike)
		out[i] = effectiveSpot + hourlyPremium
	}
	return out
}

// TaskWorkload represents a discrete compute job to be scheduled.
type TaskWorkload struct {
	TaskID         int
	ArrivalHour    int
	DurationHours  int
	DeadlineHours  int // Must finish by ArrivalHour + DeadlineHours
	EstimatedCost  float64
	ImmediateCost  float64
	OptimizedCost  float64
	ScheduledHour  int
	DeferredHours  int
	SLABreached    bool
}

// DeferralStrategy evaluates window optimization across diurnal tariffs and deferrals.
type DeferralStrategy struct {
	DiurnalMultipliers []float64
	CostThresholdUSD   float64
}

func (s *DeferralStrategy) Name() string { return "Execution Window Deferral" }

// ScheduleWorkloads simulates a series of tasks arriving over time and evaluates naive vs deferred execution.
func (s *DeferralStrategy) ScheduleWorkloads(tasks []TaskWorkload, pricePath []float64) ([]TaskWorkload, float64, float64) {
	nHours := len(pricePath)
	multipliers := s.DiurnalMultipliers
	if len(multipliers) < 24 {
		multipliers = []float64{
			0.82, 0.80, 0.79, 0.79, 0.80, 0.83, 0.90, 0.95,
			1.08, 1.15, 1.18, 1.20, 1.22, 1.22, 1.20, 1.18,
			1.15, 1.10, 1.05, 0.98, 0.95, 0.92, 0.88, 0.85,
		}
	}

	evaluatedTasks := make([]TaskWorkload, len(tasks))
	totalImmediate := 0.0
	totalOptimized := 0.0

	for i, task := range tasks {
		t := task
		arr := t.ArrivalHour % nHours
		dur := t.DurationHours
		if dur <= 0 {
			dur = 1
		}
		sla := t.DeadlineHours
		if sla < dur {
			sla = dur
		}

		// 1. Immediate Execution Cost (Naive)
		immCost := 0.0
		for h := 0; h < dur; h++ {
			slot := (arr + h) % nHours
			immCost += pricePath[slot]
		}
		t.ImmediateCost = immCost
		totalImmediate += immCost

		// 2. Window Search for Lowest Cost Slot within SLA: [arr, arr + (sla - dur)]
		bestCost := immCost
		bestStart := arr

		maxStartOffset := sla - dur
		for offset := 0; offset <= maxStartOffset; offset++ {
			candidateStart := arr + offset
			candidateCost := 0.0
			for h := 0; h < dur; h++ {
				slot := (candidateStart + h) % nHours
				candidateCost += pricePath[slot]
			}
			if candidateCost < bestCost {
				bestCost = candidateCost
				bestStart = candidateStart
			}
		}

		t.ScheduledHour = bestStart
		t.DeferredHours = bestStart - arr
		t.OptimizedCost = bestCost
		t.SLABreached = (t.ScheduledHour + dur) > (arr + sla)
		totalOptimized += bestCost

		evaluatedTasks[i] = t
	}

	return evaluatedTasks, totalImmediate, totalOptimized
}

// NashBiddingStrategy simulates game-theoretic spot auction bidding under variable congestion.
type NashBiddingStrategy struct {
	ValuationUSD float64
	NumBidders   int
}

func (s *NashBiddingStrategy) Name() string { return "Game-Theoretic Nash Bidding" }

// Bid returns the optimal bid given cluster utilization rho in [0, 1].
func (s *NashBiddingStrategy) Bid(baseSpot float64, rho float64) float64 {
	n := s.NumBidders
	if n <= 1 {
		n = 5
	}
	val := s.ValuationUSD
	if val <= 0 {
		val = baseSpot * 1.5
	}

	// In a competitive first-price / congestion auction: b*(v) = (n-1)/n * v + congestion_surcharge
	congestionSurcharge := baseSpot * (math.Pow(rho, 3.0) * 0.40)
	optimalBid := ((float64(n)-1.0)/float64(n))*val + congestionSurcharge
	return math.Min(optimalBid, val*1.2)
}
