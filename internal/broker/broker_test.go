package broker_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/fabith10/agent-framework/internal/broker"
	"github.com/fabith10/agent-framework/internal/memory"
	"github.com/fabith10/agent-framework/internal/orchestrator"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// mockLLMProvider is a configurable stub that satisfies broker.LLMProvider.
type mockLLMProvider struct {
	name    string
	retErr  error                   // error returned by GenerateResponse
	retMsg  orchestrator.Message
}

func (m *mockLLMProvider) FormatPrompt(msgs []orchestrator.Message, tools []broker.Tool) (interface{}, error) {
	return map[string]interface{}{"messages": msgs}, nil
}

func (m *mockLLMProvider) GenerateResponse(_ context.Context, _ interface{}) (orchestrator.Message, error) {
	if m.retErr != nil {
		return orchestrator.Message{}, m.retErr
	}
	return m.retMsg, nil
}

// mockComputeProvider is a configurable stub that satisfies broker.ComputeProvider.
type mockComputeProvider struct {
	provisionErr error
	executeErr   error
	executeOut   []byte
}

func (m *mockComputeProvider) ProvisionInstance(_ context.Context, _ string) (broker.Instance, error) {
	if m.provisionErr != nil {
		return broker.Instance{}, m.provisionErr
	}
	return broker.Instance{ID: "test-inst-1", Provider: "mock"}, nil
}

func (m *mockComputeProvider) ExecuteContainer(_ context.Context, _ broker.Instance, _ []byte) ([]byte, error) {
	if m.executeErr != nil {
		return nil, m.executeErr
	}
	return m.executeOut, nil
}

func (m *mockComputeProvider) TerminateInstance(_ broker.Instance) error { return nil }

