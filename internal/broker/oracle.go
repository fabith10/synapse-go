package broker

import (
	"fmt"
	"sync"
)

// ---------------------------------------------------------------------------
// Pricing Oracle
// ---------------------------------------------------------------------------

// PricingOracle is the in-memory, thread-safe matrix that tracks all
// registered providers across the three compute tiers simultaneously.
// It is the single source of truth for cost estimation and provider ranking.
//
// Concurrency model (PDR-001 / Antigravity.md):
//   - sync.RWMutex guards the provider slices.
//   - Multiple broker goroutines may call Score() concurrently (read path).
//   - RegisterLLM / RegisterCompute / UpdateLLMLatency / UpdateSpotRate
//     are the only write operations and are expected to be infrequent.
type PricingOracle struct {
	mu       sync.RWMutex
	llmNodes     []*LLMNode
	computeNodes []*ComputeNode
}

// NewPricingOracle constructs an empty oracle. Providers must be registered
// before the broker can route any tasks.
func NewPricingOracle() *PricingOracle {
	return &PricingOracle{}
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

// RegisterLLM adds or replaces an LLM provider node in the oracle matrix.
// Nodes with the same Name are deduplicated (last write wins).
func (o *PricingOracle) RegisterLLM(node LLMNode) {
	o.mu.Lock()
	defer o.mu.Unlock()

	for i, n := range o.llmNodes {
		if n.Name == node.Name {
			o.llmNodes[i] = &node
			return
		}
	}
	o.llmNodes = append(o.llmNodes, &node)
}

// RegisterCompute adds or replaces a spot-GPU provider node in the oracle matrix.
func (o *PricingOracle) RegisterCompute(node ComputeNode) {
	o.mu.Lock()
	defer o.mu.Unlock()

	for i, n := range o.computeNodes {
		if n.Name == node.Name {
			o.computeNodes[i] = &node
			return
		}
	}
	o.computeNodes = append(o.computeNodes, &node)
}

// ---------------------------------------------------------------------------
// Live price / latency updates
// ---------------------------------------------------------------------------

// UpdateLLMLatency records a new latency observation for an LLM node using
// an exponential moving average (α = 0.2) to smooth outliers. Safe to call
// from any goroutine after a GenerateResponse completes.
func (o *PricingOracle) UpdateLLMLatency(name string, observedMs float64) {
	const alpha = 0.2
	o.mu.Lock()
	defer o.mu.Unlock()

	for _, n := range o.llmNodes {
		if n.Name == name {
			if n.AvgLatencyMs == 0 {
				n.AvgLatencyMs = observedMs
			} else {
				n.AvgLatencyMs = alpha*observedMs + (1-alpha)*n.AvgLatencyMs
			}
			return
		}
	}
}

// UpdateSpotRate sets the live spot price for a ComputeNode. In production
// this would be called by a background goroutine ingesting CME/provider feeds.
func (o *PricingOracle) UpdateSpotRate(name string, ratePerSecUSD float64) {
	o.mu.Lock()
	defer o.mu.Unlock()

	for _, n := range o.computeNodes {
		if n.Name == name {
			n.SpotRatePerSecondUSD = ratePerSecUSD
			return
		}
	}
}

// UpdateComputeLatency records a new latency observation for a ComputeNode.
func (o *PricingOracle) UpdateComputeLatency(name string, observedMs float64) {
	const alpha = 0.2
	o.mu.Lock()
	defer o.mu.Unlock()

	for _, n := range o.computeNodes {
		if n.Name == name {
			if n.AvgLatencyMs == 0 {
				n.AvgLatencyMs = observedMs
			} else {
				n.AvgLatencyMs = alpha*observedMs + (1-alpha)*n.AvgLatencyMs
			}
			return
		}
	}
}

// ---------------------------------------------------------------------------
// Cost estimation
// ---------------------------------------------------------------------------

// EstimateLLMCost calculates the USD cost for a token-priced API call.
// The split between input and output tokens is assumed 60/40 when only a
// total estimate is available (conservative heuristic).
//
//	EstCost = (estInput * InputRate + estOutput * OutputRate) / 1000
func EstimateLLMCost(node *LLMNode, estimatedTokens int) float64 {
	if node.Tier == TierLocal {
		return 0.0 // Tier 0 is always free
	}
	estInput := float64(estimatedTokens) * 0.60
	estOutput := float64(estimatedTokens) * 0.40
	return (estInput*node.InputRateUSD + estOutput*node.OutputRateUSD) / 1_000.0
}

// EstimateComputeCost calculates the USD cost for a hardware-priced GPU call.
//
//	EstCost = estimatedSeconds * SpotRatePerSecondUSD
func EstimateComputeCost(node *ComputeNode, estimatedSeconds float64) float64 {
	return estimatedSeconds * node.SpotRatePerSecondUSD
}

// ---------------------------------------------------------------------------
// Penalty scoring (PDR-003 §4)
// ---------------------------------------------------------------------------

// weightSet holds the normalised Cost and Latency weights for a given strategy.
type weightSet struct{ cost, latency float64 }

// strategyWeights returns the weight pair for an OptimizationStrategy value.
// The two weights always sum to 1.0.
func strategyWeights(s OptimizationStrategy) weightSet {
	switch s {
	case strategyMaxSavings:
		return weightSet{cost: 1.0, latency: 0.0}
	case strategyLowLatency:
		return weightSet{cost: 0.0, latency: 1.0}
	default: // StrategyBalanced
		return weightSet{cost: 0.5, latency: 0.5}
	}
}

// OptimizationStrategy mirrors orchestrator.OptimizationStrategy so the
// oracle package does not import the orchestrator package (avoids cycles).
type OptimizationStrategy int

const (
	strategyMaxSavings  OptimizationStrategy = 0
	strategyLowLatency  OptimizationStrategy = 1
	strategyBalanced    OptimizationStrategy = 2
)

// ScoredLLMNode pairs an LLM node with its computed penalty score.
type ScoredLLMNode struct {
	Node    *LLMNode
	Penalty float64
	EstCost float64
}

// RankLLMNodes returns all LLM nodes for the given tier ranked by penalty
// score (ascending — lower is better), filtered by budget and minimum tier.
// Nodes currently circuit-broken are excluded via the openCircuits set.
//
// Parameters:
//
//	minTier        – skip nodes below this tier (e.g., skip TierLocal when escalating)
//	estimatedTokens – used by EstimateLLMCost
//	maxCostUSD      – hard budget ceiling; nodes above this are excluded
//	strategy        – controls cost vs. latency weights
//	openCircuits    – set of node names whose circuit breaker is currently open
func (o *PricingOracle) RankLLMNodes(
	minTier ProviderTier,
	estimatedTokens int,
	maxCostUSD float64,
	strategy OptimizationStrategy,
	openCircuits map[string]struct{},
) ([]ScoredLLMNode, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	w := strategyWeights(strategy)
	var ranked []ScoredLLMNode

	for _, n := range o.llmNodes {
		if n.Tier < minTier {
			continue
		}
		if _, open := openCircuits[n.Name]; open {
			continue
		}

		estCost := EstimateLLMCost(n, estimatedTokens)
		fmt.Printf("[Oracle DEBUG] RankLLMNodes checking node %s: tier=%d, estCost=%f, maxCost=%f, inputRate=%f, outputRate=%f\n", n.Name, n.Tier, estCost, maxCostUSD, n.InputRateUSD, n.OutputRateUSD)
		if maxCostUSD > 0 && estCost > maxCostUSD {
			fmt.Printf("[Oracle DEBUG] Node %s skipped: estCost %f > maxCost %f\n", n.Name, estCost, maxCostUSD)
			continue // exceeds budget
		}

		// Normalise cost to ms-scale so both axes are comparable.
		// We use a simple USD→ms conversion of 1 USD ≈ 10 000 ms (tuneable).
		const costToMsFactor = 10_000.0
		penalty := (estCost*costToMsFactor)*w.cost + n.AvgLatencyMs*w.latency
		ranked = append(ranked, ScoredLLMNode{Node: n, Penalty: penalty, EstCost: estCost})
	}

	if len(ranked) == 0 {
		var nodeNames []string
		for _, n := range o.llmNodes {
			nodeNames = append(nodeNames, fmt.Sprintf("%s(tier:%d)", n.Name, n.Tier))
		}
		fmt.Printf("[Oracle DEBUG] RankLLMNodes: 0 providers matched minTier=%d, maxCost=%f. Registered nodes: %v\n", minTier, maxCostUSD, nodeNames)
		return nil, fmt.Errorf("%w: tier>=%d", ErrNoProviders, minTier)
	}

	// Insertion sort — provider lists are small (< 10 entries typically).
	for i := 1; i < len(ranked); i++ {
		for j := i; j > 0 && ranked[j].Penalty < ranked[j-1].Penalty; j-- {
			ranked[j], ranked[j-1] = ranked[j-1], ranked[j]
		}
	}

	return ranked, nil
}

// ScoredComputeNode pairs a ComputeNode with its computed penalty score.
type ScoredComputeNode struct {
	Node    *ComputeNode
	Penalty float64
	EstCost float64
}

// RankComputeNodes ranks all registered ComputeNodes by penalty score.
//
//	estimatedSeconds – used for hardware cost estimation
//	maxCostUSD       – budget ceiling
//	strategy         – weight selector
//	openCircuits     – excluded nodes
func (o *PricingOracle) RankComputeNodes(
	estimatedSeconds float64,
	maxCostUSD float64,
	strategy OptimizationStrategy,
	openCircuits map[string]struct{},
) ([]ScoredComputeNode, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	w := strategyWeights(strategy)
	var ranked []ScoredComputeNode

	for _, n := range o.computeNodes {
		if _, open := openCircuits[n.Name]; open {
			continue
		}

		estCost := EstimateComputeCost(n, estimatedSeconds)
		if maxCostUSD > 0 && estCost > maxCostUSD {
			continue
		}

		const costToMsFactor = 10_000.0
		penalty := (estCost*costToMsFactor)*w.cost + n.AvgLatencyMs*w.latency
		ranked = append(ranked, ScoredComputeNode{Node: n, Penalty: penalty, EstCost: estCost})
	}

	if len(ranked) == 0 {
		return nil, fmt.Errorf("%w: compute tier", ErrNoProviders)
	}

	for i := 1; i < len(ranked); i++ {
		for j := i; j > 0 && ranked[j].Penalty < ranked[j-1].Penalty; j-- {
			ranked[j], ranked[j-1] = ranked[j-1], ranked[j]
		}
	}

	return ranked, nil
}
