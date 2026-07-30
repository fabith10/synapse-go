# Agent Development Kit (ADK) Standard

> **Status:** Active  
> **Version:** 1.0  
> **Depends on:** PDR-001 (Architecture), PDR-003 (Compute Broker), PDR-004 (Tooling Sandbox), A2A_Protocol

---

## 1. Core Philosophy

The ADK is the **official surface area** developers use to build and deploy agents within the framework. It wraps the lower-level orchestrator, broker, and memory packages behind ergonomic interfaces, so agent authors focus on **business logic** rather than infrastructure plumbing.

An agent built with the ADK **must be portable**: it should not contain any direct references to a specific LLM provider SDK, cloud vendor library, or hardware tier. The broker handles all of that automatically at runtime based on the registered `adk.Config`.

**Design invariants:**

- An ADK agent is a Go struct that implements `AgentInterface`. Nothing more.
- All infrastructure (logging, sandboxing, LLM calls, failover) is injected — never imported directly by agent code.
- A developer must be able to swap from local Ollama to GPT-4o to a Spot H100 by changing **one config field**, with zero agent code changes.

---

## 2. The ADK Interface Contracts

### 2.1 `AgentInterface`

Every custom agent **must** implement this interface. The Orchestrator and broker will only interact with agents through these three methods.

```go
// AgentInterface is the mandatory contract for every agent in the framework.
// Implement all three methods to register a valid ADK agent.
type AgentInterface interface {
    // Handle is the agent's inbound message handler. It is called by the
    // Orchestrator's router goroutine for every message delivered to this
    // agent's Mailbox. Implementations must be non-blocking where possible;
    // spawn a goroutine for long-running work and return nil immediately.
    //
    // The context carries the deadline inherited from the originating
    // TaskRequest. Never ignore ctx — always forward it to broker calls.
    Handle(ctx context.Context, msg Message) error

    // SystemPrompt returns the agent's static system prompt injected at the
    // start of every LLM conversation. Keep it focused and ≤ 500 tokens.
    SystemPrompt() string

    // Tools returns the list of MCP-schema tools this agent exposes to its
    // LLM backend. The broker uses this slice in FormatPrompt calls.
    Tools() []Tool
}
```

### 2.2 `Message` (ADK surface alias)

The ADK re-exports the canonical `Message` type from the orchestrator package with the extended `Metadata` field defined in the A2A Protocol. Agent code should import from `adk`, not from `internal/orchestrator` directly.

```go
type Message struct {
    Sender    string
    Recipient string
    Content   string
    Metadata  map[string]string
}
```

### 2.3 `Tool` (ADK surface alias)

The ADK re-exports `broker.Tool` so agent authors work with one import path. The MCP JSON Schema in `Parameters` is what the LLM backend receives verbatim.

```go
type Tool struct {
    Name        string
    Description string
    Parameters  map[string]interface{} // MCP JSON Schema "input_schema.properties"
    Execute     func(ctx context.Context, args []byte) (string, error)
}
```

---

## 3. Standard Library Components

The ADK ships a **Standard Library** of pre-built components. Agent authors should always prefer these over rolling their own, both for consistency and because they integrate automatically with the framework's observability and failover stack.

### 3.1 `adk.Logger`

Automatic, zero-config logging to the `context_logs` SQLite table. Every call to `Logger.Log` writes a `ContextLog` entry via `CheckpointStore.AppendLog`. The middleware layer (A2A §4.1) logs inter-agent messages automatically; `adk.Logger` is for intra-agent logging (reasoning steps, tool results, errors).

```go
type Logger interface {
    // Log writes one structured entry to context_logs for agentID.
    Log(ctx context.Context, agentID, sessionID string, role Role, content string) error

    // LogWithEmbedding is identical to Log but also stores a pre-computed
    // vector embedding for RAG retrieval. Use this for reasoning steps that
    // should be semantically searchable.
    LogWithEmbedding(ctx context.Context, agentID, sessionID string, role Role, content string, embedding []float32) error
}
```

---

### 3.2 `adk.Sandbox`

A pre-configured execution wrapper that routes LLM-generated code through the correct isolation tier (PDR-004):

| Tier | Backend | When to use |
|---|---|---|
| **Tier 1** | Native Go function | Everyday tasks: API calls, DB queries, file reads |
| **Tier 2** | `wazero` (WebAssembly) | LLM-generated JS/Python snippets, JSON mapping |
| **Tier 3** | Docker SDK (ephemeral) | Heavy quant/GPU workloads, `pandas`/`PyTorch` pipelines |

```go
type Sandbox interface {
    // Execute routes the request to the appropriate execution tier based on
    // req.Language and req.RequiresGPU. The context deadline is always
    // enforced — no execution runs beyond ctx's timeout.
    Execute(ctx context.Context, req ExecutionRequest) ExecutionResult
}

type ExecutionRequest struct {
    Language       string // "go-native", "javascript", "python"
    RawCode        string
    RequiresGPU    bool
    TimeoutSeconds int
}

type ExecutionResult struct {
    Stdout   string
    Stderr   string
    ExitCode int
    Error    error
}
```

> [!IMPORTANT]
> Agents **must never** call shell commands, spawn processes, or import `os/exec` directly. All code execution must go through `adk.Sandbox` to enforce the tiered isolation boundary.

---

### 3.3 `adk.LLMClient`

