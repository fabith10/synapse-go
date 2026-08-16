package adapter_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fabith10/synapse-go/internal/agent/pricing/adapter"
	"github.com/fabith10/synapse-go/internal/agent/pricing/types"
)

func TestDeclarativeHTTPAdapter_ObjectPathAndHeaders(t *testing.T) {
	t.Setenv("TEST_API_SECRET", "super_secret_token_123")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify custom header expansion
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer super_secret_token_123" {
			t.Errorf("expected expanded Authorization header, got: %q", authHeader)
		}

		resp := map[string]interface{}{
			"rates": map[string]interface{}{
				"H100_SXM":  2.85,
				"RTX_4090":  0.42,
				"A100_80GB": 1.25,
			},
			"volatility": map[string]interface{}{
				"dvol_index": 0.55,
			},
			"futures": map[string]interface{}{
				"30d_mark": 2.95,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := adapter.CustomHTTPAdapterConfig{
		Name:    "declarative_cloud",
		BaseURL: server.URL,
		Headers: map[string]string{
			"Authorization": "Bearer ${TEST_API_SECRET}",
		},
		Mappings: adapter.DeclarativeMapping{
			SpotRatePath:    "rates.{{asset}}",
			ForwardRatePath: "futures.30d_mark",
			ImpliedVolPath:  "volatility.dvol_index",
		},
	}

	ad := adapter.NewCustomHTTPAdapter(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res, err := ad.Query(ctx, types.PricingQuery{
		Asset:      "H100_SXM",
		MarketType: "spot",
	})
	if err != nil {
		t.Fatalf("unexpected query error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(res.Raw), &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if parsed["status"] != "success" {
		t.Errorf("expected status success, got %v", parsed["status"])
	}
	if spot, ok := parsed["spot_price"].(float64); !ok || spot != 2.85 {
		t.Errorf("expected spot_price 2.85, got %v", parsed["spot_price"])
	}
	if forward, ok := parsed["forward_rate_usd"].(float64); !ok || forward != 2.95 {
		t.Errorf("expected forward_rate_usd 2.95, got %v", parsed["forward_rate_usd"])
	}
	if vol, ok := parsed["atm_vol"].(float64); !ok || vol != 0.55 {
		t.Errorf("expected atm_vol 0.55, got %v", parsed["atm_vol"])
	}
}

func TestDeclarativeHTTPAdapter_ArraySearchAndTokenMultiplier(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"id": "deepseek/deepseek-r1",
					"pricing": map[string]interface{}{
						"prompt":     "0.00000055",
						"completion": "0.00000219",
					},
				},
				{
					"id": "meta-llama/llama-3.3-70b-instruct",
					"pricing": map[string]interface{}{
						"prompt":     "0.00000012",
						"completion": "0.00000030",
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := adapter.CustomHTTPAdapterConfig{
		Name:    "declarative_tokens",
		BaseURL: server.URL,
		Mappings: adapter.DeclarativeMapping{
			InputTokenRatePath:  "data.pricing.prompt",
			OutputTokenRatePath: "data.pricing.completion",
			ArrayMatchKey:       "id",
			TokenRateMultiplier: 1000000.0,
		},
	}

	ad := adapter.NewCustomHTTPAdapter(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res, err := ad.Query(ctx, types.PricingQuery{
		Asset:      "deepseek-r1",
		MarketType: "prompt_cost",
	})
	if err != nil {
		t.Fatalf("unexpected query error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(res.Raw), &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if inCost, ok := parsed["input_cost_per_1m"].(float64); !ok || inCost != 0.55 {
		t.Errorf("expected input_cost_per_1m 0.55, got %v", parsed["input_cost_per_1m"])
	}
	if outCost, ok := parsed["output_cost_per_1m"].(float64); !ok || outCost != 2.19 {
		t.Errorf("expected output_cost_per_1m 2.19, got %v", parsed["output_cost_per_1m"])
	}
}
