package agent

import (
	"github.com/fabith10/synapse-go/internal/agent/rag"
	"github.com/fabith10/synapse-go/internal/memory"
)

type VectorStore = rag.VectorStore
type MemoryVectorStore = rag.MemoryVectorStore

func NewMemoryVectorStore(store memory.CheckpointStore) *MemoryVectorStore {
	return rag.NewMemoryVectorStore(store)
}
