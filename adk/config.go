package adk

// ---------------------------------------------------------------------------
// Config — four-step bootstrap input (ADK_Standard.md §4)
// ---------------------------------------------------------------------------

// Config is the single configuration object passed to NewRuntime. It declares
// all provider preferences, budget constraints, and the storage path so that
// the entire framework can be constructed deterministically from one value.
type Config struct {
	// LLMProviders is the ordered list of token-priced inference backends
	// (Tier 0 Ollama, Tier 1 OpenAI/Anthropic/Google) to register in the
	// PricingOracle. The broker ranks them by penalty score at routing time.
	LLMProviders []LLMNode

	// ComputeProviders is the ordered list of spot-GPU backends (Tier 2 AWS,
	// DeepInfra, etc.) to register in the PricingOracle.
	ComputeProviders []ComputeNode

	// DefaultStrategy controls the penalty-score weighting the broker uses
	// when no per-task override is supplied.
	DefaultStrategy OptimizationStrategy

	// GlobalMaxCostUSD is a hard budget ceiling enforced by the broker across
	// all routing decisions. Individual TaskRequest.MaxWillingToPay values
	// may be lower but never higher than this global cap.
	GlobalMaxCostUSD float64

	// MaxProcessMemoryMB is a soft cap on how much RAM the framework process
	// itself may consume (heap + stack). Applied at startup via
	// runtime/debug.SetMemoryLimit. The GC will be triggered more aggressively
	// when approaching this threshold. Zero means no cap (not recommended for
	// production). Recommended: 1024 (1 GB).
	MaxProcessMemoryMB int64

	// ContainerMemoryMB is the per-container hard RAM limit enforced by the
	// Docker daemon for every ephemeral Python/Bash execution container.
	// Zero means 2048 MB (2 GB) default is used. Tune this lower on memory-
	// constrained hosts.
	ContainerMemoryMB int64

	// SQLiteDSN is the data-source name for the embedded SQLite database.
	// Use "file:agent.db" for a persistent on-disk store, or
	// "file::memory:?cache=shared" for an in-process ephemeral store.
	SQLiteDSN string

	// ACL is the authorization provider that the AuthMiddleware consults for
	// every message crossing the bus. If nil, AllowAll is used (no restrictions).
	ACL ACLProvider

	// DisableDefaultMiddleware skips the automatic Tracing → Auth → Logging
	// middleware chain. Set to true only when wiring custom middleware manually.
	DisableDefaultMiddleware bool
}

// ---------------------------------------------------------------------------
// ACLProvider — authorization interface used by AuthMiddleware
// ---------------------------------------------------------------------------

// ACLProvider is consulted by the AuthMiddleware for every message on the bus.
// Return false from IsAllowed to silently drop the message (the sender receives
// no error — the framework never exposes routing internals to agent code).
type ACLProvider interface {
	IsAllowed(sender, recipient string) bool
}

// AllowAll is the default ACLProvider that permits all sender → recipient
// combinations. Use it during development and replace with a rule-based
// implementation before production deployment.
type AllowAll struct{}

// IsAllowed always returns true.
func (AllowAll) IsAllowed(_, _ string) bool { return true }
