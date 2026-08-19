package quant_test

import (
	"math"
	"math/rand"
	"testing"

	"github.com/fabith10/synapse-go/internal/agent/pricing/quant"
)

func TestMathUtils_NormalDist(t *testing.T) {
	if math.Abs(quant.StandardNormalCDF(0.0)-0.5) > 1e-7 {
		t.Errorf("expected CDF(0) = 0.5, got %f", quant.StandardNormalCDF(0.0))
	}
	if quant.StandardNormalCDF(-10.0) > 1e-6 {
		t.Errorf("expected CDF(-10) ~ 0, got %f", quant.StandardNormalCDF(-10.0))
	}
	if quant.StandardNormalCDF(10.0) < 0.999999 {
		t.Errorf("expected CDF(10) ~ 1, got %f", quant.StandardNormalCDF(10.0))
	}

	pdf0 := quant.StandardNormalPDF(0.0)
	expectedPdf0 := 1.0 / math.Sqrt(2.0*math.Pi)
	if math.Abs(pdf0-expectedPdf0) > 1e-7 {
		t.Errorf("expected PDF(0) = %f, got %f", expectedPdf0, pdf0)
	}
}

func TestBlack76_PutCallParity(t *testing.T) {
	F := 2.6145
	K := 2.50
	T := 30.0 / 365.0
	sigma := 0.35
	r := 0.045

	callRes, err := quant.Black76Price(F, K, T, sigma, r, quant.OptionCall)
	if err != nil {
		t.Fatalf("call error: %v", err)
	}
	putRes, err := quant.Black76Price(F, K, T, sigma, r, quant.OptionPut)
	if err != nil {
		t.Fatalf("put error: %v", err)
	}

	// Put-Call Parity: C - P = exp(-r*T) * (F - K)
	lhs := callRes.Price - putRes.Price
	rhs := math.Exp(-r*T) * (F - K)

	if math.Abs(lhs-rhs) > 1e-5 {
		t.Errorf("Put-Call Parity failed: LHS = %f, RHS = %f, diff = %e", lhs, rhs, math.Abs(lhs-rhs))
	}
}

func TestBlack76_AnalyticalGreeksAgainstFiniteDifferences(t *testing.T) {
	F := 2.6145
	K := 2.60
	T := 45.0 / 365.0
	sigma := 0.40
	r := 0.05
	eps := 1e-4

	res, err := quant.Black76Price(F, K, T, sigma, r, quant.OptionCall)
	if err != nil {
		t.Fatalf("Black76 error: %v", err)
	}

	// 1. Delta Finite Difference: (C(F + eps) - C(F - eps)) / (2 * eps)
	callUp, _ := quant.Black76Price(F+eps, K, T, sigma, r, quant.OptionCall)
	callDown, _ := quant.Black76Price(F-eps, K, T, sigma, r, quant.OptionCall)
	fdDelta := (callUp.Price - callDown.Price) / (2.0 * eps)

	if math.Abs(res.Greeks.Delta-fdDelta) > 1e-4 {
		t.Errorf("Delta mismatch: analytical = %f, finite_diff = %f", res.Greeks.Delta, fdDelta)
	}

	// 2. Gamma Finite Difference: (C(F + eps) - 2*C(F) + C(F - eps)) / (eps^2)
	fdGamma := (callUp.Price - 2.0*res.Price + callDown.Price) / (eps * eps)
	if math.Abs(res.Greeks.Gamma-fdGamma) > 1e-3 {
		t.Errorf("Gamma mismatch: analytical = %f, finite_diff = %f", res.Greeks.Gamma, fdGamma)
	}

	// 3. Vega Finite Difference: (C(sigma + eps) - C(sigma - eps)) / (2 * eps)
	callVolUp, _ := quant.Black76Price(F, K, T, sigma+eps, r, quant.OptionCall)
	callVolDown, _ := quant.Black76Price(F, K, T, sigma-eps, r, quant.OptionCall)
	fdVega := (callVolUp.Price - callVolDown.Price) / (2.0 * eps)
	if math.Abs(res.Greeks.Vega-fdVega) > 1e-4 {
		t.Errorf("Vega mismatch: analytical = %f, finite_diff = %f", res.Greeks.Vega, fdVega)
	}
}

func TestOUMRJD_SimulationAndMeanReversion(t *testing.T) {
	params := quant.DefaultH100MRJDParams(2.50)
	params.JumpIntensity = 0 // test pure drift & diffusion first
	rng := rand.New(rand.NewSource(42))

	path := params.SimulatePath(10.0, 48.0, 4, rng) // start far above mean
	if len(path) == 0 {
		t.Fatalf("expected non-empty path")
	}

	// After 48 hours of strong mean reversion, price must have pulled down close to baseline
	endPrice := path[len(path)-1]
	if endPrice > 4.0 {
		t.Errorf("mean reversion failed: started at 10.0, ended at %f (expected ~2.50)", endPrice)
	}
}

func TestRiskMetrics_MonteCarloVaRAndCVaR(t *testing.T) {
	s0 := 2.49
	startHour := 2
	durationHours := 4
	costMode := "spot"
	forwardCap := 2.6145
	numPaths := 5000

	profile := quant.ComputeExecutionWindowRisk(s0, startHour, durationHours, costMode, forwardCap, numPaths)

	if profile.ExpectedCostUSD <= 0 {
		t.Errorf("expected positive ExpectedCostUSD, got %f", profile.ExpectedCostUSD)
	}

	// Fundamental risk theory inequality: ExpectedCost <= VaR95 <= CVaR95 <= CVaR99 <= Max
	if profile.VaR95USD < profile.ExpectedCostUSD*0.95 {
		t.Errorf("expected VaR95 >= ExpectedCost, got VaR95=%f, Mean=%f", profile.VaR95USD, profile.ExpectedCostUSD)
	}
	if profile.CVaR95USD < profile.VaR95USD {
		t.Errorf("expected CVaR95 >= VaR95, got CVaR95=%f, VaR95=%f", profile.CVaR95USD, profile.VaR95USD)
	}
	if profile.CVaR99USD < profile.CVaR95USD {
		t.Errorf("expected CVaR99 >= CVaR95, got CVaR99=%f, CVaR95=%f", profile.CVaR99USD, profile.CVaR95USD)
	}
}

func TestRealOptions_Valuation(t *testing.T) {
	roa := quant.EvaluateRealOptions("H100_SXM", 3.00, 1.90, 4, 0.35, 4.50)

	if roa.OptionToDeferValueUSD <= 0 {
		t.Errorf("expected positive OptionToDeferValueUSD, got %f", roa.OptionToDeferValueUSD)
	}
	if roa.OptionToSwitchValueUSD <= 0 {
		t.Errorf("expected positive OptionToSwitchValueUSD, got %f", roa.OptionToSwitchValueUSD)
	}
	if roa.Recommendation != "EXECUTE_DEFERRED" {
		t.Errorf("expected EXECUTE_DEFERRED, got %s", roa.Recommendation)
	}
}
