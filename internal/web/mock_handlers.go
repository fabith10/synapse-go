package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fabith10/agent-framework/internal/agent"
)

// MockTask represents an asynchronous compute/data task submitted to the mock server.
type MockTask struct {
	ID                  string                 `json:"id"`
	TaskType            string                 `json:"task_type"`
	Status              string                 `json:"status"` // "queued", "running", "completed", "failed", "cancelled"
	ProgressPercent     int                    `json:"progress_percent"`
	Payload             map[string]interface{} `json:"payload,omitempty"`
	Result              map[string]interface{} `json:"result,omitempty"`
	ErrorMessage        string                 `json:"error_message,omitempty"`
	Priority            string                 `json:"priority,omitempty"`
	CallbackURL         string                 `json:"callback_url,omitempty"`
	CreatedAt           time.Time              `json:"created_at"`
	UpdatedAt           time.Time              `json:"updated_at"`
	CompletedAt         *time.Time             `json:"completed_at,omitempty"`
	EstimatedDurationMs int64                  `json:"estimated_duration_ms"`
}

// MockTaskStore manages thread-safe mock task lifecycle state.
type MockTaskStore struct {
	mu     sync.RWMutex
	tasks  map[string]*MockTask
	counter int64
}

// NewMockTaskStore creates a new task store.
func NewMockTaskStore() *MockTaskStore {
	return &MockTaskStore{
		tasks: make(map[string]*MockTask),
	}
}

// CreateTask registers a new mock task and simulates its execution asynchronously.
func (s *MockTaskStore) CreateTask(taskType string, payload map[string]interface{}, priority, callbackURL string) *MockTask {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.counter++
	id := fmt.Sprintf("mock-task-%d-%d", time.Now().UnixNano()%100000, s.counter)
	now := time.Now()

	task := &MockTask{
		ID:                  id,
		TaskType:            taskType,
		Status:              "queued",
		ProgressPercent:     0,
		Payload:             payload,
		Priority:            priority,
		CallbackURL:         callbackURL,
		CreatedAt:           now,
		UpdatedAt:           now,
		EstimatedDurationMs: 1500, // 1.5 seconds default execution simulation
	}
	s.tasks[id] = task

	// Background worker to simulate state progression: queued -> running -> completed
	go s.simulateTaskExecution(id)

	return task
}

func (s *MockTaskStore) simulateTaskExecution(id string) {
	// Move to running after 200ms
	time.Sleep(200 * time.Millisecond)
	s.mu.Lock()
	task, exists := s.tasks[id]
	if !exists || task.Status == "cancelled" {
		s.mu.Unlock()
		return
	}
	task.Status = "running"
	task.ProgressPercent = 35
	task.UpdatedAt = time.Now()
	s.mu.Unlock()

	// Progress updates
	time.Sleep(400 * time.Millisecond)
	s.mu.Lock()
	task, exists = s.tasks[id]
	if !exists || task.Status == "cancelled" {
		s.mu.Unlock()
		return
	}
	task.ProgressPercent = 75
	task.UpdatedAt = time.Now()
	s.mu.Unlock()

	// Complete task
	time.Sleep(400 * time.Millisecond)
	s.mu.Lock()
	defer s.mu.Unlock()
	task, exists = s.tasks[id]
	if !exists || task.Status == "cancelled" {
		return
	}

	completedAt := time.Now()
	task.Status = "completed"
	task.ProgressPercent = 100
	task.UpdatedAt = completedAt
	task.CompletedAt = &completedAt
	task.Result = map[string]interface{}{
		"status":          "success",
		"task_id":         id,
		"processed_items": 42,
		"compute_units":   "1.25 GPU-hours",
		"output_summary":  fmt.Sprintf("Mock task %s completed successfully", id),
	}
}

// GetTask fetches a single task by ID.
func (s *MockTaskStore) GetTask(id string) (*MockTask, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tasks[id]
	if !ok {
		return nil, false
	}
	// Return shallow copy
	taskCopy := *t
	return &taskCopy, true
}

// ListTasks returns all tasks, optionally filtered by status.
func (s *MockTaskStore) ListTasks(statusFilter string) []*MockTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*MockTask, 0, len(s.tasks))
	for _, t := range s.tasks {
		if statusFilter == "" || strings.EqualFold(t.Status, statusFilter) {
			taskCopy := *t
			result = append(result, &taskCopy)
		}
	}
	return result
}

