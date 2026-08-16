package adapter_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fabith10/synapse-go/internal/agent/pricing/adapter"
	"github.com/fabith10/synapse-go/internal/agent/pricing/types"
)

func TestOpenRouterAdapter(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"id": "deepseek/deepseek-r1",
					"pricing": map[string]string{
						"prompt":     "0.00000055",
						"completion": "0.00000219",
					},
				},
				{
					"id": "meta-llama/llama-3.3-70b-instruct",
					"pricing": map[string]string{
						"prompt":     "0.00000012",
						"completion": "0.00000030",
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	ad := adapter.NewOpenRouterPricingAdapter(mockServer.URL)
	if ad.Name() != "openrouter" {
		t.Fatalf("expected name 'openrouter', got %s", ad.Name())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res, err := ad.Query(ctx, types.PricingQuery{
		Asset:      "deepseek-r1",
		MarketType: "spot",
	})
	if err != nil {
		t.Fatalf("unexpected query error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(res.Raw), &parsed); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if parsed["status"] != "success" {
		t.Errorf("expected status success, got %v", parsed["status"])
	}
	if inCost, ok := parsed["input_cost_per_1m"].(float64); !ok || inCost <= 0 {
		t.Errorf("expected positive input_cost_per_1m, got %v", parsed["input_cost_per_1m"])
	}
	if outCost, ok := parsed["output_cost_per_1m"].(float64); !ok || outCost <= 0 {
		t.Errorf("expected positive output_cost_per_1m, got %v", parsed["output_cost_per_1m"])
	}
}

func TestVastAIAdapter(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"offers": []map[string]interface{}{
				{"gpu_name": "RTX 4090", "dph_total": 0.42, "num_gpus": 1, "rentable": true},
				{"gpu_name": "RTX 4090", "dph_total": 0.38, "num_gpus": 1, "rentable": true},
				{"gpu_name": "NVIDIA A100-SXM4-80GB", "dph_total": 1.25, "num_gpus": 1, "rentable": true},
				{"gpu_name": "NVIDIA H100 80GB HBM3", "dph_total": 2.45, "num_gpus": 1, "rentable": true},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	ad := adapter.NewVastAIPricingAdapter(mockServer.URL)
	if ad.Name() != "vastai" {
		t.Fatalf("expected name 'vastai', got %s", ad.Name())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res, err := ad.Query(ctx, types.PricingQuery{
		Asset:      "RTX_4090",
		MarketType: "spot",
	})
	if err != nil {
		t.Fatalf("unexpected query error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(res.Raw), &parsed); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if parsed["status"] != "success" {
		t.Errorf("expected status success, got %v", parsed["status"])
	}
	if matrix, ok := parsed["matrix"].(map[string]interface{}); !ok || len(matrix) == 0 {
		t.Errorf("expected matrix in response, got %v", parsed["matrix"])
	}
}

func TestDeribitAdapter(t *testing.T) {
	mockVolServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"result": [][]interface{}{
				{1700000000000, 48.5},
				{1700000060000, 52.0},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockVolServer.Close()

	ad := adapter.NewDeribitPricingAdapter("", mockVolServer.URL)
	if ad.Name() != "deribit" {
		t.Fatalf("expected name 'deribit', got %s", ad.Name())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Test Futures forward curve
	res, err := ad.Query(ctx, types.PricingQuery{
		Asset:      "H100_SXM",
		MarketType: "futures",
	})
	if err != nil {
		t.Fatalf("unexpected futures query error: %v", err)
	}

	var parsedFutures map[string]interface{}
	if err := json.Unmarshal([]byte(res.Raw), &parsedFutures); err != nil {
		t.Fatalf("failed to unmarshal futures JSON: %v", err)
	}
	if forward, ok := parsedFutures["forward_rate_usd"].(float64); !ok || forward <= 0 {
		t.Errorf("expected positive forward_rate_usd, got %v", parsedFutures["forward_rate_usd"])
	}

	// Test Vol surface
	volRes, err := ad.Query(ctx, types.PricingQuery{
		Asset:      "H100_SXM",
		MarketType: "vol_surface",
	})
	if err != nil {
		t.Fatalf("unexpected vol query error: %v", err)
	}

	var parsedVol map[string]interface{}
	if err := json.Unmarshal([]byte(volRes.Raw), &parsedVol); err != nil {
		t.Fatalf("failed to unmarshal vol JSON: %v", err)
	}
	if atmVol, ok := parsedVol["atm_vol_30d"].(float64); !ok || atmVol <= 0 {
		t.Errorf("expected positive atm_vol_30d, got %v", parsedVol["atm_vol_30d"])
	}
}

func TestFreeCompositeAdapter(t *testing.T) {
	comp := adapter.NewFreeCompositePricingAdapter()
	if comp.Name() != "free_live" {
		t.Fatalf("expected name 'free_live', got %s", comp.Name())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 1. LLM Model Query -> Should return token rates + spot matrix
	resModel, err := comp.Query(ctx, types.PricingQuery{
		Asset:      "deepseek-r1",
		MarketType: "spot",
	})
	if err != nil {
		t.Fatalf("unexpected composite model query error: %v", err)
	}
	if !strings.Contains(resModel.Raw, "deepseek-r1") {
		t.Errorf("expected raw to contain deepseek-r1, got %s", resModel.Raw)
	}

	// 2. GPU Hardware Query -> Should return GPU spot rate
	resGPU, err := comp.Query(ctx, types.PricingQuery{
		Asset:      "RTX_4090",
		MarketType: "spot",
	})
	if err != nil {
		t.Fatalf("unexpected composite GPU query error: %v", err)
	}
	if !strings.Contains(resGPU.Raw, "4090") {
		t.Errorf("expected raw to contain 4090, got %s", resGPU.Raw)
	}

	// 3. Futures Query -> Should return forward rate
	resFutures, err := comp.Query(ctx, types.PricingQuery{
		Asset:      "H100_SXM",
		MarketType: "futures",
	})
	if err != nil {
		t.Fatalf("unexpected composite futures query error: %v", err)
	}
	if !strings.Contains(resFutures.Raw, "forward_rate_usd") {
		t.Errorf("expected raw to contain forward_rate_usd, got %s", resFutures.Raw)
	}
}
