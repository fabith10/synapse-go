# Human-in-the-Loop (HITL) Specification

> **Status:** Implemented — `Orchestrator.HumanApprovalChan()` is live in `internal/orchestrator/router.go`.

---

## 1. Core Philosophy

Security and control are paramount in agentic frameworks. When an agent decides to perform a high-risk action — executing code, deleting files, modifying production databases, or submitting a financial transaction — that action **must not happen automatically without human oversight**.

The framework achieves this without external locking primitives, polling loops, or `time.Sleep` hacks. Instead, it uses **Go's native unbuffered channel semantics** to create a zero-cost synchronisation checkpoint:

> "An unbuffered channel send blocks the sender until a receiver is ready. This is not a hack — it is the language's intended concurrency primitive for synchronisation."

Because the channel is unbuffered, the router goroutine physically cannot deliver the next bus message until a human reads from the approval channel. This converts a software concept ("wait for approval") into a hardware-level instruction ("park this goroutine until a reader appears"). No CPU cycles are wasted.

---

## 2. Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Central Message Bus                       │
│              (buffered chan Message, cap = 256)                   │
└────────────────────────────┬────────────────────────────────────┘
                             │  router goroutine reads continuously
                             ▼
                    ┌──────────────────┐
                    │  dispatch()      │   checks msg.Recipient
                    └──────┬───────────┘
                           │
              ┌────────────▼────────────┐
              │  msg.Recipient == "USER" │
              └────────────┬────────────┘
                           │
                           ▼
              ┌────────────────────────────┐
              │  humanApproval <- msg      │  ← UNBUFFERED SEND
              │  (router goroutine PARKS   │     router blocks here
              │   until human reads)       │     until operator acts
              └────────────────────────────┘
                           │
              ┌────────────▼────────────────────┐
              │  Human-Facing Layer (Terminal /  │
              │  Web UI / Webhook handler)        │
              │  reads from HumanApprovalChan()  │
              │  prompts operator for Y/N        │
              └────────────┬─────────────────────┘
                           │
              ┌────────────▼────────────────────┐
              │  Orchestrator.Send(reply)        │
              │  Recipient = originating AgentID │
              └──────────────────────────────────┘