// CancelTask cancels a task if it is queued or running.
func (s *MockTaskStore) CancelTask(id string) (*MockTask, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, exists := s.tasks[id]
	if !exists {
		return nil, false, fmt.Errorf("task not found")
	}

	if task.Status == "completed" || task.Status == "failed" {
		return task, false, fmt.Errorf("task is already %s", task.Status)
	}

	now := time.Now()
	task.Status = "cancelled"
	task.UpdatedAt = now
	task.CompletedAt = &now
	taskCopy := *task
	return &taskCopy, true, nil
}

// ---------------------------------------------------------------------------
// HTTP Handlers
// ---------------------------------------------------------------------------

// ComputePriceItem represents a compute rate entry.
type ComputePriceItem struct {
	Provider             string  `json:"provider"`
	GPUType              string  `json:"gpu_type"`
	VRAMGB               int     `json:"vram_gb"`
	Region               string  `json:"region"`
	SpotPricePerHour     float64 `json:"spot_price_per_hour"`
	OnDemandPricePerHour float64 `json:"on_demand_price_per_hour"`
	Availability         string  `json:"availability"`
}

// handleMockComputePrices returns mock GPU/CPU spot and on-demand market prices.
// Route: GET /api/mock/compute/prices
func (s *Server) handleMockComputePrices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	providerFilter := strings.ToLower(r.URL.Query().Get("provider"))
	gpuFilter := strings.ToUpper(r.URL.Query().Get("gpu_type"))
	regionFilter := strings.ToLower(r.URL.Query().Get("region"))

	allPrices := []ComputePriceItem{
		{Provider: "RunPod", GPUType: "NVIDIA-H100-SXM", VRAMGB: 80, Region: "us-east", SpotPricePerHour: 2.49, OnDemandPricePerHour: 3.89, Availability: "high"},
		{Provider: "RunPod", GPUType: "NVIDIA-A100-80GB", VRAMGB: 80, Region: "us-east", SpotPricePerHour: 1.29, OnDemandPricePerHour: 1.89, Availability: "high"},
		{Provider: "Lambda", GPUType: "NVIDIA-H100-SXM", VRAMGB: 80, Region: "us-west", SpotPricePerHour: 2.39, OnDemandPricePerHour: 3.75, Availability: "medium"},
		{Provider: "Lambda", GPUType: "NVIDIA-A100-80GB", VRAMGB: 80, Region: "us-west", SpotPricePerHour: 1.19, OnDemandPricePerHour: 1.79, Availability: "high"},
		{Provider: "AWS", GPUType: "NVIDIA-H100-SXM", VRAMGB: 80, Region: "us-east-1", SpotPricePerHour: 3.12, OnDemandPricePerHour: 4.10, Availability: "high"},
		{Provider: "AWS", GPUType: "NVIDIA-A100-80GB", VRAMGB: 80, Region: "us-east-1", SpotPricePerHour: 1.65, OnDemandPricePerHour: 2.20, Availability: "high"},
		{Provider: "GCP", GPUType: "NVIDIA-H100-SXM", VRAMGB: 80, Region: "us-central1", SpotPricePerHour: 2.95, OnDemandPricePerHour: 3.95, Availability: "medium"},
		{Provider: "GCP", GPUType: "NVIDIA-A100-80GB", VRAMGB: 80, Region: "us-central1", SpotPricePerHour: 1.45, OnDemandPricePerHour: 2.10, Availability: "high"},
		{Provider: "Azure", GPUType: "NVIDIA-H100-SXM", VRAMGB: 80, Region: "eastus", SpotPricePerHour: 3.05, OnDemandPricePerHour: 4.05, Availability: "low"},
		{Provider: "Together", GPUType: "NVIDIA-RTX-4090", VRAMGB: 24, Region: "us-east", SpotPricePerHour: 0.45, OnDemandPricePerHour: 0.75, Availability: "high"},
	}

	var filtered []ComputePriceItem
	for _, item := range allPrices {
		if providerFilter != "" && !strings.Contains(strings.ToLower(item.Provider), providerFilter) {
			continue
		}
		if gpuFilter != "" && !strings.Contains(strings.ToUpper(item.GPUType), gpuFilter) {
			continue
		}
		if regionFilter != "" && !strings.Contains(strings.ToLower(item.Region), regionFilter) {
			continue
		}
		filtered = append(filtered, item)
	}

	if filtered == nil {
		filtered = []ComputePriceItem{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "success",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"count":     len(filtered),
		"currency":  "USD",
		"prices":    filtered,
	})
}

