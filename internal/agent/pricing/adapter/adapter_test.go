package adapter_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/fabith10/synapse-go/internal/agent/pricing/adapter"
	"github.com/fabith10/synapse-go/internal/agent/pricing/types"
)

func TestPricingAdapterMock_Fallback(t *testing.T) {
	mockAdapter := adapter.NewMockPricingAdapter("http://127.0.0.1:0")
	ctx := context.Background()

	marketTypes := []string{"spot", "futures", "options", "vol_surface"}
	for _, mt := range marketTypes {
		t.Run("fallback_"+mt, func(t *testing.T) {
			result, err := mockAdapter.Query(ctx, types.PricingQuery{Asset: "H100_SXM", MarketType: mt})
			if err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			if result.Raw == "" {
				t.Error("expected non-empty Raw JSON")
			}
			var out map[string]interface{}
			if err := json.Unmarshal([]byte(result.Raw), &out); err != nil {
				t.Errorf("Raw is not valid JSON: %v — got: %s", err, result.Raw)
			}
			if result.Provider != "mock" {
				t.Errorf("expected provider=mock, got %q", result.Provider)
			}
		})
	}
}

func TestPricingAdapterMock_LiveServer(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/mock/compute/prices", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","prices":[{"gpu_type":"H100_SXM","spot_price_per_hour":2.49}]}`))
	})
	mux.HandleFunc("/api/mock/compute/forward-curves", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","asset":"H100_SXM","curve":[]}`))
	})
	mux.HandleFunc("/api/mock/compute/options", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","asset":"H100_SXM","options":[]}`))
	})
	mux.HandleFunc("/api/mock/compute/vol-surface", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","asset":"H100_SXM","vol_surface":[]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mockAdapter := adapter.NewMockPricingAdapter(srv.URL)
	ctx := context.Background()

	cases := []struct {
		mt      string
		wantKey string
	}{
		{"spot", "prices"},
		{"futures", "curve"},
		{"options", "options"},
		{"vol_surface", "vol_surface"},
	}
	for _, tc := range cases {
		t.Run("live_"+tc.mt, func(t *testing.T) {
			result, err := mockAdapter.Query(ctx, types.PricingQuery{Asset: "H100_SXM", MarketType: tc.mt})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var out map[string]interface{}
			if err := json.Unmarshal([]byte(result.Raw), &out); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}
			if out["status"] != "success" {
				t.Errorf("expected status=success, got %v", out["status"])
			}
		})
	}
}

func TestPricingAdapterStatic_NoFile(t *testing.T) {
	staticAdapter := adapter.NewStaticJSONAdapter("static_test", "/tmp/does_not_exist_9z8x7.json")
	result, err := staticAdapter.Query(context.Background(), types.PricingQuery{Asset: "H100_SXM", MarketType: "spot"})
	if err != nil {
		t.Fatalf("expected no error on missing file, got: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(result.Raw), &out); err != nil {
		t.Fatalf("invalid JSON from baseline matrix: %v", err)
	}
	if out["status"] != "success" {
		t.Errorf("expected status=success, got %v", out["status"])
	}
	matrix, ok := out["matrix"].(map[string]interface{})
	if !ok || len(matrix) == 0 {
		t.Error("expected non-empty matrix in baseline response")
	}
}

func TestPricingAdapterStatic_WithFile(t *testing.T) {
	tmp, err := os.CreateTemp("", "pricing_matrix_*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())

	content := `{"status":"success","H100_SXM":2.49,"custom_field":"test_value"}`
	if _, err := tmp.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmp.Close()

	staticAdapter := adapter.NewStaticJSONAdapter("static_test", tmp.Name())
	result, err := staticAdapter.Query(context.Background(), types.PricingQuery{Asset: "H100_SXM", MarketType: "spot"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(result.Raw), &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if out["custom_field"] != "test_value" {
		t.Errorf("expected custom_field=test_value, got %v", out["custom_field"])
	}
}

func TestPricingAdapterHTTP_DefaultPath(t *testing.T) {
	var capturedPath, capturedAsset, capturedMT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedAsset = r.URL.Query().Get("asset")
		capturedMT = r.URL.Query().Get("market_type")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","price":1.23}`))
	}))
	defer srv.Close()

	httpAdapter := adapter.NewCustomHTTPAdapter(adapter.CustomHTTPAdapterConfig{
		Name:    "test_http",
		BaseURL: srv.URL,
	})
	result, err := httpAdapter.Query(context.Background(), types.PricingQuery{Asset: "H100_SXM", MarketType: "spot"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedPath != "/prices" {
		t.Errorf("expected path=/prices, got %q", capturedPath)
	}
	if capturedAsset != "H100_SXM" {
		t.Errorf("expected asset=H100_SXM, got %q", capturedAsset)
	}
	if capturedMT != "spot" {
		t.Errorf("expected market_type=spot, got %q", capturedMT)
	}
	if result.Provider != "test_http" {
		t.Errorf("expected provider=test_http, got %q", result.Provider)
	}
}

func TestNullPricingAdapter(t *testing.T) {
	nullAdapter := &adapter.NullPricingAdapter{}

	if nullAdapter.Name() != "null" {
		t.Errorf("expected name=null, got %q", nullAdapter.Name())
	}

	for _, mt := range []string{"spot", "futures", "options", "vol_surface"} {
		t.Run(mt, func(t *testing.T) {
			result, err := nullAdapter.Query(context.Background(), types.PricingQuery{Asset: "H100_SXM", MarketType: mt})
			if err != nil {
				t.Fatalf("NullAdapter must never return an error, got: %v", err)
			}
			var out map[string]interface{}
			if err := json.Unmarshal([]byte(result.Raw), &out); err != nil {
				t.Fatalf("NullAdapter Raw must be valid JSON: %v", err)
			}
			if out["status"] != "unconfigured" {
				t.Errorf("expected status=unconfigured, got %v", out["status"])
			}
		})
	}
}
