package quant

import (
	"math"
	"math/rand"
)

// LuciaSchwartzParams configures the Two-Factor Structural Commodity Model for GPU compute.
type LuciaSchwartzParams struct {
	// Factor 1: Short-term mean-reverting factor X_t (hourly diurnal cycles)
	KappaX  float64 // Speed of mean reversion for X_t per hour (e.g. 0.8)
	SigmaX  float64 // Volatility of short-term factor per sqrt(hour) (e.g. 0.15)

	// Factor 2: Long-term structural trend Y_t (Moore's Law / hardware obsolescence drift)
	MuY     float64 // Annual technological deflation drift (e.g. -0.30 for -30%/year)
	SigmaY  float64 // Volatility of long-term structural factor per sqrt(year) (e.g. 0.10)

	// Correlation between short-term shocks and long-term tech shocks
	RhoXY   float64 // Correlation rho in [-1, 1]

	// Deterministic Diurnal Fourier Seasonality f(t)
	BaseLogSpot float64   // a0 baseline
	FourierCos  []float64 // Cosine amplitudes
	FourierSin  []float64 // Sine amplitudes
}

// DefaultH100LuciaSchwartzParams returns calibrated parameters for H100 GPU compute.
func DefaultH100LuciaSchwartzParams(baseSpot float64) LuciaSchwartzParams {
	if baseSpot <= 0 {
		baseSpot = 2.49
	}
	return LuciaSchwartzParams{
		KappaX:      0.85,
		SigmaX:      0.14,
		MuY:         -0.25, // -25% annual price decay as next-gen GPUs launch
		SigmaY:      0.08,
		RhoXY:       0.10,
		BaseLogSpot: math.Log(baseSpot),
		FourierCos:  []float64{-0.15, 0.05},
		FourierSin:  []float64{0.08, -0.02},
	}
}

// Seasonality evaluates deterministic Fourier seasonality f(t) in log-space.
func (p *LuciaSchwartzParams) Seasonality(tHours float64) float64 {
	val := p.BaseLogSpot
	for k := 0; k < len(p.FourierCos); k++ {
		harmonic := float64(k + 1)
		freq := 2.0 * math.Pi * harmonic * tHours / 24.0
		val += p.FourierCos[k] * math.Cos(freq)
		if k < len(p.FourierSin) {
			val += p.FourierSin[k] * math.Sin(freq)
		}
	}
	return val
}

// ForwardPrice evaluates the exact closed-form Lucia-Schwartz Forward Contract Curve F(t, T).
// tHours: Current evaluation time in hours.
// THours: Forward delivery maturity time in hours.
// currentX: Current state of short-term factor X_t.
// currentY: Current state of long-term factor Y_t.
func (p *LuciaSchwartzParams) ForwardPrice(tHours, THours, currentX, currentY float64) float64 {
	tauHours := THours - tHours
	if tauHours <= 0 {
		return math.Exp(p.Seasonality(tHours) + currentX + currentY)
	}

	tauYears := tauHours / 8760.0 // 8760 hours in a year
	decay := math.Exp(-p.KappaX * tauHours)

	// 1. Conditional Expectation of log-spot E[ln S_T | F_t]
	expLogSpot := p.Seasonality(THours) + currentX*decay + currentY + p.MuY*tauYears

	// 2. Variance of log-spot Var(ln S_T | F_t)
	varX := (p.SigmaX * p.SigmaX / (2.0 * p.KappaX)) * (1.0 - math.Exp(-2.0*p.KappaX*tauHours))
	varY := p.SigmaY * p.SigmaY * tauYears
	covXY := (2.0 * p.RhoXY * p.SigmaX * (p.SigmaY / math.Sqrt(8760.0)) / p.KappaX) * (1.0 - decay)

	totalVar := varX + varY + covXY
	if totalVar < 0 {
		totalVar = 0
	}

	// 3. F(t, T) = exp( E[ln S_T] + 0.5 * Var(ln S_T) )
	forwardRate := math.Exp(expLogSpot + 0.5*totalVar)
	return math.Max(0.01, roundTo4(forwardRate))
}

// SimulatePath generates a 2-factor path trajectory.
func (p *LuciaSchwartzParams) SimulatePath(s0 float64, totalHours float64, stepsPerHour int, r *rand.Rand) []float64 {
	if stepsPerHour <= 0 {
		stepsPerHour = 1
	}
	numSteps := int(totalHours * float64(stepsPerHour))
	dtHours := 1.0 / float64(stepsPerHour)
	dtYears := dtHours / 8760.0
	sqrtDtHours := math.Sqrt(dtHours)
	sqrtDtYears := math.Sqrt(dtYears)

	path := make([]float64, numSteps+1)
	path[0] = s0

	currentX := 0.0
	currentY := 0.0

	for step := 1; step <= numSteps; step++ {
		tHours := float64(step) * dtHours

		// Correlated standard normals
		z1 := r.NormFloat64()
		z2Raw := r.NormFloat64()
		z2 := p.RhoXY*z1 + math.Sqrt(1.0-p.RhoXY*p.RhoXY)*z2Raw

		// Factor 1: dX = -kappa*X*dt + sigma_x*sqrt(dt)*Z1
		currentX = currentX*(1.0-p.KappaX*dtHours) + p.SigmaX*sqrtDtHours*z1

		// Factor 2: dY = mu_y*dt + sigma_y*sqrt(dt)*Z2
		currentY = currentY + p.MuY*dtYears + p.SigmaY*sqrtDtYears*z2

		logPrice := p.Seasonality(tHours) + currentX + currentY
		path[step] = math.Max(0.01, math.Exp(logPrice))
	}

	return path
}

// LuciaSchwartzForwardCurveResult contains term structure points for agent queries.
type LuciaSchwartzForwardCurveResult struct {
	Asset             string             `json:"asset"`
	Model             string             `json:"model"`
	SpotBaseUSD       float64            `json:"spot_base_usd_hr"`
	AnnualDeflation   float64            `json:"annual_hardware_deflation_pct"`
	TermStructureUSD  map[string]float64 `json:"term_structure_forward_usd_hr"`
	DecayAnalysisNote string             `json:"decay_analysis_note"`
}

// ComputeLuciaSchwartzCurve builds the complete 2-factor term structure.
func ComputeLuciaSchwartzCurve(asset string, baseSpot float64) LuciaSchwartzForwardCurveResult {
	if baseSpot <= 0 {
		baseSpot = 2.49
	}
	params := DefaultH100LuciaSchwartzParams(baseSpot)

	maturities := map[string]float64{
		"1d_spot":   24.0,
		"7d_term":   7.0 * 24.0,
		"30d_term":  30.0 * 24.0,
		"90d_term":  90.0 * 24.0,
		"180d_term": 180.0 * 24.0,
		"365d_term": 365.0 * 24.0,
	}

	curve := make(map[string]float64)
	for k, h := range maturities {
		curve[k] = params.ForwardPrice(0, h, 0, 0)
	}

	return LuciaSchwartzForwardCurveResult{
		Asset:           asset,
		Model:           "Lucia-Schwartz Two-Factor Commodity Model (Diurnal Mean-Reversion + Hardware Deflation)",
		SpotBaseUSD:     baseSpot,
		AnnualDeflation: math.Abs(params.MuY) * 100.0,
		TermStructureUSD: curve,
		DecayAnalysisNote: "Near-term rates reflect diurnal hourly mean reversion. Long-term (90d-365d) rates reflect technological deflation as next-gen compute architectures reduce hardware amortized cost.",
	}
}
