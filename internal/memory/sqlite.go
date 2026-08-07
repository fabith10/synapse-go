package memory

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	// Pure-Go SQLite driver (no CGo required). The blank import registers the
	// "sqlite" driver name with database/sql automatically.
	_ "modernc.org/sqlite"
)

// ---------------------------------------------------------------------------
// Schema DDL
// ---------------------------------------------------------------------------

// schema contains every CREATE TABLE statement the store requires.
// Migrations run inside a single transaction at Open time; subsequent opens
// are idempotent because every statement uses IF NOT EXISTS.
const schema = `
CREATE TABLE IF NOT EXISTS context_logs (
    id          TEXT      PRIMARY KEY,
    agent_id    TEXT      NOT NULL,
    session_id  TEXT      NOT NULL,
    role        TEXT      NOT NULL,
    content     TEXT      NOT NULL,
    embedding   BLOB,
    created_at  DATETIME  NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_context_logs_agent    ON context_logs (agent_id);
CREATE INDEX IF NOT EXISTS idx_context_logs_session  ON context_logs (session_id);

CREATE TABLE IF NOT EXISTS step_checkpoints (
    task_id        TEXT      NOT NULL,
    tool_name      TEXT      NOT NULL,
    status         TEXT      NOT NULL,
    output_payload TEXT      NOT NULL DEFAULT '',
    executed_at    DATETIME  NOT NULL,
    PRIMARY KEY (task_id, tool_name)
);
CREATE INDEX IF NOT EXISTS idx_step_checkpoints_task ON step_checkpoints (task_id);

CREATE TABLE IF NOT EXISTS active_tasks (
    task_id           TEXT  PRIMARY KEY,
    current_agent     TEXT  NOT NULL,
    status            TEXT  NOT NULL,
    compute_tier_used TEXT  NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS long_term_memories (
    id          TEXT      PRIMARY KEY,
    agent_id    TEXT      NOT NULL,
    key         TEXT      NOT NULL,
    value       TEXT      NOT NULL,
    embedding   BLOB,
    created_at  DATETIME  NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_ltm_agent_key ON long_term_memories (agent_id, key);
CREATE INDEX IF NOT EXISTS idx_ltm_agent ON long_term_memories (agent_id);

CREATE TABLE IF NOT EXISTS exemplars (
    id          TEXT      PRIMARY KEY,
    agent_id    TEXT      NOT NULL,
    task_goal   TEXT      NOT NULL,
    tool_action TEXT      NOT NULL,
    created_at  DATETIME  NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_exemplars_agent ON exemplars (agent_id);

CREATE TABLE IF NOT EXISTS audit_records (
    id                    TEXT      PRIMARY KEY,
    correlation_id        TEXT      NOT NULL,
    agent_id              TEXT      NOT NULL,
    model_name            TEXT      NOT NULL,
    prompt_tokens         INTEGER   NOT NULL DEFAULT 0,
    completion_tokens     INTEGER   NOT NULL DEFAULT 0,
    total_cost_usd        REAL      NOT NULL DEFAULT 0.0,
    execution_duration_ms INTEGER   NOT NULL DEFAULT 0,
    tool_calls_count      INTEGER   NOT NULL DEFAULT 0,
    status                TEXT      NOT NULL,
    created_at            DATETIME  NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_agent ON audit_records (agent_id);
CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_records (created_at);

`

// ---------------------------------------------------------------------------
// sqliteStore — concrete CheckpointStore implementation
// ---------------------------------------------------------------------------

// sqliteStore wraps a *sql.DB connection to a local SQLite file.
// All public methods are safe for concurrent use by multiple goroutines
// because database/sql manages an internal connection pool and SQLite is
// running in WAL mode (readers never block writers).
type sqliteStore struct {
	db *sql.DB
}

