// Package memory defines the abstract storage interface and shared domain types
// for all agent state persistence. Concrete implementations (SQLite, Redis, …)
// live in sub-packages or alongside this file; callers always code against the
// CheckpointStore interface so the backend is swappable without touching agents.
package memory

import (
	"context"
	"time"
)

// ---------------------------------------------------------------------------
// Shared enums
// ---------------------------------------------------------------------------

// Role identifies who produced a particular context log entry.
type Role string

const (
	RoleUser   Role = "USER"
	RoleSystem Role = "SYSTEM"
	RoleAgent  Role = "AGENT"
	RoleTool   Role = "TOOL"
)

// TaskStatus tracks where a task currently sits in its lifecycle.
type TaskStatus string

const (
	TaskStatusInProgress     TaskStatus = "IN_PROGRESS"
	TaskStatusAwaitingHuman  TaskStatus = "AWAITING_HUMAN"
	TaskStatusResolved       TaskStatus = "RESOLVED"
)

// ComputeTier records which execution tier was ultimately used to fulfil a task.
type ComputeTier string

const (
	TierLocalOllama  ComputeTier = "TIER_0_LOCAL"
	TierCommercialAPI ComputeTier = "TIER_1_API"
	TierSpotGPU      ComputeTier = "TIER_2_SPOT"
)

// StepStatus is the execution state of a single tool checkpoint.
type StepStatus string

const (
	StepPending StepStatus = "PENDING"
	StepSuccess StepStatus = "SUCCESS"
	StepFailed  StepStatus = "FAILED"
)

// ---------------------------------------------------------------------------
// Domain records (PDR-002 schemas)
// ---------------------------------------------------------------------------

// ContextLog represents one turn of agent memory stored in context_logs.
// The Embedding field carries a serialised float32 vector (blob) and is used
// by the RAG pipeline to perform semantic nearest-neighbour retrieval.
type ContextLog struct {
	ID        string    // UUID primary key
	AgentID   string    // which agent produced this entry
	SessionID string    // groups entries that belong to one task run
	Role      Role      // who produced this entry
	Content   string    // raw text payload
	Embedding []byte    // serialised []float32 for cosine-similarity queries
	CreatedAt time.Time
}

// LongTermMemory represents a persistent learning or fact stored by an agent across sessions.
type LongTermMemory struct {
	ID        string    // UUID primary key
	AgentID   string    // which agent saved this learning
	Key       string    // concept identifier/lookup key
	Value     string    // raw text learning payload
	Embedding []byte    // serialised []float32 for similarity queries
	CreatedAt time.Time
}

// StepCheckpoint records the pre- and post-execution state of one tool call.
// A PENDING record is written *before* execution so that a retry can detect
// a partially completed action and return the cached OutputPayload instead of
// physically re-running the tool (idempotency contract from PDR-002 §5).
type StepCheckpoint struct {
	TaskID        string     // links back to active_tasks
	ToolName      string     // MCP tool identifier
	Status        StepStatus
	OutputPayload string     // JSON-encoded result (populated on SUCCESS)
	ExecutedAt    time.Time
}

// ActiveTask is a lightweight routing record that the Hybrid Compute Broker
// updates as a task moves through the compute tiers.
type ActiveTask struct {
	TaskID         string
	CurrentAgent   string
	Status         TaskStatus
	ComputeTierUsed ComputeTier
}

// Exemplar records a verified successful turn (prompt -> tool action) for dynamic few-shot learning.
type Exemplar struct {
	ID         string
	AgentID    string
	TaskGoal   string
	ToolAction string
	CreatedAt  time.Time
}

// AuditRecord stores telemetry, token usage, latency, and cost metadata for one execution turn.
type AuditRecord struct {
	ID                  string    `json:"id"`
	CorrelationID       string    `json:"correlation_id"`
	AgentID             string    `json:"agent_id"`
	ModelName           string    `json:"model_name"`
	PromptTokens        int       `json:"prompt_tokens"`
	CompletionTokens    int       `json:"completion_tokens"`
	TotalCostUSD        float64   `json:"total_cost_usd"`
	ExecutionDurationMS int64     `json:"execution_duration_ms"`
	ToolCallsCount      int       `json:"tool_calls_count"`
	Status              string    `json:"status"`
	CreatedAt           time.Time `json:"created_at"`
}

// AuditFilter contains query criteria for filtering audit records.
type AuditFilter struct {
	AgentID   string
	ModelName string
	Status    string
	Search    string
	Limit     int
}

// AuditSummary contains aggregated metrics for financial and telemetry reporting.
type AuditSummary struct {
	TotalCostUSD  float64            `json:"total_cost_usd"`
	TotalTokens   int                `json:"total_tokens"`
	TotalTasks    int                `json:"total_tasks"`
	AvgDurationMS int64              `json:"avg_duration_ms"`
	CostByAgent   map[string]float64 `json:"cost_by_agent"`
	CostByModel   map[string]float64 `json:"cost_by_model"`
}

// ---------------------------------------------------------------------------
// CheckpointStore — the single interface all storage backends must satisfy
// ---------------------------------------------------------------------------

