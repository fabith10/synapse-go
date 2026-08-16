package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fabith10/synapse-go/internal/agent/pricing/types"
)

const defaultOpenRouterURL = "https://openrouter.ai/api/v1/models"

// OpenRouterPricingAdapter fetches live LLM token rates from OpenRouter's free public endpoint.
type OpenRouterPricingAdapter struct {
	apiURL     string
	httpClient *http.Client

	mu        sync.RWMutex
	cacheTTL  time.Duration
	lastFetch time.Time
	cachedRaw []byte
}

// NewOpenRouterPricingAdapter creates a new OpenRouter adapter with a 5-minute cache.
func NewOpenRouterPricingAdapter(apiURL string) *OpenRouterPricingAdapter {
	if apiURL == "" {
		apiURL = defaultOpenRouterURL
	}
	return &OpenRouterPricingAdapter{
		apiURL:     apiURL,
		httpClient: &http.Client{Timeout: 8 * time.Second},
		cacheTTL:   5 * time.Minute,
	}
}

func (a *OpenRouterPricingAdapter) Name() string { return "openrouter" }

func (a *OpenRouterPricingAdapter) Query(ctx context.Context, q types.PricingQuery) (*types.PricingResult, error) {
	asset := q.Asset
	if asset == "" {
		asset = "deepseek-r1"
	}

	data, err := a.fetchModels(ctx)
	if err != nil {
		raw := a.fallback(asset, q.MarketType)
		return BuildResult("openrouter", asset, q.MarketType, raw), nil
	}

	inputRatePer1M, outputRatePer1M, matchedID := a.extractRates(data, asset)
	baseSpot := 1.25
	if strings.Contains(strings.ToLower(asset), "4090") || strings.Contains(strings.ToLower(asset), "qwen") {
		baseSpot = 0.45
	} else if strings.Contains(strings.ToLower(asset), "llama-3-70b") || strings.Contains(strings.ToLower(asset), "h100") {
		baseSpot = 1.85
	}

	resultMap := map[string]interface{}{
		"status":                 "success",
		"provider":               "openrouter",
		"asset":                  asset,
		"matched_model_id":       matchedID,
		"market":                 q.MarketType,
		"input_cost_per_1m":      inputRatePer1M,
		"output_cost_per_1m":     outputRatePer1M,
		"base_spot_per_hour":     baseSpot,
		"models": map[string]interface{}{
			asset: map[string]interface{}{
				"input_cost_per_1m":  inputRatePer1M,
				"output_cost_per_1m": outputRatePer1M,
				"base_spot_per_hour": baseSpot,
			},
		},
		"matrix": map[string]interface{}{
			asset: baseSpot,
		},
	}

	b, _ := json.Marshal(resultMap)
	return BuildResult("openrouter", asset, q.MarketType, string(b)), nil
}

func (a *OpenRouterPricingAdapter) fetchModels(ctx context.Context) ([]byte, error) {
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
		return nil, fmt.Errorf("openrouter returned status %d", resp.StatusCode)
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

type openRouterModel struct {
	ID      string `json:"id"`
	Pricing struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
}

type openRouterResponse struct {
	Data []openRouterModel `json:"data"`
}

func (a *OpenRouterPricingAdapter) extractRates(body []byte, asset string) (float64, float64, string) {
	var resp openRouterResponse
	if err := json.Unmarshal(body, &resp); err != nil || len(resp.Data) == 0 {
		return 0.55, 2.19, asset
	}

	normAsset := strings.ToLower(strings.ReplaceAll(asset, "-", ""))
	normAsset = strings.ReplaceAll(normAsset, "_", "")
	normAsset = strings.ReplaceAll(normAsset, ".", "")

	var bestMatch *openRouterModel
	for i := range resp.Data {
		m := &resp.Data[i]
		normID := strings.ToLower(strings.ReplaceAll(m.ID, "-", ""))
		normID = strings.ReplaceAll(normID, "_", "")
		normID = strings.ReplaceAll(normID, ".", "")
		normID = strings.ReplaceAll(normID, "/", "")

		if strings.Contains(normID, normAsset) || strings.Contains(normAsset, normID) {
			bestMatch = m
			break
		}
	}

	// Secondary heuristic matches
	if bestMatch == nil {
		targetKeywords := strings.Fields(strings.ToLower(strings.ReplaceAll(asset, "-", " ")))
		for i := range resp.Data {
			m := &resp.Data[i]
			mLower := strings.ToLower(m.ID)
			allMatch := true
			for _, kw := range targetKeywords {
				if !strings.Contains(mLower, kw) {
					allMatch = false
					break
				}
			}
			if allMatch && len(targetKeywords) > 0 {
				bestMatch = m
				break
			}
		}
	}

	if bestMatch == nil && len(resp.Data) > 0 {
		bestMatch = &resp.Data[0]
	}

	if bestMatch != nil {
		pRate, _ := strconv.ParseFloat(bestMatch.Pricing.Prompt, 64)
		cRate, _ := strconv.ParseFloat(bestMatch.Pricing.Completion, 64)
		// OpenRouter returns price per 1 token; convert to per 1,000,000 tokens
		inCost1M := pRate * 1000000.0
		outCost1M := cRate * 1000000.0
		if inCost1M > 0 || outCost1M > 0 {
			return inCost1M, outCost1M, bestMatch.ID
		}
	}

	return 0.55, 2.19, asset
}

func (a *OpenRouterPricingAdapter) fallback(asset, marketType string) string {
	res := map[string]interface{}{
		"status":             "success",
		"provider":           "openrouter_fallback",
		"asset":              asset,
		"market":             marketType,
		"input_cost_per_1m":  0.55,
		"output_cost_per_1m": 2.19,
		"base_spot_per_hour": 1.25,
		"models": map[string]interface{}{
			asset: map[string]interface{}{
				"input_cost_per_1m":  0.55,
				"output_cost_per_1m": 2.19,
				"base_spot_per_hour": 1.25,
			},
		},
		"matrix": map[string]interface{}{
			asset: 1.25,
		},
	}
	b, _ := json.Marshal(res)
	return string(b)
}
