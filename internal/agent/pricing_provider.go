package agent

import "context"

// MarketType constants for the supported market classifications.
const (
	MarketTypeSpot           = "spot"
	MarketTypeFutures        = "futures"
	MarketTypeOptions        = "options"
	MarketTypeVolSurface     = "vol_surface"
	MarketTypeExecWindow     = "execution_window"
)

// CostMode constants for execution_window queries.
const (
	// CostModeSpot minimizes raw spot cost over the job window (default).
	CostModeSpot = "spot"

	// CostModeHedgedSpot applies a synthetic cap: each hour costs
	// min(spot_rate, forward_rate), reflecting ownership of a forward contract
	// that locks in a ceiling price. Spot is preferred when below the forward.
	CostModeHedgedSpot = "hedged_spot"

	// CostModeRiskAdjusted applies a risk premium to spot rates based on options
	// implied volatility surfaces to protect scheduling against expected price spikes.
	CostModeRiskAdjusted = "risk_adjusted"
)

// PricingQuery is the canonical, provider-agnostic input to every pricing adapter.
type PricingQuery struct {
	// Asset is the target compute asset or GPU model (e.g. "H100_SXM", "A100_80GB").
	Asset string

	// MarketType classifies the requested market data:
	// "spot", "futures" (alias "forward"), "options", "vol_surface", "execution_window".
	MarketType string

	// ProviderFilter optionally restricts results to a specific cloud provider
	// or region (e.g. "RunPod", "AWS", "us-east").
	ProviderFilter string

	// Options carries market-type-specific parameters that vary per query type.
	// For execution_window: "duration_hours" (int string), "cost_mode" ("spot"|"hedged_spot"),
	//                       "forward_rate" (float string, USD/hr ceiling price).
	Options map[string]string
}

// PricingResult is the canonical normalised response from every pricing adapter.
// Each adapter is responsible for populating Raw with a valid JSON string.
// Data is populated by the manager after parsing Raw, for optional structured access.
type PricingResult struct {
	// Provider is the name of the adapter that produced this result.
	Provider string

	// Asset echoes the queried asset.
	Asset string

	// MarketType echoes the queried market type.
	MarketType string

	// Raw is the verbatim JSON string returned by the provider.
	// This is what callers (tools) receive directly.
	Raw string

	// Data is the parsed representation of Raw, available for in-process inspection.
	Data map[string]interface{}
}

// PricingProvider is the core abstraction that every pricing adapter must implement.
// Adding a new pricing source requires only implementing this interface and registering
// the adapter via PricingOracleManager.RegisterAdapter — no changes to the manager or
// oracle core are required.
type PricingProvider interface {
	// Name returns the unique identifier of this adapter (e.g. "mock", "custom_http").
	Name() string

	// Query executes a pricing lookup for the given query and returns a normalised result.
	// Implementations are responsible for network I/O, auth, parsing, and any fallbacks.
	Query(ctx context.Context, q PricingQuery) (*PricingResult, error)
}
