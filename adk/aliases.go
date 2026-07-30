// Package adk provides the public Agent Development Kit surface.
// This file re-exports internal message, tool, request, and status types so
// that agent authors only import the "adk" package, enforcing the strict
// framework portability boundary defined in ADK_Standard.md.
package adk

import (
	"github.com/fabith10/synapse-go/internal/broker"
	"github.com/fabith10/synapse-go/internal/memory"
	"github.com/fabith10/synapse-go/internal/orchestrator"
	"github.com/fabith10/synapse-go/internal/tools"
)

// ---------------------------------------------------------------------------
// Messaging & Context (A2A Protocol & ADK Standard §2)
// ---------------------------------------------------------------------------

// Message is the canonical message unit passed across the orchestrator bus.
type Message = orchestrator.Message

// Agent is the base building block for every participant in the framework.
type Agent = orchestrator.Agent

// NewAgent constructs an Agent with a pre-allocated, buffered mailbox.
func NewAgent(id string) Agent {
	return orchestrator.NewAgent(id)
}

// Role identifies who produced a particular context log entry.
type Role = memory.Role

const (
	RoleUser   = memory.RoleUser
	RoleSystem = memory.RoleSystem
	RoleAgent  = memory.RoleAgent
	RoleTool   = memory.RoleTool
)

// ContextLog holds a single log entry.
type ContextLog = memory.ContextLog

// ---------------------------------------------------------------------------
// Optimization Strategies & Task Requests (PDR-003)
// ---------------------------------------------------------------------------

// TaskRequest carries provider requirements and ceiling options for a task.
type TaskRequest = orchestrator.TaskRequest

// OptimizationStrategy governs penalty weighting (MaxSavings, LowLatency, Balanced).
type OptimizationStrategy = orchestrator.OptimizationStrategy

const (
	StrategyMaxSavings = orchestrator.StrategyMaxSavings
	StrategyLowLatency = orchestrator.StrategyLowLatency
	StrategyBalanced   = orchestrator.StrategyBalanced
)

// ---------------------------------------------------------------------------
// Tool Registry & Sandboxes (PDR-004)
// ---------------------------------------------------------------------------

// Tool represents a registered agent capability.
type Tool = tools.Tool

// ToolTier identifies the execution sandbox (Native Go, Wazero WASM, Docker).
type ToolTier = tools.ToolTier

const (
	TierNative = tools.TierNative
	TierWasm   = tools.TierWasm
	TierDocker = tools.TierDocker
)

// Sandbox defines the code execution sandboxing contract.
type Sandbox = tools.Sandbox

// ExecutionRequest carries parameters for executing generated code.
type ExecutionRequest = tools.ExecutionRequest

// ExecutionResult holds stdout/stderr and process exit info.
type ExecutionResult = tools.ExecutionResult

// Registry is the central catalogue of all tools available to agents.
type Registry = tools.Registry

// ---------------------------------------------------------------------------
// Infrastructure Components & Orchestrator (ADK Standard §4)
// ---------------------------------------------------------------------------

// CheckpointStore abstract SQLite state persistence.
type CheckpointStore = memory.CheckpointStore

// PricingOracle tracks registered provider pricing and latency.
type PricingOracle = broker.PricingOracle

// Broker is the Hybrid Compute Broker.
type Broker = broker.Broker

// Orchestrator is the central message bus.
type Orchestrator = orchestrator.Orchestrator

// LLMNode is a registered LLM provider node.
type LLMNode = broker.LLMNode

// ComputeNode is a registered spot GPU node.
type ComputeNode = broker.ComputeNode

// MiddlewareFunc intercepts messages crossing the bus.
type MiddlewareFunc = orchestrator.MiddlewareFunc
