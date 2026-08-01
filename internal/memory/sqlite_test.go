package memory_test

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/fabith10/synapse-go/internal/memory"
)

// openTestStore returns an in-memory SQLite store that is automatically closed
// when the test finishes. Using a shared-cache URI ensures that the schema is
// properly initialised for the duration of the test.
func openTestStore(t *testing.T) memory.CheckpointStore {
	t.Helper()
	// Each test gets its own in-memory database via a unique name to prevent
	// cross-test state bleed when tests run in parallel.
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	store, err := memory.Open(dsn)
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// ---------------------------------------------------------------------------
// Context Logs
// ---------------------------------------------------------------------------

func TestAppendAndQueryLog(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)

	entry := memory.ContextLog{
		ID:        "log-001",
		AgentID:   "research-agent",
		SessionID: "session-abc",
		Role:      memory.RoleAgent,
		Content:   "Fetched 3 results from the pricing oracle.",
		CreatedAt: time.Now().UTC(),
	}

	if err := store.AppendLog(ctx, entry); err != nil {
		t.Fatalf("AppendLog: %v", err)
	}

	logs, err := store.QueryRelevantLogs(ctx, "research-agent", nil, 5)
	if err != nil {
		t.Fatalf("QueryRelevantLogs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].Content != entry.Content {
		t.Errorf("content mismatch: got %q, want %q", logs[0].Content, entry.Content)
	}
}

func TestQueryLog_UnknownAgent(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)

	logs, err := store.QueryRelevantLogs(ctx, "ghost-agent", nil, 5)
	if err != nil {
		t.Fatalf("QueryRelevantLogs: %v", err)
	}
	if len(logs) != 0 {
		t.Errorf("expected 0 logs for unknown agent, got %d", len(logs))
	}
}

func TestQueryLog_RespectsLimit(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)

	for i := 0; i < 10; i++ {
		entry := memory.ContextLog{
			ID:        fmt.Sprintf("log-%02d", i),
			AgentID:   "bulk-agent",
			SessionID: "s1",
			Role:      memory.RoleSystem,
			Content:   fmt.Sprintf("message %d", i),
			CreatedAt: time.Now().UTC().Add(time.Duration(i) * time.Second),
		}
		if err := store.AppendLog(ctx, entry); err != nil {
			t.Fatalf("AppendLog %d: %v", i, err)
		}
	}

	logs, err := store.QueryRelevantLogs(ctx, "bulk-agent", nil, 3)
	if err != nil {
		t.Fatalf("QueryRelevantLogs: %v", err)
	}
	if len(logs) != 3 {
		t.Errorf("expected 3 logs (limit), got %d", len(logs))
	}
}

// ---------------------------------------------------------------------------
// Tool Checkpoints — idempotency contract
// ---------------------------------------------------------------------------

func TestAcquireAndCompleteStep(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)

	// 1. Lock should succeed on first call.
	if err := store.AcquireStepLock(ctx, "task-1", "web_search"); err != nil {
		t.Fatalf("AcquireStepLock: %v", err)
	}

	// 2. Re-acquiring a PENDING lock must return ErrAlreadyLocked.
	err := store.AcquireStepLock(ctx, "task-1", "web_search")
	if err != memory.ErrAlreadyLocked {
		t.Fatalf("expected ErrAlreadyLocked, got: %v", err)
	}

	// 3. Complete the step.
	if err := store.CompleteStep(ctx, "task-1", "web_search", `{"result":"ok"}`); err != nil {
		t.Fatalf("CompleteStep: %v", err)
	}

	// 4. A SUCCESS lock must also be guarded — idempotency prevents re-running.
	err = store.AcquireStepLock(ctx, "task-1", "web_search")
	if err != memory.ErrAlreadyLocked {
		t.Fatalf("expected ErrAlreadyLocked after success, got: %v", err)
	}

	// 5. GetStep should return the completed record.
	step, err := store.GetStep(ctx, "task-1", "web_search")
	if err != nil {
		t.Fatalf("GetStep: %v", err)
	}
	if step.Status != memory.StepSuccess {
		t.Errorf("expected SUCCESS, got %s", step.Status)
	}
	if step.OutputPayload != `{"result":"ok"}` {
		t.Errorf("unexpected payload: %s", step.OutputPayload)
	}
}

func TestFailStep_AllowsRetry(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)

	_ = store.AcquireStepLock(ctx, "task-2", "docker_exec")
	_ = store.FailStep(ctx, "task-2", "docker_exec")

	// After FAILED, AcquireStepLock should succeed (broker is retrying / escalating).
	if err := store.AcquireStepLock(ctx, "task-2", "docker_exec"); err != nil {
		t.Fatalf("expected retry to succeed after failure, got: %v", err)
	}
}

func TestGetStep_NotFound(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)

	_, err := store.GetStep(ctx, "nonexistent-task", "some_tool")
	if err != memory.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Active Task Routing
// ---------------------------------------------------------------------------

