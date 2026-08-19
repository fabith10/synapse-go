package quant

import (
	"math"
	"math/rand"
	"time"
)

// OUMRJDParams configures the Ornstein-Uhlenbeck Mean-Reverting Jump Diffusion model.
type OUMRJDParams struct {
	Kappa         float64   // Speed of mean reversion per hour (e.g. 0.8)
	BaseMean      float64   // Base long-term spot price (e.g. 2.49)
	FourierCos    []float64 // Cosine harmonic amplitudes for diurnal cycle
	FourierSin    []float64 // Sine harmonic amplitudes for diurnal cycle
	DiffusionVol  float64   // Continuous diffusion volatility per sqrt(hour) (e.g. 0.12)
	JumpIntensity float64   // Poisson jump arrival rate per hour (e.g. 0.05 = ~1.2 spikes/day)
	JumpMean      float64   // Mean of log-jump size ln(J) (e.g. 0.35 = ~+40% spike)
	JumpStd       float64   // Standard deviation of log-jump size (e.g. 0.15)
}

// DefaultH100MRJDParams returns standard empirical parameters calibrated for H100 spot compute.
func DefaultH100MRJDParams(baseSpot float64) OUMRJDParams {
	if baseSpot <= 0 {
		baseSpot = 2.49
	}
	return OUMRJDParams{
		Kappa:         0.75,
		BaseMean:      baseSpot,
		FourierCos:    []float64{-0.25 * baseSpot, 0.08 * baseSpot},
		FourierSin:    []float64{0.15 * baseSpot, -0.04 * baseSpot},
		DiffusionVol:  0.10,
		JumpIntensity: 0.04, // ~1 eviction/demand spike per 24 hours
		JumpMean:      0.40, // 40% jump surge
		JumpStd:       0.15,
	}
}

// HarmonicMean evaluates the deterministic diurnal Fourier curve at hour t.
// theta(t) = a0 + sum_k [ a_k * cos(2*pi*k*t / 24) + b_k * sin(2*pi*k*t / 24) ]
func (p *OUMRJDParams) HarmonicMean(tHours float64) float64 {
	mean := p.BaseMean
	for k := 0; k < len(p.FourierCos); k++ {
		harmonic := float64(k + 1)
		freq := 2.0 * math.Pi * harmonic * tHours / 24.0
		mean += p.FourierCos[k] * math.Cos(freq)
		if k < len(p.FourierSin) {
			mean += p.FourierSin[k] * math.Sin(freq)
		}
	}
	return math.Max(0.01, mean)
}

// SimulatePath generates a single Monte Carlo spot price trajectory using exact discretization of OU-MRJD.
// S_{t+dt} = S_t * exp(-kappa*dt) + theta(t)*(1 - exp(-kappa*dt)) + sigma*S_t*sqrt(dt)*Z + J*N(dt)
func (p *OUMRJDParams) SimulatePath(s0 float64, totalHours float64, stepsPerHour int, r *rand.Rand) []float64 {
	if stepsPerHour <= 0 {
		stepsPerHour = 1
	}
	numSteps := int(totalHours * float64(stepsPerHour))
	if numSteps <= 0 {
		return []float64{s0}
	}

	dt := 1.0 / float64(stepsPerHour)
	sqrtDt := math.Sqrt(dt)
	decay := math.Exp(-p.Kappa * dt)
	oneMinusDecay := 1.0 - decay

	path := make([]float64, numSteps+1)
	path[0] = s0

	currentS := s0
	for step := 1; step <= numSteps; step++ {
		t := float64(step-1) * dt
		theta := p.HarmonicMean(t)

		// 1. Mean-reverting drift
		driftedS := currentS*decay + theta*oneMinusDecay

		// 2. Continuous Gaussian diffusion
		z := r.NormFloat64()
		diffusion := p.DiffusionVol * currentS * sqrtDt * z

		// 3. Poisson jump component
		jumpSum := 0.0
		if p.JumpIntensity > 0 {
			// Probability of jump in dt: P(N=1) ~ lambda * dt
			jumpProb := p.JumpIntensity * dt
			if r.Float64() < jumpProb {
				// Jump size drawn from LogNormal
				jumpLog := r.NormFloat64()*p.JumpStd + p.JumpMean
				jumpMultiplier := math.Exp(jumpLog) - 1.0
				jumpSum = currentS * jumpMultiplier
			}
		}

		currentS = math.Max(0.01, driftedS+diffusion+jumpSum)
		path[step] = currentS
	}

	return path
}

// SimulateBatch generates N independent paths.
func (p *OUMRJDParams) SimulateBatch(s0 float64, totalHours float64, stepsPerHour int, numPaths int) [][]float64 {
	if numPaths <= 0 {
		numPaths = 1000
	}
	paths := make([][]float64, numPaths)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < numPaths; i++ {
		paths[i] = p.SimulatePath(s0, totalHours, stepsPerHour, r)
	}
	return paths
}
