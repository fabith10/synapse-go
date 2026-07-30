package agent

import (
	"context"
	"github.com/fabith10/synapse-go/internal/memory"
)

// VectorStore defines the interface for RAG document retrieval providers.
type VectorStore interface {
	Store(ctx context.Context, id string, content string, embedding []float64) error
	Query(ctx context.Context, queryEmbedding []float64, topN int) ([]memory.ContextLog, error)
}

// MemoryVectorStore is an in-memory/SQLite backed implementation of VectorStore.
type MemoryVectorStore struct {
	store memory.CheckpointStore
}

// NewMemoryVectorStore creates a new MemoryVectorStore.
func NewMemoryVectorStore(store memory.CheckpointStore) *MemoryVectorStore {
	return &MemoryVectorStore{store: store}
}

func (m *MemoryVectorStore) Store(ctx context.Context, id string, content string, embedding []float64) error {
	blob := SerializeEmbedding(embedding)
	return m.store.AppendLog(ctx, memory.ContextLog{
		ID:        id,
		AgentID:   "vector-store",
		Role:      memory.RoleSystem,
		Content:   content,
		Embedding: blob,
	})
}

func (m *MemoryVectorStore) Query(ctx context.Context, queryEmbedding []float64, topN int) ([]memory.ContextLog, error) {
	queryBlob := SerializeEmbedding(queryEmbedding)
	return m.store.QueryRelevantLogs(ctx, "vector-store", queryBlob, topN)
}
