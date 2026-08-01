package agent

import (
	"github.com/fabith10/synapse-go/internal/agent/rag"
	"github.com/fabith10/synapse-go/internal/memory"
)

func ComputeEmbedding(text string) []float64 { return rag.ComputeEmbedding(text) }
func SerializeEmbedding(v []float64) []byte { return rag.SerializeEmbedding(v) }
func DeserializeEmbedding(b []byte) []float64 { return rag.DeserializeEmbedding(b) }
func CosineSimilarity(a, b []float64) float64 { return rag.CosineSimilarity(a, b) }
func RankLogs(query []float64, logs []memory.ContextLog, topN int) []memory.ContextLog {
	return rag.RankLogs(query, logs, topN)
}
func FormatContextBlock(logs []memory.ContextLog) string { return rag.FormatContextBlock(logs) }
