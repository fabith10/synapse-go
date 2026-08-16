package adapter

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/fabith10/synapse-go/internal/agent/pricing/types"
)

// FreeCompositePricingAdapter dynamically routes queries across OpenRouter, Vast.ai, and Deribit.
type FreeCompositePricingAdapter struct {
	openRouter *OpenRouterPricingAdapter
	vastAI     *VastAIPricingAdapter
	deribit    *DeribitPricingAdapter
}

// NewFreeCompositePricingAdapter creates a composite free-tier market adapter.
func NewFreeCompositePricingAdapter() *FreeCompositePricingAdapter {
	return &FreeCompositePricingAdapter{
		openRouter: NewOpenRouterPricingAdapter(""),
		vastAI:     NewVastAIPricingAdapter(""),
		deribit:    NewDeribitPricingAdapter("", ""),
	}
}

func (a *FreeCompositePricingAdapter) Name() string { return "free_live" }

func (a *FreeCompositePricingAdapter) Query(ctx context.Context, q types.PricingQuery) (*types.PricingResult, error) {
	marketType := strings.ToLower(q.MarketType)

	switch marketType {
	case "futures", "forward", "forwards":
		return a.deribit.Query(ctx, q)

	case "options", "vol_surface", "volatility":
		return a.deribit.Query(ctx, q)

	default: // Spot or general query
		if a.isLLMModelAsset(q.Asset) {
			res, err := a.openRouter.Query(ctx, q)
			if err == nil {
				// Inject live GPU spot matrix into the response
				var m map[string]interface{}
				if json.Unmarshal([]byte(res.Raw), &m) == nil {
					gpuRes, _ := a.vastAI.Query(ctx, types.PricingQuery{Asset: "RTX_4090", MarketType: "spot"})
					if gpuRes != nil && gpuRes.Data != nil {
						if matrix, ok := gpuRes.Data["matrix"].(map[string]interface{}); ok {
							if existingMatrix, ok := m["matrix"].(map[string]interface{}); ok {
								for k, v := range matrix {
									existingMatrix[k] = v
								}
							} else {
								m["matrix"] = matrix
							}
						}
					}
					b, _ := json.Marshal(m)
					return BuildResult("free_live", q.Asset, q.MarketType, string(b)), nil
				}
			}
			return res, err
		}

		// GPU asset query -> Vast.ai
		return a.vastAI.Query(ctx, q)
	}
}

func (a *FreeCompositePricingAdapter) isLLMModelAsset(asset string) bool {
	lower := strings.ToLower(asset)
	modelKeywords := []string{
		"r1", "deepseek", "llama", "qwen", "gpt", "claude", "mistral",
		"gemma", "phi", "command", "chat", "instruct",
	}
	for _, kw := range modelKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
