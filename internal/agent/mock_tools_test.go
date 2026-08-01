package agent_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fabith10/synapse-go/internal/agent"
	"github.com/fabith10/synapse-go/internal/agent/pricing"
	"github.com/fabith10/synapse-go/internal/web"
)

func TestMockToolsWithServer(t *testing.T) {
	// Start test HTTP server with web.Server handler
	webServer := web.NewServer(nil)
	ts := httptest.NewServer(webServer)
	defer ts.Close()

	// Direct tools to point to test HTTP server
	agent.SetMockServerBaseURL(ts.URL)
	defer agent.SetMockServerBaseURL("http://localhost:8080")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Query Compute Prices
	t.Run("query_compute_prices", func(t *testing.T) {
		tool := agent.GetQueryComputePricesTool()
		args, _ := json.Marshal(map[string]interface{}{"provider": "runpod", "gpu_type": "H100"})
		res, err := tool.Execute(ctx, args)
		if err != nil {
			t.Fatalf("unexpected error executing query_compute_prices: %v", err)
		}
		var resMap map[string]interface{}
		if err := json.Unmarshal([]byte(res), &resMap); err != nil {
			t.Fatalf("failed to parse output: %v", err)
		}
		if resMap["status"] != "success" {
			t.Errorf("expected status success, got %v", resMap["status"])
		}
	})

	// 2. Query Forward Curves
	t.Run("query_forward_curves", func(t *testing.T) {
		tool := agent.GetQueryForwardCurvesTool()
		args, _ := json.Marshal(map[string]interface{}{"asset": "H100_SXM"})
		res, err := tool.Execute(ctx, args)
		if err != nil {
			t.Fatalf("unexpected error executing query_forward_curves: %v", err)
		}
		var resMap map[string]interface{}
		json.Unmarshal([]byte(res), &resMap)
		if resMap["asset"] != "H100_SXM" {
			t.Errorf("expected asset H100_SXM, got %v", resMap["asset"])
		}
	})

	// 3. Query Options Chain
	t.Run("query_options_chain", func(t *testing.T) {
		tool := agent.GetQueryOptionsChainTool()
		args, _ := json.Marshal(map[string]interface{}{"asset": "H100_SXM", "expiration": "30d"})
		res, err := tool.Execute(ctx, args)
		if err != nil {
			t.Fatalf("unexpected error executing query_options_chain: %v", err)
		}
		var resMap map[string]interface{}
		json.Unmarshal([]byte(res), &resMap)
		if resMap["expiration"] != "30d" {
			t.Errorf("expected expiration 30d, got %v", resMap["expiration"])
		}
	})

	// 4. Submit Mock Task & Check Task Status
	t.Run("submit_and_check_mock_task", func(t *testing.T) {
		submitTool := agent.GetSubmitMockTaskTool()
		submitArgs, _ := json.Marshal(map[string]interface{}{
			"task_type": "model_training",
			"payload":   map[string]interface{}{"epochs": 10},
			"priority":  "high",
		})

		submitRes, err := submitTool.Execute(ctx, submitArgs)
		if err != nil {
			t.Fatalf("unexpected error submitting task: %v", err)
		}

		var submitMap map[string]interface{}
		json.Unmarshal([]byte(submitRes), &submitMap)

		taskData, ok := submitMap["task"].(map[string]interface{})
		if !ok {
			t.Fatalf("task key missing in response: %v", submitMap)
		}
		taskID := taskData["id"].(string)

		checkTool := agent.GetCheckMockTaskTool()
		checkArgs, _ := json.Marshal(map[string]interface{}{"task_id": taskID})
		checkRes, err := checkTool.Execute(ctx, checkArgs)
		if err != nil {
			t.Fatalf("unexpected error checking task: %v", err)
		}

		var checkMap map[string]interface{}
		json.Unmarshal([]byte(checkRes), &checkMap)
		if checkMap["status"] != "success" {
			t.Errorf("expected check_mock_task status success, got %v", checkMap["status"])
		}
	})

	// 5. Query Pricing Oracle Integration
	t.Run("query_pricing_oracle_integration", func(t *testing.T) {
		// Ensure the global oracle uses the mock adapter regardless of any
		// pricing_providers.json on disk that may reference an unregistered provider.
		_ = pricing.GetPricingOracleManager().SetActiveProvider("mock")

		oracleTool := agent.GetQueryPricingOracleTool()

		// Spot
		spotArgs, _ := json.Marshal(map[string]interface{}{"asset_or_instance": "H100_SXM", "market_type": "spot"})
		spotRes, err := oracleTool.Execute(ctx, spotArgs)
		if err != nil {
			t.Fatalf("unexpected error on spot query: %v", err)
		}
		var spotMap map[string]interface{}
		json.Unmarshal([]byte(spotRes), &spotMap)
		if spotMap["status"] != "success" {
			t.Errorf("expected spot query success, got %v", spotMap)
		}

		// Futures / Forwards
		futuresArgs, _ := json.Marshal(map[string]interface{}{"asset_or_instance": "H100_SXM", "market_type": "futures"})
		futuresRes, err := oracleTool.Execute(ctx, futuresArgs)
		if err != nil {
			t.Fatalf("unexpected error on futures query: %v", err)
		}
		var futuresMap map[string]interface{}
		json.Unmarshal([]byte(futuresRes), &futuresMap)
		if futuresMap["status"] != "success" {
			t.Errorf("expected futures query success, got %v", futuresMap)
		}

		// Options
		optionsArgs, _ := json.Marshal(map[string]interface{}{"asset_or_instance": "H100_SXM", "market_type": "options"})
		optionsRes, err := oracleTool.Execute(ctx, optionsArgs)
		if err != nil {
			t.Fatalf("unexpected error on options query: %v", err)
		}
		var optionsMap map[string]interface{}
		json.Unmarshal([]byte(optionsRes), &optionsMap)
		if optionsMap["status"] != "success" {
			t.Errorf("expected options query success, got %v", optionsMap)
		}
	})
}

