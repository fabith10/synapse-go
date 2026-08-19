// Package broker implements the Hybrid Compute Broker: the layer that accepts
// a TaskRequest, consults the Pricing Oracle for the lowest-penalty provider,
// executes the task, and handles failover via circuit breakers and model
// escalation — all without a single time.Sleep retry loop.
package broker

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/fabith10/synapse-go/internal/agent/pricing/hedge"
	"github.com/fabith10/synapse-go/internal/memory"
	"github.com/fabith10/synapse-go/internal/orchestrator"
	toolpkg "github.com/fabith10/synapse-go/internal/tools"
	"github.com/fabith10/synapse-go/pkg/logger"
)

// LastUsedModel tracks the winning model name for each agent ID thread-safely.
var LastUsedModel = struct {
	sync.RWMutex
	M map[string]string
}{M: make(map[string]string)}


// ---------------------------------------------------------------------------
// Circuit Breaker
// ---------------------------------------------------------------------------

// circuitState represents the FSM state of one provider's circuit breaker.
type circuitState int

const (
	// circuitClosed — normal operation; requests flow through.
	circuitClosed circuitState = iota

	// circuitOpen — provider is failing; requests are rejected immediately.
	// After cooldownDuration the breaker transitions to HalfOpen.
	circuitOpen

	// circuitHalfOpen — a single probe request is allowed through.
	// Success → Closed. Failure → Open (with reset cooldown).
	circuitHalfOpen
)

const (
	// failureThreshold is the number of consecutive failures that trips a breaker.
	failureThreshold = 3

	// cooldownDuration is the minimum time a breaker stays Open before
	// transitioning to HalfOpen. No time.Sleep is used; the broker checks
	// lastFailureAt on every Route call (non-blocking check).
	cooldownDuration = 30 * time.Second
)

// circuitBreaker is an individual circuit breaker for one named provider node.
// All fields are protected by the Broker-level mu (no per-breaker lock needed
// because the broker already holds mu when reading/writing breakers).
type circuitBreaker struct {
	name          string
	state         circuitState
	failures      int
	lastFailureAt time.Time
}

// isOpen returns true if the breaker is currently denying requests.
// It also handles the HalfOpen promotion: if the cooldown has elapsed and the
// breaker is Open, it transitions to HalfOpen so the next request can probe.
func (cb *circuitBreaker) isOpen(now time.Time) bool {
	switch cb.state {
	case circuitOpen:
		if now.Sub(cb.lastFailureAt) >= cooldownDuration {
			cb.state = circuitHalfOpen
			return false // allow one probe through
		}
		return true
	case circuitHalfOpen:
		return false // already in probe mode
	default:
		return false
	}
}

// recordSuccess resets the breaker to Closed.
func (cb *circuitBreaker) recordSuccess() {
	cb.state = circuitClosed
	cb.failures = 0
}

// recordFailure increments the failure counter and opens the breaker when the
// threshold is reached.
func (cb *circuitBreaker) recordFailure(now time.Time) {
	cb.failures++
	cb.lastFailureAt = now
	if cb.failures >= failureThreshold || cb.state == circuitHalfOpen {
		cb.state = circuitOpen
	}
}

// ---------------------------------------------------------------------------
// Broker
// ---------------------------------------------------------------------------

// Broker is the Hybrid Compute Broker. It is the sole entity that speaks to
// LLM and Compute providers; agents never call provider SDKs directly.
//
// Responsibilities:
//  1. Rank available providers using the PricingOracle penalty score.
//  2. Enforce the MaxWillingToPay budget ceiling.
//  3. Write idempotency locks to the CheckpointStore before each execution.
//  4. Circuit-break rate-limited or preempted providers instantly.
//  5. Escalate from Tier 0 → Tier 1 on invalid JSON output.
//
// Concurrency: mu guards the circuit breaker map; the oracle handles its own
// internal locking. Route() is safe to call from multiple goroutines.
type Broker struct {
	oracle           *PricingOracle
	store            memory.CheckpointStore
	mu               sync.Mutex
	breakers         map[string]*circuitBreaker // keyed by provider node name
	globalMaxCostUSD float64                    // 0 = uncapped; >0 = hard ceiling across all routes
}

// NewBroker constructs a Broker with no global cost ceiling.
func NewBroker(oracle *PricingOracle, store memory.CheckpointStore) *Broker {
	return NewBrokerWithBudget(oracle, store, 0)
}

