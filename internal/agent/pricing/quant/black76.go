package quant

import (
	"errors"
	"math"
)

// OptionType defines Call or Put.
type OptionType string

const (
	OptionCall OptionType = "call"
	OptionPut  OptionType = "put"
)

// Greeks contains the full first- and second-order analytical sensitivities.
type Greeks struct {
	Delta float64 `json:"delta"` // Directional exposure: dPrice/dF
	Gamma float64 `json:"gamma"` // Convexity / curvature: d^2Price/dF^2
	Vega  float64 `json:"vega"`  // Volatility sensitivity: dPrice/dSigma (per 100% vol)
	Theta float64 `json:"theta"` // Time decay: dPrice/dt (per day)
	Rho   float64 `json:"rho"`   // Interest rate sensitivity: dPrice/dr
}

// OptionResult represents the calculated pricing and Greek surface.
type OptionResult struct {
	OptionType OptionType `json:"option_type"`
	Forward    float64    `json:"forward_price"`
	Strike     float64    `json:"strike_price"`
	Expiry     float64    `json:"expiry_years"`
	Volatility float64    `json:"implied_volatility"`
	RiskFree   float64    `json:"risk_free_rate"`
	Price      float64    `json:"option_price"`
	Intrinsic  float64    `json:"intrinsic_value"`
	TimeValue  float64    `json:"time_value"`
	Greeks     Greeks     `json:"greeks"`
}

// Black76Price calculates European option prices and analytical Greeks on compute forward contracts.
// F: Forward contract rate (USD/hr)
// K: Option strike price (USD/hr)
// T: Time to expiration in years (e.g., 30 days = 30.0 / 365.0)
// sigma: Annualized volatility (e.g., 0.35 for 35%)
// r: Continuous risk-free rate (e.g., 0.045 for 4.5%)
// optType: OptionCall or OptionPut
func Black76Price(F, K, T, sigma, r float64, optType OptionType) (*OptionResult, error) {
	if F <= 0 {
		return nil, errors.New("forward price F must be positive")
	}
	if K <= 0 {
		return nil, errors.New("strike price K must be positive")
	}
	if T <= 0 {
		// At expiration
		intrinsic := 0.0
		if optType == OptionCall {
			intrinsic = math.Max(0, F-K)
		} else {
			intrinsic = math.Max(0, K-F)
		}
		return &OptionResult{
			OptionType: optType,
			Forward:    F,
			Strike:     K,
			Expiry:     0,
			Volatility: sigma,
			RiskFree:   r,
			Price:      intrinsic,
			Intrinsic:  intrinsic,
			TimeValue:  0,
		}, nil
	}
	if sigma <= 0 {
		sigma = 0.0001
	}

	sqrtT := math.Sqrt(T)
	d1 := (math.Log(F/K) + 0.5*sigma*sigma*T) / (sigma * sqrtT)
	d2 := d1 - sigma*sqrtT

	df := math.Exp(-r * T) // Discount factor
	nd1 := StandardNormalCDF(d1)
	nd2 := StandardNormalCDF(d2)
	n_minus_d1 := StandardNormalCDF(-d1)
	n_minus_d2 := StandardNormalCDF(-d2)
	pdfD1 := StandardNormalPDF(d1)

	var price, intrinsic, delta, theta, rho float64

	// Common gamma & vega for Black-76
	gamma := (df * pdfD1) / (F * sigma * sqrtT)
	vega := F * df * sqrtT * pdfD1 // Vega per unit vol change

	if optType == OptionCall {
		price = df * (F*nd1 - K*nd2)
		intrinsic = math.Max(0, F-K)
		delta = df * nd1
		// Theta per year
		thetaYear := -(F*df*pdfD1*sigma)/(2.0*sqrtT) - r*price
		theta = thetaYear / 365.0 // Convert to per-day decay
		rho = -T * price
	} else {
		price = df * (K*n_minus_d2 - F*n_minus_d1)
		intrinsic = math.Max(0, K-F)
		delta = -df * n_minus_d1
		thetaYear := -(F*df*pdfD1*sigma)/(2.0*sqrtT) - r*price
		theta = thetaYear / 365.0
		rho = -T * price
	}

	timeValue := math.Max(0, price-intrinsic)

	return &OptionResult{
		OptionType: optType,
		Forward:    F,
		Strike:     K,
		Expiry:     T,
		Volatility: sigma,
		RiskFree:   r,
		Price:      price,
		Intrinsic:  intrinsic,
		TimeValue:  timeValue,
		Greeks: Greeks{
			Delta: delta,
			Gamma: gamma,
			Vega:  vega,
			Theta: theta,
			Rho:   rho,
		},
	}, nil
}
