package agenttools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	agenttools "github.com/fabith10/synapse-go/internal/agent/tools"
)

func TestManageHedgeContractTool(t *testing.T) {
	tool := agenttools.GetManageHedgeContractTool()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 1. Enter forward contract
	fwdArgs := `{"action": "enter_forward", "asset": "H100_SXM", "forward_rate": 2.55, "duration_hours": 8}`
	res1, err := tool.Execute(ctx, []byte(fwdArgs))
	if err != nil {
		t.Fatalf("enter_forward failed: %v", err)
	}
	var map1 map[string]interface{}
	if err := json.Unmarshal([]byte(res1), &map1); err != nil {
		t.Fatalf("unmarshal res1 failed: %v", err)
	}
	if map1["status"] != "success" {
		t.Errorf("expected status success, got %v", map1["status"])
	}

	// 2. Buy call option
	optArgs := `{"action": "buy_call_option", "asset": "RTX_4090", "strike_rate": 0.50, "duration_hours": 4}`
	res2, err := tool.Execute(ctx, []byte(optArgs))
	if err != nil {
		t.Fatalf("buy_call_option failed: %v", err)
	}
	var map2 map[string]interface{}
	if err := json.Unmarshal([]byte(res2), &map2); err != nil {
		t.Fatalf("unmarshal res2 failed: %v", err)
	}
	if map2["status"] != "success" {
		t.Errorf("expected status success, got %v", map2["status"])
	}

	// 3. List positions
	listArgs := `{"action": "list_positions"}`
	res3, err := tool.Execute(ctx, []byte(listArgs))
	if err != nil {
		t.Fatalf("list_positions failed: %v", err)
	}
	var map3 map[string]interface{}
	if err := json.Unmarshal([]byte(res3), &map3); err != nil {
		t.Fatalf("unmarshal res3 failed: %v", err)
	}
	totalHedges, ok := map3["total_hedges"].(float64)
	if !ok || totalHedges < 2 {
		t.Errorf("expected at least 2 open hedges, got %v", map3["total_hedges"])
	}
}

func TestRunPricingBacktestTool(t *testing.T) {
	tool := agenttools.GetRunPricingBacktestTool()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Markdown backtest execution
	mdArgs := `{"asset": "H100_SXM", "days": 14, "model": "ou_jump_diffusion", "format": "markdown"}`
	res1, err := tool.Execute(ctx, []byte(mdArgs))
	if err != nil {
		t.Fatalf("run_pricing_backtest markdown failed: %v", err)
	}
	if len(res1) == 0 || res1[0] != '#' {
		t.Errorf("expected markdown report starting with #, got %s", res1)
	}

	// 2. JSON backtest execution
	jsonArgs := `{"asset": "A100_80GB", "days": 7, "format": "json"}`
	res2, err := tool.Execute(ctx, []byte(jsonArgs))
	if err != nil {
		t.Fatalf("run_pricing_backtest json failed: %v", err)
	}
	var resMap map[string]interface{}
	if err := json.Unmarshal([]byte(res2), &resMap); err != nil {
		t.Fatalf("unmarshal backtest json failed: %v", err)
	}
	if resMap["asset"] != "A100_80GB" {
		t.Errorf("expected asset A100_80GB, got %v", resMap["asset"])
	}

	// 3. Empirical historical dataset execution (AWS G5 xlarge)
	empArgs := `{"dataset": "aws_g5_xlarge", "format": "markdown"}`
	res3, err := tool.Execute(ctx, []byte(empArgs))
	if err != nil {
		t.Fatalf("run_pricing_backtest empirical dataset failed: %v", err)
	}
	if !strings.Contains(res3, "AWS EC2 Spot: g5.xlarge") {
		t.Errorf("expected empirical dataset name in markdown output, got %s", res3)
	}
}