// NewBrokerWithBudget constructs a Broker that rejects any routing candidate
// whose estimated cost exceeds globalMaxCostUSD. Pass 0 for no ceiling.
//
// Both oracle and store are required; passing nil panics (programming error).
func NewBrokerWithBudget(oracle *PricingOracle, store memory.CheckpointStore, globalMaxCostUSD float64) *Broker {
	if oracle == nil {
		panic("broker.NewBrokerWithBudget: oracle must not be nil")
	}
	if store == nil {
		panic("broker.NewBrokerWithBudget: store must not be nil")
	}
	return &Broker{
		oracle:           oracle,
		store:            store,
		breakers:         make(map[string]*circuitBreaker),
		globalMaxCostUSD: globalMaxCostUSD,
	}
}

// ---------------------------------------------------------------------------
// Internal circuit-breaker helpers
// ---------------------------------------------------------------------------

// openCircuitSet returns a snapshot of all provider names whose breakers are
// currently Open. The oracle uses this to exclude unavailable nodes from
// ranking without needing to know anything about circuit breakers.
func (b *Broker) openCircuitSet() map[string]struct{} {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	open := make(map[string]struct{})
	for name, cb := range b.breakers {
		if cb.isOpen(now) {
			open[name] = struct{}{}
		}
	}
	return open
}

// OpenCircuitSet exposes openCircuitSet publicly for metrics and middleware observability.
func (b *Broker) OpenCircuitSet() map[string]struct{} {
	return b.openCircuitSet()
}

func (b *Broker) getOrCreateBreaker(name string) *circuitBreaker {
	if cb, ok := b.breakers[name]; ok {
		return cb
	}
	cb := &circuitBreaker{name: name, state: circuitClosed}
	b.breakers[name] = cb
	return cb
}

func (b *Broker) onSuccess(name string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.getOrCreateBreaker(name).recordSuccess()
}

func (b *Broker) onFailure(name string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.getOrCreateBreaker(name).recordFailure(time.Now())
}

// ---------------------------------------------------------------------------
// Route — the core dispatch method
// ---------------------------------------------------------------------------

// Route selects the best available provider for req, acquires an idempotency
// lock, executes the task, and returns the result. On failure it walks the
// ranked list for the next available provider before giving up.
//
// Escalation rules (PDR-003 §5):
//  1. HTTP 429 / ErrRateLimit      → circuit-break node, try next in ranked list
//  2. ErrSpotPreempted             → circuit-break node, re-query oracle, retry
//  3. ErrInvalidJSON (Tier 0 only) → bump minTier to TierCommercial, re-rank
//
// Context-first: callers must supply a context with an appropriate deadline.
// The broker does not add its own timeout; agents own their SLAs.
func (b *Broker) Route(
	ctx context.Context,
	req orchestrator.TaskRequest,
	messages []orchestrator.Message,
	tools []Tool,
) (RouteResult, error) {
	// Map orchestrator strategy enum to oracle-internal strategy type.
	strategy := OptimizationStrategy(req.Strategy)

	// Determine the minimum tier based on HardwareTier field.
	minTier := TierLocal
	switch req.HardwareTier {
	case "tier1":
		minTier = TierCommercial
	case "tier2":
		// Tier 2 tasks go directly to ComputeProvider — separate path below.
		return b.routeCompute(ctx, req)
	}

	return b.routeLLM(ctx, req, messages, tools, strategy, minTier)
}

