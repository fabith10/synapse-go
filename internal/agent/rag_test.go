package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fabith10/agent-framework/internal/memory"
)

func TestRAG_TurnMemoryPersistenceAndRetrieval(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_rag_store.db")

	store, err := memory.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to create sqlite store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	agentID := "quant-agent"

	// 1. Store a prior preference context log
	prefText := "User preference: Always prioritize H100 SXM spot market contracts over forward contracts when volatility is below 0.15."
	prefEmb := SerializeEmbedding(ComputeEmbedding(prefText))

	err = store.AppendLog(ctx, memory.ContextLog{
		ID:        "log-pref-1",
		SessionID: "sess-123",
		AgentID:   agentID,
		Role:      memory.RoleUser,
		Content:   prefText,
		Embedding: prefEmb,
		CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to append log: %v", err)
	}

	// 2. Query relevant logs for a new task prompt
	queryText := "Which contract type should I select for H100 SXM under low volatility?"
	qEmb := SerializeEmbedding(ComputeEmbedding(queryText))

	logs, err := store.QueryRelevantLogs(ctx, agentID, qEmb, 10)
	if err != nil {
		t.Fatalf("failed to query relevant logs: %v", err)
	}
	if len(logs) == 0 {
		t.Fatalf("expected to retrieve stored context log, got 0")
	}

	// 3. Rank logs using cosine similarity
	ranked := RankLogs(ComputeEmbedding(queryText), logs, 3)
	if len(ranked) == 0 {
		t.Fatalf("expected ranked logs to be non-empty")
	}

	// 4. Verify context block formatting
	block := FormatContextBlock(ranked)
	if !strings.Contains(block, "<conversation_history>") {
		t.Errorf("expected context block to contain <conversation_history>, got: %s", block)
	}
	if !strings.Contains(block, "Always prioritize H100 SXM") {
		t.Errorf("expected context block to contain prior user preference text, got: %s", block)
	}

	_ = os.Remove(dbPath)
}
