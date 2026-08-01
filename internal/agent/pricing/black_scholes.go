package pricing

import (
	"math"
)

// normCDF returns the Cumulative Distribution Function for standard normal distribution N(x).
func normCDF(x float64) float64 {
	return 0.5 * (1.0 + math.Erf(x/math.Sqrt2))
}

// normPDF returns the Probability Density Function for standard normal distribution N'(x).
func normPDF(x float64) float64 {
	return (1.0 / math.Sqrt(2.0*math.Pi)) * math.Exp(-0.5*x*x)
}

// BlackScholesPrice computes the theoretical Black-Scholes option price and Vega (dPrice/dSigma) for European calls or puts.
func BlackScholesPrice(isCall bool, S, K, T, r, sigma float64) (price float64, vega float64) {
	if S <= 0 || K <= 0 || T <= 0 || sigma <= 0 {
		return 0, 0
	}
	sqrtT := math.Sqrt(T)
	d1 := (math.Log(S/K) + (r+0.5*sigma*sigma)*T) / (sigma * sqrtT)
	d2 := d1 - sigma*sqrtT

	vega = S * sqrtT * normPDF(d1)

	if isCall {
		price = S*normCDF(d1) - K*math.Exp(-r*T)*normCDF(d2)
	} else {
		price = K*math.Exp(-r*T)*normCDF(-d2) - S*normCDF(-d1)
	}
	return price, vega
}

// ImpliedVolatilityBS solves for the exact Black-Scholes implied volatility using Newton-Raphson root inversion.
func ImpliedVolatilityBS(isCall bool, S, K, T, r, marketPrice float64) float64 {
	if S <= 0 || K <= 0 || T <= 0 || marketPrice <= 0 {
		return 0
	}

	intrinsic := 0.0
	if isCall {
		intrinsic = math.Max(0, S-K*math.Exp(-r*T))
	} else {
		intrinsic = math.Max(0, K*math.Exp(-r*T)-S)
	}
	if marketPrice <= intrinsic {
		return 0
	}

	// Initial estimate via Brenner-Subrahmanyam formula
	sigma := (marketPrice / S) * math.Sqrt(2.0*math.Pi/T)
	if sigma < 0.05 {
		sigma = 0.20
	} else if sigma > 3.0 {
		sigma = 0.50
	}

	const maxIter = 100
	const tol = 1e-6

	for i := 0; i < maxIter; i++ {
		price, vega := BlackScholesPrice(isCall, S, K, T, r, sigma)
		diff := price - marketPrice

		if math.Abs(diff) < tol {
			return roundTo4(sigma)
		}

		if vega < 1e-12 {
			break
		}

		sigma = sigma - diff/vega

		if sigma <= 0.001 {
			sigma = 0.001
		} else if sigma > 10.0 {
			sigma = 10.0
		}
	}

	return roundTo4(sigma)
}
