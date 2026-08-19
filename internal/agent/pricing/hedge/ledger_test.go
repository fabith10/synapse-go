package hedge_test

import (
	"testing"

	"github.com/fabith10/synapse-go/internal/agent/pricing/hedge"
)

func TestHedgeLedger_ForwardContract(t *testing.T) {
	ledger := hedge.NewHedgeLedger()

	contract, err := ledger.BookForward("H100_SXM", 2.60, 4, 30.0)
	if err != nil {
		t.Fatalf("failed to book forward: %v", err)
	}

	if contract.Status != hedge.StatusActive {
		t.Errorf("expected status active, got %s", contract.Status)
	}
	if contract.StrikeRateUSD != 2.60 {
		t.Errorf("expected strike 2.60, got %f", contract.StrikeRateUSD)
	}

	// Spot spikes to $3.50/hr -> Forward protects rate at $2.60/hr
	effRate, savings, id := ledger.ConsumeHedge("H100_SXM", 2.0, 3.50)
	if id != contract.ID {
		t.Errorf("expected contract ID %s, got %s", contract.ID, id)
	}
	if effRate != 2.60 {
		t.Errorf("expected effective rate 2.60, got %f", effRate)
	}
	expectedSavings := (3.50 - 2.60) * 2.0 // $1.80
	if savings != expectedSavings {
		t.Errorf("expected savings %f, got %f", expectedSavings, savings)
	}

	positions := ledger.ListPositions()
	if len(positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(positions))
	}
	if positions[0].RemainingHours != 2.0 {
		t.Errorf("expected 2.0 remaining hours, got %f", positions[0].RemainingHours)
	}
}

func TestHedgeLedger_CallOptionCeiling(t *testing.T) {
	ledger := hedge.NewHedgeLedger()

	contract, err := ledger.BuyCallOption("RTX_4090", 0.50, 0.05, 4, 30.0)
	if err != nil {
		t.Fatalf("failed to buy call option: %v", err)
	}
	if contract.StrikeRateUSD != 0.50 {
		t.Errorf("expected strike 0.50, got %f", contract.StrikeRateUSD)
	}

	// 1. Spot is $0.40/hr (< Strike $0.50) -> Pay lower spot rate $0.40
	effRate1, savings1, _ := ledger.ConsumeHedge("RTX_4090", 2.0, 0.40)
	if effRate1 != 0.40 {
		t.Errorf("expected effective rate 0.40, got %f", effRate1)
	}
	if savings1 != 0.0 {
		t.Errorf("expected 0 savings, got %f", savings1)
	}

	// 2. Spot spikes to $0.80/hr (> Strike $0.50) -> Exercise option, cap rate at $0.50
	effRate2, savings2, _ := ledger.ConsumeHedge("RTX_4090", 2.0, 0.80)
	if effRate2 != 0.50 {
		t.Errorf("expected effective rate 0.50, got %f", effRate2)
	}
	expectedSavings2 := (0.80 - 0.50) * 2.0 // $0.60
	if savings2 != expectedSavings2 {
		t.Errorf("expected savings %f, got %f", expectedSavings2, savings2)
	}

	// 3. Position should now be closed after consuming all 4 hours
	positions := ledger.ListPositions()
	if positions[0].Status != hedge.StatusClosed {
		t.Errorf("expected status closed after consuming all hours, got %s", positions[0].Status)
	}
}

func TestHedgeLedger_ClosePosition(t *testing.T) {
	ledger := hedge.NewHedgeLedger()
	c, _ := ledger.BookForward("A100_80GB", 1.35, 4, 30)

	if err := ledger.ClosePosition(c.ID); err != nil {
		t.Fatalf("failed to close position: %v", err)
	}

	// After close, consume should pay raw spot
	effRate, savings, id := ledger.ConsumeHedge("A100_80GB", 1.0, 2.00)
	if id != "" {
		t.Errorf("expected no active contract, got %s", id)
	}
	if effRate != 2.00 {
		t.Errorf("expected raw spot rate 2.00, got %f", effRate)
	}
	if savings != 0.0 {
		t.Errorf("expected 0 savings, got %f", savings)
	}
}
