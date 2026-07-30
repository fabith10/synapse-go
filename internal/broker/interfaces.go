// Package broker defines the universal provider interfaces and supporting
// types used by the Hybrid Compute Broker. Nothing in this file is tied to a
// concrete implementation — every LLM client and every GPU cloud vendor must
// satisfy these contracts so the broker can swap them at runtime.
package broker

import (
	"context"

	"github.com/fabith10/synapse-go/internal/orchestrator"
)

// ---------------------------------------------------------------------------
// Tool — MCP-schema representation used by LLM providers
// ---------------------------------------------------------------------------

// Tool is the Go representation of the Model Context Protocol (MCP) tool
// schema. Both LLMProvider.FormatPrompt and the Tool Registry operate on this
// type. The Parameters field carries the raw JSON Schema object that the LLM
// uses to understand what arguments it may supply.
type Tool struct {
	Name        string                 // MCP "name" field
	Description string                 // MCP "description" field
	Parameters  map[string]interface{} // MCP "input_schema.properties" mapping
}

// ---------------------------------------------------------------------------
// LLMProvider — abstraction over token-priced inference backends
// ---------------------------------------------------------------------------

// LLMProvider is implemented by every text-in / text-out backend:
//   - Local Ollama  (Tier 0, $0 cost)
//   - OpenAI API    (Tier 1)
//   - Anthropic API (Tier 1)
//   - Google AI     (Tier 1)
//
// The two-step design (FormatPrompt → GenerateResponse) lets the oracle
// measure prompt size in tokens before committing to a provider, which is
// essential for cost gating via TaskRequest.MaxWillingToPay.
type LLMProvider interface {
	// FormatPrompt serialises messages and tool definitions into the
	// provider-specific wire format (OpenAI Chat JSON, Anthropic Messages, etc.).
	// The returned interface{} is opaque to the broker and passed straight
	// to GenerateResponse.
	FormatPrompt(messages []orchestrator.Message, tools []Tool) (interface{}, error)

	// GenerateResponse submits the formatted payload and blocks until the
	// provider returns a complete response. The context controls the deadline;
	// callers must always supply a context.WithTimeout to honour PDR-004 §3.
	GenerateResponse(ctx context.Context, payload interface{}) (orchestrator.Message, error)
}

// ---------------------------------------------------------------------------
// ComputeProvider — abstraction over hardware-priced spot-GPU backends
// ---------------------------------------------------------------------------

// Instance represents a provisioned spot compute node. The fields are kept
// intentionally minimal so different cloud SDKs can embed their own state via
// the Metadata bag without leaking cloud-specific types into the core broker.
type Instance struct {
	// ID is the provider-assigned instance identifier (e.g., AWS instance-id).
	ID string

	// Provider is the human-readable provider name ("aws-spot", "deepinfra", …).
	Provider string

	// Region is the data-centre region the instance was provisioned in.
	Region string

	// Metadata carries provider-specific fields (IP, port, auth token, …)
	// without coupling the broker to any SDK type.
	Metadata map[string]string
}

// ComputeProvider is implemented by every hardware-priced GPU/CPU backend:
//   - AWS Spot Instances
//   - DeepInfra GPU clusters
//   - Custom on-premise GPU pools
//
// All three methods accept a context.Context so the broker can enforce strict
// timeouts at every lifecycle boundary (PDR-004 §3 mandates context.WithTimeout
// for every container execution).
type ComputeProvider interface {
	// ProvisionInstance requests a new spot instance of the given hardware
	// tier (e.g., "H100-80GB", "A10G-24GB"). Returns when the instance is
	// ready to accept container workloads or when ctx expires.
	ProvisionInstance(ctx context.Context, hardwareTier string) (Instance, error)

	// ExecuteContainer runs a pre-built Docker image on inst and streams
	// stdout back as a byte slice. The broker wraps this call in a
	// context.WithTimeout derived from TaskRequest to prevent runaway jobs.
	ExecuteContainer(ctx context.Context, inst Instance, payload []byte) ([]byte, error)

	// TerminateInstance destroys inst immediately. Must be called in a
	// deferred block after ExecuteContainer to guarantee the ephemeral
	// container is never left running after task completion or failure.
	TerminateInstance(inst Instance) error
}

// ---------------------------------------------------------------------------
// ProviderNode — a registered, scorable provider entry in the oracle matrix
// ---------------------------------------------------------------------------

// ProviderTier maps to the three compute paradigms in PDR-003 §3.
type ProviderTier int

const (
	TierLocal      ProviderTier = 0 // Ollama — always $0
	TierCommercial ProviderTier = 1 // OpenAI / Anthropic / Google
	TierSpotGPU    ProviderTier = 2 // AWS Spot / DeepInfra
)

// LLMNode holds pricing metadata for one token-based provider entry.
// The oracle uses these rates to calculate the EstimatedTaskCost for Tier 0
// and Tier 1 providers.
type LLMNode struct {
	Name     string       // e.g. "anthropic-claude-3-5", "openai-gpt-4o"
	Tier     ProviderTier
	Provider LLMProvider

	// Token pricing in USD per 1 000 tokens (use 0.0 for Tier 0).
	InputRateUSD  float64
	OutputRateUSD float64
	CachedRateUSD float64

	// AvgLatencyMs is a rolling average of the last N response times.
	// Updated by the broker after every successful GenerateResponse call.
	AvgLatencyMs float64
}

// ComputeNode holds pricing metadata for one spot-GPU provider entry.
type ComputeNode struct {
	Name     string // e.g. "aws-us-east-1-h100"
	Provider ComputeProvider

	// SpotRatePerSecondUSD is the current live spot price. Updated by the
	// oracle's price-feed goroutine (stubbed in this implementation).
	SpotRatePerSecondUSD float64

	// AvgLatencyMs is a rolling average of container round-trip times.
	AvgLatencyMs float64
}

// ---------------------------------------------------------------------------
// Routing result
// ---------------------------------------------------------------------------

// RouteResult is returned by Broker.Route after a successful provider call.
type RouteResult struct {
	// Response is the message returned by the winning provider.
	Response orchestrator.Message

	// ProviderName identifies which node ultimately served the request.
	ProviderName string

	// EstimatedCostUSD is the broker's post-hoc cost estimate for this call.
	EstimatedCostUSD float64

	// TierUsed records which compute tier was selected.
	TierUsed ProviderTier
}

// ---------------------------------------------------------------------------
// Sentinel errors
// ---------------------------------------------------------------------------

// ErrRateLimit signals that a provider returned HTTP 429 and the circuit
// breaker has opened for that node.
var ErrRateLimit = brokerError("provider rate-limited (429): circuit open")

// ErrSpotPreempted signals that a spot instance was terminated mid-execution.
var ErrSpotPreempted = brokerError("spot instance preempted")

// ErrBudgetExceeded signals that no provider can fulfil the request within
// MaxWillingToPay.
var ErrBudgetExceeded = brokerError("estimated cost exceeds MaxWillingToPay for all providers")

// ErrNoProviders signals that no providers are registered for the required tier.
var ErrNoProviders = brokerError("no providers registered for the required tier")

// ErrInvalidJSON signals that a Tier 0 provider returned unparseable output,
// triggering model escalation to Tier 1.
var ErrInvalidJSON = brokerError("provider returned invalid JSON; escalating to Tier 1")

type brokerError string

func (e brokerError) Error() string { return string(e) }
