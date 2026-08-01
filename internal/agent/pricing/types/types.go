// Package types holds the shared interfaces and value types for the pricing oracle system.
// This package is a dependency-free leaf that can be imported by both the pricing package
// and the pricing/adapter subpackage without creating import cycles.
package types

import "context"

// MarketType constants for the supported market classifications.
const (
	MarketTypeSpot       = "spot"
	MarketTypeFutures    = "futures"
	MarketTypeOptions    = "options"
	MarketTypeVolSurface = "vol_surface"
	MarketTypeExecWindow = "execution_window"
)

// CostMode constants for execution_window queries.
const (
	CostModeSpot         = "spot"
	CostModeHedgedSpot   = "hedged_spot"
	CostModeRiskAdjusted = "risk_adjusted"
)

// PricingQuery is the canonical, provider-agnostic input to every pricing adapter.
type PricingQuery struct {
	// Asset is the target compute asset or GPU model (e.g. "H100_SXM", "A100_80GB").
	Asset string

	// MarketType classifies the requested market data:
	// "spot", "futures" (alias "forward"), "options", "vol_surface", "execution_window".
	MarketType string

	// ProviderFilter optionally restricts results to a specific cloud provider or region.
	ProviderFilter string

	// Options carries market-type-specific parameters that vary per query type.
	Options map[string]string
}

// PricingResult is the canonical normalised response from every pricing adapter.
type PricingResult struct {
	// Provider is the name of the adapter that produced this result.
	Provider string

	// Asset echoes the queried asset.
	Asset string

	// MarketType echoes the queried market type.
	MarketType string

	// Raw is the verbatim JSON string returned by the provider.
	Raw string

	// Data is the parsed representation of Raw, available for in-process inspection.
	Data map[string]interface{}
}

// PricingProvider is the core abstraction that every pricing adapter must implement.
type PricingProvider interface {
	// Name returns the unique identifier of this adapter.
	Name() string

	// Query executes a pricing lookup and returns a normalised result.
	Query(ctx context.Context, q PricingQuery) (*PricingResult, error)
}
