package pricing

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPricingOracleDynamicSwitching(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "pricing_providers.json")

	mgr := NewPricingOracleManager(configPath)
	defer mgr.SetActiveProvider("mock")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 1. Default mock provider
	if prov := mgr.GetActiveProviderName(); prov != "mock" {
		t.Fatalf("expected default provider mock, got %s", prov)
	}

	// 2. Dynamic switch to static_json (built-in adapter, always available)
	if err := mgr.SetActiveProvider("static_json"); err != nil {
		t.Fatalf("failed to set active provider: %v", err)
	}

	if prov := mgr.GetActiveProviderName(); prov != "static_json" {
		t.Errorf("expected active provider static_json, got %s", prov)
	}

	// Execute static_json query — returns the baseline matrix (no file needed)
	res, err := mgr.ExecuteQuery(ctx, "H100_SXM", "spot", "", nil)
	if err != nil {
		t.Fatalf("unexpected query error: %v", err)
	}
	var resMap map[string]interface{}
	json.Unmarshal([]byte(res), &resMap)
	if resMap["status"] != "success" {
		t.Errorf("expected status=success in output, got %v", resMap)
	}

	// 3. Environment Variable Override without recompilation
	t.Setenv("PRICING_PROVIDER", "mock")
	if prov := mgr.GetActiveProviderName(); prov != "mock" {
		t.Errorf("expected env override mock, got %s", prov)
	}

	// 4. Persistence check: verify saved pricing_providers.json
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read persisted config file: %v", err)
	}
	if !bytes.Contains(data, []byte(`"active_provider": "static_json"`)) {
		t.Errorf("expected persisted config to contain active_provider static_json, got %s", string(data))
	}
}

func TestPricingOracle_ExecutionWindow(t *testing.T) {
	mgr := NewPricingOracleManager("")
	ctx := context.Background()

	// 1. Default duration (4h)
	t.Run("default_duration", func(t *testing.T) {
		res, err := mgr.ExecuteQuery(ctx, "H100_SXM", "execution_window", "", nil)
		if err != nil {
			t.Fatalf("unexpected execution_window query error: %v", err)
		}

		var resp map[string]interface{}
		if err := json.Unmarshal([]byte(res), &resp); err != nil {
			t.Fatalf("invalid JSON response: %v", err)
		}

		if resp["status"] != "success" {
			t.Errorf("expected status=success, got %v", resp["status"])
		}

		optimal, ok := resp["optimal_window"].(map[string]interface{})
		if !ok {
			t.Fatal("expected optimal_window to be an object")
		}
		if optimal["start_hour"] == nil {
			t.Error("expected start_hour in optimal_window")
		}
	})

	// 2. Custom duration (8h)
	t.Run("custom_duration_8h", func(t *testing.T) {
		res, err := mgr.ExecuteQuery(ctx, "A100_80GB", "execution_window", "", map[string]string{
			"duration_hours": "8",
		})
		if err != nil {
			t.Fatalf("unexpected execution_window query error: %v", err)
		}

		var resp map[string]interface{}
		json.Unmarshal([]byte(res), &resp)

		if resp["duration_hours"].(float64) != 8 {
			t.Errorf("expected duration_hours=8, got %v", resp["duration_hours"])
		}
	})

	// 3. Hedged Spot Mode
	t.Run("hedged_spot_mode", func(t *testing.T) {
		res, err := mgr.ExecuteQuery(ctx, "H100_SXM", "execution_window", "", map[string]string{
			"duration_hours": "4",
			"cost_mode":      "hedged_spot",
			"forward_rate":   "2.20",
		})
		if err != nil {
			t.Fatalf("unexpected execution_window query error: %v", err)
		}

		var resp map[string]interface{}
		json.Unmarshal([]byte(res), &resp)

		if resp["cost_mode"] != "hedged_spot" {
			t.Errorf("expected cost_mode=hedged_spot, got %v", resp["cost_mode"])
		}

		optimal := resp["optimal_window"].(map[string]interface{})
		optCost := optimal["total_cost_usd"].(float64)
		if optCost > 8.80 {
			t.Errorf("expected capped optimal cost to be <= 8.80, got %.4f", optCost)
		}
	})

	// 4. Risk Adjusted Mode
	t.Run("risk_adjusted_mode", func(t *testing.T) {
		res, err := mgr.ExecuteQuery(ctx, "H100_SXM", "execution_window", "", map[string]string{
			"duration_hours": "4",
			"cost_mode":      "risk_adjusted",
		})
		if err != nil {
			t.Fatalf("unexpected execution_window query error: %v", err)
		}

		var resp map[string]interface{}
		json.Unmarshal([]byte(res), &resp)

		if resp["cost_mode"] != "risk_adjusted" {
			t.Errorf("expected cost_mode=risk_adjusted, got %v", resp["cost_mode"])
		}

		optimal := resp["optimal_window"].(map[string]interface{})
		startHour := int(optimal["start_hour"].(float64))

		// Due to the volatility spikes in hours 1 to 4 UTC (implied vol > 70%),
		// the optimal execution window should NOT start between 1 and 4 UTC,
		// even though those hours have the absolute lowest raw spot rates.
		if startHour >= 1 && startHour <= 4 {
			t.Errorf("expected optimal start hour in risk_adjusted mode to shift away from vol spike hours 1-4, got %d", startHour)
		}
	})
}

