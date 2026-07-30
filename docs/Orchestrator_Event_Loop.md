# Orchestrator Event Loop & Routing Specification

## 1. Core Philosophy
The Orchestrator is the engine that runs everything: it executes steps, manages retries, enforces security constraints, and routes state. It utilizes an Event Loop pattern, functioning as a central coordinator that listens to a unified MessageBus channel.

By decoupling the agents from each other, an agent never directly invokes another agent. Instead, it places a Message onto the bus, which passes through a middleware pipeline (including our InjectionGuardrail) before being routed to the destination agent's personal mailbox channel.

## 2. Orchestrator Struct Definitions
The Orchestrator maintains references to all agent channels and the pre-compiled middleware pipeline.

```go
package orchestrator

import (
    "context"
    "fmt"
    "log"
)

// Handler defines the standard signature for passing messages
type Handler func(ctx context.Context, msg Message) error

// Orchestrator manages the core event loop and agent state
type Orchestrator struct {
    MessageBus           chan Message               // The central incoming event stream
    HumanApprovalChannel chan bool                  // HITL unblocking channel
    AgentMailboxes       map[string]chan Message    // Target channels for specific agents
    Pipeline             Handler                    // The compiled middleware chain
}
```

## 3. The Main Event Loop
The core loop consists of an infinite for loop combined with a select statement. This ensures non-blocking, fair event selection from multiple sources and provides a clean shutdown mechanism.

```go
// Start begins the central event loop in a dedicated goroutine.
func (o *Orchestrator) Start(ctx context.Context) {
    log.Println("Starting Orchestrator Event Loop...")

    for {
        select {
        case <-ctx.Done():
            // The stop channel provides clean shutdown without data loss
            log.Println("Event Loop: Received stop signal, shutting down...")
            return

        case msg := <-o.MessageBus:
            // 1. Pass the message into the Middleware Pipeline
            // This executes the Semantic Guardrail BEFORE any routing happens.
            if err := o.Pipeline(ctx, msg); err != nil {
                log.Printf("Message dropped by security guardrail: %v", err)
                
                // Optionally route an error back to the sender
                o.routeToAgent(Message{
                    Sender:    "SYSTEM",
                    Recipient: msg.Sender,
                    Content:   fmt.Sprintf("Security Error: %v", err),
                })
                continue
            }
        }
    }
}
```

## 4. Pipeline Compilation & Routing
Before starting the Orchestrator, the framework wraps the core routing logic with the required middlewares.

If a message safely passes through the InjectionGuardrail (where isMalicious() returned false), it reaches the final handler: baseRouter.

```go
// NewOrchestrator initializes the system and compiles the middleware
func NewOrchestrator() *Orchestrator {
    o := &Orchestrator{
        MessageBus:           make(chan Message, 100), // Buffered to prevent blocking
        HumanApprovalChannel: make(chan bool),
        AgentMailboxes:       make(map[string]chan Message),
    }

    // The base router is the final step of the pipeline
    baseRouter := func(ctx context.Context, msg Message) error {
        
        // 1. Check for Question Router / HITL state
        if msg.Recipient == "USER" {
            log.Printf("Follow-up required from %s: %s", msg.Sender, msg.Content)
            // SQLite Checkpoint: update active_tasks to 'AWAITING_CLARIFICATION'
            // Trigger Server-Sent Events (SSE) to the HTMX frontend here...
            return nil
        }

        // 2. Standard Agent Routing
        return o.routeToAgent(msg)
    }

    // Chain the middlewares: Guardrail -> BaseRouter
    o.Pipeline = InjectionGuardrail(baseRouter)

    return o
}

// routeToAgent delivers the message to the target agent's specific channel
func (o *Orchestrator) routeToAgent(msg Message) error {
    mailbox, exists := o.AgentMailboxes[msg.Recipient]
    if !exists {
        return fmt.Errorf("failed to route: Agent '%s' not found", msg.Recipient)
    }

    // Deliver message to specific agent's goroutine
    mailbox <- msg
    return nil
}
```

## Why this Architectural Flow Excels
- **Purity of Execution:** Agents are completely oblivious to the security layer. They just send messages. The orchestrator intercepts everything, runs the free local Ollama semantic check, and routes it.
- **Backpressure Handling:** By using buffered channels (e.g., `make(chan Message, 100)`), the system prevents event sources from blocking even if the central orchestrator is momentarily busy parsing a complex guardrail check.
- **No Infinite Loops:** If a malicious prompt breaks an agent and forces it to spam the bus, the middleware logs it and drops the payloads at the routing layer before they reach the expensive API models.