// Open creates (or opens) a SQLite database at dsn, applies the WAL pragma
// and schema migrations, and returns a ready-to-use CheckpointStore.
//
// dsn examples:
//
//	"file:agent.db"              → persistent file on disk
//	"file::memory:?cache=shared" → in-memory DB (useful for tests)
func Open(dsn string) (CheckpointStore, error) {
	isMemory := strings.Contains(dsn, ":memory:") || strings.Contains(dsn, "mode=memory")

	effectiveDSN := dsn
	if !isMemory && !strings.Contains(dsn, "_pragma=") && !strings.Contains(dsn, "_journal_mode=") {
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		effectiveDSN = dsn + sep + "_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)"
	}

	db, err := sql.Open("sqlite", effectiveDSN)
	if err != nil {
		return nil, fmt.Errorf("memory: open sqlite %q: %w", dsn, err)
	}

	if err := applySchema(db, isMemory); err != nil {
		db.Close()
		return nil, err
	}

	return &sqliteStore{db: db}, nil
}

// applySchema runs the DDL inside a single transaction.
func applySchema(db *sql.DB, isMemory bool) error {
	var pragmas []string
	if !isMemory {
		pragmas = []string{
			"PRAGMA journal_mode=WAL;",
			"PRAGMA synchronous=NORMAL;",
			"PRAGMA busy_timeout=5000;",
		}
	} else {
		pragmas = []string{
			"PRAGMA busy_timeout=5000;",
		}
	}
	for _, p := range pragmas {
		_, _ = db.Exec(p)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("memory: begin schema tx: %w", err)
	}

	if _, err := tx.Exec(schema); err != nil {
		tx.Rollback()
		return fmt.Errorf("memory: apply schema: %w", err)
	}

	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Context Logs
// ---------------------------------------------------------------------------

// AppendLog inserts one ContextLog row. The caller must supply a unique ID
// (UUID recommended) and a pre-formatted CreatedAt timestamp.
func (s *sqliteStore) AppendLog(ctx context.Context, e ContextLog) error {
	const q = `
INSERT INTO context_logs (id, agent_id, session_id, role, content, embedding, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`

	_, err := s.db.ExecContext(ctx, q,
		e.ID, e.AgentID, e.SessionID, string(e.Role),
		e.Content, e.Embedding, e.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("memory: AppendLog: %w", err)
	}
	return nil
}

// DeserializeEmbedding decodes a byte slice of LittleEndian float64s into []float64.
func DeserializeEmbedding(b []byte) []float64 {
	if len(b) == 0 || len(b)%8 != 0 {
		return nil
	}
	v := make([]float64, len(b)/8)
	for i := range v {
		bits := binary.LittleEndian.Uint64(b[i*8 : i*8+8])
		v[i] = math.Float64frombits(bits)
	}
	return v
}

// CosineSimilarity computes the dot product of two L2-normalised vectors.
func CosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	shorter, longer := a, b
	if len(a) > len(b) {
		shorter, longer = b, a
	}
	var dot float64
	for i, v := range shorter {
		dot += v * longer[i]
	}
	return dot
}