```

---

## 3. Implementation Mechanism

### 3.1 Phase 1 — Routing High-Risk Actions

An agent signals a high-risk intent by constructing a `Message` with `Recipient: "USER"`. The `Type` metadata key distinguishes approval requests from clarification questions (see Question Router Spec for the latter).

```go
// Inside an agent's Handle method, before executing a destructive action:
func (a *QuantAgent) Handle(ctx context.Context, msg orchestrator.Message) error {
    // Construct the HITL approval request.
    approval := orchestrator.Message{
        Sender:    a.ID,
        Recipient: "USER",
        Content:   fmt.Sprintf("Agent %q is about to submit a $%.2f SELL order on %s. Approve?", a.ID, amount, ticker),
        Metadata: map[string]string{
            "Type":           "HITL_APPROVAL",
            "correlation_id": msg.Metadata["correlation_id"],
            "reply_to":       a.ID,            // where to send the human's response
            "risk_level":     "HIGH",
            "action":         "trade.execute",
        },
    }

    // Enqueue the approval request. The router goroutine will park on the
    // unbuffered humanApproval channel until the operator responds.
    orch.Send(approval)

    // The agent then waits on its own Mailbox for the operator's reply.
    // Because Run() is running in a separate goroutine (BaseAgent.Run),
    // this select does not block the orchestrator bus.
    select {
    case reply := <-a.Mailbox:
        if reply.Content == "APPROVED" {
            return a.executeOrder(ctx)
        }
        return fmt.Errorf("order rejected by operator: %s", reply.Content)
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

### 3.2 Phase 2 — Native Goroutine Parking

The router goroutine in `router.go` delivers messages to `"USER"` via a **blocking send** on the `humanApproval` channel:

```go
// internal/orchestrator/router.go — deliver()
case "USER":
    // This send is a deliberate blocking operation. The router goroutine
    // parks here until the human-facing layer reads the message.
    // No polling. No time.Sleep. Pure channel semantics.
    o.humanApproval <- msg
```

Because `humanApproval` is unbuffered (`make(chan Message)`), the send cannot complete until there is a reader. This means:

- No other `"USER"` messages can be delivered until the operator responds.
- The bus continues processing non-`"USER"` messages from other agents (they go to buffered agent Mailboxes, unblocked).
- CPU usage during the wait is **zero** — the goroutine is descheduled by the Go runtime.

### 3.3 Phase 3 — Human Authorisation

The human-facing layer (terminal CLI, HTTP webhook, or web UI) reads from `HumanApprovalChan()` and prompts the operator:

```go
// Example terminal HITL handler — wire this into your main.go or cmd/ package.
func RunTerminalHITL(ctx context.Context, orch *orchestrator.Orchestrator) {
    approvalCh := orch.HumanApprovalChan()
    reader := bufio.NewReader(os.Stdin)

    for {
        select {
        case <-ctx.Done():
            return
        case req := <-approvalCh:
            fmt.Printf("\n╔══════════════════════════════════════╗\n")
            fmt.Printf("║  ⚠  HUMAN APPROVAL REQUIRED           ║\n")
            fmt.Printf("╚══════════════════════════════════════╝\n")
            fmt.Printf("  Agent:   %s\n", req.Sender)
            fmt.Printf("  Action:  %s\n", req.Metadata["action"])
            fmt.Printf("  Risk:    %s\n", req.Metadata["risk_level"])
            fmt.Printf("\n  %s\n\n", req.Content)
            fmt.Print("  Approve? [y/N]: ")

            line, _ := reader.ReadString('\n')
            line = strings.TrimSpace(strings.ToLower(line))

            replyTo := req.Metadata["reply_to"]
            if replyTo == "" {
                replyTo = req.Sender
            }

            if line == "y" || line == "yes" {
                orch.Send(orchestrator.Message{
                    Sender:    "USER",
                    Recipient: replyTo,
                    Content:   "APPROVED",
                    Metadata: map[string]string{
                        "correlation_id": req.Metadata["correlation_id"],
                        "Type":           "HITL_RESPONSE",
                    },
                })
            } else {
                orch.Send(orchestrator.Message{
                    Sender:    "USER",
                    Recipient: replyTo,
                    Content:   "REJECTED",
                    Metadata: map[string]string{
                        "correlation_id": req.Metadata["correlation_id"],
                        "Type":           "HITL_RESPONSE",
                        "reason":         "operator declined",
                    },
                })
            }
        }
    }
}
```

---

## 4. Metadata Keys for HITL Messages

All HITL approval messages should populate the following `Metadata` keys. These are consumed by the terminal handler, the logging middleware, and the audit trail written to SQLite.

| Key | Required | Description |
|---|---|---|
| `Type` | ✅ | Always `"HITL_APPROVAL"` for approval requests |
| `reply_to` | ✅ | AgentID the operator's response is routed back to |
| `correlation_id` | ✅ | Trace ID from the originating task (inherited from A2A Tracing Middleware) |
| `risk_level` | ✅ | `"LOW"` / `"MEDIUM"` / `"HIGH"` / `"CRITICAL"` |
| `action` | ✅ | Namespaced action string, e.g., `"trade.execute"`, `"file.delete"`, `"db.write"` |
| `timeout_seconds` | ⬜ | If set, the HITL handler auto-rejects after this many seconds of no response |
| `auth_scope` | ⬜ | Optional scope hint for future role-based approval routing |

---

## 5. Security Guarantees

| Property | Mechanism |
|---|---|
| **No action without approval** | The agent goroutine cannot proceed past the `select <-a.Mailbox` until the operator sends a reply. |
| **No CPU wasted waiting** | Go deschedules the parked goroutine entirely — zero busy-wait. |
| **Full audit trail** | The logging middleware (applied on the bus) writes every HITL message to `context_logs` in SQLite before delivery — including rejected ones. |
| **Crash-safe** | If the process crashes before the operator responds, the pending task can be recovered from SQLite (`step_checkpoints`) on restart. |
| **Bus not blocked** | Only the router goroutine blocks on the unbuffered send. Other agents continue routing messages to their buffered Mailboxes concurrently. |

---

## 6. State Persistence During HITL Wait

If the operator walks away for hours, the agent's task state must survive. The framework handles this via the `memory.CheckpointStore`:

```go
// Before sending the HITL message, the agent saves its working state:
if err := store.SaveCheckpoint(ctx, memory.Checkpoint{
    TaskID:    taskID,
    AgentID:   a.ID,
    Step:      "pre-trade-approval",
    Status:    memory.StatusAwaitingHuman,  // prevents re-execution on restart
    Payload:   currentWorkingState,
}); err != nil {
    return fmt.Errorf("hitl: save checkpoint: %w", err)
}

// Then the agent acquires an idempotency lock so that if the process restarts
// before approval, the framework knows not to re-run steps already completed:
if _, err := store.AcquireStepLock(ctx, taskID, "pre-trade-approval"); err != nil {
    return fmt.Errorf("hitl: lock: %w", err)
}
```

On process restart, the orchestrator's startup logic queries for tasks with `Status = AWAITING_HUMAN` and re-sends the HITL request to the terminal — the agent does not re-execute completed steps thanks to the idempotency lock.

---

## 7. Integration Checklist

Before shipping an agent that uses HITL, verify:

- [ ] Agent sends `Recipient: "USER"` with `Type: "HITL_APPROVAL"` in Metadata.
- [ ] Agent waits on `<-a.Mailbox` (not a sleep or a poll) for the operator reply.
- [ ] Agent checks `reply.Content == "APPROVED"` before executing the action.
- [ ] Agent saves a `step_checkpoint` with `Status: AWAITING_HUMAN` before sending the HITL message.
- [ ] `RunTerminalHITL` (or an equivalent HTTP/UI handler) is started in `main.go`.
- [ ] The `reply_to` metadata key is set to the agent's own ID.
- [ ] The `correlation_id` is inherited from the originating task message.
- [ ] All HITL messages are tested with the auth middleware ACL allowing `"USER"` as a sender.