// CheckpointStore is the central persistence abstraction for the framework.
// All agent goroutines interact with memory exclusively through this interface;
// no SQL or file-system calls are permitted outside an implementation struct.
//
// Context-first: every method accepts a context.Context so the caller can
// enforce timeouts and cancellation across slow disk or network storage.
type CheckpointStore interface {
	// ---- Context Logs -------------------------------------------------------

	// AppendLog inserts a new ContextLog row. The caller is responsible for
	// generating the UUID and pre-computing the Embedding if one is needed.
	AppendLog(ctx context.Context, entry ContextLog) error

	// QueryRelevantLogs retrieves up to limit ContextLog entries for the given
	// agentID. Implementations that support vector search should rank by cosine
	// similarity to queryEmbedding; plain SQL implementations may rank by
	// created_at DESC as a fallback.
	QueryRelevantLogs(ctx context.Context, agentID string, queryEmbedding []byte, limit int) ([]ContextLog, error)

	// UpdateLogEmbedding updates the embedding BLOB of an existing context log.
	UpdateLogEmbedding(ctx context.Context, id string, embedding []byte) error

	// ---- Audit Ledger & Cost Explorer ---------------------------------------

	// RecordAuditEntry records an audit entry with token usage and cost metrics.
	RecordAuditEntry(ctx context.Context, entry AuditRecord) error

	// QueryAuditRecords retrieves audit log entries matching the specified filter.
	QueryAuditRecords(ctx context.Context, filter AuditFilter) ([]AuditRecord, error)

	// GetAuditSummary calculates aggregated telemetry and cost metrics.
	GetAuditSummary(ctx context.Context) (AuditSummary, error)

	// PruneAuditRecords deletes audit records older than maxAgeDays (e.g. 30 days).
	PruneAuditRecords(ctx context.Context, maxAgeDays int) (int64, error)

	// ---- Exemplar Few-Shot Memory -------------------------------------------

	// SaveExemplar records a verified prompt -> tool action pair for an agent.
	SaveExemplar(ctx context.Context, agentID, taskGoal, toolAction string) error

	// GetExemplars fetches up to limit recent verified exemplars for an agent.
	GetExemplars(ctx context.Context, agentID string, limit int) ([]Exemplar, error)

	// ---- Tool Checkpoints ---------------------------------------------------

	// AcquireStepLock writes a PENDING StepCheckpoint before a tool executes.
	// Returns ErrAlreadyLocked if a PENDING or SUCCESS record for the same
	// (taskID, toolName) pair already exists — the caller must treat this as a
	// signal to skip execution and fetch the cached result instead.
	AcquireStepLock(ctx context.Context, taskID, toolName string) error

	// CompleteStep updates the checkpoint status to SUCCESS and persists the
	// JSON-encoded output payload. Must be called after successful execution.
	CompleteStep(ctx context.Context, taskID, toolName, outputPayload string) error

	// FailStep marks a checkpoint as FAILED. The broker may choose to escalate
	// to a higher compute tier rather than retrying the same tool.
	FailStep(ctx context.Context, taskID, toolName string) error

	// GetStep retrieves the most recent checkpoint for (taskID, toolName).
	// Returns ErrNotFound if no record exists.
	GetStep(ctx context.Context, taskID, toolName string) (StepCheckpoint, error)

	// ---- Long-Term Memory ---------------------------------------------------

	// SaveLongTermMemory saves or updates a persistent learning/fact across sessions.
	SaveLongTermMemory(ctx context.Context, agentID, key, value string, embedding []byte) error

	// QueryLongTermMemories retrieves up to limit persistent memories for the agent.
	QueryLongTermMemories(ctx context.Context, agentID string, queryEmbedding []byte, limit int) ([]LongTermMemory, error)

	// GetLongTermMemory fetches a specific learning/fact by agent and key.
	GetLongTermMemory(ctx context.Context, agentID, key string) (LongTermMemory, error)

	// ---- Active Task Routing ------------------------------------------------

	// UpsertTask creates or updates an ActiveTask routing record.
	UpsertTask(ctx context.Context, task ActiveTask) error

	// GetTask returns the current state of a task by ID.
	// Returns ErrNotFound if no record exists.
	GetTask(ctx context.Context, taskID string) (ActiveTask, error)

	// ---- Lifecycle ----------------------------------------------------------

	// Close releases all held resources (DB connections, file handles, etc.).
	// After Close returns, the store must not be used again.
	Close() error
}

// ---------------------------------------------------------------------------
// Sentinel errors
// ---------------------------------------------------------------------------

// ErrNotFound is returned by GetStep and GetTask when no record matches.
var ErrNotFound = storeError("record not found")

// ErrAlreadyLocked is returned by AcquireStepLock when a checkpoint for the
// given (taskID, toolName) pair is already PENDING or SUCCESS.
var ErrAlreadyLocked = storeError("step lock already held")

// storeError is a string-based error type that satisfies the error interface
// without importing the errors package at the top level.
type storeError string

func (e storeError) Error() string { return string(e) }
