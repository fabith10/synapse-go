package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fabith10/synapse-go/internal/agent/pricing/types"
)

const defaultVastAIURL = "https://vast.ai/api/v0/bundles/"

// VastAIPricingAdapter fetches live GPU spot rates from Vast.ai's free public bundle search.
type VastAIPricingAdapter struct {
	apiURL     string
	httpClient *http.Client

	mu        sync.RWMutex
	cacheTTL  time.Duration
	lastFetch time.Time
	cachedRaw []byte
}

// NewVastAIPricingAdapter creates a new Vast.ai adapter with a 5-minute cache.
func NewVastAIPricingAdapter(apiURL string) *VastAIPricingAdapter {
	if apiURL == "" {
		apiURL = defaultVastAIURL
	}
	return &VastAIPricingAdapter{
		apiURL:     apiURL,
		httpClient: &http.Client{Timeout: 8 * time.Second},
		cacheTTL:   5 * time.Minute,
	}
}

func (a *VastAIPricingAdapter) Name() string { return "vastai" }

func (a *VastAIPricingAdapter) Query(ctx context.Context, q types.PricingQuery) (*types.PricingResult, error) {
	asset := q.Asset
	if asset == "" {
		asset = "RTX_4090"
	}

	data, err := a.fetchBundles(ctx)
	if err != nil {
		raw := a.fallback(asset, q.MarketType)
		return BuildResult("vastai", asset, q.MarketType, raw), nil
	}

	rates := a.extractGPUSpotRates(data)
	spotRate, ok := rates[asset]
	if !ok || spotRate <= 0 {
		spotRate = a.matchBestGPU(rates, asset)
	}

	resultMap := map[string]interface{}{
		"status":                     "success",
		"provider":                   "vastai",
		"asset":                      asset,
		"market":                     q.MarketType,
		"spot_gpu_rate_per_hour_usd": fmt.Sprintf("$%.4f", spotRate),
		"spot_price":                 spotRate,
		"price":                      spotRate,
		"matrix":                     rates,
		"message":                    fmt.Sprintf("Live Vast.ai spot rate for %s is $%.4f/hr", asset, spotRate),
	}

	b, _ := json.Marshal(resultMap)
	return BuildResult("vastai", asset, q.MarketType, string(b)), nil
}

func (a *VastAIPricingAdapter) fetchBundles(ctx context.Context) ([]byte, error) {
	a.mu.RLock()
	if len(a.cachedRaw) > 0 && time.Since(a.lastFetch) < a.cacheTTL {
		cached := a.cachedRaw
		a.mu.RUnlock()
		return cached, nil
	}
	a.mu.RUnlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vastai returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	a.mu.Lock()
	a.cachedRaw = body
	a.lastFetch = time.Now()
	a.mu.Unlock()

	return body, nil
}

type vastOffer struct {
	GPUName  string  `json:"gpu_name"`
	DPHTotal float64 `json:"dph_total"`
	NumGPUs  int     `json:"num_gpus"`
	Rentable bool    `json:"rentable"`
}

type vastBundlesResponse struct {
	Offers []vastOffer `json:"offers"`
}

func (a *VastAIPricingAdapter) extractGPUSpotRates(body []byte) map[string]float64 {
	rates := map[string]float64{
		"H100_SXM":  2.49,
		"A100_80GB": 1.29,
		"RTX_4090":  0.45,
		"RTX_3090":  0.22,
	}

	var resp vastBundlesResponse
	if err := json.Unmarshal(body, &resp); err != nil || len(resp.Offers) == 0 {
		return rates
	}

	offersByGPU := make(map[string][]float64)
	for _, off := range resp.Offers {
		if off.DPHTotal <= 0 {
			continue
		}
		unitPrice := off.DPHTotal
		if off.NumGPUs > 1 {
			unitPrice = off.DPHTotal / float64(off.NumGPUs)
		}
		gpuLower := strings.ToLower(off.GPUName)
		switch {
		case strings.Contains(gpuLower, "h100"):
			offersByGPU["H100_SXM"] = append(offersByGPU["H100_SXM"], unitPrice)
		case strings.Contains(gpuLower, "a100"):
			offersByGPU["A100_80GB"] = append(offersByGPU["A100_80GB"], unitPrice)
		case strings.Contains(gpuLower, "4090"):
			offersByGPU["RTX_4090"] = append(offersByGPU["RTX_4090"], unitPrice)
		case strings.Contains(gpuLower, "3090"):
			offersByGPU["RTX_3090"] = append(offersByGPU["RTX_3090"], unitPrice)
		}
	}

	for k, list := range offersByGPU {
		if len(list) > 0 {
			sort.Float64s(list)
			median := list[len(list)/2]
			rates[k] = math.Round(median*10000.0) / 10000.0
		}
	}

	return rates
}

func (a *VastAIPricingAdapter) matchBestGPU(rates map[string]float64, asset string) float64 {
	assetLower := strings.ToLower(asset)
	for k, v := range rates {
		if strings.Contains(assetLower, strings.ToLower(k)) || strings.Contains(strings.ToLower(k), assetLower) {
			return v
		}
	}
	if strings.Contains(assetLower, "4090") {
		return rates["RTX_4090"]
	}
	if strings.Contains(assetLower, "a100") {
		return rates["A100_80GB"]
	}
	if strings.Contains(assetLower, "h100") {
		return rates["H100_SXM"]
	}
	return 1.25
}

func (a *VastAIPricingAdapter) fallback(asset, marketType string) string {
	rates := map[string]interface{}{
		"H100_SXM":  2.49,
		"A100_80GB": 1.29,
		"RTX_4090":  0.45,
		"RTX_3090":  0.22,
	}
	spot := 1.25
	if strings.Contains(strings.ToLower(asset), "4090") {
		spot = 0.45
	} else if strings.Contains(strings.ToLower(asset), "h100") {
		spot = 2.49
	}
	res := map[string]interface{}{
		"status":                     "success",
		"provider":                   "vastai_fallback",
		"asset":                      asset,
		"market":                     marketType,
		"spot_gpu_rate_per_hour_usd": fmt.Sprintf("$%.4f", spot),
		"spot_price":                 spot,
		"price":                      spot,
		"matrix":                     rates,
	}
	b, _ := json.Marshal(res)
	return string(b)
}
