package quant_test

import (
	"math"
	"testing"

	"github.com/fabith10/synapse-go/internal/agent/pricing/quant"
)

func TestBoltzmannNash_DistributionAndEntropy(t *testing.T) {
	// 24-hour tariff profile
	costs := []float64{
		2.1, 1.9, 1.8, 1.7, 1.6, 1.7, 2.0, 2.5, 3.2, 3.8, 4.0, 4.2,
		4.1, 3.9, 3.7, 3.5, 3.4, 3.6, 4.0, 3.8, 3.2, 2.8, 2.4, 2.2,
	}

	res := quant.ComputeBoltzmannNashDistribution(costs, 2.5)

	if len(res.ProbMassFunction) != 24 {
		t.Fatalf("expected 24 probabilities in PMF, got %d", len(res.ProbMassFunction))
	}

	sumP := 0.0
	for _, p := range res.ProbMassFunction {
		sumP += p
	}
	if math.Abs(sumP-1.0) > 0.01 {
		t.Errorf("expected PMF sum ~ 1.0, got %f", sumP)
	}

	if res.ShannonEntropy <= 0 || res.ShannonEntropy > res.MaxEntropy {
		t.Errorf("invalid Shannon entropy: %f (max: %f)", res.ShannonEntropy, res.MaxEntropy)
	}

	if res.MixedStrategySampled < 0 || res.MixedStrategySampled >= 24 {
		t.Errorf("sampled hour out of range: %d", res.MixedStrategySampled)
	}
}

func TestMinorityGame_CrowdPenalty(t *testing.T) {
	costs := []float64{
		2.1, 1.9, 1.8, 1.7, 1.6, 1.7, 2.0, 2.5, 3.2, 3.8, 4.0, 4.2,
		4.1, 3.9, 3.7, 3.5, 3.4, 3.6, 4.0, 3.8, 3.2, 2.8, 2.4, 2.2,
	}

	res := quant.EvaluateMinorityGameCrowding(costs, 0.45, 2.0)

	if len(res.CrowdAdjustedCosts) != 24 {
		t.Fatalf("expected 24 adjusted costs, got %d", len(res.CrowdAdjustedCosts))
	}

	// At hour 4 (trough 1.6), crowd density should be high
	if res.EstimatedCrowdLoad[4] <= res.EstimatedCrowdLoad[10] {
		t.Errorf("expected higher crowd load at trough hour 4 than peak hour 10, got h4=%f, h10=%f",
			res.EstimatedCrowdLoad[4], res.EstimatedCrowdLoad[10])
	}
}

func TestColonelBlotto_Allocation(t *testing.T) {
	res := quant.ComputeBlottoAllocation("H100_SXM", 20.0, 2.49)

	if res.TotalBatchHours != 20.0 {
		t.Errorf("expected 20 batch hours, got %f", res.TotalBatchHours)
	}

	if len(res.Allocations) != 3 {
		t.Fatalf("expected 3 cluster allocations, got %d", len(res.Allocations))
	}

	sumWeights := 0.0
	for _, a := range res.Allocations {
		sumWeights += a.AllocationWeightPct
	}
	if math.Abs(sumWeights-100.0) > 0.01 {
		t.Errorf("expected allocation weights to sum to 100%%, got %f%%", sumWeights)
	}

	if res.BlottoJointFailureRiskPct >= res.SingleClusterRiskPct {
		t.Errorf("expected joint risk (%f%%) to be lower than single cluster risk (%f%%)",
			res.BlottoJointFailureRiskPct, res.SingleClusterRiskPct)
	}
}
