# Question Router & Follow-Up Specification

> **Status:** Implemented via the existing A2A bus and `memory.CheckpointStore`. No new infrastructure required — this spec defines the **convention** agents must follow.

---

## 1. Core Philosophy

In a truly autonomous framework, agents will inevitably encounter ambiguous tasks or missing parameters. A robust framework must handle these edge cases without failing silently or making dangerous assumptions. The **Question Router** (also called the Clarification Engine) allows an agent to:

1. Asynchronously pause its own execution thread.
2. Route a specific question back to the human operator.
3. Persist its working state to SQLite so the pause can outlast the process lifetime.
4. Seamlessly resume from exactly where it stopped once the answer arrives.

Because the orchestrator is built entirely on native Go channels, asking a follow-up question requires **zero new infrastructure** — it reuses the same `"USER"` routing path defined in the HITL spec, extended with a `"CLARIFICATION_REQUIRED"` message type.

---

## 2. Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                      QuantAgent.Handle()                             │
│                                                                      │
│  1. Detects missing parameter ("strike price not provided")          │
│  2. Saves checkpoint → SQLite (Status: AWAITING_CLARIFICATION)       │
│  3. Sends Message{Recipient:"USER", Type:"CLARIFICATION_REQUIRED"}   │
│  4. Parks goroutine on  select { case reply := <-a.Mailbox }         │
└───────────────────────────────────┬─────────────────────────────────┘
                                    │ orch.Send(clarificationMsg)
                                    ▼
                    ┌───────────────────────────────┐
                    │   Central Message Bus          │
                    │   (dispatcher sees "USER")     │
                    └───────────────┬───────────────┘
                                    │ router goroutine parks (unbuffered)
                                    ▼
                    ┌───────────────────────────────┐
                    │   Terminal / Web UI Handler    │
                    │   Displays question to user    │
                    │   Reads user's typed answer    │
                    └───────────────┬───────────────┘
                                    │ orch.Send(reply → QuantAgent)
                                    ▼
                    ┌───────────────────────────────┐
                    │   QuantAgent.Mailbox           │
                    │   goroutine wakes, continues   │
                    │   execution with new context   │
                    └───────────────────────────────┘
