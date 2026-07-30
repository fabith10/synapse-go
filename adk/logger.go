package adk

import (
	"context"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Logger — adk.Logger standard library component (ADK_Standard.md §3.1)
// ---------------------------------------------------------------------------

// Logger is the standard ADK logging interface. Every agent should use it
// for intra-agent logging (reasoning steps, tool results, error conditions)
// rather than writing directly to SQLite or stdout. The middleware layer
// (A2A §4.1) logs inter-agent bus messages automatically — Logger is for
// the content *within* an agent's processing.
//
// All entries are written to the context_logs SQLite table and are available
// to the RAG retrieval pipeline (PDR-002 §4).
type Logger interface {
	// Log writes one entry to context_logs with the given role and content.
	// SessionID groups log entries that belong to the same task run;
	// pass the correlation_id from the incoming message's Metadata map.
	Log(ctx context.Context, agentID, sessionID string, role Role, content string) error

	// LogWithEmbedding is identical to Log but also stores a pre-computed
	// float32 vector embedding (serialised as []byte) for semantic RAG search.
	// Use this for reasoning steps that should be semantically retrievable.
	LogWithEmbedding(ctx context.Context, agentID, sessionID string, role Role, content string, embedding []byte) error
}

// ---------------------------------------------------------------------------
// sqliteLogger — concrete Logger backed by CheckpointStore
// ---------------------------------------------------------------------------

type sqliteLogger struct {
	store CheckpointStore
}

// NewLogger constructs a Logger that writes to the given CheckpointStore.
// The store is normally obtained from Runtime.Store().
func NewLogger(store CheckpointStore) Logger {
	return &sqliteLogger{store: store}
}

// Log writes a context_logs entry with an auto-generated UUID.
func (l *sqliteLogger) Log(
	ctx context.Context,
	agentID, sessionID string,
	role Role,
	content string,
) error {
	return l.store.AppendLog(ctx, ContextLog{
		ID:        uuid.New().String(),
		AgentID:   agentID,
		SessionID: sessionID,
		Role:      role,
		Content:   content,
	})
}

// LogWithEmbedding writes a context_logs entry that includes a vector
// embedding for semantic nearest-neighbour retrieval (PDR-002 §4).
func (l *sqliteLogger) LogWithEmbedding(
	ctx context.Context,
	agentID, sessionID string,
	role Role,
	content string,
	embedding []byte,
) error {
	return l.store.AppendLog(ctx, ContextLog{
		ID:        uuid.New().String(),
		AgentID:   agentID,
		SessionID: sessionID,
		Role:      role,
		Content:   content,
		Embedding: embedding,
	})
}

// ---------------------------------------------------------------------------
// BusLogger — thin wrapper that logs Message routing events
// ---------------------------------------------------------------------------

// busLogger adapts Logger to the Message schema so the logging
// middleware can use Logger without importing memory directly.
type busLogger struct {
	inner Logger
}

func (b *busLogger) logMessage(ctx context.Context, msg Message) {
	sessionID := ""
	if msg.Metadata != nil {
		sessionID = msg.Metadata["correlation_id"]
	}
	_ = b.inner.Log(ctx, msg.Sender, sessionID, RoleAgent, msg.Content)
}