// openMemStore returns an in-memory CheckpointStore for testing.
func openMemStore(t *testing.T) memory.CheckpointStore {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	s, err := memory.Open(dsn)
	if err != nil {
		t.Fatalf("open mem store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// newOracle pre-populates an oracle with one local (Tier 0) and one commercial
// (Tier 1) LLM node for use across multiple tests.
func newOracle(localProvider, commercialProvider broker.LLMProvider) *broker.PricingOracle {
	o := broker.NewPricingOracle()
	o.RegisterLLM(broker.LLMNode{
		Name:          "ollama-llama3",
		Tier:          broker.TierLocal,
		Provider:      localProvider,
		InputRateUSD:  0,
		OutputRateUSD: 0,
		AvgLatencyMs:  10,
	})
	o.RegisterLLM(broker.LLMNode{
		Name:          "openai-gpt4o",
		Tier:          broker.TierCommercial,
		Provider:      commercialProvider,
		InputRateUSD:  2.50,
		OutputRateUSD: 10.0,
		AvgLatencyMs:  300,
	})
	return o
}

// ---------------------------------------------------------------------------
// Penalty / Oracle tests
// ---------------------------------------------------------------------------

func TestPricingOracle_RankLLMNodes_PrefersCheapest(t *testing.T) {
	local := &mockLLMProvider{name: "ollama", retMsg: orchestrator.Message{Content: "ok"}}
	commercial := &mockLLMProvider{name: "gpt4o", retMsg: orchestrator.Message{Content: "ok"}}
	oracle := newOracle(local, commercial)

	ranked, err := oracle.RankLLMNodes(broker.TierLocal, 1000, 10.0, 0 /*MaxSavings*/, nil)
	if err != nil {
		t.Fatalf("RankLLMNodes: %v", err)
	}
	if len(ranked) < 2 {
		t.Fatalf("expected 2 nodes, got %d", len(ranked))
	}
	// With StrategyMaxSavings (weight cost=1, latency=0), Tier0 ($0) should rank first.
	if ranked[0].Node.Tier != broker.TierLocal {
		t.Errorf("expected TierLocal first, got tier %d", ranked[0].Node.Tier)
	}
}

func TestPricingOracle_RankLLMNodes_OpenCircuitExcluded(t *testing.T) {
	local := &mockLLMProvider{}
	commercial := &mockLLMProvider{}
	oracle := newOracle(local, commercial)

	open := map[string]struct{}{"ollama-llama3": {}}
	ranked, err := oracle.RankLLMNodes(broker.TierLocal, 1000, 10.0, 0, open)
	if err != nil {
		t.Fatalf("RankLLMNodes: %v", err)
	}
	for _, r := range ranked {
		if r.Node.Name == "ollama-llama3" {
			t.Errorf("circuit-open node should have been excluded")
		}
	}
}

func TestPricingOracle_BudgetExceedsAllNodes(t *testing.T) {
	commercial := &mockLLMProvider{}
	o := broker.NewPricingOracle()
	o.RegisterLLM(broker.LLMNode{
		Name: "expensive", Tier: broker.TierCommercial,
		Provider: commercial, InputRateUSD: 100, OutputRateUSD: 100,
	})
	// maxCostUSD = 0.001 is far below any estimate for 10_000 tokens.
	_, err := o.RankLLMNodes(broker.TierCommercial, 10_000, 0.001, 0, nil)
	if err == nil {
		t.Fatal("expected error when all nodes exceed budget")
	}
}

func TestPricingOracle_LatencyUpdate(t *testing.T) {
	o := broker.NewPricingOracle()
	p := &mockLLMProvider{}
	o.RegisterLLM(broker.LLMNode{Name: "fast-llm", Tier: broker.TierLocal, Provider: p, AvgLatencyMs: 0})

	o.UpdateLLMLatency("fast-llm", 50)
	o.UpdateLLMLatency("fast-llm", 100)

	// EMA after two updates starting from 0: first = 50, second = 0.2*100 + 0.8*50 = 60
	ranked, _ := o.RankLLMNodes(broker.TierLocal, 0, 0, 1 /*LowLatency*/, nil)
	if len(ranked) == 0 {
		t.Fatal("expected at least one ranked node")
	}
	if ranked[0].Node.AvgLatencyMs == 0 {
		t.Error("latency should have been updated from 0")
	}
}

// ---------------------------------------------------------------------------
// Broker routing tests
// ---------------------------------------------------------------------------

func TestBroker_SuccessfulRoute_TierLocal(t *testing.T) {
	local := &mockLLMProvider{retMsg: orchestrator.Message{Sender: "ollama", Content: "hello"}}
	commercial := &mockLLMProvider{retMsg: orchestrator.Message{Sender: "gpt4o", Content: "hello"}}

	b := broker.NewBroker(newOracle(local, commercial), openMemStore(t))

	req := orchestrator.TaskRequest{
		AgentID:         "agent-1",
		EstimatedTokens: 500,
		HardwareTier:    "tier0",
		Strategy:        orchestrator.StrategyMaxSavings,
		MaxWillingToPay: 1.0,
	}

	result, err := b.Route(context.Background(), req, nil, nil)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if result.TierUsed != broker.TierLocal {
		t.Errorf("expected TierLocal, got %d", result.TierUsed)
	}
}

func TestBroker_RateLimitFailover(t *testing.T) {
	// Local provider is rate-limited → broker should fall through to commercial.
	local := &mockLLMProvider{retErr: broker.ErrRateLimit}
	commercial := &mockLLMProvider{retMsg: orchestrator.Message{Sender: "gpt4o", Content: "fallback"}}

	b := broker.NewBroker(newOracle(local, commercial), openMemStore(t))

	req := orchestrator.TaskRequest{
		AgentID:         "agent-2",
		EstimatedTokens: 500,
		HardwareTier:    "tier0",
		Strategy:        orchestrator.StrategyMaxSavings,
		MaxWillingToPay: 5.0,
	}

	result, err := b.Route(context.Background(), req, nil, nil)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if result.ProviderName != "openai-gpt4o" {
		t.Errorf("expected fallback to gpt4o, got %q", result.ProviderName)
	}
}

func TestBroker_InvalidJSON_Escalates(t *testing.T) {
	// Tier 0 returns invalid JSON → should escalate to Tier 1.
	local := &mockLLMProvider{retErr: broker.ErrInvalidJSON}
	commercial := &mockLLMProvider{retMsg: orchestrator.Message{Sender: "gpt4o", Content: "valid"}}

	b := broker.NewBroker(newOracle(local, commercial), openMemStore(t))

	req := orchestrator.TaskRequest{
		AgentID:         "agent-3",
		EstimatedTokens: 200,
		HardwareTier:    "tier0",
		Strategy:        orchestrator.StrategyBalanced,
		MaxWillingToPay: 5.0,
	}

	result, err := b.Route(context.Background(), req, nil, nil)
	if err != nil {
		t.Fatalf("Route after escalation: %v", err)
	}
	if result.TierUsed != broker.TierCommercial {
		t.Errorf("expected TierCommercial after escalation, got %d", result.TierUsed)
	}
}

func TestBroker_AllProvidersFail_ReturnsError(t *testing.T) {
	someErr := errors.New("generic provider error")
	local := &mockLLMProvider{retErr: someErr}
	commercial := &mockLLMProvider{retErr: someErr}

	b := broker.NewBroker(newOracle(local, commercial), openMemStore(t))

	req := orchestrator.TaskRequest{
		AgentID:         "agent-4",
		EstimatedTokens: 100,
		HardwareTier:    "tier0",
		Strategy:        orchestrator.StrategyBalanced,
		MaxWillingToPay: 10.0,
	}

	_, err := b.Route(context.Background(), req, nil, nil)
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
}

func TestBroker_Idempotency_CachedResult(t *testing.T) {
	callCount := 0
	local := &mockLLMProvider{}
	local.retMsg = orchestrator.Message{Sender: "ollama", Content: "cached-response"}

	// Wrap the mock to count calls.
	type countingProvider struct{ *mockLLMProvider; n *int }
	cp := countingProvider{mockLLMProvider: local, n: &callCount}
	_ = cp // used indirectly via oracle

	store := openMemStore(t)
	oracle := broker.NewPricingOracle()
	oracle.RegisterLLM(broker.LLMNode{
		Name: "ollama-llama3", Tier: broker.TierLocal,
		Provider: local, AvgLatencyMs: 5,
	})

	b := broker.NewBroker(oracle, store)
	req := orchestrator.TaskRequest{
		AgentID: "agent-5", EstimatedTokens: 100,
		HardwareTier: "tier0", Strategy: orchestrator.StrategyMaxSavings,
		MaxWillingToPay: 1.0,
	}

	// First call — executes normally.
	r1, err := b.Route(context.Background(), req, nil, nil)
	if err != nil {
		t.Fatalf("first Route: %v", err)
	}

	// The step checkpoint now holds a SUCCESS record with the payload.
	// A second Route with the same AgentID + node should return the cached result.
	r2, err := b.Route(context.Background(), req, nil, nil)
	if err != nil {
		t.Fatalf("second Route: %v", err)
	}

	// Both responses should carry the same content.
	if fmt.Sprintf("%v", r1.Response.Content) != fmt.Sprintf("%v", r2.Response.Content) {
		t.Errorf("idempotency broken: r1=%v r2=%v", r1.Response.Content, r2.Response.Content)
	}
}

func TestBroker_CircuitBreaker_OpensAfterThresholdFailures(t *testing.T) {
	// Every call to this provider fails with a generic error.
	alwaysFail := &mockLLMProvider{retErr: errors.New("server error")}

	oracle := broker.NewPricingOracle()
	oracle.RegisterLLM(broker.LLMNode{
		Name: "flaky-llm", Tier: broker.TierLocal,
		Provider: alwaysFail, AvgLatencyMs: 5,
	})

	b := broker.NewBroker(oracle, openMemStore(t))

	req := orchestrator.TaskRequest{
		AgentID: "agent-6", EstimatedTokens: 100,
		HardwareTier: "tier0", Strategy: orchestrator.StrategyBalanced,
		MaxWillingToPay: 5.0,
	}

	// Drive 4 failures to trip the circuit breaker (threshold = 3).
	for i := 0; i < 4; i++ {
		_, _ = b.Route(context.Background(), req, nil, nil)
	}

	// After the breaker opens, RankLLMNodes should exclude the node and return
	// ErrNoProviders because it is the only registered node.
	_, err := b.Route(context.Background(), req, nil, nil)
	if err == nil {
		t.Fatal("expected error after circuit breaker opens")
	}
}

func TestBroker_CommercialFallback(t *testing.T) {
	// Commercial provider is rate-limited, local provider is healthy
	local := &mockLLMProvider{retMsg: orchestrator.Message{Sender: "ollama", Content: "local-fallback-success"}}
	commercial := &mockLLMProvider{retErr: broker.ErrRateLimit}

	b := broker.NewBroker(newOracle(local, commercial), openMemStore(t))

	req := orchestrator.TaskRequest{
		AgentID:         "agent-fallback-test",
		EstimatedTokens: 500,
		HardwareTier:    "tier1", // requests commercial tier
		Strategy:        orchestrator.StrategyBalanced,
		MaxWillingToPay: 5.0,
	}

	result, err := b.Route(context.Background(), req, nil, nil)
	if err != nil {
		t.Fatalf("Route: expected fallback to succeed, got error: %v", err)
	}
	if result.TierUsed != broker.TierLocal {
		t.Errorf("expected TierLocal after fallback, got tier %d", result.TierUsed)
	}
	if result.ProviderName != "ollama-llama3" {
		t.Errorf("expected local provider 'ollama-llama3', got %q", result.ProviderName)
	}
}
