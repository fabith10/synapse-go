package pricing

import "github.com/fabith10/synapse-go/internal/agent/pricing/types"

// Re-export shared types so existing callers within package pricing continue to work.
type PricingQuery = types.PricingQuery
type PricingResult = types.PricingResult
type PricingProvider = types.PricingProvider

const (
	MarketTypeSpot       = types.MarketTypeSpot
	MarketTypeFutures    = types.MarketTypeFutures
	MarketTypeOptions    = types.MarketTypeOptions
	MarketTypeVolSurface = types.MarketTypeVolSurface
	MarketTypeExecWindow = types.MarketTypeExecWindow

	CostModeSpot         = types.CostModeSpot
	CostModeHedgedSpot   = types.CostModeHedgedSpot
	CostModeRiskAdjusted = types.CostModeRiskAdjusted
)