func TestUpsertAndGetTask(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)

	task := memory.ActiveTask{
		TaskID:          "task-alpha",
		CurrentAgent:    "triage-agent",
		Status:          memory.TaskStatusInProgress,
		ComputeTierUsed: memory.TierLocalOllama,
	}

	if err := store.UpsertTask(ctx, task); err != nil {
		t.Fatalf("UpsertTask: %v", err)
	}

	got, err := store.GetTask(ctx, "task-alpha")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != memory.TaskStatusInProgress {
		t.Errorf("expected IN_PROGRESS, got %s", got.Status)
	}
	if got.ComputeTierUsed != memory.TierLocalOllama {
		t.Errorf("unexpected tier: %s", got.ComputeTierUsed)
	}
}

func TestUpsertTask_UpdatesExisting(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)

	_ = store.UpsertTask(ctx, memory.ActiveTask{
		TaskID:          "task-beta",
		CurrentAgent:    "triage-agent",
		Status:          memory.TaskStatusInProgress,
		ComputeTierUsed: memory.TierLocalOllama,
	})

	// Simulate the broker escalating from Tier 0 → Tier 1 after Ollama fails.
	_ = store.UpsertTask(ctx, memory.ActiveTask{
		TaskID:          "task-beta",
		CurrentAgent:    "gpt4-agent",
		Status:          memory.TaskStatusInProgress,
		ComputeTierUsed: memory.TierCommercialAPI,
	})

	got, err := store.GetTask(ctx, "task-beta")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.ComputeTierUsed != memory.TierCommercialAPI {
		t.Errorf("expected TIER_1_API after escalation, got %s", got.ComputeTierUsed)
	}
	if got.CurrentAgent != "gpt4-agent" {
		t.Errorf("expected gpt4-agent, got %s", got.CurrentAgent)
	}
}

