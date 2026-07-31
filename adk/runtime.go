package adk

import (
	"context"
	"fmt"
	"sync"

	"github.com/fabith10/synapse-go/internal/broker"
	"github.com/fabith10/synapse-go/internal/memory"
	"github.com/fabith10/synapse-go/internal/orchestrator"
	"github.com/fabith10/synapse-go/internal/tools"
	syslog "github.com/fabith10/synapse-go/pkg/logger"
)

// ---------------------------------------------------------------------------
// Runtime — the wired-up framework instance (ADK_Standard.md §4)
// ---------------------------------------------------------------------------

// Runtime holds every infrastructure component constructed by NewRuntime and
// provides the four-step bootstrap API described in ADK_Standard.md §4.
// All exported methods are safe for concurrent use after NewRuntime returns.
type Runtime struct {
	store    CheckpointStore
	oracle   *PricingOracle
	broker   *Broker
	registry *Registry
	orch     *Orchestrator
	logger   Logger
	llm      LLMClient

	mu     sync.Mutex  // guards agents slice
	agents []*BaseAgent

	cfg Config
}

// NewRuntime executes ADK bootstrap steps 1 and 2 (ADK_Standard.md §4):
//
//  1. Opens the SQLite store and runs schema migrations.
//  2. Registers all LLM and Compute provider nodes with the PricingOracle.
//  3. Constructs the Broker wired to the oracle and store.
//  4. Constructs the Orchestrator and attaches the default middleware chain
//     (Logging → Auth → Tracing) unless Config.DisableDefaultMiddleware is set.
//
// Call RegisterAgent (step 3) and Start (step 4) on the returned Runtime to
// complete the bootstrap sequence.
func NewRuntime(cfg Config) (*Runtime, error) {
	// --- Step 1: Storage -----------------------------------------------------
	if cfg.SQLiteDSN == "" {
		cfg.SQLiteDSN = "file:agent.db"
	}
	store, err := memory.Open(cfg.SQLiteDSN)
	if err != nil {
		return nil, fmt.Errorf("adk: open store: %w", err)
	}

	// --- Step 2: Pricing Oracle + Broker -------------------------------------
	if cfg.ClassifierModel != "" {
		SetClassifierModel(cfg.ClassifierModel)
	}
	oracle := broker.NewPricingOracle()
	for _, node := range cfg.LLMProviders {
		oracle.RegisterLLM(node)
	}
	for _, node := range cfg.ComputeProviders {
		oracle.RegisterCompute(node)
	}

	b := broker.NewBrokerWithBudget(oracle, store, cfg.GlobalMaxCostUSD)
	if cfg.GlobalMaxCostUSD > 0 {
		syslog.Info("Global routing ceiling configured", "max_cost_usd", cfg.GlobalMaxCostUSD)
	}

	// --- Step 3: Tool Registry + Logger + LLM Client -------------------------
	registry := tools.NewRegistry()
	logger := NewLogger(store)
	llm := newLLMClient(b, registry)

	// --- Step 4: Orchestrator + Middleware chain ------------------------------
	orch := orchestrator.NewOrchestrator()

	if !cfg.DisableDefaultMiddleware {
		// Execution order (outermost first): Logging → InjectionGuardrail → Auth → Tracing → CostLimit → CircuitBreak → deliver
		// Matches A2A_Protocol.md §4 middleware chain definition.
		acl := cfg.ACL
		if acl == nil {
			acl = AllowAll{}
		}
		orch.Use(Middleware.Logging(logger))
		orch.Use(Middleware.InjectionGuardrail(llm))
		orch.Use(Middleware.Auth(acl))
		orch.Use(Middleware.Tracing())
		if cfg.GlobalMaxCostUSD > 0 {
			orch.Use(Middleware.CostLimit(oracle, cfg.GlobalMaxCostUSD))
		}
		orch.Use(Middleware.CircuitBreak(b))
	}

	return &Runtime{
		store:    store,
		oracle:   oracle,
		broker:   b,
		registry: registry,
		orch:     orch,
		logger:   logger,
		llm:      llm,
		cfg:      cfg,
	}, nil
}

// ---------------------------------------------------------------------------
// Step 3 — Register Agents
// ---------------------------------------------------------------------------