// ForwardCurvePoint represents one contract along the forward curve.
type ForwardCurvePoint struct {
	Tenor          string  `json:"tenor"`
	Contract       string  `json:"contract"`
	ForwardPrice   float64 `json:"forward_price_usd"`
	Expiration     string  `json:"expiration"`
	OpenInterest   int     `json:"open_interest"`
	Volume24h      int     `json:"volume_24h"`
	ImpliedCostPerGPU float64 `json:"implied_cost_per_gpu_hour"`
}

// handleMockForwardCurves returns mock futures and forward curves for compute contracts.
// Route: GET /api/mock/compute/forward-curves
func (s *Server) handleMockForwardCurves(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	asset := strings.ToUpper(r.URL.Query().Get("asset"))
	if asset == "" {
		asset = "H100_SXM"
	}

	baseSpot := 2.49
	switch asset {
	case "A100_80GB":
		baseSpot = 1.29
	case "RTX_4090":
		baseSpot = 0.45
	}

	curve := []ForwardCurvePoint{
		{Tenor: "1M", Contract: fmt.Sprintf("%s-2026-08", asset), ForwardPrice: baseSpot * 1.02, Expiration: "2026-08-31", OpenInterest: 1240, Volume24h: 310, ImpliedCostPerGPU: baseSpot * 1.02},
		{Tenor: "2M", Contract: fmt.Sprintf("%s-2026-09", asset), ForwardPrice: baseSpot * 1.04, Expiration: "2026-09-30", OpenInterest: 2150, Volume24h: 420, ImpliedCostPerGPU: baseSpot * 1.04},
		{Tenor: "3M", Contract: fmt.Sprintf("%s-2026-10", asset), ForwardPrice: baseSpot * 1.06, Expiration: "2026-10-31", OpenInterest: 3400, Volume24h: 680, ImpliedCostPerGPU: baseSpot * 1.06},
		{Tenor: "6M", Contract: fmt.Sprintf("%s-2027-01", asset), ForwardPrice: baseSpot * 1.11, Expiration: "2027-01-31", OpenInterest: 5120, Volume24h: 950, ImpliedCostPerGPU: baseSpot * 1.11},
		{Tenor: "12M", Contract: fmt.Sprintf("%s-2027-07", asset), ForwardPrice: baseSpot * 1.18, Expiration: "2027-07-31", OpenInterest: 2890, Volume24h: 410, ImpliedCostPerGPU: baseSpot * 1.18},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":          "success",
		"asset":           asset,
		"base_spot_price": baseSpot,
		"currency":        "USD",
		"as_of":           time.Now().UTC().Format(time.RFC3339),
		"curve":           curve,
	})
}

// OptionContract represents an option contract entry.
type OptionContract struct {
	Type              string  `json:"type"` // "call" or "put"
	Strike            float64 `json:"strike"`
	Bid               float64 `json:"bid"`
	Ask               float64 `json:"ask"`
	LastPrice         float64 `json:"last_price"`
	ImpliedVolatility float64 `json:"implied_volatility"`
	Delta             float64 `json:"delta"`
	Gamma             float64 `json:"gamma"`
	Vega              float64 `json:"vega"`
	Theta             float64 `json:"theta"`
	OpenInterest      int     `json:"open_interest"`
}