// QueryRelevantLogs returns up to limit entries for agentID ordered by
// vector cosine similarity relevance to queryEmbedding when queryEmbedding is provided.
// If queryEmbedding is empty or no embeddings exist, fallbacks to recency order (created_at DESC).
func (s *sqliteStore) QueryRelevantLogs(ctx context.Context, agentID string, queryEmbedding []byte, limit int) ([]ContextLog, error) {
	var sessionID string
	if ctx != nil {
		if val, ok := ctx.Value("session_id").(string); ok {
			sessionID = val
		}
	}

	var q string
	var rows *sql.Rows
	var err error

	// If queryEmbedding is supplied, fetch candidate logs with non-null embeddings for vector ranking.
	qLimit := limit
	if len(queryEmbedding) > 0 {
		qLimit = limit * 10 // Fetch candidate pool for similarity re-ranking
	}

	if agentID != "" && sessionID != "" {
		q = `
SELECT id, agent_id, session_id, role, content, embedding, created_at
FROM   context_logs
WHERE  agent_id = ? AND session_id = ?
ORDER  BY created_at DESC
LIMIT  ?`
		rows, err = s.db.QueryContext(ctx, q, agentID, sessionID, qLimit)
	} else if agentID != "" {
		q = `
SELECT id, agent_id, session_id, role, content, embedding, created_at
FROM   context_logs
WHERE  agent_id = ?
ORDER  BY created_at DESC
LIMIT  ?`
		rows, err = s.db.QueryContext(ctx, q, agentID, qLimit)
	} else if sessionID != "" {
		q = `
SELECT id, agent_id, session_id, role, content, embedding, created_at
FROM   context_logs
WHERE  session_id = ?
ORDER  BY created_at DESC
LIMIT  ?`
		rows, err = s.db.QueryContext(ctx, q, sessionID, qLimit)
	} else {
		q = `
SELECT id, agent_id, session_id, role, content, embedding, created_at
FROM   context_logs
ORDER  BY created_at DESC
LIMIT  ?`
		rows, err = s.db.QueryContext(ctx, q, qLimit)
	}

	if err != nil {
		return nil, fmt.Errorf("memory: QueryRelevantLogs: %w", err)
	}
	defer rows.Close()

	var logs []ContextLog
	for rows.Next() {
		var e ContextLog
		var createdAtStr string
		if err := rows.Scan(
			&e.ID, &e.AgentID, &e.SessionID, &e.Role,
			&e.Content, &e.Embedding, &createdAtStr,
		); err != nil {
			return nil, fmt.Errorf("memory: QueryRelevantLogs scan: %w", err)
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAtStr)
		logs = append(logs, e)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Perform vector cosine-similarity ranking if queryEmbedding is present
	qVec := DeserializeEmbedding(queryEmbedding)
	if len(qVec) > 0 && len(logs) > 0 {
		type scored struct {
			log   ContextLog
			score float64
		}
		scoredLogs := make([]scored, 0, len(logs))
		hasValidEmbedding := false

		for _, l := range logs {
			score := 0.0
			if len(l.Embedding) > 0 {
				embVec := DeserializeEmbedding(l.Embedding)
				if len(embVec) > 0 {
					score = CosineSimilarity(qVec, embVec)
					hasValidEmbedding = true
				}
			}
			scoredLogs = append(scoredLogs, scored{log: l, score: score})
		}

		if hasValidEmbedding {
			// ponytail: In-memory O(N log N) sort via stdlib slices.SortFunc over candidate log slice. For million-scale vector indexing, upgrade to sqlite-vss or FAISS.
			// Sort by similarity score DESC
			slices.SortFunc(scoredLogs, func(a, b scored) int {
				if b.score > a.score {
					return 1
				} else if b.score < a.score {
					return -1
				}
				return 0
			})
			outLimit := limit
			if outLimit > len(scoredLogs) {
				outLimit = len(scoredLogs)
			}
			out := make([]ContextLog, outLimit)
			for i := 0; i < outLimit; i++ {
				out[i] = scoredLogs[i].log
			}
			return out, nil
		}
	}

	// Recency fallback if vector ranking not applicable
	if len(logs) > limit {
		logs = logs[:limit]
	}
	return logs, nil
}

// UpdateLogEmbedding updates the embedding BLOB of an existing context log.
func (s *sqliteStore) UpdateLogEmbedding(ctx context.Context, id string, embedding []byte) error {
	const q = `UPDATE context_logs SET embedding = ? WHERE id = ?`
	_, err := s.db.ExecContext(ctx, q, embedding, id)
	if err != nil {
		return fmt.Errorf("memory: UpdateLogEmbedding: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Tool Checkpoints — idempotency layer
// ---------------------------------------------------------------------------

// AcquireStepLock writes a PENDING checkpoint before a tool executes.
// If a record (PENDING or SUCCESS) already exists for (taskID, toolName),
// ErrAlreadyLocked is returned and the caller should skip execution.
func (s *sqliteStore) AcquireStepLock(ctx context.Context, taskID, toolName string) error {
	// Check for an existing record first so we can surface ErrAlreadyLocked
	// with a meaningful status rather than relying on a primary-key conflict.
	existing, err := s.GetStep(ctx, taskID, toolName)
	if err == nil {
		// A record exists — only block if it is PENDING or SUCCESS.
		if existing.Status == StepPending || existing.Status == StepSuccess {
			return ErrAlreadyLocked
		}
		// A FAILED record exists; allow the caller to retry by overwriting.
	}

	const q = `
INSERT OR REPLACE INTO step_checkpoints (task_id, tool_name, status, output_payload, executed_at)
VALUES (?, ?, ?, '', ?)`

	_, err = s.db.ExecContext(ctx, q,
		taskID, toolName, string(StepPending),
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("memory: AcquireStepLock: %w", err)
	}
	return nil
}

// CompleteStep transitions a checkpoint from PENDING → SUCCESS and persists
// the JSON-encoded output payload for future idempotency checks.
func (s *sqliteStore) CompleteStep(ctx context.Context, taskID, toolName, outputPayload string) error {
	const q = `
UPDATE step_checkpoints
SET    status = ?, output_payload = ?, executed_at = ?
WHERE  task_id = ? AND tool_name = ?`

	res, err := s.db.ExecContext(ctx, q,
		string(StepSuccess), outputPayload,
		time.Now().UTC().Format(time.RFC3339Nano),
		taskID, toolName,
	)
	if err != nil {
		return fmt.Errorf("memory: CompleteStep: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("memory: CompleteStep: no checkpoint found for task=%q tool=%q", taskID, toolName)
	}
	return nil
}

// FailStep transitions a checkpoint to FAILED. The broker can subsequently
// escalate to a higher compute tier on the next TaskRequest.
func (s *sqliteStore) FailStep(ctx context.Context, taskID, toolName string) error {
	const q = `
UPDATE step_checkpoints
SET    status = ?, executed_at = ?
WHERE  task_id = ? AND tool_name = ?`

	_, err := s.db.ExecContext(ctx, q,
		string(StepFailed),
		time.Now().UTC().Format(time.RFC3339Nano),
		taskID, toolName,
	)
	if err != nil {
		return fmt.Errorf("memory: FailStep: %w", err)
	}
	return nil
}

// GetStep retrieves the checkpoint for (taskID, toolName).
// Returns ErrNotFound when no record exists.
func (s *sqliteStore) GetStep(ctx context.Context, taskID, toolName string) (StepCheckpoint, error) {
	const q = `
SELECT task_id, tool_name, status, output_payload, executed_at
FROM   step_checkpoints
WHERE  task_id = ? AND tool_name = ?`

	var c StepCheckpoint
	var execAtStr string
	err := s.db.QueryRowContext(ctx, q, taskID, toolName).Scan(
		&c.TaskID, &c.ToolName, &c.Status, &c.OutputPayload, &execAtStr,
	)
	if err == sql.ErrNoRows {
		return StepCheckpoint{}, ErrNotFound
	}
	if err != nil {
		return StepCheckpoint{}, fmt.Errorf("memory: GetStep: %w", err)
	}
	c.ExecutedAt, _ = time.Parse(time.RFC3339Nano, execAtStr)
	return c, nil
}

// ---------------------------------------------------------------------------
// Active Task Routing
// ---------------------------------------------------------------------------

// UpsertTask inserts or replaces an ActiveTask record. Used by the Compute
// Broker whenever it transitions a task to a new agent or compute tier.
func (s *sqliteStore) UpsertTask(ctx context.Context, t ActiveTask) error {
	const q = `
INSERT OR REPLACE INTO active_tasks (task_id, current_agent, status, compute_tier_used)
VALUES (?, ?, ?, ?)`

	_, err := s.db.ExecContext(ctx, q,
		t.TaskID, t.CurrentAgent, string(t.Status), string(t.ComputeTierUsed),
	)
	if err != nil {
		return fmt.Errorf("memory: UpsertTask: %w", err)
	}
	return nil
}

// GetTask returns the routing record for taskID.
// Returns ErrNotFound when no record exists.
func (s *sqliteStore) GetTask(ctx context.Context, taskID string) (ActiveTask, error) {
	const q = `
SELECT task_id, current_agent, status, compute_tier_used
FROM   active_tasks
WHERE  task_id = ?`

	var t ActiveTask
	err := s.db.QueryRowContext(ctx, q, taskID).Scan(
		&t.TaskID, &t.CurrentAgent, &t.Status, &t.ComputeTierUsed,
	)
	if err == sql.ErrNoRows {
		return ActiveTask{}, ErrNotFound
	}
	if err != nil {
		return ActiveTask{}, fmt.Errorf("memory: GetTask: %w", err)
	}
	return t, nil
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// Close releases the underlying database connection pool. After Close returns,
// no methods on this store may be called.
func (s *sqliteStore) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("memory: Close: %w", err)
	}
	return nil
}

// SaveLongTermMemory saves or updates a persistent learning/fact.
func (s *sqliteStore) SaveLongTermMemory(ctx context.Context, agentID, key, value string, embedding []byte) error {
	const q = `
INSERT INTO long_term_memories (id, agent_id, key, value, embedding, created_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(agent_id, key) DO UPDATE SET
	value = excluded.value,
	embedding = excluded.embedding,
	created_at = excluded.created_at`

	id := uuid.New().String()
	_, err := s.db.ExecContext(ctx, q,
		id, agentID, key, value, embedding, time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("memory: SaveLongTermMemory: %w", err)
	}
	return nil
}

// GetLongTermMemory fetches a specific learning/fact by agent and key.
func (s *sqliteStore) GetLongTermMemory(ctx context.Context, agentID, key string) (LongTermMemory, error) {
	const q = `
SELECT id, agent_id, key, value, embedding, created_at
FROM   long_term_memories
WHERE  agent_id = ? AND key = ?`

	var m LongTermMemory
	var createdAtStr string
	err := s.db.QueryRowContext(ctx, q, agentID, key).Scan(
		&m.ID, &m.AgentID, &m.Key, &m.Value, &m.Embedding, &createdAtStr,
	)
	if err == sql.ErrNoRows {
		return LongTermMemory{}, ErrNotFound
	}
	if err != nil {
		return LongTermMemory{}, fmt.Errorf("memory: GetLongTermMemory: %w", err)
	}
	m.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAtStr)
	return m, nil
}

// QueryLongTermMemories retrieves up to limit persistent memories for the agent ordered by cosine similarity when queryEmbedding is provided.
func (s *sqliteStore) QueryLongTermMemories(ctx context.Context, agentID string, queryEmbedding []byte, limit int) ([]LongTermMemory, error) {
	var q string
	var rows *sql.Rows
	var err error

	qLimit := limit
	if len(queryEmbedding) > 0 {
		qLimit = limit * 10
	}

	if agentID == "" || agentID == "global" {
		q = `
SELECT id, agent_id, key, value, embedding, created_at
FROM   long_term_memories
ORDER  BY created_at DESC
LIMIT  ?`
		rows, err = s.db.QueryContext(ctx, q, qLimit)
	} else {
		q = `
SELECT id, agent_id, key, value, embedding, created_at
FROM   long_term_memories
WHERE  agent_id = ? OR agent_id = 'global'
ORDER  BY created_at DESC
LIMIT  ?`
		rows, err = s.db.QueryContext(ctx, q, agentID, qLimit)
	}

	if err != nil {
		return nil, fmt.Errorf("memory: QueryLongTermMemories: %w", err)
	}
	defer rows.Close()

	var memories []LongTermMemory
	for rows.Next() {
		var m LongTermMemory
		var createdAtStr string
		if err := rows.Scan(
			&m.ID, &m.AgentID, &m.Key, &m.Value, &m.Embedding, &createdAtStr,
		); err != nil {
			return nil, fmt.Errorf("memory: QueryLongTermMemories scan: %w", err)
		}
		m.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAtStr)
		memories = append(memories, m)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Perform vector cosine-similarity ranking if queryEmbedding is present
	qVec := DeserializeEmbedding(queryEmbedding)
	if len(qVec) > 0 && len(memories) > 0 {
		type scored struct {
			memory LongTermMemory
			score  float64
		}
		scoredMems := make([]scored, 0, len(memories))
		hasValidEmbedding := false

		for _, m := range memories {
			score := 0.0
			if len(m.Embedding) > 0 {
				embVec := DeserializeEmbedding(m.Embedding)
				if len(embVec) > 0 {
					score = CosineSimilarity(qVec, embVec)
					hasValidEmbedding = true
				}
			}
			scoredMems = append(scoredMems, scored{memory: m, score: score})
		}

		if hasValidEmbedding {
			for i := 1; i < len(scoredMems); i++ {
				for j := i; j > 0 && scoredMems[j].score > scoredMems[j-1].score; j-- {
					scoredMems[j], scoredMems[j-1] = scoredMems[j-1], scoredMems[j]
				}
			}
			outLimit := limit
			if outLimit > len(scoredMems) {
				outLimit = len(scoredMems)
			}
			out := make([]LongTermMemory, outLimit)
			for i := 0; i < outLimit; i++ {
				out[i] = scoredMems[i].memory
			}
			return out, nil
		}
	}

	if len(memories) > limit {
		memories = memories[:limit]
	}
	return memories, nil
}

// SaveExemplar records a verified prompt -> tool action pair for dynamic few-shot learning.
func (s *sqliteStore) SaveExemplar(ctx context.Context, agentID, taskGoal, toolAction string) error {
	id := uuid.New().String()
	nowStr := time.Now().Format(time.RFC3339Nano)
	q := `
INSERT INTO exemplars (id, agent_id, task_goal, tool_action, created_at)
VALUES (?, ?, ?, ?, ?)`
	_, err := s.db.ExecContext(ctx, q, id, agentID, taskGoal, toolAction, nowStr)
	if err != nil {
		return fmt.Errorf("memory: SaveExemplar: %w", err)
	}
	return nil
}

// GetExemplars fetches up to limit recent verified exemplars for an agent.
func (s *sqliteStore) GetExemplars(ctx context.Context, agentID string, limit int) ([]Exemplar, error) {
	q := `
SELECT id, agent_id, task_goal, tool_action, created_at
FROM   exemplars
WHERE  agent_id = ? OR agent_id = 'global'
ORDER  BY created_at DESC
LIMIT  ?`
	rows, err := s.db.QueryContext(ctx, q, agentID, limit)
	if err != nil {
		return nil, fmt.Errorf("memory: GetExemplars: %w", err)
	}
	defer rows.Close()

	var exemplars []Exemplar
	for rows.Next() {
		var ex Exemplar
		var createdAtStr string
		if err := rows.Scan(&ex.ID, &ex.AgentID, &ex.TaskGoal, &ex.ToolAction, &createdAtStr); err != nil {
			return nil, fmt.Errorf("memory: GetExemplars scan: %w", err)
		}
		ex.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAtStr)
		exemplars = append(exemplars, ex)
	}
	return exemplars, rows.Err()
}



// RecordAuditEntry records an audit entry with token usage and cost metrics.
func (s *sqliteStore) RecordAuditEntry(ctx context.Context, entry AuditRecord) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	nowStr := entry.CreatedAt.Format(time.RFC3339Nano)

	q := `
INSERT INTO audit_records (id, correlation_id, agent_id, model_name, prompt_tokens, completion_tokens, total_cost_usd, execution_duration_ms, tool_calls_count, status, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := s.db.ExecContext(ctx, q, entry.ID, entry.CorrelationID, entry.AgentID, entry.ModelName, entry.PromptTokens, entry.CompletionTokens, entry.TotalCostUSD, entry.ExecutionDurationMS, entry.ToolCallsCount, entry.Status, nowStr)
	if err != nil {
		return fmt.Errorf("memory: RecordAuditEntry: %w", err)
	}
	return nil
}

// QueryAuditRecords retrieves audit log entries matching the specified filter.
func (s *sqliteStore) QueryAuditRecords(ctx context.Context, filter AuditFilter) ([]AuditRecord, error) {
	q := `SELECT id, correlation_id, agent_id, model_name, prompt_tokens, completion_tokens, total_cost_usd, execution_duration_ms, tool_calls_count, status, created_at FROM audit_records WHERE 1=1`
	var args []interface{}

	if filter.AgentID != "" {
		q += " AND agent_id = ?"
		args = append(args, filter.AgentID)
	}
	if filter.ModelName != "" {
		q += " AND model_name = ?"
		args = append(args, filter.ModelName)
	}
	if filter.Status != "" {
		q += " AND status = ?"
		args = append(args, filter.Status)
	}
	if filter.Search != "" {
		q += " AND (correlation_id LIKE ? OR agent_id LIKE ? OR model_name LIKE ?)"
		searchPattern := "%" + filter.Search + "%"
		args = append(args, searchPattern, searchPattern, searchPattern)
	}

	q += " ORDER BY created_at DESC"
	if filter.Limit <= 0 {
		filter.Limit = 100
	}
	q += " LIMIT ?"
	args = append(args, filter.Limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("memory: QueryAuditRecords: %w", err)
	}
	defer rows.Close()

	var records []AuditRecord
	for rows.Next() {
		var rec AuditRecord
		var createdAtStr string
		if err := rows.Scan(&rec.ID, &rec.CorrelationID, &rec.AgentID, &rec.ModelName, &rec.PromptTokens, &rec.CompletionTokens, &rec.TotalCostUSD, &rec.ExecutionDurationMS, &rec.ToolCallsCount, &rec.Status, &createdAtStr); err != nil {
			return nil, fmt.Errorf("memory: QueryAuditRecords scan: %w", err)
		}
		rec.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAtStr)
		records = append(records, rec)
	}
	return records, rows.Err()
}

// GetAuditSummary calculates aggregated telemetry and cost metrics.
func (s *sqliteStore) GetAuditSummary(ctx context.Context) (AuditSummary, error) {
	summary := AuditSummary{
		CostByAgent: make(map[string]float64),
		CostByModel: make(map[string]float64),
	}

	qTotal := `SELECT COUNT(*), COALESCE(SUM(prompt_tokens + completion_tokens), 0), COALESCE(SUM(total_cost_usd), 0.0), COALESCE(AVG(execution_duration_ms), 0) FROM audit_records`
	var avgDur float64
	err := s.db.QueryRowContext(ctx, qTotal).Scan(&summary.TotalTasks, &summary.TotalTokens, &summary.TotalCostUSD, &avgDur)
	if err != nil && err != sql.ErrNoRows {
		return summary, fmt.Errorf("memory: GetAuditSummary total: %w", err)
	}
	summary.AvgDurationMS = int64(avgDur)

	qAgent := `SELECT agent_id, COALESCE(SUM(total_cost_usd), 0.0) FROM audit_records GROUP BY agent_id`
	aRows, err := s.db.QueryContext(ctx, qAgent)
	if err == nil {
		defer aRows.Close()
		for aRows.Next() {
			var agentID string
			var cost float64
			if err := aRows.Scan(&agentID, &cost); err == nil {
				summary.CostByAgent[agentID] = cost
			}
		}
		if err := aRows.Err(); err != nil {
			return summary, fmt.Errorf("memory: GetAuditSummary aRows: %w", err)
		}
	}

	qModel := `SELECT model_name, COALESCE(SUM(total_cost_usd), 0.0) FROM audit_records GROUP BY model_name`
	mRows, err := s.db.QueryContext(ctx, qModel)
	if err == nil {
		defer mRows.Close()
		for mRows.Next() {
			var modelName string
			var cost float64
			if err := mRows.Scan(&modelName, &cost); err == nil {
				summary.CostByModel[modelName] = cost
			}
		}
		if err := mRows.Err(); err != nil {
			return summary, fmt.Errorf("memory: GetAuditSummary mRows: %w", err)
		}
	}

	return summary, nil
}

// PruneAuditRecords deletes audit records older than maxAgeDays (defaults to 30 days).
func (s *sqliteStore) PruneAuditRecords(ctx context.Context, maxAgeDays int) (int64, error) {
	if maxAgeDays <= 0 {
		maxAgeDays = 30
	}
	cutoff := time.Now().AddDate(0, 0, -maxAgeDays).Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(ctx, `DELETE FROM audit_records WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("memory: PruneAuditRecords: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

