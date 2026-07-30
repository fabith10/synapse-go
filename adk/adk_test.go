package adk_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/fabith10/agent-framework/adk"
	"github.com/fabith10/agent-framework/internal/broker"
	"github.com/fabith10/agent-framework/internal/memory"
	"github.com/fabith10/agent-framework/internal/orchestrator"
	"github.com/fabith10/agent-framework/internal/tools"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// mockLLMProvider satisfies broker.LLMProvider.
type mockLLMProvider struct {
	mu      sync.Mutex
	calls   int
	retMsg  orchestrator.Message
	retErr  error
}

func (m *mockLLMProvider) FormatPrompt(msgs []orchestrator.Message, ts []broker.Tool) (interface{}, error) {
	return msgs, nil
}

func (m *mockLLMProvider) GenerateResponse(_ context.Context, _ interface{}) (orchestrator.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	return m.retMsg, m.retErr
}

// echoAgent echoes every received message's Content to a result channel so
// tests can assert delivery without polling.
type echoAgent struct {
	results chan string
}

func (a *echoAgent) Handle(_ context.Context, msg orchestrator.Message) error {
	a.results <- msg.Content
	return nil
}
func (a *echoAgent) SystemPrompt() string   { return "I am an echo agent." }
func (a *echoAgent) Tools() []tools.Tool    { return nil }

// collectAgent accumulates received message Contents.
type collectAgent struct {
	mu      sync.Mutex
	received []string
	done    chan struct{}
	target  int
}

func newCollectAgent(target int) *collectAgent {
	return &collectAgent{done: make(chan struct{}), target: target}
}

func (a *collectAgent) Handle(_ context.Context, msg orchestrator.Message) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.received = append(a.received, msg.Content)
	if len(a.received) >= a.target {
		select {
		case <-a.done: // already closed
		default:
			close(a.done)
		}
	}
	return nil
}
func (a *collectAgent) SystemPrompt() string { return "" }
func (a *collectAgent) Tools() []tools.Tool  { return nil }

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// testConfig returns a Config backed by an in-memory SQLite store with one
// mock LLM provider pre-registered at Tier 0.
func testConfig(t *testing.T, provider broker.LLMProvider) adk.Config {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	return adk.Config{
		SQLiteDSN: dsn,
		LLMProviders: []broker.LLMNode{{
			Name:         "mock-ollama",
			Tier:         broker.TierLocal,
			Provider:     provider,
			AvgLatencyMs: 1,
		}},
		DefaultStrategy:          orchestrator.StrategyBalanced,
		DisableDefaultMiddleware: false,
	}
}

// ---------------------------------------------------------------------------
// Test: NewRuntime — four-step bootstrap
// ---------------------------------------------------------------------------

func TestNewRuntime_Bootstrap(t *testing.T) {
	provider := &mockLLMProvider{retMsg: orchestrator.Message{Content: "ok"}}
	rt, err := adk.NewRuntime(testConfig(t, provider))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	defer rt.Shutdown()

	if rt.Orchestrator() == nil {
		t.Error("Orchestrator() should not be nil")
	}
	if rt.Store() == nil {
		t.Error("Store() should not be nil")
	}
	if rt.LLMClient() == nil {
		t.Error("LLMClient() should not be nil")
	}
	if rt.Registry() == nil {
		t.Error("Registry() should not be nil")
	}
	if rt.Oracle() == nil {
		t.Error("Oracle() should not be nil")
	}
	if rt.Logger() == nil {
		t.Error("Logger() should not be nil")
	}
}

// ---------------------------------------------------------------------------
// Test: RegisterAgent + message delivery
// ---------------------------------------------------------------------------