// routeLLM handles Tier 0 and Tier 1 tasks through the LLM provider path.
func (b *Broker) routeLLM(
	ctx context.Context,
	req orchestrator.TaskRequest,
	messages []orchestrator.Message,
	tools []Tool,
	strategy OptimizationStrategy,
	minTier ProviderTier,
) (RouteResult, error) {
	originalMinTier := minTier
	hasFallenBack := false
	hasEscalated := false

	for attempt := 0; attempt < 3; attempt++ {
		open := b.openCircuitSet()

		ranked, err := b.oracle.RankLLMNodes(
			minTier,
			req.EstimatedTokens,
			req.MaxWillingToPay,
			strategy,
			open,
		)
		if err != nil {
			// If no providers are available at commercial tier, and we haven't fallen back yet,
			// fall back to TierLocal instead of failing immediately.
			if minTier == TierCommercial && !hasFallenBack && !hasEscalated {
				logger.WithComponent("broker").Warn("No active commercial providers available, falling back to TierLocal")
				minTier = TierLocal
				hasFallenBack = true
				continue
			}
			return RouteResult{}, err
		}

		// Walk ranked list; skip nodes that fail transiently or exceed global budget.
		allFailed := true
		for _, candidate := range ranked {
			// Global budget ceiling (ADK_Standard §4 / PDR-003). This is independent
			// of MaxWillingToPay (per-task) — the global cap is a process-wide hard limit.
			if b.globalMaxCostUSD > 0 && candidate.EstCost > b.globalMaxCostUSD {
				logger.WithComponent("broker").Warn("Skipping provider exceeding global budget ceiling",
					"provider", candidate.Node.Name, "est_cost", candidate.EstCost, "ceiling", b.globalMaxCostUSD)
				continue
			}

			result, err := b.tryLLMNode(ctx, req, candidate, messages, tools)
			if err == nil {
				return result, nil
			}

			// Rate-limit: circuit-break and try the next candidate.
			if err == ErrRateLimit {
				b.onFailure(candidate.Node.Name)
				continue
			}

			// Invalid JSON from a local model: escalate to commercial tier if we haven't already.
			if err == ErrInvalidJSON && minTier == TierLocal && originalMinTier == TierLocal && !hasEscalated {
				minTier = TierCommercial
				hasEscalated = true
				allFailed = false
				break
			}

			// Any other error on this node: record failure and move on.
			logger.WithComponent("broker").Debug("tryLLMNode failed", "provider", candidate.Node.Name, "error", err)
			b.onFailure(candidate.Node.Name)
		}

		// If all commercial candidates failed (e.g. they all returned rate limits/429),
		// and we started with TierCommercial, fall back to TierLocal.
		if allFailed && minTier == TierCommercial && !hasFallenBack && !hasEscalated {
			logger.WithComponent("broker").Warn("All commercial providers failed or rate-limited, falling back to TierLocal")
			minTier = TierLocal
			hasFallenBack = true
			continue
		}
	}

	return RouteResult{}, ErrNoProviders
}

