// Package orchestrator provides the central message bus and routing engine for
// the agent framework. All inter-agent communication is mediated here using
// purely idiomatic Go: goroutines and channels — no external dependencies.
package orchestrator

import (
	"context"
	"fmt"
	"sync"

	"github.com/fabith10/agent-framework/pkg/logger"
)

// busBufferSize is the capacity of the central message bus channel.
// A generous buffer decouples producers (agents) from the router goroutine so
// that a slow consumer never blocks the calling agent.
const busBufferSize = 256

// Orchestrator is the single, shared message bus. It maintains a registry of
// all live agents, a topic subscription table for pub/sub fan-out, and an
// ordered middleware chain that intercepts every message before delivery.
//
// Concurrency model:
//   - One router goroutine (started by Start) reads from bus exclusively.
//   - Agent registrations are protected by mu.
//   - Topic subscriptions are protected by topicMu.
//   - Middleware is registered via Use() before Start(); a snapshot is taken
//     at Start time and used immutably by the router goroutine.
//   - The HITL channel is unbuffered; the router blocks here until a human
//     reads from HumanApprovalChan() and sends a response via Send().
type Orchestrator struct {
	bus           chan Message
	agents        map[string]*Agent
	mu            sync.RWMutex // guards agents

	humanApproval chan Message

	topics  map[string][]*Agent
	topicMu sync.RWMutex // guards topics

	// pendingMiddleware is the user-facing slice populated via Use().
	pendingMiddleware []MiddlewareFunc
	mwMu             sync.RWMutex // guards pendingMiddleware

	// activeMiddleware is a snapshot of pendingMiddleware taken at Start().
	// It is read-only after Start() returns and requires no lock.
	activeMiddleware []MiddlewareFunc

	wg     sync.WaitGroup
	cancel context.CancelFunc

	blackboard   map[string]map[string]interface{}
	blackboardMu sync.RWMutex

	FallbackResolver func(targetID string, taskContent string) string
}

// NewOrchestrator constructs and returns an Orchestrator ready to be started.
// The caller must invoke Start() to activate message routing.
func NewOrchestrator() *Orchestrator {
	return &Orchestrator{
		bus:           make(chan Message, busBufferSize),
		agents:        make(map[string]*Agent),
		topics:        make(map[string][]*Agent),
		humanApproval: make(chan Message), // unbuffered — blocks until human responds
		blackboard:    make(map[string]map[string]interface{}),
	}
}

// GetBlackboard returns a snapshot map of all key-value state variables stored
// for a given correlationID.
func (o *Orchestrator) GetBlackboard(correlationID string) map[string]interface{} {
	o.blackboardMu.RLock()
	defer o.blackboardMu.RUnlock()
	res := make(map[string]interface{})
	if m, ok := o.blackboard[correlationID]; ok {
		for k, v := range m {
			res[k] = v
		}
	}
	return res
}

// SetBlackboard updates or inserts a key-value state variable for a given
// correlationID in a thread-safe manner.
func (o *Orchestrator) SetBlackboard(correlationID, key string, val interface{}) {
	o.blackboardMu.Lock()
	defer o.blackboardMu.Unlock()
	if o.blackboard == nil {
		o.blackboard = make(map[string]map[string]interface{})
	}
	if _, ok := o.blackboard[correlationID]; !ok {
		o.blackboard[correlationID] = make(map[string]interface{})
	}
	o.blackboard[correlationID][key] = val
}

// ---------------------------------------------------------------------------
// Middleware
// ---------------------------------------------------------------------------

// Use registers a MiddlewareFunc to be executed for every message that crosses
// the bus. Middleware is applied in registration order (first-in, outermost).
// Use may be called concurrently from multiple goroutines and before or after
// Start(); however, middleware added after Start() only takes effect on the
// *next* Start() call (the router goroutine uses a frozen snapshot).
func (o *Orchestrator) Use(mw MiddlewareFunc) {
	o.mwMu.Lock()
	defer o.mwMu.Unlock()
	o.pendingMiddleware = append(o.pendingMiddleware, mw)
}