// RegisterAgent is ADK bootstrap step 3. It:
//  1. Constructs a BaseAgent with id and impl.
//  2. Registers the agent's Tools() with the shared Tool Registry.
//  3. Registers the agent's Mailbox with the Orchestrator for message delivery.
//
// Returns the constructed BaseAgent so the caller can access its ID or embed
// it in a larger struct. Run() is called automatically by Start().
//
// Call RegisterAgent before Start() for guaranteed delivery from the first
// message. Calling it after Start() is safe due to the Orchestrator's RWMutex,
// but the agent's goroutine won't be started until the next Start() call.
func (r *Runtime) RegisterAgent(id string, impl AgentInterface) (*BaseAgent, error) {
	if id == "" {
		return nil, fmt.Errorf("adk: agent ID must not be empty")
	}
	if impl == nil {
		return nil, fmt.Errorf("adk: agent implementation must not be nil")
	}

	agent := NewBaseAgent(id, impl)

	// Register all of the agent's tools with the shared registry.
	for _, t := range impl.Tools() {
		if err := r.registry.Register(t); err != nil {
			// Collision is non-fatal (logged by Register); continue.
			syslog.Warn("adk: tool registration warning", "error", err)
		}
	}

	// Register the agent's Mailbox with the Orchestrator.
	if err := r.orch.Register(&agent.Agent); err != nil {
		return nil, fmt.Errorf("adk: register agent %q: %w", id, err)
	}

	r.mu.Lock()
	r.agents = append(r.agents, agent)
	r.mu.Unlock()

	return agent, nil
}

// GetAgents returns a slice copy of all registered agents in the runtime.
func (r *Runtime) GetAgents() []*BaseAgent {
	r.mu.Lock()
	defer r.mu.Unlock()
	res := make([]*BaseAgent, len(r.agents))
	copy(res, r.agents)
	return res
}

// ---------------------------------------------------------------------------
// Step 4 — Start
// ---------------------------------------------------------------------------

// Start is ADK bootstrap step 4. It:
//  1. Starts the Orchestrator's router goroutine (with the frozen middleware chain).
//  2. Starts a dedicated dispatch goroutine for every registered BaseAgent.
//
// ctx controls the lifetime of the entire runtime. Cancel it (or call
// Shutdown) to begin graceful shutdown.
func (r *Runtime) Start(ctx context.Context) {
	r.orch.Start(ctx)

	r.mu.Lock()
	agents := make([]*BaseAgent, len(r.agents))
	copy(agents, r.agents)
	r.mu.Unlock()

	for _, a := range agents {
		a.Run(ctx)
	}
}

// Shutdown cancels the orchestrator router goroutine, drains the bus, and
// closes the SQLite store. Call Shutdown instead of cancelling the context
// directly when you want to guarantee the store is flushed before exit.
func (r *Runtime) Shutdown() {
	r.orch.Shutdown()
	_ = r.store.Close()
}

// Wait blocks until the orchestrator router goroutine exits. Use alongside
// an external context cancel for clean signal-based shutdown:
//
//	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
//	defer cancel()
//	runtime.Start(ctx)
//	runtime.Wait()
func (r *Runtime) Wait() {
	r.orch.Wait()
}

// ---------------------------------------------------------------------------
// Accessors
// ---------------------------------------------------------------------------

// Orchestrator returns the underlying Orchestrator for advanced use cases
// (e.g., calling Subscribe for pub/sub topics, or Send directly).
func (r *Runtime) Orchestrator() *Orchestrator { return r.orch }

// Store returns the CheckpointStore for direct memory access (advanced use).
func (r *Runtime) Store() CheckpointStore { return r.store }

// LLMClient returns the hybrid routing LLM client for use inside agent Handle
// methods. Pass it to agents at construction time, not via a global.
func (r *Runtime) LLMClient() LLMClient { return r.llm }

// Registry returns the shared Tool Registry. Use it to register additional
// tools after bootstrap or to query MCP schemas.
func (r *Runtime) Registry() *Registry { return r.registry }

// Oracle returns the PricingOracle for live price updates (e.g., wiring a
// background goroutine that ingests CME spot rates via UpdateSpotRate).
func (r *Runtime) Oracle() *PricingOracle { return r.oracle }

// Logger returns the shared Logger for use inside agent Handle methods.
func (r *Runtime) Logger() Logger { return r.logger }