```

---

## 3. The Clarification Lifecycle

### 3.1 Trigger — Agent detects missing context

The agent inspects its task parameters and identifies a required value that was not provided. It does **not** make an assumption, does **not** use a default, and does **not** fail with an error. Instead, it initiates the clarification flow.

```go
func (a *QuantAgent) Handle(ctx context.Context, msg orchestrator.Message) error {
    var req PricingRequest
    if err := json.Unmarshal([]byte(msg.Content), &req); err != nil {
        return fmt.Errorf("parse request: %w", err)
    }

    // Detect missing parameter before doing any expensive work.
    if req.StrikePrice == 0 {
        return a.requestClarification(ctx, msg, "strike_price",
            "I have the historical dataset loaded, but what strike price should "+
            "I use for the Black-Scholes calculation? (e.g., 150.00)")
    }

    // Proceed with full context...
    return a.runBlackScholes(ctx, req)
}
```

### 3.2 Routing to User — constructing the clarification message

The agent constructs a `Message` with `Recipient: "USER"` and populates the standardised metadata keys defined in §4.

```go
func (a *QuantAgent) requestClarification(
    ctx context.Context,
    originalMsg orchestrator.Message,
    missingParam string,
    question string,
) error {
    sessionID := originalMsg.Metadata["correlation_id"]

    // Step 1: Save the agent's working state before parking.
    if err := a.store.SaveCheckpoint(ctx, memory.Checkpoint{
        TaskID:  sessionID,
        AgentID: a.ID,
        Step:    "awaiting-" + missingParam,
        Status:  memory.StatusAwaitingClarification,
        Payload: originalMsg.Content, // preserve original task context
    }); err != nil {
        return fmt.Errorf("clarification: save checkpoint: %w", err)
    }

    // Step 2: Send the question to the USER recipient.
    a.orch.Send(orchestrator.Message{
        Sender:    a.ID,
        Recipient: "USER",
        Content:   question,
        Metadata: map[string]string{
            "Type":           "CLARIFICATION_REQUIRED",
            "correlation_id": sessionID,
            "reply_to":       a.ID,
            "missing_param":  missingParam,
            "task_context":   originalMsg.Content,
        },
    })

    // Step 3: Park this goroutine on the Mailbox until the user replies.
    // The orchestrator bus continues processing all other agents' messages.
    select {
    case reply := <-a.Mailbox:
        if reply.Metadata["Type"] != "CLARIFICATION_RESPONSE" {
            // Unexpected message type — re-queue and wait again.
            a.Mailbox <- reply
            return nil
        }
        // Resume with the clarified value injected into the original request.
        return a.resumeWithClarification(ctx, originalMsg, missingParam, reply.Content)
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

### 3.3 Thread Parking — non-blocking orchestration

The Go runtime deschedules the agent goroutine the moment it blocks on `<-a.Mailbox`. This is critical:

- The **central bus goroutine** is already unblocked (it forwarded the message to the `humanApproval` channel and is now free to process other messages).
- The **agent goroutine** is parked at zero CPU cost — no polling, no `time.Sleep`.
- Every other agent in the framework continues processing tasks as normal.
- The SQLite checkpoint means a process restart does not lose the pause state.

### 3.4 Resumption — injecting the clarified value

The terminal handler (or web UI) reads the question from `HumanApprovalChan()`, displays it, and sends the user's reply back to the agent:

```go
// In the terminal handler — handles both HITL_APPROVAL and CLARIFICATION_REQUIRED:
case req := <-approvalCh:
    msgType := req.Metadata["Type"]

    switch msgType {
    case "CLARIFICATION_REQUIRED":
        fmt.Printf("\n❓  %s asks:\n\n    %s\n\n", req.Sender, req.Content)
        fmt.Printf("    Missing: %s\n", req.Metadata["missing_param"])
        fmt.Print("\n  Your answer: ")

        answer, _ := reader.ReadString('\n')
        answer = strings.TrimSpace(answer)

        replyTo := req.Metadata["reply_to"]
        orch.Send(orchestrator.Message{
            Sender:    "USER",
            Recipient: replyTo,
            Content:   answer,
            Metadata: map[string]string{
                "Type":           "CLARIFICATION_RESPONSE",
                "correlation_id": req.Metadata["correlation_id"],
                "param":          req.Metadata["missing_param"],
            },
        })

    case "HITL_APPROVAL":
        // ... (see Human_In_The_Loop.md)
    }
```

---

## 4. Message Schema

All clarification messages must use the following `Metadata` keys. The `Type` field enables the terminal handler and the logging middleware to distinguish clarification requests from HITL approval requests.

### Clarification Request (Agent → USER)

```go
msg := orchestrator.Message{
    Sender:    "QuantAgent",
    Recipient: "USER",
    Content:   "I have the historical dataset, but what risk-free interest rate should I use for the Black-Scholes calculation?",
    Metadata: map[string]string{
        // --- Required ---
        "Type":           "CLARIFICATION_REQUIRED",
        "correlation_id": "task-uuid-1234",   // inherited from originating message
        "reply_to":       "quant-agent",      // AgentID that should receive the answer
        "missing_param":  "risk_free_rate",   // machine-readable parameter name

        // --- Optional ---
        "task_context":  originalPayload,     // task JSON for UI display
        "options":       "0.03,0.04,0.05",   // comma-separated suggestions for UI
        "default":       "0.04",             // shown in UI if user presses Enter
    },
}
```

### Clarification Response (USER → Agent)

```go
msg := orchestrator.Message{
    Sender:    "USER",
    Recipient: "quant-agent",          // must match reply_to from the request
    Content:   "0.045",               // the operator's typed answer
    Metadata: map[string]string{
        "Type":           "CLARIFICATION_RESPONSE",
        "correlation_id": "task-uuid-1234",
        "param":          "risk_free_rate",
    },
}
```

---

## 5. State & Memory Handling

### 5.1 Context Retention

Before parking, the agent writes a `step_checkpoint` to SQLite. This preserves the full task context so execution can resume exactly where it paused — even across process restarts.

```go
store.SaveCheckpoint(ctx, memory.Checkpoint{
    TaskID:  sessionID,
    AgentID: agentID,
    Step:    "awaiting-risk_free_rate",
    Status:  memory.StatusAwaitingClarification,
    Payload: originalTaskJSON,  // the full task context at the pause point
})
```

The `Payload` field stores the agent's working state as a JSON blob. On restart, the orchestrator queries for tasks with `Status = AWAITING_CLARIFICATION` and re-displays the pending question to the operator.

### 5.2 Non-Blocking Orchestration

The status flag `AWAITING_CLARIFICATION` in `active_tasks` signals to the framework's startup logic that:

| Concern | How Handled |
|---|---|
| **No CPU waste** | Agent goroutine is descheduled by Go runtime — zero busy-wait |
| **Other agents unaffected** | Each agent has its own goroutine; one parking agent never stalls others |
| **Process crash** | `step_checkpoint` in SQLite preserves state; orchestrator re-sends question on restart |
| **Long waits** | No timeout by default; add `timeout_seconds` metadata to auto-reject after N seconds |
| **Idempotency** | `AcquireStepLock` on the `awaiting-<param>` step prevents duplicate question sends on restart |

### 5.3 SQLite Status Values

The `status` column in `step_checkpoints` uses the following convention for HITL and clarification states:

| Status constant | Meaning |
|---|---|
| `memory.StatusAwaitingHuman` | HITL approval pending |
| `memory.StatusAwaitingClarification` | Clarification question pending |
| `memory.StatusRunning` | Normal execution in progress |
| `memory.StatusCompleted` | Step finished; idempotency lock held |

---

## 6. Multiple Sequential Clarifications

An agent may require several clarifications for a single task. The recommended pattern is a **clarification loop** that handles each missing parameter in turn:

```go
func (a *QuantAgent) ensureParameters(ctx context.Context, req *PricingRequest, sessionID string) error {
    missingParams := validateRequest(req)

    for _, param := range missingParams {
        answer, err := a.ask(ctx, sessionID, param)
        if err != nil {
            return err
        }
        if err := applyParam(req, param, answer); err != nil {
            return fmt.Errorf("apply param %s: %w", param, err)
        }
    }
    return nil
}

func (a *QuantAgent) ask(ctx context.Context, sessionID, param string) (string, error) {
    a.orch.Send(orchestrator.Message{
        Sender: a.ID, Recipient: "USER",
        Content: questionFor(param),
        Metadata: map[string]string{
            "Type": "CLARIFICATION_REQUIRED",
            "correlation_id": sessionID,
            "reply_to": a.ID,
            "missing_param": param,
        },
    })
    select {
    case reply := <-a.Mailbox:
        return reply.Content, nil
    case <-ctx.Done():
        return "", ctx.Err()
    }
}
```

---

## 7. Integration Checklist

Before shipping an agent that uses the Question Router:

- [ ] Agent sends `Recipient: "USER"` with `Type: "CLARIFICATION_REQUIRED"` in Metadata.
- [ ] `reply_to` is set to the agent's own ID.
- [ ] `correlation_id` is inherited from the originating message's Metadata.
- [ ] `missing_param` is a machine-readable snake_case parameter name.
- [ ] Agent saves a `step_checkpoint` with `StatusAwaitingClarification` before sending.
- [ ] Agent parks on `<-a.Mailbox` (not `time.Sleep`) and checks `reply.Metadata["Type"] == "CLARIFICATION_RESPONSE"`.
- [ ] Terminal handler's `switch msgType` block handles `"CLARIFICATION_REQUIRED"` separately from `"HITL_APPROVAL"`.
- [ ] ACL allows `"USER"` as a sender for replies (default `AllowAll` satisfies this).
- [ ] `AcquireStepLock` is called for the `awaiting-<param>` step to prevent duplicate sends on restart.
