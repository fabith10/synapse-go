package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// ---------------------------------------------------------------------------
// MockPricingAdapter tests
// ---------------------------------------------------------------------------

func TestPricingAdapterMock_Fallback(t *testing.T) {
	// No mock server running; adapter must return a valid fallback for every market type.
	adapter := NewMockPricingAdapter("http://127.0.0.1:0") // unreachable port
	ctx := context.Background()

	marketTypes := []string{"spot", "futures", "options", "vol_surface"}
	for _, mt := range marketTypes {
		t.Run("fallback_"+mt, func(t *testing.T) {
			result, err := adapter.Query(ctx, PricingQuery{Asset: "H100_SXM", MarketType: mt})
			if err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			if result.Raw == "" {
				t.Error("expected non-empty Raw JSON")
			}
			// Raw must be valid JSON
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
	// Spin up a small test HTTP handler that mimics the mock API.
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

	adapter := NewMockPricingAdapter(srv.URL)
	ctx := context.Background()

	cases := []struct {
		mt       string
		wantKey  string
	}{
		{"spot", "prices"},
		{"futures", "curve"},
		{"options", "options"},
		{"vol_surface", "vol_surface"},
	}
	for _, tc := range cases {
		t.Run("live_"+tc.mt, func(t *testing.T) {
			result, err := adapter.Query(ctx, PricingQuery{Asset: "H100_SXM", MarketType: tc.mt})
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

// ---------------------------------------------------------------------------
// StaticJSONAdapter tests
// ---------------------------------------------------------------------------

func TestPricingAdapterStatic_NoFile(t *testing.T) {
	adapter := NewStaticJSONAdapter("static_test", "/tmp/does_not_exist_9z8x7.json")
	result, err := adapter.Query(context.Background(), PricingQuery{Asset: "H100_SXM", MarketType: "spot"})
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

	adapter := NewStaticJSONAdapter("static_test", tmp.Name())
	result, err := adapter.Query(context.Background(), PricingQuery{Asset: "H100_SXM", MarketType: "spot"})
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

// ---------------------------------------------------------------------------
// CustomHTTPAdapter tests
// ---------------------------------------------------------------------------

func TestPricingAdapterHTTP_DefaultPath(t *testing.T) {
	// Server that checks asset and market_type query params.
	var capturedPath, capturedAsset, capturedMT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedAsset = r.URL.Query().Get("asset")
		capturedMT = r.URL.Query().Get("market_type")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","price":1.23}`))
	}))
	defer srv.Close()

	adapter := NewCustomHTTPAdapter(CustomHTTPAdapterConfig{
		Name:    "test_http",
		BaseURL: srv.URL,
	})
	result, err := adapter.Query(context.Background(), PricingQuery{Asset: "H100_SXM", MarketType: "spot"})
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

func TestPricingAdapterHTTP_PerMarketEndpoints(t *testing.T) {
	// Server records the path for each request.
	paths := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mt := r.URL.Query().Get("market_type")
		paths[mt] = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	adapter := NewCustomHTTPAdapter(CustomHTTPAdapterConfig{
		Name:    "multi_endpoint",
		BaseURL: srv.URL,
		Endpoints: map[string]HTTPEndpointTemplate{
			"spot":        {Path: "/v1/spot"},
			"options":     {Path: "/v1/options/chain"},
			"vol_surface": {Path: "/v1/surface"},
		},
	})
	ctx := context.Background()

	cases := []struct{ mt, wantPath string }{
		{"spot", "/v1/spot"},
		{"options", "/v1/options/chain"},
		{"vol_surface", "/v1/surface"},
		{"futures", "/prices"}, // falls back to default
	}
	for _, tc := range cases {
		_, err := adapter.Query(ctx, PricingQuery{Asset: "H100_SXM", MarketType: tc.mt})
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", tc.mt, err)
		}
		if got := paths[tc.mt]; got != tc.wantPath {
			t.Errorf("market_type=%s: expected path %q, got %q", tc.mt, tc.wantPath, got)
		}
	}
}

func TestPricingAdapterHTTP_NoBaseURL(t *testing.T) {
	adapter := NewCustomHTTPAdapter(CustomHTTPAdapterConfig{Name: "empty"})
	_, err := adapter.Query(context.Background(), PricingQuery{Asset: "H100_SXM", MarketType: "spot"})
	if err == nil {
		t.Error("expected error when base_url is empty")
	}
}

// ---------------------------------------------------------------------------
// PricingOracleManager registry tests
// ---------------------------------------------------------------------------

func TestPricingOracleManager_RegisterAdapter(t *testing.T) {
	m := NewPricingOracleManager("")

	// Stub adapter
	stub := &stubAdapter{name: "my_provider", response: `{"status":"ok","custom":true}`}
	m.RegisterAdapter("my_provider", stub)
	_ = m.SetActiveProvider("my_provider")

	raw, err := m.ExecuteQuery(context.Background(), "H100_SXM", "spot", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if out["custom"] != true {
		t.Errorf("expected custom=true from stub adapter, got %v", out["custom"])
	}
}

func TestPricingOracleManager_NoAdapter(t *testing.T) {
	m := NewPricingOracleManager("")
	_ = m.SetActiveProvider("nonexistent_provider")

	// Should NOT return an error — graceful unconfigured response.
	raw, err := m.ExecuteQuery(context.Background(), "H100_SXM", "spot", "", nil)
	if err != nil {
		t.Fatalf("expected no Go error for missing adapter, got: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("expected valid JSON from null fallback, got: %s", raw)
	}
	if out["status"] != "unconfigured" {
		t.Errorf("expected status=unconfigured, got %v", out["status"])
	}
	if out["message"] == nil {
		t.Error("expected a non-empty message field in unconfigured response")
	}
}

func TestNullPricingAdapter(t *testing.T) {
	adapter := &NullPricingAdapter{}

	if adapter.Name() != "null" {
		t.Errorf("expected name=null, got %q", adapter.Name())
	}

	for _, mt := range []string{"spot", "futures", "options", "vol_surface"} {
		t.Run(mt, func(t *testing.T) {
			result, err := adapter.Query(context.Background(), PricingQuery{Asset: "H100_SXM", MarketType: mt})
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

func TestPricingOracle_ShouldDefer(t *testing.T) {
	m := NewPricingOracleManager("")

	// Override config directly for deterministic testing
	m.mu.Lock()
	m.config.DeferralPolicy = DeferralPolicyConfig{
		Enabled:          true,
		BypassAll:        false,
		CostThresholdUSD: 1.0,
		TokenThreshold:   50000,
		PeakStartHour:    0,  // Always peak
		PeakEndHour:      24, // Always peak
	}
	m.mu.Unlock()

	// 1. Should defer because cost ($2.50) is greater than threshold ($1.00) during peak hours
	deferJob, reason := m.ShouldDefer(2.50, 0, false)
	if !deferJob {
		t.Errorf("expected ShouldDefer=true for high cost, got false. Reason: %s", reason)
	}

	// 2. Should defer because tokens (60,000) are greater than threshold (50,000) during peak hours
	deferTokens, reason := m.ShouldDefer(0.50, 60000, false)
	if !deferTokens {
		t.Errorf("expected ShouldDefer=true for high tokens, got false. Reason: %s", reason)
	}

	// 3. Should NOT defer if cost and tokens are below threshold
	deferLow, reason := m.ShouldDefer(0.50, 10000, false)
	if deferLow {
		t.Errorf("expected ShouldDefer=false for low cost/tokens, got true. Reason: %s", reason)
	}

	// 4. Should NOT defer if forceImmediate is true
	deferForce, reason := m.ShouldDefer(2.50, 0, true)
	if deferForce {
		t.Errorf("expected ShouldDefer=false when forceImmediate is true, got true. Reason: %s", reason)
	}

	// 5. Should NOT defer if FORCE_URGENT env override is active
	t.Setenv("FORCE_URGENT", "true")
	deferEnv, reason := m.ShouldDefer(2.50, 0, false)
	if deferEnv {
		t.Errorf("expected ShouldDefer=false when FORCE_URGENT is active, got true. Reason: %s", reason)
	}
}

// stubAdapter is a test double implementing PricingProvider.
type stubAdapter struct {
	name     string
	response string
}

func (s *stubAdapter) Name() string { return s.name }
func (s *stubAdapter) Query(_ context.Context, q PricingQuery) (*PricingResult, error) {
	return buildResult(s.name, q.Asset, q.MarketType, s.response), nil
}
