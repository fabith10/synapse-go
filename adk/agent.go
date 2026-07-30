package adk

import (
	"context"

	"github.com/fabith10/synapse-go/pkg/logger"
)

// ---------------------------------------------------------------------------
// AgentInterface — the mandatory ADK contract (ADK_Standard.md §2.1)
// ---------------------------------------------------------------------------

// AgentInterface is the single interface every custom agent must implement.
// The framework interacts with agents exclusively through these three methods,
// which means an agent implementation contains zero imports from internal
// packages — it only imports from the adk surface.
//
// Authoring checklist (ADK_Standard.md §5):
//   - Handle respects ctx.Done() for clean shutdown.
//   - All LLM calls go through adk.LLMClient, not a raw SDK.
//   - All code execution goes through adk.Sandbox.
//   - Metadata["correlation_id"] is propagated on every forwarded message.
//   - No time.Sleep retry loops, no panic().
type AgentInterface interface {
	// Handle is called by the BaseAgent's dispatch goroutine for every message
	// delivered to this agent's Mailbox. Implementations must return promptly
	// for short-lived work, or spawn a goroutine and return nil immediately
	// for long-running tasks.
	Handle(ctx context.Context, msg Message) error

	// SystemPrompt returns the agent's static LLM system prompt. It is
	// injected as the first message in every Generate call. Keep it ≤ 500 tokens.
	SystemPrompt() string

	// Tools returns the MCP-schema tools this agent contributes to the shared
	// Tool Registry at bootstrap time. Return nil if the agent has no tools.
	Tools() []Tool
}

// ---------------------------------------------------------------------------
// BaseAgent — convenience wrapper that runs the Handle dispatch loop
// ---------------------------------------------------------------------------

// BaseAgent bundles an orchestrator.Agent (providing ID and Mailbox) with an
// AgentInterface implementation. Its Run method starts the goroutine that
// reads from Mailbox and calls Handle for every received message.
//
// Agents are expected to embed BaseAgent rather than orchestrator.Agent
// directly. However, agents that need fine-grained control over their dispatch
// loop may skip BaseAgent and manage the goroutine themselves.
//
//	type ResearchAgent struct {
//	    *adk.BaseAgent
//	    llm   adk.LLMClient
//	    store memory.CheckpointStore
//	}
type BaseAgent struct {
	Agent          // provides ID and Mailbox
	impl  AgentInterface
}

// NewBaseAgent constructs a BaseAgent with the given ID and implementation.
// The returned agent must be registered with the orchestrator (via
// Runtime.RegisterAgent or Orchestrator.Register) before Start is called.
func NewBaseAgent(id string, impl AgentInterface) *BaseAgent {
	return &BaseAgent{
		Agent: NewAgent(id),
		impl:  impl,
	}
}

// ResponseDispatcher is a callback hook populated by the agent runtime.
// If a message's correlation_id is registered with a waiting tool or loop,
// it dispatches the response to it and returns true to prevent the main Run loop
// from starting a concurrent ReAct loop/handler.
var ResponseDispatcher func(correlationID string, msg Message) bool

// Run starts the agent's private dispatch goroutine. It reads from Mailbox and
// calls impl.Handle for each message until ctx is cancelled. Errors returned
// by Handle are printed to stdout; they do not stop the loop.
//
// Run must be called after the agent is registered with the orchestrator but
// before (or immediately after) Orchestrator.Start. It is safe to call from
// any goroutine. Calling Run twice on the same BaseAgent starts two competing
// readers and is a programming error.
func (b *BaseAgent) Run(ctx context.Context) {
	go func() {
		for {
			select {
			case msg := <-b.Mailbox:
				if msg.Metadata != nil && msg.Metadata["correlation_id"] != "" && ResponseDispatcher != nil {
					if ResponseDispatcher(msg.Metadata["correlation_id"], msg) {
						continue // Routed to waiting tool/loop; do not run Handle
					}
				}

				agentCtx := context.WithValue(ctx, "executing_agent_id", b.ID)
				if msg.Metadata != nil && msg.Metadata["correlation_id"] != "" {
					agentCtx = context.WithValue(agentCtx, "session_id", msg.Metadata["correlation_id"])
				}
				go func(m Message) {
					if err := b.impl.Handle(agentCtx, m); err != nil {
						logger.Error("agent Handle error", "agent_id", b.ID, "error", err)
					}
				}(msg)
			case <-ctx.Done():
				return
			}
		}
	}()
}

// SystemPrompt delegates to the underlying AgentInterface.
func (b *BaseAgent) SystemPrompt() string { return b.impl.SystemPrompt() }

// Tools delegates to the underlying AgentInterface.
func (b *BaseAgent) Tools() []Tool { return b.impl.Tools() }