// ---------------------------------------------------------------------------
// Pub/Sub
// ---------------------------------------------------------------------------

// Subscribe registers agent to receive a copy of every message whose Recipient
// field matches topic. Multiple agents may subscribe to the same topic; the
// router fans out to all of them concurrently (non-blocking send). Subscribe
// is safe to call after Start().
func (o *Orchestrator) Subscribe(topic string, agent *Agent) error {
	if agent == nil {
		return fmt.Errorf("orchestrator: cannot subscribe nil agent to topic %q", topic)
	}
	o.topicMu.Lock()
	defer o.topicMu.Unlock()
	o.topics[topic] = append(o.topics[topic], agent)
	return nil
}

// ---------------------------------------------------------------------------
// Agent Registration
// ---------------------------------------------------------------------------

// Register adds an agent to the routing table. It is safe to call Register
// after Start() has been called; the router goroutine will see the new agent
// on its next delivery attempt.
//
// If an agent with the same ID is already registered, Register overwrites the
// previous entry and returns an error so the caller can log the collision.
func (o *Orchestrator) Register(agent *Agent) error {
	if agent == nil {
		return fmt.Errorf("orchestrator: cannot register a nil agent")
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	if _, exists := o.agents[agent.ID]; exists {
		o.agents[agent.ID] = agent
		return fmt.Errorf("orchestrator: agent %q already registered; overwriting", agent.ID)
	}

	o.agents[agent.ID] = agent
	return nil
}

// GetAgents returns a map snapshot of all currently registered agents.
func (o *Orchestrator) GetAgents() map[string]*Agent {
	o.mu.RLock()
	defer o.mu.RUnlock()
	res := make(map[string]*Agent)
	for k, v := range o.agents {
		res[k] = v
	}
	return res
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// Start launches the background router goroutine and returns immediately.
// It takes a frozen snapshot of all registered middleware so the router
// goroutine never needs to lock the middleware slice during dispatch.
//
// The provided context controls the lifetime of the router; cancelling ctx
// initiates a clean shutdown (see Wait / Shutdown).
func (o *Orchestrator) Start(ctx context.Context) {

	// Freeze the middleware chain for the router goroutine.
	o.mwMu.RLock()
	snapshot := make([]MiddlewareFunc, len(o.pendingMiddleware))
	copy(snapshot, o.pendingMiddleware)
	o.mwMu.RUnlock()
	o.activeMiddleware = snapshot

	routerCtx, cancel := context.WithCancel(ctx)
	o.cancel = cancel

	o.wg.Add(1)
	go o.route(routerCtx)
}

// route is the central dispatch loop. It runs in its own goroutine for the
// full lifetime of the Orchestrator and must never be called directly.
func (o *Orchestrator) route(ctx context.Context) {
	defer o.wg.Done()

	for {
		select {
		case <-ctx.Done():
			// Context cancelled — drain remaining messages then exit.
			for {
				select {
				case msg := <-o.bus:
					o.dispatch(ctx, msg)
				default:
					return
				}
			}

		case msg := <-o.bus:
			o.dispatch(ctx, msg)
		}
	}
}

// dispatch builds the middleware chain (from the frozen activeMiddleware slice)
// and executes it, terminating at deliver which performs the actual routing.
// Called exclusively from the route goroutine; no locking on activeMiddleware.
func (o *Orchestrator) dispatch(ctx context.Context, msg Message) {
	handler := func(m Message) {
		o.deliver(ctx, m)
	}

	// Build the chain by wrapping from right to left so middleware[0] runs
	// outermost (first to intercept, last to yield control).
	for i := len(o.activeMiddleware) - 1; i >= 0; i-- {
		mw := o.activeMiddleware[i]  // capture loop var
		next := handler              // capture current inner handler
		handler = func(m Message) {
			mw(ctx, m, next)
		}
	}

	handler(msg)
}

// deliver is the final stage of the dispatch chain: it routes a message to its
// intended destination without any middleware interference. It handles three
// routing modes:
//
//  1. "USER"  → HITL blocking channel (router goroutine parks until a human reads)
//  2. topic   → non-blocking fan-out to all subscribers
//  3. agentID → non-blocking send to that agent's private Mailbox
func (o *Orchestrator) deliver(ctx context.Context, msg Message) {

	if msg.Recipient == "USER" {
		if msg.Metadata != nil && msg.Metadata["goal_mode"] == "true" && msg.Metadata["supervisor_bypass"] != "true" && msg.Metadata["Type"] != "HITL_APPROVAL" {
			msg.Recipient = "supervisor-agent"
		}
	}

	switch msg.Recipient {
	case "USER":
		// HITL block: the router goroutine parks here until the human-facing
		// layer reads from HumanApprovalChan() and sends a response via Send().
		// No polling, no time.Sleep — pure channel semantics (PDR-001 §Security).
		select {
		case o.humanApproval <- msg:
		case <-ctx.Done():
		}

	default:
		// Check whether Recipient is a registered topic before agent lookup.
		o.topicMu.RLock()
		subscribers, isTopic := o.topics[msg.Recipient]
		o.topicMu.RUnlock()

		if isTopic {
			// Fan out: non-blocking send to each subscriber. A full mailbox
			// is logged and skipped so one slow subscriber never blocks others.
			for _, sub := range subscribers {
				select {
				case sub.Mailbox <- msg:
				default:
					logger.WithComponent("orchestrator").Warn("Mailbox full for subscriber; message dropped",
						"subscriber_id", sub.ID, "topic", msg.Recipient)
				}
			}
			return
		}

		// Direct delivery to a named agent.
		o.mu.RLock()
		agent, ok := o.agents[msg.Recipient]
		fallbackFn := o.FallbackResolver
		o.mu.RUnlock()

		if !ok && fallbackFn != nil {
			resolvedID := fallbackFn(msg.Recipient, msg.Content)
			if resolvedID != "" && resolvedID != msg.Recipient {
				logger.WithComponent("orchestrator").Info("Recipient not registered; dynamically rerouting",
					"recipient", msg.Recipient, "resolved_id", resolvedID)
				msg.Recipient = resolvedID
				o.mu.RLock()
				agent, ok = o.agents[resolvedID]
				o.mu.RUnlock()
			}
		}

		if !ok {
			logger.WithComponent("orchestrator").Warn("No agent registered for recipient; message dropped", "recipient", msg.Recipient)
			return
		}

		select {
		case agent.Mailbox <- msg:
		default:
			logger.WithComponent("orchestrator").Warn("Mailbox full for agent; message dropped", "recipient", msg.Recipient)
		}
	}
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// Send enqueues a message onto the central bus. It is safe to call from any
// goroutine. Send blocks only when the bus channel is full (busBufferSize
// messages already queued), which indicates severe back-pressure.
func (o *Orchestrator) Send(msg Message) {
	o.bus <- msg
}

// HumanApprovalChan exposes the read-only HITL channel to the human-facing
// layer (terminal handler, HTTP handler, etc.) so it can receive pending
// approval requests. The human layer responds by calling Send with the
// Recipient set back to the originating AgentID.
func (o *Orchestrator) HumanApprovalChan() <-chan Message {
	return o.humanApproval
}

// Wait blocks until the router goroutine has exited. Call Shutdown() (or cancel
// the context passed to Start) before Wait to initiate shutdown; otherwise
// Wait blocks indefinitely.
func (o *Orchestrator) Wait() {
	o.wg.Wait()
}

// Shutdown cancels the internal context and waits for the router goroutine to
// drain remaining bus messages and exit.
func (o *Orchestrator) Shutdown() {
	if o.cancel != nil {
		o.cancel()
	}
	o.wg.Wait()
}