func TestOpenWeightPromptCostQuery(t *testing.T) {
	mgr := NewPricingOracleManager("")
	_ = mgr.SetActiveProvider("mock")
	ctx := context.Background()

	// Query prompt_cost for llama-3-70b with 50,000 input tokens and 10,000 output tokens
	res, err := mgr.ExecuteQuery(ctx, "llama-3-70b", "prompt_cost", "", map[string]string{
		"input_tokens":  "50000",
		"output_tokens": "10000",
	})
	if err != nil {
		t.Fatalf("unexpected prompt_cost query error: %v", err)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(res), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}

	if resp["status"] != "success" {
		t.Errorf("expected status=success, got %v", resp["status"])
	}
	if resp["model_asset"] != "llama-3-70b" {
		t.Errorf("expected model_asset=llama-3-70b, got %v", resp["model_asset"])
	}

	costUSD, ok := resp["estimated_prompt_cost_usd"].(float64)
	if !ok || costUSD <= 0 {
		t.Errorf("expected positive estimated_prompt_cost_usd, got %v", resp["estimated_prompt_cost_usd"])
	}

	optimal, ok := resp["optimal_window"].(map[string]interface{})
	if !ok || optimal["start_hour"] == nil {
		t.Errorf("expected optimal_window object in prompt_cost query response")
	}
}

func TestExtractIVFromOptions(t *testing.T) {
	spot := 2.50

	t.Run("explicit_iv", func(t *testing.T) {
		jsonRaw := `{
			"status": "success",
			"options": [
				{"type": "call", "strike": 2.50, "implied_volatility": 0.42, "expiration": "30d"},
				{"type": "put", "strike": 2.50, "implied_volatility": 0.44, "expiration": "30d"}
			]
		}`
		iv := extractIVFromOptions(jsonRaw, spot)
		if iv < 0.42 || iv > 0.44 {
			t.Errorf("expected extracted IV around 0.43, got %.4f", iv)
		}
	})

	t.Run("derived_iv_from_premium", func(t *testing.T) {
		jsonRaw := `{
			"status": "success",
			"options": [
				{"type": "call", "strike": 2.50, "bid": 0.19, "ask": 0.21, "expiration": "30d"}
			]
		}`
		iv := extractIVFromOptions(jsonRaw, spot)
		if iv <= 0 || iv > 2.0 {
			t.Errorf("expected valid derived IV, got %.4f", iv)
		}
	})
}

func TestNewtonRaphsonBlackScholesIV(t *testing.T) {
	S := 100.0
	T := 30.0 / 365.0
	r := 0.02
	targetSigma := 0.35

	// Test Call option IV recovery
	callPrice, _ := BlackScholesPrice(true, S, 100.0, T, r, targetSigma)
	solvedCallIV := ImpliedVolatilityBS(true, S, 100.0, T, r, callPrice)
	if math.Abs(solvedCallIV-targetSigma) > 0.001 {
		t.Errorf("expected Call IV %.4f, got %.4f", targetSigma, solvedCallIV)
	}

	// Test Put option IV recovery
	putPrice, _ := BlackScholesPrice(false, S, 95.0, T, r, targetSigma)
	solvedPutIV := ImpliedVolatilityBS(false, S, 95.0, T, r, putPrice)
	if math.Abs(solvedPutIV-targetSigma) > 0.001 {
		t.Errorf("expected Put IV %.4f, got %.4f", targetSigma, solvedPutIV)
	}
}

func TestRiskAdjustedMode_OptionsFallback(t *testing.T) {
	fakeAdapter := &mockFallbackAdapter{}
	mgr := NewPricingOracleManager("")
	mgr.RegisterAdapter("mock_fallback", fakeAdapter)
	_ = mgr.SetActiveProvider("mock_fallback")
	defer mgr.SetActiveProvider("mock")

	ctx := context.Background()
	res, err := mgr.ExecuteQuery(ctx, "H100_SXM", "execution_window", "", map[string]string{
		"cost_mode": "risk_adjusted",
	})
	if err != nil {
		t.Fatalf("execution_window query failed: %v", err)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(res), &resp); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if resp["status"] != "success" {
		t.Errorf("expected status=success, got %v", resp["status"])
	}
}

