package quant

import (
	"math"
)

// StandardNormalPDF returns the probability density function value for the standard normal distribution:
// phi(x) = (1 / sqrt(2*pi)) * exp(-x^2 / 2)
func StandardNormalPDF(x float64) float64 {
	return math.Exp(-0.5*x*x) / math.Sqrt(2.0*math.Pi)
}

// StandardNormalCDF returns the cumulative distribution function for standard normal N(0, 1) using math.Erf:
// Phi(x) = 0.5 * (1 + erf(x / sqrt(2)))
func StandardNormalCDF(x float64) float64 {
	return 0.5 * (1.0 + math.Erf(x/math.Sqrt2))
}
