# Agent-to-Agent (A2A) Communication Protocol

> **Status:** Active  
> **Version:** 1.0  
> **Depends on:** PDR-001 (Architecture), PDR-002 (Memory & State)

---

## 1. Core Philosophy

A2A communication within the framework is **strictly asynchronous and event-driven**. Agents do not perform direct function calls on one another. Instead, they interact exclusively through the central **Orchestrator message bus**, which routes messages via native Go channels.

This design enforces three invariants:

- **No circular dependencies.** Agent A cannot hold a reference to Agent B. It only knows B's string ID.
- **Independent lifecycles.** An agent can be restarted, swapped, or scaled without affecting its peers.
- **Back-pressure by design.** Each agent's `Mailbox` is a buffered channel; the bus never blocks the sender unless the recipient's mailbox is full, which surfaces as an observable signal rather than a hidden deadlock.

```
┌───────────┐   Send(msg)   ┌─────────────────┐   chan Message   ┌───────────┐
│  Agent A  │ ───────────▶  │  Orchestrator   │ ───────────────▶ │  Agent B  │
│           │               │  (message bus)  │                  │           │
└───────────┘               └─────────────────┘                  └───────────┘
```

---

## 2. The Message Schema

Every A2A interaction **must** adhere to the `Message` struct. No agent or tool is permitted to pass data between participants outside of this contract.

```go
type Message struct {
    Sender    string            // AgentID of the originator, e.g. "triage-agent-01"
    Recipient string            // AgentID of the target, "USER" for HITL, or a topic name
    Content   string            // Primary payload (plain text, JSON string, MCP response body)
    Metadata  map[string]string // Optional key/value envelope for routing hints and tracing
}
```

### Reserved `Metadata` Keys

| Key | Purpose |
|---|---|
| `reply_to` | The AgentID the receiver must respond to (Request/Response pattern) |
| `correlation_id` | UUID linking a request and its response for tracing |
| `topic` | Set by the bus on broadcast messages; contains the topic name |
| `traceparent` | W3C TraceContext header value injected by the tracing middleware |
| `auth_scope` | Permission scope verified by the authorization middleware |

> [!IMPORTANT]
> The `Content` field is typed as `string`. Agents that need to exchange structured data **must** JSON-encode their payload into `Content` and JSON-decode on receipt. The `Metadata` map is for routing and observability only — never for business data.

---

## 3. Communication Patterns

### 3.1 Direct Hand-off (Point-to-Point)

The simplest pattern. Agent A constructs a `Message` with a specific `Recipient` AgentID. The Orchestrator looks up the matching `Mailbox` channel and delivers the message directly.

```go
orchestrator.Send(Message{
    Sender:    "research-agent",
    Recipient: "summariser-agent",
    Content:   `{"raw_text": "..."}`,
})
```

**Delivery guarantee:** At-most-once. If the recipient's mailbox buffer is full, the message is dropped and a dead-letter metric is incremented. Agents requiring guaranteed delivery must implement acknowledgement via the Request/Response pattern.

---

### 3.2 Broadcast (Pub/Sub)

Agents may subscribe to named **Topics** on the Orchestrator. When a message is sent to a topic address, the bus **duplicates** the message into every subscriber's `Mailbox`.

**Topic naming convention:** `<domain>.<event>` using lowercase dot-notation.

| Example Topic | Intended Subscribers |
|---|---|
| `system.alerts` | Monitoring agent, logging agent |
| `data.updates` | Any agent that caches external data feeds |
| `pricing.spot_rate_changed` | Pricing Oracle update handler |
| `task.completed` | Orchestration coordinator, audit logger |

```go
// Publisher
orchestrator.Send(Message{
    Sender:    "spot-feed-agent",
    Recipient: "pricing.spot_rate_changed",  // topic address
    Content:   `{"provider": "aws-us-east", "rate_usd": 0.00042}`,
})

// Subscriber registration (called at bootstrap)
orchestrator.Subscribe("pricing.spot_rate_changed", oracleAgent)
```

> [!NOTE]
> The bus fans out by ranging over the subscriber slice and performing a non-blocking send to each `Mailbox`. Slow subscribers do not stall the publisher or other subscribers.

---

### 3.3 Request / Response

For cases where Agent A needs a **result** from Agent B before continuing, the sender populates `Metadata["reply_to"]` with its own AgentID and a `Metadata["correlation_id"]` UUID. The receiver processes the request and sends a new `Message` back to the `reply_to` address.