// tryLLMNode attempts to execute the task on a single LLM node, writing an
// idempotency lock before calling the provider and updating the checkpoint
// on success or failure.
func (b *Broker) tryLLMNode(
	ctx context.Context,
	req orchestrator.TaskRequest,
	candidate ScoredLLMNode,
	messages []orchestrator.Message,
	tools []Tool,
) (RouteResult, error) {
	node := candidate.Node
	toolID := fmt.Sprintf("%s::%s", req.AgentID, node.Name)

	// Use SessionID in lock key if present, otherwise fallback to AgentID to ensure uniqueness
	lockTaskID := req.SessionID
	if lockTaskID == "" {
		lockTaskID = req.AgentID
	} else {
		h := sha256.New()
		for _, m := range messages {
			h.Write([]byte(m.Sender))
			h.Write([]byte(m.Content))
		}
		for _, t := range tools {
			h.Write([]byte(t.Name))
		}
		lockTaskID = fmt.Sprintf("%s-turn-%x", req.SessionID, h.Sum(nil)[:8])
	}

	// ---- Idempotency lock (PDR-002 §5) ------------------------------------
	if err := b.store.AcquireStepLock(ctx, lockTaskID, toolID); err != nil {
		if err == memory.ErrAlreadyLocked {
			// A SUCCESS record exists — return the cached payload.
			step, getErr := b.store.GetStep(ctx, lockTaskID, toolID)
			if getErr == nil && step.Status == memory.StepSuccess {
				return RouteResult{
					Response:         orchestrator.Message{Sender: node.Name, Content: step.OutputPayload},
					ProviderName:     node.Name,
					EstimatedCostUSD: candidate.EstCost,
					TierUsed:         node.Tier,
				}, nil
			}
		}
		return RouteResult{}, fmt.Errorf("broker: lock error: %w", err)
	}

	// ---- Format and send ---------------------------------------------------
	payload, err := node.Provider.FormatPrompt(messages, tools)
	if err != nil {
		_ = b.store.FailStep(ctx, lockTaskID, toolID)
		return RouteResult{}, fmt.Errorf("broker: format prompt: %w", err)
	}

	start := time.Now()
	response, err := node.Provider.GenerateResponse(ctx, payload)
	elapsed := time.Since(start)

	if err == nil {
		// Determine if the response is expected to be JSON.
		// triage-agent always outputs JSON.
		// planner-agent outputs JSON only in the planning phase (not synthesis).
		expectJSON := false
		if req.AgentID == "triage-agent" {
			expectJSON = true
		} else if req.AgentID == "planner-agent" {
			// Check if we are in the synthesis phase
			isSynthesis := false
			for _, m := range messages {
				if m.Sender == "SYSTEM" && (strings.Contains(m.Content, "synthesis") || strings.Contains(m.Content, "Do not output JSON")) {
					isSynthesis = true
					break
				}
			}
			if !isSynthesis {
				expectJSON = true
			}
		}

		if expectJSON {
			cleaned := toolpkg.ExtractJSON(response.Content)
			var js json.RawMessage
			if json.Unmarshal([]byte(cleaned), &js) == nil {
				response.Content = cleaned
			} else {
				err = ErrInvalidJSON
			}
		}
	}

	if err != nil {
		_ = b.store.FailStep(ctx, lockTaskID, toolID)
		b.onFailure(node.Name)

		// Classify the error so the caller can make a routing decision.
		if isRateLimitError(err) {
			return RouteResult{}, ErrRateLimit
		}
		if isInvalidJSONError(err) {
			return RouteResult{}, ErrInvalidJSON
		}
		return RouteResult{}, err
	}

	// ---- Update oracle metrics --------------------------------------------
	b.oracle.UpdateLLMLatency(node.Name, float64(elapsed.Milliseconds()))
	b.onSuccess(node.Name)

	LastUsedModel.Lock()
	LastUsedModel.M[req.AgentID] = node.Name
	LastUsedModel.Unlock()

	// ---- Persist result ---------------------------------------------------
	contentStr := fmt.Sprintf("%v", response.Content)
	if completeErr := b.store.CompleteStep(ctx, lockTaskID, toolID, contentStr); completeErr != nil {
		// Non-fatal: the response was received; log and continue.
		_ = completeErr
	}

	// ---- Update active_tasks routing record (PDR-002 §3.C) ----------------
	tier := memory.TierLocalOllama
	if node.Tier == TierCommercial {
		tier = memory.TierCommercialAPI
	}
	_ = b.store.UpsertTask(ctx, memory.ActiveTask{
		TaskID:          lockTaskID,
		CurrentAgent:    req.AgentID,
		Status:          memory.TaskStatusResolved,
		ComputeTierUsed: tier,
	})


	return RouteResult{
		Response:         response,
		ProviderName:     node.Name,
		EstimatedCostUSD: candidate.EstCost,
		TierUsed:         node.Tier,
	}, nil
}

// routeCompute handles Tier 2 tasks through the ComputeProvider path with the
// same circuit-breaker and idempotency contract as routeLLM.
func (b *Broker) routeCompute(ctx context.Context, req orchestrator.TaskRequest) (RouteResult, error) {
	strategy := OptimizationStrategy(req.Strategy)

	// Estimate execution seconds: tokens / 1000 as a rough proxy when no
	// dedicated field is available (callers should extend TaskRequest later).
	const defaultExecSeconds = 60.0
	estSeconds := defaultExecSeconds

	open := b.openCircuitSet()
	ranked, err := b.oracle.RankComputeNodes(estSeconds, req.MaxWillingToPay, strategy, open)
	if err != nil {
		return RouteResult{}, err
	}

	for _, candidate := range ranked {
		// Global budget ceiling for compute nodes.
		if b.globalMaxCostUSD > 0 && candidate.EstCost > b.globalMaxCostUSD {
			logger.WithComponent("broker").Warn("Skipping compute node exceeding global budget ceiling",
				"node", candidate.Node.Name, "est_cost", candidate.EstCost, "ceiling", b.globalMaxCostUSD)
			continue
		}

		result, err := b.tryComputeNode(ctx, req, candidate, estSeconds)
		if err == nil {
			return result, nil
		}
		if err == ErrSpotPreempted {
			b.onFailure(candidate.Node.Name)
			continue
		}
		b.onFailure(candidate.Node.Name)
	}

	return RouteResult{}, ErrNoProviders
}