// handleMockOptions returns mock option chains for compute contracts.
// Route: GET /api/mock/compute/options
func (s *Server) handleMockOptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	asset := strings.ToUpper(r.URL.Query().Get("asset"))
	if asset == "" {
		asset = "H100_SXM"
	}
	expiration := r.URL.Query().Get("expiration")
	if expiration == "" {
		expiration = "30d"
	}

	underlying := 2.50
	if asset == "A100_80GB" {
		underlying = 1.30
	}

	strikes := []float64{underlying * 0.90, underlying * 0.95, underlying, underlying * 1.05, underlying * 1.10}

	options := make([]OptionContract, 0, len(strikes)*2)
	for _, strike := range strikes {
		// Call
		callBid := fmt.Sprintf("%.3f", maxFloat(0.01, (underlying-strike)*0.8+0.12))
		callAsk := fmt.Sprintf("%.3f", maxFloat(0.02, (underlying-strike)*0.8+0.15))
		callBidF, _ := strconv.ParseFloat(callBid, 64)
		callAskF, _ := strconv.ParseFloat(callAsk, 64)

		options = append(options, OptionContract{
			Type:              "call",
			Strike:            strike,
			Bid:               callBidF,
			Ask:               callAskF,
			LastPrice:         callAskF,
			ImpliedVolatility: 0.35,
			Delta:             0.52,
			Gamma:             0.18,
			Vega:              0.04,
			Theta:             -0.01,
			OpenInterest:      450,
		})

		// Put
		putBid := fmt.Sprintf("%.3f", maxFloat(0.01, (strike-underlying)*0.8+0.10))
		putAsk := fmt.Sprintf("%.3f", maxFloat(0.02, (strike-underlying)*0.8+0.13))
		putBidF, _ := strconv.ParseFloat(putBid, 64)
		putAskF, _ := strconv.ParseFloat(putAsk, 64)

		options = append(options, OptionContract{
			Type:              "put",
			Strike:            strike,
			Bid:               putBidF,
			Ask:               putAskF,
			LastPrice:         putAskF,
			ImpliedVolatility: 0.37,
			Delta:             -0.48,
			Gamma:             0.18,
			Vega:              0.04,
			Theta:             -0.01,
			OpenInterest:      380,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":          "success",
		"asset":           asset,
		"expiration":      expiration,
		"underlying_spot": underlying,
		"currency":        "USD",
		"as_of":           time.Now().UTC().Format(time.RFC3339),
		"options":         options,
	})
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// roundTo4 rounds a float64 to 4 decimal places.
func roundTo4(v float64) float64 {
	// multiply by 10000, add 0.5, truncate, divide back
	shifted := v * 10000.0
	if shifted < 0 {
		shifted -= 0.5
	} else {
		shifted += 0.5
	}
	truncated := float64(int64(shifted))
	return truncated / 10000.0
}

// ---------------------------------------------------------------------------
// Volatility Surface Endpoint
// ---------------------------------------------------------------------------

// VolSurfaceStrike represents one point on the vol surface (fixed tenor, varying K).
type VolSurfaceStrike struct {
	Strike        float64 `json:"strike_usd"`
	Moneyness     float64 `json:"moneyness"` // K / S0
	ImpliedVol    float64 `json:"implied_vol"`
	CallPremium   float64 `json:"call_premium_usd"`
	PutPremium    float64 `json:"put_premium_usd"`
	Delta         float64 `json:"delta_call"`
	Gamma         float64 `json:"gamma"`
	Vega          float64 `json:"vega"`
	Theta         float64 `json:"theta_per_day"`
}

// VolSurfaceTenor represents one tenor slice of the volatility surface.
type VolSurfaceTenor struct {
	Tenor     string             `json:"tenor"`
	TenorDays int                `json:"tenor_days"`
	ATMVol    float64            `json:"atm_vol"` // ATM implied vol for this tenor
	Strikes   []VolSurfaceStrike `json:"strikes"`
}

// handleMockVolSurface returns a mock implied volatility surface across strikes and expiry tenors.
// Models a realistic vol smile (SVI-inspired): higher IV for deep OTM strikes, lower for ATM.
// Route: GET /api/mock/compute/vol-surface?asset=H100_SXM&tenors=30,60,90,180,365
func (s *Server) handleMockVolSurface(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	asset := strings.ToUpper(r.URL.Query().Get("asset"))
	if asset == "" {
		asset = "H100_SXM"
	}

	// Spot baseline (USD/hr)
	spot := 2.49
	switch {
	case strings.Contains(asset, "A100"):
		spot = 1.29
	case strings.Contains(asset, "4090"):
		spot = 0.45
	}

	// ATM vol term structure — longer tenors have lower ATM vol (mean reversion)
	type tenorSpec struct {
		label  string
		days   int
		atmVol float64 // annualised
	}
	tenors := []tenorSpec{
		{"30d", 30, 0.380},
		{"60d", 60, 0.355},
		{"90d", 90, 0.335},
		{"180d", 180, 0.310},
		{"365d", 365, 0.285},
	}

	// Strike moneyness grid: 80% → 125% of spot
	moneynessGrid := []float64{0.80, 0.85, 0.90, 0.95, 1.00, 1.05, 1.10, 1.15, 1.20, 1.25}

	surface := make([]VolSurfaceTenor, 0, len(tenors))
	for _, t := range tenors {
		T := float64(t.days) / 365.0 // time to expiry in years
		atmVol := t.atmVol
		strikes := make([]VolSurfaceStrike, 0, len(moneynessGrid))
		for _, m := range moneynessGrid {
			K := spot * m
			// SVI-inspired smile: σ(m) = atmVol * (1 + skew*(m-1) + convexity*(m-1)^2)
			// skew = -0.15 (put skew: OTM puts have higher IV)
			// convexity = 0.30 (smile wings)
			skew := -0.15
			convexity := 0.30
			iv := atmVol * (1.0 + skew*(m-1.0) + convexity*(m-1.0)*(m-1.0))
			if iv < 0.05 {
				iv = 0.05
			}

			// Simplified Black-Scholes analytic approximation for premium
			// Using: premium ≈ S * σ * sqrt(T) * Φ-like factor
			sqrtT := 0.0
			if T > 0 {
				// manual sqrt via Newton's method (no math import needed here — use approximation)
				sqrtT = T
				for i := 0; i < 8; i++ {
					sqrtT = (sqrtT + T/sqrtT) / 2.0
				}
			}
			// Approximate d1, d2
			lnMInv := 0.0
			if m > 0 {
				// ln(1/m) ≈ 1-m for near-ATM; use series: ln(m) ≈ (m-1) - (m-1)^2/2
				x := m - 1.0
				lnMInv = -(x - x*x/2.0 + x*x*x/3.0)
			}
			d1 := (lnMInv + (iv*iv/2.0)*T) / (iv * sqrtT + 1e-12)
			d2 := d1 - iv*sqrtT

			// Standard Normal CDF approximation (Hart's rational approximation)
			ndCDF := func(z float64) float64 {
				sign := 1.0
				if z < 0 {
					sign = -1.0
					z = -z
				}
				t2 := 1.0 / (1.0 + 0.2316419*z)
				poly := t2 * (0.319381530 + t2*(-0.356563782+t2*(1.781477937+t2*(-1.821255978+t2*1.330274429))))
				exp2 := z * z / 2.0
				// e^(-x) approximation: truncated series for small x, cap for large
				var eNeg float64
				if exp2 > 20 {
					eNeg = 0
				} else {
					eNeg = 1.0
					term := 1.0
					for i := 1; i <= 12; i++ {
						term *= -exp2 / float64(i)
						eNeg += term
					}
					if eNeg < 0 {
						eNeg = 0
					}
				}
				phi := eNeg * 0.3989422804
				cdf := 1.0 - phi*poly
				if sign < 0 {
					return 1.0 - cdf
				}
				return cdf
			}
			Nd1 := ndCDF(d1)
			Nd2 := ndCDF(d2)
			Nd1neg := ndCDF(-d1)
			Nd2neg := ndCDF(-d2)

			callPremium := spot*Nd1 - K*Nd2
			if callPremium < 0 {
				callPremium = 0
			}
			putPremium := K*Nd2neg - spot*Nd1neg
			if putPremium < 0 {
				putPremium = 0
			}

			// Delta (call), Gamma (both), Vega, Theta (per calendar day)
			delta := Nd1
			ndPrime := 0.0 // φ(d1): standard normal PDF at d1
			{
				exp2 := d1 * d1 / 2.0
				eNeg := 1.0
				if exp2 < 20 {
					term := 1.0
					for i := 1; i <= 12; i++ {
						term *= -exp2 / float64(i)
						eNeg += term
					}
					if eNeg < 0 {
						eNeg = 0
					}
				} else {
					eNeg = 0
				}
				ndPrime = eNeg * 0.3989422804
			}
			gamma := ndPrime / (spot * iv * sqrtT + 1e-12)
			vega := spot * ndPrime * sqrtT / 100.0 // per 1% vol move
			theta := -(spot*ndPrime*iv)/(2.0*sqrtT+1e-12) / 365.0

			strikes = append(strikes, VolSurfaceStrike{
				Strike:      roundTo4(K),
				Moneyness:   roundTo4(m),
				ImpliedVol:  roundTo4(iv),
				CallPremium: roundTo4(callPremium),
				PutPremium:  roundTo4(putPremium),
				Delta:       roundTo4(delta),
				Gamma:       roundTo4(gamma),
				Vega:        roundTo4(vega),
				Theta:       roundTo4(theta),
			})
		}
		surface = append(surface, VolSurfaceTenor{
			Tenor:     t.label,
			TenorDays: t.days,
			ATMVol:    roundTo4(atmVol),
			Strikes:   strikes,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":           "success",
		"asset":            asset,
		"underlying_spot":  spot,
		"currency":         "USD",
		"model":            "SVI-inspired smile (skew=-0.15, convexity=0.30)",
		"as_of":            time.Now().UTC().Format(time.RFC3339),
		"vol_surface":      surface,
	})
}



// ---------------------------------------------------------------------------
// Mock Task Endpoints
// ---------------------------------------------------------------------------

// handleMockTasks routes task creation and task listing requests.
// Routes:
//   POST /api/mock/tasks
//   GET  /api/mock/tasks
//   GET  /api/mock/tasks/{id}
//   POST /api/mock/tasks/{id}/cancel
//   DELETE /api/mock/tasks/{id}
func (s *Server) handleMockTasks(w http.ResponseWriter, r *http.Request) {
	s.mockTaskStoreMu.Do(func() {
		if s.mockTaskStore == nil {
			s.mockTaskStore = NewMockTaskStore()
		}
	})

	path := strings.TrimPrefix(r.URL.Path, "/api/mock/tasks")
	path = strings.TrimPrefix(path, "/")

	if path == "" {
		switch r.Method {
		case http.MethodPost:
			s.handleCreateMockTask(w, r)
			return
		case http.MethodGet:
			s.handleListMockTasks(w, r)
			return
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
	}

	parts := strings.Split(path, "/")
	id := parts[0]

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			s.handleGetMockTask(w, r, id)
			return
		case http.MethodDelete:
			s.handleCancelMockTask(w, r, id)
			return
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
	}

	if len(parts) == 2 && parts[1] == "cancel" && r.Method == http.MethodPost {
		s.handleCancelMockTask(w, r, id)
		return
	}

	http.NotFound(w, r)
}

func (s *Server) handleCreateMockTask(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TaskType    string                 `json:"task_type"`
		Payload     map[string]interface{} `json:"payload"`
		Priority    string                 `json:"priority"`
		CallbackURL string                 `json:"callback_url"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != http.ErrBodyReadAfterClose {
		// allow empty body if default task
	}

	if body.TaskType == "" {
		body.TaskType = "generic_compute"
	}
	if body.Priority == "" {
		body.Priority = "medium"
	}

	task := s.mockTaskStore.CreateTask(body.TaskType, body.Payload, body.Priority, body.CallbackURL)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "created",
		"message": "Task queued successfully",
		"task":    task,
	})
}

func (s *Server) handleListMockTasks(w http.ResponseWriter, r *http.Request) {
	statusFilter := r.URL.Query().Get("status")
	tasks := s.mockTaskStore.ListTasks(statusFilter)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
		"count":  len(tasks),
		"tasks":  tasks,
	})
}

func (s *Server) handleGetMockTask(w http.ResponseWriter, _ *http.Request, id string) {
	task, exists := s.mockTaskStore.GetTask(id)
	if !exists {
		http.Error(w, fmt.Sprintf(`{"status":"error","message":"task %q not found"}`, id), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
		"task":   task,
	})
}

func (s *Server) handleCancelMockTask(w http.ResponseWriter, _ *http.Request, id string) {
	task, ok, err := s.mockTaskStore.CancelTask(id)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "cancelled",
		"message": fmt.Sprintf("Task %s cancelled successfully", id),
		"task":    task,
	})
}

// handlePricingProviderConfig handles reading and updating the active Pricing Oracle provider profile.
// Routes: GET/POST /api/config/pricing-provider
func (s *Server) handlePricingProviderConfig(w http.ResponseWriter, r *http.Request) {
	mgr := agent.GetPricingOracleManager()

	if r.Method == http.MethodPost {
		var req struct {
			ActiveProvider string `json:"active_provider"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ActiveProvider == "" {
			req.ActiveProvider = r.FormValue("active_provider")
		}

		if req.ActiveProvider == "" {
			http.Error(w, `{"status":"error","message":"missing active_provider parameter"}`, http.StatusBadRequest)
			return
		}

		if err := mgr.SetActiveProvider(req.ActiveProvider); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "error",
				"message": err.Error(),
			})
			return
		}

		s.LogEvent("WEB", "SYSTEM", fmt.Sprintf("Pricing Oracle provider switched dynamically to %q", req.ActiveProvider))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":          "success",
			"message":         fmt.Sprintf("Pricing provider updated to %s", req.ActiveProvider),
			"active_provider": mgr.GetActiveProviderName(),
			"config":          mgr.GetConfigRoot(),
		})
		return
	}

	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":          "success",
			"active_provider": mgr.GetActiveProviderName(),
			"config":          mgr.GetConfigRoot(),
		})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}