The hybrid routing client that wraps the `broker.Broker` behind a one-method interface. Agents call `Generate` with a `TaskRequest` and get back a `Message`; the broker silently handles provider selection, penalty scoring, circuit breaking, and tier escalation.

```go
type LLMClient interface {
    // Generate submits a task to the Hybrid Compute Broker and blocks until
    // a response is received or ctx expires. The broker handles all failover
    // and escalation transparently.
    Generate(ctx context.Context, req TaskRequest, messages []Message) (Message, error)
}
```

**Example agent usage:**

```go
func (a *ResearchAgent) Handle(ctx context.Context, msg Message) error {
    result, err := a.llm.Generate(ctx, TaskRequest{
        AgentID:         a.ID,
        EstimatedTokens: 1500,
        HardwareTier:    "tier0",           // prefer free local model
        Strategy:        StrategyBalanced,
        MaxWillingToPay: 0.05,              // hard $0.05 budget ceiling
    }, []Message{
        {Sender: "SYSTEM", Content: a.SystemPrompt()},
        msg,
    })
    if err != nil {
        return fmt.Errorf("research agent generate: %w", err)
    }
    // Forward result to the next agent in the pipeline.
    a.orchestrator.Send(Message{
        Sender:    a.ID,
        Recipient: "summariser-agent",
        Content:   result.Content,
        Metadata:  msg.Metadata, // propagate correlation_id and traceparent
    })
    return nil
}
```

---

### 3.4 `adk.Middleware`

Pre-built middleware factories for the most common cross-cutting concerns. Compose them into the Orchestrator's middleware chain at bootstrap.

| Middleware | Purpose |
|---|---|
| `adk.Middleware.Logging(store)` | Writes every bus message to `context_logs` |
| `adk.Middleware.Auth(acl)` | Enforces sender → recipient ACL rules |
| `adk.Middleware.Tracing(tracer)` | Injects / extracts OpenTelemetry spans |
| `adk.Middleware.CostLimit(oracle, maxUSD)` | Rejects TaskRequests whose estimated cost exceeds a global ceiling |
| `adk.Middleware.CircuitBreak(broker)` | Surfaces circuit-breaker state as Orchestrator-level metrics |

---

## 4. Deployment Pattern

Every ADK project follows a **four-step bootstrap sequence**. This sequence is the only supported way to initialise the framework; ad-hoc wiring outside of it will break middleware ordering guarantees.

### Step 1 — Build `adk.Config`

Declare provider preferences, latency/cost policy, and database path.

```go
cfg := adk.Config{
    // Provider registration — ranked by default preference.
    LLMProviders: []broker.LLMNode{
        {Name: "ollama-llama3",  Tier: broker.TierLocal,      /* ... */},
        {Name: "openai-gpt-4o", Tier: broker.TierCommercial, /* ... */},
    },
    ComputeProviders: []broker.ComputeNode{
        {Name: "aws-spot-h100", /* ... */},
    },

    // Cost / latency policy.
    DefaultStrategy:    broker.StrategyBalanced,
    GlobalMaxCostUSD:   10.0,

    // Storage.
    SQLiteDSN: "file:agent_state.db",
}
```

### Step 2 — Initialise the runtime

```go
runtime, err := adk.NewRuntime(cfg)
if err != nil {
    log.Fatalf("adk: init runtime: %v", err)
}
defer runtime.Shutdown()
```

`adk.NewRuntime` internally:
1. Opens the SQLite store (`memory.Open`) and runs schema migrations.
2. Registers all provider nodes with the `PricingOracle`.
3. Constructs the `Broker` wired to the oracle and store.
4. Constructs the `Orchestrator` and attaches the middleware chain.

### Step 3 — Register Agents

```go
researchAgent := &ResearchAgent{
    Agent: orchestrator.NewAgent("research-agent"),
    llm:   runtime.LLMClient(),
    store: runtime.Store(),
}

if err := runtime.Orchestrator().Register(&researchAgent.Agent); err != nil {
    log.Fatalf("register agent: %v", err)
}
```

### Step 4 — Start the event loop

```go
ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
defer cancel()

runtime.Orchestrator().Start(ctx)

// Seed the first task.
runtime.Orchestrator().Send(orchestrator.Message{
    Sender:    "USER",
    Recipient: "research-agent",
    Content:   `{"query": "Summarise today's GPU spot prices."}`,
})

// Block until SIGINT / SIGTERM, then drain and shut down gracefully.
runtime.Orchestrator().Wait()
```

---

## 5. Agent Authoring Checklist

Use this checklist when building a new ADK agent to ensure compliance with the framework's architectural rules.

- [ ] Struct embeds `orchestrator.Agent` (provides `ID` and `Mailbox`)
- [ ] Implements all three `AgentInterface` methods (`Handle`, `SystemPrompt`, `Tools`)
- [ ] `Handle` accepts `context.Context` as first parameter and respects its deadline
- [ ] All LLM calls go through `adk.LLMClient.Generate`, not a raw provider SDK
- [ ] All code execution goes through `adk.Sandbox.Execute`, not `os/exec`
- [ ] All logging goes through `adk.Logger` or the automatic bus middleware
- [ ] `Metadata["correlation_id"]` and `Metadata["traceparent"]` are propagated on every forwarded message
- [ ] No `time.Sleep` retry loops — rely on the broker's circuit-breaker and the context deadline
- [ ] No `panic()` in any method — return errors explicitly

> [!TIP]
> Use the `/grill-me` slash command to have an interactive review of your agent's design against these rules before submitting a PR.