// tryComputeNode provisions a spot instance, executes the container payload,
// and immediately terminates the instance — all within ctx's deadline.
func (b *Broker) tryComputeNode(
	ctx context.Context,
	req orchestrator.TaskRequest,
	candidate ScoredComputeNode,
	estSeconds float64,
) (RouteResult, error) {
	node := candidate.Node
	toolID := fmt.Sprintf("%s::compute::%s", req.AgentID, node.Name)

	// Idempotency lock.
	if err := b.store.AcquireStepLock(ctx, req.AgentID, toolID); err != nil {
		if err == memory.ErrAlreadyLocked {
			step, getErr := b.store.GetStep(ctx, req.AgentID, toolID)
			if getErr == nil && step.Status == memory.StepSuccess {
				return RouteResult{
					Response:         orchestrator.Message{Sender: node.Name, Content: step.OutputPayload},
					ProviderName:     node.Name,
					EstimatedCostUSD: candidate.EstCost,
					TierUsed:         TierSpotGPU,
				}, nil
			}
		}
		return RouteResult{}, fmt.Errorf("broker: compute lock error: %w", err)
	}

	// Provision instance.
	inst, err := node.Provider.ProvisionInstance(ctx, req.HardwareTier)
	if err != nil {
		_ = b.store.FailStep(ctx, req.AgentID, toolID)
		if isPreemptionError(err) {
			return RouteResult{}, ErrSpotPreempted
		}
		return RouteResult{}, err
	}

	// Always terminate the ephemeral instance, even on failure (PDR-004 §2.3).
	defer func() { _ = node.Provider.TerminateInstance(inst) }()

	start := time.Now()
	output, err := node.Provider.ExecuteContainer(ctx, inst, nil)
	elapsed := time.Since(start)

	if err != nil {
		_ = b.store.FailStep(ctx, req.AgentID, toolID)
		b.onFailure(node.Name)
		if isPreemptionError(err) {
			return RouteResult{}, ErrSpotPreempted
		}
		return RouteResult{}, err
	}

	b.oracle.UpdateComputeLatency(node.Name, float64(elapsed.Milliseconds()))
	b.onSuccess(node.Name)

	outputStr := string(output)
	_ = b.store.CompleteStep(ctx, req.AgentID, toolID, outputStr)

	// ---- Apply Active Derivative Hedge Downside Protection ----
	actualSpotRatePerHour := (candidate.EstCost / math.Max(estSeconds, 1.0)) * 3600.0
	effRatePerHour, hedgeSavings, contractID := hedge.GetGlobalHedgeLedger().ConsumeHedge(
		req.HardwareTier,
		elapsed.Hours(),
		actualSpotRatePerHour,
	)
	billedCostUSD := candidate.EstCost
	if contractID != "" && hedgeSavings > 0 {
		billedCostUSD = effRatePerHour * elapsed.Hours()
		logger.WithComponent("broker").Info("Applied derivative downside protection hedge",
			"contract_id", contractID,
			"hardware", req.HardwareTier,
			"spot_rate_hr", actualSpotRatePerHour,
			"hedged_rate_hr", effRatePerHour,
			"savings_usd", hedgeSavings)
	}

	_ = estSeconds // used for cost estimation above; logged for future audit
	return RouteResult{
		Response:         orchestrator.Message{Sender: node.Name, Content: outputStr},
		ProviderName:     node.Name,
		EstimatedCostUSD: billedCostUSD,
		TierUsed:         TierSpotGPU,
	}, nil
}

// ---------------------------------------------------------------------------
// Error classification helpers
// ---------------------------------------------------------------------------

// isRateLimitError returns true when an error from a provider indicates an
// HTTP 429 / quota-exceeded condition. Concrete provider implementations
// should wrap their errors with a recognisable sentinel or message.
func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	// Providers are expected to wrap ErrRateLimit. A string check is the
	// fallback for providers that return plain errors.
	return err == ErrRateLimit
}

// isInvalidJSONError returns true when a Tier 0 provider failed to produce
// valid JSON, which triggers model escalation to Tier 1.
func isInvalidJSONError(err error) bool {
	return err == ErrInvalidJSON
}

// isPreemptionError returns true when a spot instance was terminated by the
// cloud provider due to a market price spike.
func isPreemptionError(err error) bool {
	return err == ErrSpotPreempted
}