```go
// --- Agent A (requester) ---
corrID := uuid.New().String()

orchestrator.Send(Message{
    Sender:    "broker-agent",
    Recipient: "pricing-oracle-agent",
    Content:   `{"query": "rank_providers", "tokens": 2000}`,
    Metadata: map[string]string{
        "reply_to":       "broker-agent",
        "correlation_id": corrID,
    },
})

// Agent A blocks on its own Mailbox waiting for the correlated reply.
// The context deadline enforces the timeout — no time.Sleep.
for {
    select {
    case reply := <-brokerAgent.Mailbox:
        if reply.Metadata["correlation_id"] == corrID {
            // process reply
            return
        }
    case <-ctx.Done():
        return ctx.Err()
    }
}

// --- Agent B (responder) ---
func (a *PricingOracleAgent) Handle(ctx context.Context, msg Message) error {
    result := a.rankProviders(msg.Content)
    a.orchestrator.Send(Message{
        Sender:    "pricing-oracle-agent",
        Recipient: msg.Metadata["reply_to"],
        Content:   result,
        Metadata: map[string]string{
            "correlation_id": msg.Metadata["correlation_id"],
        },
    })
    return nil
}
```

> [!WARNING]
> Agents must **always** set a `context.WithTimeout` before entering the reply-wait loop. An unguarded loop waiting for a reply that never arrives will leak the goroutine permanently.

---

## 4. Middleware & Hooks

Every message crossing the Orchestrator bus passes through an ordered middleware chain **before** being delivered to the recipient. Middleware is synchronous within the router goroutine and must be non-blocking.

### Execution Order

```
Send(msg)
    │
    ▼
[1] Logging Middleware       → writes to context_logs (SQLite)
    │
    ▼
[2] Authorization Middleware → checks Sender → Recipient ACL
    │
    ▼
[3] Tracing Middleware       → injects/extracts OpenTelemetry span
    │
    ▼
dispatch(msg) → Recipient Mailbox / HITL channel / Topic fan-out
```

---

### 4.1 Logging Middleware

Intercepts every message and writes an entry to the `context_logs` SQLite table (PDR-002 §3.A) via the `CheckpointStore.AppendLog` interface. This provides a complete, searchable audit trail for the RAG context-injection pipeline.

```go
func loggingMiddleware(store memory.CheckpointStore) MiddlewareFunc {
    return func(ctx context.Context, msg Message, next func(Message)) {
        _ = store.AppendLog(ctx, memory.ContextLog{
            ID:        uuid.New().String(),
            AgentID:   msg.Sender,
            SessionID: msg.Metadata["correlation_id"],
            Role:      memory.RoleAgent,
            Content:   msg.Content,
            CreatedAt: time.Now().UTC(),
        })
        next(msg) // always forward, even if logging fails
    }
}
```

---

### 4.2 Authorization Middleware

Verifies that the `Sender` holds the required `auth_scope` to target `Recipient`. The ACL is loaded from the `adk.Config` at bootstrap and is read-only at runtime (no `sync.Mutex` required on the hot path).

```go
func authMiddleware(acl ACLProvider) MiddlewareFunc {
    return func(ctx context.Context, msg Message, next func(Message)) {
        if !acl.IsAllowed(msg.Sender, msg.Recipient) {
            // Drop and log; never deliver an unauthorised message.
            log.Printf("A2A auth: %q → %q denied", msg.Sender, msg.Recipient)
            return
        }
        next(msg)
    }
}
```

---

### 4.3 Tracing Middleware (OpenTelemetry)

Injects a W3C `traceparent` header into `Metadata` on outbound messages and extracts it on inbound messages to continue the distributed trace across agent boundaries. This allows a single end-user request to be traced as one cohesive span tree in any OTLP-compatible backend (Jaeger, Honeycomb, etc.).

```go
func tracingMiddleware(tracer trace.Tracer) MiddlewareFunc {
    return func(ctx context.Context, msg Message, next func(Message)) {
        // Extract parent span from incoming metadata.
        carrier := propagation.MapCarrier(msg.Metadata)
        ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)

        ctx, span := tracer.Start(ctx, fmt.Sprintf("a2a/%s→%s", msg.Sender, msg.Recipient))
        defer span.End()

        // Inject updated traceparent for the receiver.
        otel.GetTextMapPropagator().Inject(ctx, carrier)
        msg.Metadata = map[string]string(carrier)

        next(msg)
    }
}
```

> [!NOTE]
> The tracing middleware is the only one that mutates `msg.Metadata`. All other middleware must treat the message as read-only to preserve immutability guarantees across the bus.