func TestGetTask_NotFound(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)

	_, err := store.GetTask(ctx, "ghost-task")
	if err != memory.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestQueryLog_IsolatesSession(t *testing.T) {
	store := openTestStore(t)
	baseCtx := context.Background()

	// Insert log for session 1
	err := store.AppendLog(baseCtx, memory.ContextLog{
		ID:        "log-1",
		AgentID:   "research-agent",
		SessionID: "session-s1",
		Role:      memory.RoleAgent,
		Content:   "Session 1 entry",
		CreatedAt: time.Now().UTC().Add(-10 * time.Second),
	})
	if err != nil {
		t.Fatalf("failed to append log 1: %v", err)
	}

	// Insert log for session 2
	err = store.AppendLog(baseCtx, memory.ContextLog{
		ID:        "log-2",
		AgentID:   "research-agent",
		SessionID: "session-s2",
		Role:      memory.RoleAgent,
		Content:   "Session 2 entry",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("failed to append log 2: %v", err)
	}

	// 1. Query with context containing session-s1
	ctx1 := context.WithValue(baseCtx, "session_id", "session-s1")
	logs1, err := store.QueryRelevantLogs(ctx1, "", nil, 5)
	if err != nil {
		t.Fatalf("QueryRelevantLogs for s1: %v", err)
	}
	if len(logs1) != 1 {
		t.Fatalf("expected 1 log for s1, got %d", len(logs1))
	}
	if logs1[0].Content != "Session 1 entry" {
		t.Errorf("expected 'Session 1 entry', got %q", logs1[0].Content)
	}

	// 2. Query with context containing session-s2
	ctx2 := context.WithValue(baseCtx, "session_id", "session-s2")
	logs2, err := store.QueryRelevantLogs(ctx2, "", nil, 5)
	if err != nil {
		t.Fatalf("QueryRelevantLogs for s2: %v", err)
	}
	if len(logs2) != 1 {
		t.Fatalf("expected 1 log for s2, got %d", len(logs2))
	}
	if logs2[0].Content != "Session 2 entry" {
		t.Errorf("expected 'Session 2 entry', got %q", logs2[0].Content)
	}

	// 3. Query without session context (legacy/tests behavior)
	logsAll, err := store.QueryRelevantLogs(baseCtx, "", nil, 5)
	if err != nil {
		t.Fatalf("QueryRelevantLogs without session: %v", err)
	}
	if len(logsAll) != 2 {
		t.Errorf("expected all 2 logs to be returned when no session is in context, got %d", len(logsAll))
	}
}

func TestLongTermMemoryPersistence(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	// 1. Save long-term memory
	err := store.SaveLongTermMemory(ctx, "developer-agent", "workaround-pe", "Use custom library for pe ratio", nil)
	if err != nil {
		t.Fatalf("SaveLongTermMemory failed: %v", err)
	}

	// 2. Query it back by key
	m, err := store.GetLongTermMemory(ctx, "developer-agent", "workaround-pe")
	if err != nil {
		t.Fatalf("GetLongTermMemory failed: %v", err)
	}
	if m.Value != "Use custom library for pe ratio" {
		t.Errorf("expected 'Use custom library for pe ratio', got: %q", m.Value)
	}

	// 3. Upsert update (conflict resolution)
	err = store.SaveLongTermMemory(ctx, "developer-agent", "workaround-pe", "Use updated custom library for pe ratio", nil)
	if err != nil {
		t.Fatalf("SaveLongTermMemory update failed: %v", err)
	}
	mUpdated, _ := store.GetLongTermMemory(ctx, "developer-agent", "workaround-pe")
	if mUpdated.Value != "Use updated custom library for pe ratio" {
		t.Errorf("expected updated value, got: %q", mUpdated.Value)
	}

	// 4. Query list
	list, err := store.QueryLongTermMemories(ctx, "developer-agent", nil, 5)
	if err != nil {
		t.Fatalf("QueryLongTermMemories failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 memory record, got %d", len(list))
	}
}

func TestAuditLedgerStore(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	rec1 := memory.AuditRecord{
		ID:                  "audit-1",
		CorrelationID:       "task-100",
		AgentID:             "quant-agent",
		ModelName:           "gpt-4o",
		PromptTokens:        150,
		CompletionTokens:    80,
		TotalCostUSD:        0.0025,
		ExecutionDurationMS: 320,
		ToolCallsCount:      1,
		Status:              "COMPLETED",
		CreatedAt:           time.Now().UTC(),
	}

	rec2 := memory.AuditRecord{
		ID:                  "audit-2",
		CorrelationID:       "task-101",
		AgentID:             "developer-agent",
		ModelName:           "mock-ollama",
		PromptTokens:        200,
		CompletionTokens:    100,
		TotalCostUSD:        0.0000,
		ExecutionDurationMS: 150,
		ToolCallsCount:      2,
		Status:              "COMPLETED",
		CreatedAt:           time.Now().UTC(),
	}

	if err := store.RecordAuditEntry(ctx, rec1); err != nil {
		t.Fatalf("RecordAuditEntry rec1 failed: %v", err)
	}
	if err := store.RecordAuditEntry(ctx, rec2); err != nil {
		t.Fatalf("RecordAuditEntry rec2 failed: %v", err)
	}

	records, err := store.QueryAuditRecords(ctx, memory.AuditFilter{})
	if err != nil {
		t.Fatalf("QueryAuditRecords failed: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 audit records, got %d", len(records))
	}

	summary, err := store.GetAuditSummary(ctx)
	if err != nil {
		t.Fatalf("GetAuditSummary failed: %v", err)
	}
	if summary.TotalTasks != 2 {
		t.Errorf("expected 2 total tasks, got %d", summary.TotalTasks)
	}
	if summary.TotalTokens != 530 {
		t.Errorf("expected 530 tokens, got %d", summary.TotalTokens)
	}
	if summary.CostByAgent["quant-agent"] != 0.0025 {
		t.Errorf("expected 0.0025 cost for quant-agent, got %f", summary.CostByAgent["quant-agent"])
	}

	pruned, err := store.PruneAuditRecords(ctx, 30)
	if err != nil {
		t.Fatalf("PruneAuditRecords failed: %v", err)
	}
	if pruned != 0 {
		t.Errorf("expected 0 records pruned for recent data, got %d", pruned)
	}
}

func TestQueryLog_VectorCosineSimilarityRanking(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)

	// Helper to serialize float64 slice to LittleEndian binary
	ser := func(v []float64) []byte {
		b := make([]byte, len(v)*8)
		for i, f := range v {
			bits := math.Float64bits(f)
			binary.LittleEndian.PutUint64(b[i*8:], bits)
		}
		return b
	}

	// Store log 1 (weather topic: vector [1.0, 0.0, 0.0])
	_ = store.AppendLog(ctx, memory.ContextLog{
		ID:        "log-weather",
		AgentID:   "rag-agent",
		Role:      memory.RoleAgent,
		Content:   "Sunny forecast for tomorrow",
		Embedding: ser([]float64{1.0, 0.0, 0.0}),
		CreatedAt: time.Now().UTC().Add(2 * time.Second), // Newer timestamp
	})

	// Store log 2 (finance topic: vector [0.0, 1.0, 0.0])
	_ = store.AppendLog(ctx, memory.ContextLog{
		ID:        "log-finance",
		AgentID:   "rag-agent",
		Role:      memory.RoleAgent,
		Content:   "Stock prices surged in options market",
		Embedding: ser([]float64{0.0, 1.0, 0.0}),
		CreatedAt: time.Now().UTC().Add(1 * time.Second), // Older timestamp
	})

	// Query with finance query vector [0.1, 0.9, 0.0] -> should rank log-finance FIRST despite older timestamp
	queryVec := ser([]float64{0.1, 0.9, 0.0})
	logs, err := store.QueryRelevantLogs(ctx, "rag-agent", queryVec, 2)
	if err != nil {
		t.Fatalf("QueryRelevantLogs vector query failed: %v", err)
	}

	if len(logs) != 2 {
		t.Fatalf("expected 2 logs, got %d", len(logs))
	}

	if logs[0].ID != "log-finance" {
		t.Errorf("expected vector similarity ranking to return 'log-finance' first, got %q", logs[0].ID)
	}
}