type mockFallbackAdapter struct{}

func (a *mockFallbackAdapter) Name() string { return "mock_fallback" }
func (a *mockFallbackAdapter) Query(ctx context.Context, q PricingQuery) (*PricingResult, error) {
	if q.MarketType == "vol_surface" {
		return nil, os.ErrNotExist
	}
	if q.MarketType == "options" {
		raw := `{"status":"success","options":[{"type":"call","strike":2.49,"implied_volatility":0.45}]}`
		return buildResult("mock_fallback", q.Asset, q.MarketType, raw), nil
	}
	raw := `{"status":"success","price":2.49}`
	return buildResult("mock_fallback", q.Asset, q.MarketType, raw), nil
}

func TestPricingOracle_FreeLiveIntegration(t *testing.T) {
	mgr := NewPricingOracleManager("")
	_ = mgr.SetActiveProvider("free_live")
	defer mgr.SetActiveProvider("mock")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 1. LLM Prompt cost with free_live
	promptRes, err := mgr.ExecutePromptCostQuery(ctx, "deepseek-r1", "", map[string]string{
		"input_tokens":  "10000",
		"output_tokens": "2000",
	})
	if err != nil {
		t.Fatalf("ExecutePromptCostQuery failed on free_live: %v", err)
	}
	var promptMap map[string]interface{}
	if err := json.Unmarshal([]byte(promptRes), &promptMap); err != nil {
		t.Fatalf("unmarshal prompt response failed: %v", err)
	}
	if promptMap["status"] != "success" {
		t.Errorf("expected status success, got %v", promptMap["status"])
	}

	// 2. Execution window with hedged_spot on free_live
	windowRes, err := mgr.ExecuteExecutionWindowQuery(ctx, "H100_SXM", "", map[string]string{
		"cost_mode": "hedged_spot",
	})
	if err != nil {
		t.Fatalf("ExecuteExecutionWindowQuery failed on free_live: %v", err)
	}
	var windowMap map[string]interface{}
	if err := json.Unmarshal([]byte(windowRes), &windowMap); err != nil {
		t.Fatalf("unmarshal window response failed: %v", err)
	}
	if windowMap["status"] != "success" {
		t.Errorf("expected status success, got %v", windowMap["status"])
	}
}

