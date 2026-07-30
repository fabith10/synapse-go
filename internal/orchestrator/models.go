// Package orchestrator defines the core domain models, interfaces, and
// concurrency primitives for the agent framework. All inter-agent communication
// is performed exclusively via typed Go channels (no mutexes on the hot path).
package orchestrator

import "context"

// ---------------------------------------------------------------------------
// Messaging
// ---------------------------------------------------------------------------

// Message is the atomic unit of communication passed between agents and the
// central orchestrator bus. The Recipient field is matched by the router to
// determine which agent mailbox, topic, or special channel receives the payload.
//
// Content is always a string; agents that need to exchange structured data must
// JSON-encode into Content and JSON-decode on receipt (A2A Protocol §2).
// Metadata carries routing hints and observability fields only — never business
// data (see reserved keys in A2A_Protocol.md §2).
type Message struct {
	// Sender is the AgentID or "ORCHESTRATOR" / "USER" for system messages.
	Sender string

	// Recipient is the AgentID, "USER" (HITL channel), or a topic name
	// (e.g., "system.alerts") for pub/sub fan-out.
	Recipient string

	// Content is the primary string payload (plain text or JSON-encoded body).
	Content string

	// Metadata carries optional key/value routing and observability fields.
	// Reserved keys: "reply_to", "correlation_id", "topic", "traceparent", "auth_scope".
	Metadata map[string]string
}

// ---------------------------------------------------------------------------
// Middleware
// ---------------------------------------------------------------------------

// MiddlewareFunc intercepts every message crossing the central bus before it
// is delivered to its recipient. Implementations MUST call next(msg) to
// continue the chain. Omitting the call silently drops the message — this is
// the correct behaviour for the auth middleware when a sender is not permitted
// to target a given recipient.
//
// The context carries the router goroutine's lifetime. Any I/O inside a
// middleware function (e.g., SQLite logging) must respect ctx to participate
// in graceful shutdown.
//
// Middleware execution order follows registration order (first registered runs
// outermost). The built-in delivery to agent Mailbox / HITL / topic channels
// is always the innermost step.
type MiddlewareFunc func(ctx context.Context, msg Message, next func(Message))

// ---------------------------------------------------------------------------
// Optimization Strategy
// ---------------------------------------------------------------------------

// OptimizationStrategy is a typed integer enum that governs how the Hybrid
// Compute Broker selects and ranks available providers for a given task.
// The broker uses this field as a weight selector in its penalty-score formula:
//
//	Penalty = (NormalizedCost * WeightCost) + (AverageLatencyMs * WeightLatency)
type OptimizationStrategy int

const (
	// StrategyMaxSavings maximises cost reduction; latency weight → 0.
	StrategyMaxSavings OptimizationStrategy = iota

	// StrategyLowLatency minimises round-trip time; cost weight → 0.
	StrategyLowLatency

	// StrategyBalanced applies equal weight to cost and latency (default).
	StrategyBalanced
)

// String returns a human-readable representation of the strategy, useful for
// logging and audit trails without importing fmt into hot paths.
func (s OptimizationStrategy) String() string {
	switch s {
	case StrategyMaxSavings:
		return "MaxSavings"
	case StrategyLowLatency:
		return "LowLatency"
	case StrategyBalanced:
		return "Balanced"
	default:
		return "Unknown"
	}
}

// ---------------------------------------------------------------------------
// Task Request
// ---------------------------------------------------------------------------

// TaskRequest carries all information the Hybrid Compute Broker needs to
// select a provider, calculate the penalty score, and route the task to the
// correct execution tier (Local Ollama, Commercial API, or Spot GPU).
type TaskRequest struct {
	// AgentID identifies the originating agent so the broker can return results
	// directly to that agent's mailbox.
	AgentID string

	// EstimatedTokens is the broker's pre-flight estimate of combined input +
	// output tokens. Used to calculate Tier 1 (commercial API) costs.
	EstimatedTokens int

	// HardwareTier specifies the minimum execution tier required:
	//   "tier0" → Local Ollama only
	//   "tier1" → Commercial API (OpenAI / Anthropic / Google)
	//   "tier2" → Data-Center Spot GPU
	HardwareTier string

	// Strategy controls the penalty-score weighting used by the broker.
	Strategy OptimizationStrategy

	// MaxWillingToPay is the agent-level cost ceiling in USD. The broker will
	// not route to a provider whose estimated cost exceeds this value.
	MaxWillingToPay float64

	// SessionID associates this request with a specific user task or session.
	// Used by the broker's step-caching mechanism to prevent cross-task cache collisions.
	SessionID string
}


// ---------------------------------------------------------------------------
// Agent
// ---------------------------------------------------------------------------

// Agent is the base building block for every participant in the framework.
// Each Agent owns a dedicated, buffered Mailbox channel so the router can
// deliver messages without blocking the central bus goroutine. Concrete agent
// implementations embed this struct and extend it with domain-specific state.
//
// Concurrency contract:
//   - Only the owning agent goroutine should read from Mailbox.
//   - The orchestrator router is the sole writer to Mailbox.
//   - No mutex is required on Mailbox itself; channel semantics provide safety.
type Agent struct {
	// ID is the unique, stable identifier used for routing.
	// Convention: use lowercase-kebab-case (e.g., "research-agent-01").
	ID string

	// Mailbox is the agent's private inbound message queue. A buffer of 64
	// provides enough slack to absorb burst traffic without stalling the router.
	Mailbox chan Message
}

// NewAgent constructs an Agent with a pre-allocated, buffered mailbox.
// Callers should embed the returned Agent in their concrete agent struct.
func NewAgent(id string) Agent {
	return Agent{
		ID:      id,
		Mailbox: make(chan Message, 64),
	}
}
