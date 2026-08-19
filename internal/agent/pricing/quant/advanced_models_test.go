package quant_test

import (
	"math"
	"math/rand"
	"testing"

	"github.com/fabith10/synapse-go/internal/agent/pricing/quant"
)

func TestLuciaSchwartz_ForwardCurveAndDeflation(t *testing.T) {
	baseSpot := 2.49
	params := quant.DefaultH100LuciaSchwartzParams(baseSpot)

	// 1. Forward price at T=24h (1 day) vs T=8760h (1 year)
	fwd1d := params.ForwardPrice(0, 24.0, 0, 0)
	fwd1y := params.ForwardPrice(0, 8760.0, 0, 0)

	if fwd1d <= 0 || fwd1y <= 0 {
		t.Fatalf("expected positive forward rates, got 1d=%f, 1y=%f", fwd1d, fwd1y)
	}

	// Because mu_Y = -0.25 (25% technological price deflation per year), 1y forward must be cheaper than base spot
	if fwd1y >= baseSpot {
		t.Errorf("expected 1-year forward to exhibit technological deflation (< %f), got %f", baseSpot, fwd1y)
	}

	// 2. Full curve evaluation
	curve := quant.ComputeLuciaSchwartzCurve("H100_SXM", baseSpot)
	if curve.AnnualDeflation <= 0 {
		t.Errorf("expected positive annual deflation rate, got %f", curve.AnnualDeflation)
	}
	if len(curve.TermStructureUSD) < 5 {
		t.Errorf("expected at least 5 term structure points, got %d", len(curve.TermStructureUSD))
	}
}

func TestMarkovRegimeSwitching_BayesianFilterAndSimulation(t *testing.T) {
	baseSpot := 2.49
	params := quant.DefaultH100MRSParams(baseSpot)

	// 1. When observed spot is at calm baseline $2.49 -> Prob(Normal) should be high
	pNorm1, pCong1 := params.FilterRegimeProbabilities(2.49)
	if pNorm1 <= pCong1 {
		t.Errorf("expected pNormal > pCongested for base spot $2.49, got pNorm=%f, pCong=%f", pNorm1, pCong1)
	}

	// 2. When observed spot spikes to $6.50 -> Prob(Congested) should be high
	pNorm2, pCong2 := params.FilterRegimeProbabilities(6.50)
	if pCong2 <= pNorm2 {
		t.Errorf("expected pCongested > pNormal for spike spot $6.50, got pNorm=%f, pCong=%f", pNorm2, pCong2)
	}

	// 3. Path simulation
	r := rand.New(rand.NewSource(42))
	prices, regimes := params.SimulateMRSPath(baseSpot, 24.0, 1, quant.RegimeNormal, r)
	if len(prices) != 25 || len(regimes) != 25 {
		t.Errorf("expected 25 price and regime points, got prices=%d, regimes=%d", len(prices), len(regimes))
	}

	// 4. MRS Evaluation
	profile := quant.EvaluateRegimeSwitchingRisk("H100_SXM", baseSpot, 4, 1000)
	if profile.CurrentRegimeLabel != "NORMAL_LIQUID" {
		t.Errorf("expected NORMAL_LIQUID for baseline spot, got %s", profile.CurrentRegimeLabel)
	}
}

func TestQueuingCongestion_PriceAndInversion(t *testing.T) {
	baseSpot := 2.49
	params := quant.DefaultQueuingParams(baseSpot)

	// 1. As rho -> 0, price -> baseSpot
	priceLow := params.PriceFromUtilization(0.0)
	if math.Abs(priceLow-baseSpot) > 1e-4 {
		t.Errorf("expected price at rho=0 to be %f, got %f", baseSpot, priceLow)
	}

	// 2. As rho increases (e.g. 0.90), price must increase convexly
	priceHigh := params.PriceFromUtilization(0.90)
	if priceHigh <= baseSpot*2.0 {
		t.Errorf("expected high congestion surcharge at rho=0.90, got %f", priceHigh)
	}

	// 3. Inversion test: ImpliedUtilization(PriceFromUtilization(rho)) ~ rho
	testRho := 0.75
	calculatedPrice := params.PriceFromUtilization(testRho)
	inferredRho := params.ImpliedUtilization(calculatedPrice)

	if math.Abs(inferredRho-testRho) > 0.01 {
		t.Errorf("inversion failed: started with rho=%f, calculated price=%f, inferred rho=%f", testRho, calculatedPrice, inferredRho)
	}

	// 4. Analytics result
	analytics := quant.ComputeQueuingCongestionAnalytics("H100_SXM", 4.50)
	if analytics.ImpliedClusterLoadPct <= 50.0 {
		t.Errorf("expected high cluster load for $4.50 spot rate, got %f%%", analytics.ImpliedClusterLoadPct)
	}
}