func TestPricingOracle_QuantDerivativesAndRiskMetrics(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "pricing_providers.json")
	cfgData := `{
		"active_provider": "mock",
		"providers": {
			"mock": { "type": "mock" }
		}
	}`
	if err := os.WriteFile(configPath, []byte(cfgData), 0600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	mgr := NewPricingOracleManager(configPath)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 1. Black-76 Options Query
	optRaw, err := mgr.ExecuteQuery(ctx, "H100_SXM", "options", "", map[string]string{
		"strike":     "2.60",
		"days":       "30",
		"volatility": "0.35",
	})
	if err != nil {
		t.Fatalf("options query failed: %v", err)
	}
	var optMap map[string]interface{}
	if err := json.Unmarshal([]byte(optRaw), &optMap); err != nil {
		t.Fatalf("unmarshal options failed: %v", err)
	}
	if optMap["status"] != "success" {
		t.Errorf("expected status success, got %v", optMap["status"])
	}
	if optMap["model"] != "Black-76 Commodity Option & Greeks Engine" {
		t.Errorf("unexpected model name: %v", optMap["model"])
	}
	if parity, ok := optMap["put_call_parity"].(bool); !ok || !parity {
		t.Errorf("expected put_call_parity to be true")
	}

	// 2. Risk Metrics Query (Monte Carlo VaR & CVaR)
	riskRaw, err := mgr.ExecuteQuery(ctx, "H100_SXM", "risk_metrics", "", map[string]string{
		"start_hour": "2",
		"duration":   "4",
		"cost_mode":  "spot",
	})
	if err != nil {
		t.Fatalf("risk_metrics query failed: %v", err)
	}
	var riskMap map[string]interface{}
	if err := json.Unmarshal([]byte(riskRaw), &riskMap); err != nil {
		t.Fatalf("unmarshal risk_metrics failed: %v", err)
	}
	if riskMap["status"] != "success" {
		t.Errorf("expected status success, got %v", riskMap["status"])
	}

	// 3. Real Options Analysis Query
	roaRaw, err := mgr.ExecuteQuery(ctx, "H100_SXM", "real_options", "", map[string]string{
		"duration": "4",
	})
	if err != nil {
		t.Fatalf("real_options query failed: %v", err)
	}
	var roaMap map[string]interface{}
	if err := json.Unmarshal([]byte(roaRaw), &roaMap); err != nil {
		t.Fatalf("unmarshal real_options failed: %v", err)
	}
	if roaMap["status"] != "success" {
		t.Errorf("expected status success, got %v", roaMap["status"])
	}

	// 4. Lucia-Schwartz Two-Factor Model Query
	lsRaw, err := mgr.ExecuteQuery(ctx, "H100_SXM", "lucia_schwartz", "", nil)
	if err != nil {
		t.Fatalf("lucia_schwartz query failed: %v", err)
	}
	var lsMap map[string]interface{}
	if err := json.Unmarshal([]byte(lsRaw), &lsMap); err != nil {
		t.Fatalf("unmarshal lucia_schwartz failed: %v", err)
	}
	if lsMap["status"] != "success" {
		t.Errorf("expected status success, got %v", lsMap["status"])
	}

	// 5. Markov Regime-Switching Model Query
	mrsRaw, err := mgr.ExecuteQuery(ctx, "H100_SXM", "regime_switching", "", map[string]string{"duration": "4"})
	if err != nil {
		t.Fatalf("regime_switching query failed: %v", err)
	}
	var mrsMap map[string]interface{}
	if err := json.Unmarshal([]byte(mrsRaw), &mrsMap); err != nil {
		t.Fatalf("unmarshal regime_switching failed: %v", err)
	}
	if mrsMap["status"] != "success" {
		t.Errorf("expected status success, got %v", mrsMap["status"])
	}

	// 6. Queuing Congestion Model Query
	congRaw, err := mgr.ExecuteQuery(ctx, "H100_SXM", "congestion", "", nil)
	if err != nil {
		t.Fatalf("congestion query failed: %v", err)
	}
	var congMap map[string]interface{}
	if err := json.Unmarshal([]byte(congRaw), &congMap); err != nil {
		t.Fatalf("unmarshal congestion failed: %v", err)
	}
	if congMap["status"] != "success" {
		t.Errorf("expected status success, got %v", congMap["status"])
	}

	// 7. Game Theory Multi-Cluster & Anti-Herding Query
	gtRaw, err := mgr.ExecuteQuery(ctx, "H100_SXM", "game_theory", "", map[string]string{"duration": "4"})
	if err != nil {
		t.Fatalf("game_theory query failed: %v", err)
	}
	var gtMap map[string]interface{}
	if err := json.Unmarshal([]byte(gtRaw), &gtMap); err != nil {
		t.Fatalf("unmarshal game_theory failed: %v", err)
	}
	if gtMap["status"] != "success" {
		t.Errorf("expected status success, got %v", gtMap["status"])
	}
	if gtMap["model"] != "Game-Theoretic Anti-Herding & Multi-Cluster Suite" {
		t.Errorf("unexpected game theory model name: %v", gtMap["model"])
	}

	// 8. Compute Pricing Oracle Backtest Query (Synthetic)
	btRaw, err := mgr.ExecuteQuery(ctx, "H100_SXM", "backtest", "", map[string]string{
		"days":   "14",
		"format": "json",
	})
	if err != nil {
		t.Fatalf("backtest query failed: %v", err)
	}
	var btMap map[string]interface{}
	if err := json.Unmarshal([]byte(btRaw), &btMap); err != nil {
		t.Fatalf("unmarshal backtest failed: %v", err)
	}
	if btMap["asset"] != "H100_SXM" {
		t.Errorf("expected asset H100_SXM in backtest, got %v", btMap["asset"])
	}
	if btMap["total_hours"] != float64(336) {
		t.Errorf("expected 336 total hours, got %v", btMap["total_hours"])
	}

	// 9. Empirical Dataset Backtest Query (Real AWS Spot Dataset)
	btEmpRaw, err := mgr.ExecuteQuery(ctx, "g5.xlarge", "backtest", "", map[string]string{
		"dataset": "aws_g5_xlarge",
		"format":  "json",
	})
	if err != nil {
		t.Fatalf("empirical backtest query failed: %v", err)
	}
	var btEmpMap map[string]interface{}
	if err := json.Unmarshal([]byte(btEmpRaw), &btEmpMap); err != nil {
		t.Fatalf("unmarshal empirical backtest failed: %v", err)
	}
	if btEmpMap["total_hours"] != float64(720) {
		t.Errorf("expected 720 hours for aws_g5_xlarge empirical backtest, got %v", btEmpMap["total_hours"])
	}
}