func TestRuntime_AgentReceivesMessage(t *testing.T) {
	provider := &mockLLMProvider{retMsg: orchestrator.Message{Content: "ok"}}
	rt, err := adk.NewRuntime(testConfig(t, provider))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	echo := &echoAgent{results: make(chan string, 4)}
	if _, err := rt.RegisterAgent("echo-agent", echo); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rt.Start(ctx)
	defer rt.Shutdown()

	rt.Orchestrator().Send(orchestrator.Message{
		Sender:    "TEST",
		Recipient: "echo-agent",
		Content:   "hello from test",
	})

	select {
	case got := <-echo.results:
		if got != "hello from test" {
			t.Errorf("expected %q, got %q", "hello from test", got)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for message delivery")
	}
}

// ---------------------------------------------------------------------------
// Test: Pub/Sub topic fan-out (A2A Protocol §3.2)
// ---------------------------------------------------------------------------

func TestRuntime_TopicFanOut(t *testing.T) {
	provider := &mockLLMProvider{}
	rt, err := adk.NewRuntime(testConfig(t, provider))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	const topic = "data.updates"
	const numSubscribers = 3

	collectors := make([]*collectAgent, numSubscribers)
	for i := range collectors {
		collectors[i] = newCollectAgent(1)
		agentID := fmt.Sprintf("collector-%d", i)
		agent, err := rt.RegisterAgent(agentID, collectors[i])
		if err != nil {
			t.Fatalf("RegisterAgent %d: %v", i, err)
		}
		// Subscribe the underlying orchestrator.Agent to the topic.
		if err := rt.Orchestrator().Subscribe(topic, &agent.Agent); err != nil {
			t.Fatalf("Subscribe %d: %v", i, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rt.Start(ctx)
	defer rt.Shutdown()

	rt.Orchestrator().Send(orchestrator.Message{
		Sender:    "publisher-agent",
		Recipient: topic,
		Content:   "price-update-42",
	})

	// All subscribers must receive the message.
	for i, c := range collectors {
		select {
		case <-c.done:
			// received
		case <-ctx.Done():
			t.Fatalf("collector %d timed out waiting for topic message", i)
		}

		c.mu.Lock()
		if len(c.received) != 1 || c.received[0] != "price-update-42" {
			t.Errorf("collector %d: unexpected messages %v", i, c.received)
		}
		c.mu.Unlock()
	}
}

// ---------------------------------------------------------------------------
// Test: HITL channel (A2A Protocol §3.1 / PDR-001 §Security)
// ---------------------------------------------------------------------------

func TestRuntime_HITLChannel(t *testing.T) {
	provider := &mockLLMProvider{}
	rt, err := adk.NewRuntime(testConfig(t, provider))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rt.Start(ctx)

	// Spin a goroutine to read from the HITL channel (simulating a human).
	hitlReceived := make(chan orchestrator.Message, 1)
	go func() {
		select {
		case msg := <-rt.Orchestrator().HumanApprovalChan():
			hitlReceived <- msg
		case <-ctx.Done():
		}
	}()

	rt.Orchestrator().Send(orchestrator.Message{
		Sender:    "research-agent",
		Recipient: "USER",
		Content:   "Please approve this financial transaction.",
	})

	select {
	case msg := <-hitlReceived:
		if msg.Content != "Please approve this financial transaction." {
			t.Errorf("unexpected content: %q", msg.Content)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for HITL message")
	}

	rt.Shutdown()
}

// ---------------------------------------------------------------------------
// Test: Middleware — logging writes to SQLite
// ---------------------------------------------------------------------------

func TestRuntime_MiddlewareLogging(t *testing.T) {
	provider := &mockLLMProvider{}
	rt, err := adk.NewRuntime(testConfig(t, provider))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	echo := &echoAgent{results: make(chan string, 4)}
	if _, err := rt.RegisterAgent("logger-echo", echo); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rt.Start(ctx)
	defer rt.Shutdown()

	rt.Orchestrator().Send(orchestrator.Message{
		Sender:    "TEST",
		Recipient: "logger-echo",
		Content:   "log-me",
	})

	// Wait for delivery.
	select {
	case <-echo.results:
	case <-ctx.Done():
		t.Fatal("timed out waiting for delivery")
	}

	// Give the logging middleware a moment to commit to SQLite.
	time.Sleep(10 * time.Millisecond)

	// The log entry should be queryable via the CheckpointStore.
	logs, err := rt.Store().QueryRelevantLogs(context.Background(), "TEST", nil, 5)
	if err != nil {
		t.Fatalf("QueryRelevantLogs: %v", err)
	}
	if len(logs) == 0 {
		t.Error("expected at least one log entry from the logging middleware")
	}
}

// ---------------------------------------------------------------------------
// Test: Auth middleware — denied messages are dropped
// ---------------------------------------------------------------------------

func TestRuntime_AuthDenied(t *testing.T) {
	provider := &mockLLMProvider{}

	// ACL that denies everything.
	cfg := testConfig(t, provider)
	cfg.ACL = denyAll{}

	rt, err := adk.NewRuntime(cfg)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	echo := &echoAgent{results: make(chan string, 4)}
	if _, err := rt.RegisterAgent("auth-echo", echo); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	rt.Start(ctx)
	defer rt.Shutdown()

	rt.Orchestrator().Send(orchestrator.Message{
		Sender:    "denied-sender",
		Recipient: "auth-echo",
		Content:   "this should be dropped",
	})

	select {
	case got := <-echo.results:
		t.Errorf("expected message to be dropped by auth, but got: %q", got)
	case <-ctx.Done():
		// Correct: message was dropped, no delivery occurred within the timeout.
	}
}

// ---------------------------------------------------------------------------
// Test: Tool registration via AgentInterface.Tools()
// ---------------------------------------------------------------------------

func TestRuntime_AgentToolsRegistered(t *testing.T) {
	provider := &mockLLMProvider{}
	rt, err := adk.NewRuntime(testConfig(t, provider))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	defer rt.Shutdown()

	toolAgent := &toolProvidingAgent{}
	if _, err := rt.RegisterAgent("tool-agent", toolAgent); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}

	// The tool provided by the agent should be in the shared registry.
	schemas := rt.Registry().AllSchemas()
	found := false
	for _, s := range schemas {
		if s.Name == "calculate_cost" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'calculate_cost' tool from agent to be in shared registry")
	}
}

// ---------------------------------------------------------------------------
// Test: SQLite Logger direct usage
// ---------------------------------------------------------------------------

func TestLogger_WriteAndQuery(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	store, err := memory.Open(dsn)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	log := adk.NewLogger(store)

	if err := log.Log(context.Background(), "test-agent", "session-1", memory.RoleAgent, "reasoning step"); err != nil {
		t.Fatalf("Log: %v", err)
	}

	entries, err := store.QueryRelevantLogs(context.Background(), "test-agent", nil, 5)
	if err != nil {
		t.Fatalf("QueryRelevantLogs: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Content != "reasoning step" {
		t.Errorf("unexpected content: %q", entries[0].Content)
	}
}

// ---------------------------------------------------------------------------
// Helper stubs
// ---------------------------------------------------------------------------

type denyAll struct{}

func (denyAll) IsAllowed(_, _ string) bool { return false }

type toolProvidingAgent struct{}

func (a *toolProvidingAgent) Handle(_ context.Context, _ orchestrator.Message) error { return nil }
func (a *toolProvidingAgent) SystemPrompt() string                                   { return "" }
func (a *toolProvidingAgent) Tools() []tools.Tool {
	return []tools.Tool{{
		Name:        "calculate_cost",
		Description: "Calculates estimated API cost.",
		Tier:        tools.TierNative,
		Execute:     func(_ context.Context, _ []byte) (string, error) { return "0.01", nil },
	}}
}
