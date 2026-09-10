package rag

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fabith10/synapse-go/internal/memory"
)

func TestRAG_TurnMemoryPersistenceAndRetrieval(t *testing.T) {
	os.Setenv("AGENT_FRAMEWORK_TESTING", "true")
	defer os.Unsetenv("AGENT_FRAMEWORK_TESTING")

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
}

func TestComputeEmbedding_FixedDimension(t *testing.T) {
	os.Setenv("AGENT_FRAMEWORK_TESTING", "true")
	defer os.Unsetenv("AGENT_FRAMEWORK_TESTING")

	// The fundamental fix: vectors from completely different texts must have
	// the same dimension so cosine similarity produces non-zero results.
	texts := []string{
		"Deploy a Kubernetes cluster on AWS with GPU nodes",
		"The quick brown fox jumps over the lazy dog",
		"Machine learning model training requires significant compute resources",
	}

	var dims []int
	var embeddings [][]float64
	for _, text := range texts {
		emb := ComputeEmbedding(text)
		if emb == nil {
			t.Fatalf("ComputeEmbedding returned nil for: %s", text)
		}
		dims = append(dims, len(emb))
		embeddings = append(embeddings, emb)
	}

	// All dimensions must be equal (the old TF-IDF approach failed this).
	for i := 1; i < len(dims); i++ {
		if dims[i] != dims[0] {
			t.Errorf("dimension mismatch: text[0] has dim %d, text[%d] has dim %d", dims[0], i, dims[i])
		}
	}

	// Cosine similarity between related texts should be non-zero.
	sim := CosineSimilarity(embeddings[0], embeddings[2]) // both about compute
	if sim == 0 {
		t.Errorf("expected non-zero cosine similarity between related texts, got 0")
	}
}
