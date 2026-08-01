package agent

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/fabith10/synapse-go/adk"
	"github.com/ollama/ollama/api"
)

// getOllamaEmbedding generates a float32 embedding vector for the given prompt using the local Ollama server.
func getOllamaEmbedding(ctx context.Context, prompt string) ([]float32, error) {
	client, err := api.ClientFromEnvironment()
	if err != nil {
		return nil, err
	}
	model := os.Getenv("EMBEDDING_MODEL")
	if model == "" {
		model = "llama3"
	}
	req := &api.EmbeddingRequest{
		Model:  model,
		Prompt: prompt,
	}
	resp, err := client.Embeddings(ctx, req)
	if err != nil {
		return nil, err
	}
	emb32 := make([]float32, len(resp.Embedding))
	for i, v := range resp.Embedding {
		emb32[i] = float32(v)
	}
	return emb32, nil
}

// float32ToBytes serialises a float32 slice to a little-endian byte slice for storage.
func float32ToBytes(slice []float32) []byte {
	buf := new(bytes.Buffer)
	for _, f := range slice {
		binary.Write(buf, binary.LittleEndian, f)
	}
	return buf.Bytes()
}

// bytesToFloat32 deserialises a little-endian byte slice back into a float32 slice.
func bytesToFloat32(b []byte) []float32 {
	if len(b)%4 != 0 {
		return nil
	}
	slice := make([]float32, len(b)/4)
	buf := bytes.NewReader(b)
	for i := range slice {
		binary.Read(buf, binary.LittleEndian, &slice[i])
	}
	return slice
}

// cosineSimilarity returns the cosine similarity between two embedding vectors.
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB))))
}

// keywordRelevanceScore returns a simple keyword-overlap score between content and a query.
func keywordRelevanceScore(content, query string) float32 {
	lowerContent := strings.ToLower(content)
	lowerQuery := strings.ToLower(query)
	words := strings.Fields(lowerQuery)
	if len(words) == 0 {
		return 0
	}
	var matches float32
	for _, w := range words {
		if strings.Contains(lowerContent, w) {
			matches++
		}
	}
	return matches / float32(len(words))
}

// GetSemanticSearchContextTool returns the Tier 1 native tool to semantically search memory/logs.
func GetSemanticSearchContextTool(store adk.CheckpointStore) adk.Tool {
	return adk.Tool{
		Name:        "semantic_search_context",
		Description: "Searches the agent's long-term and short-term context logs for entries semantically or keyword-wise similar to the query.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "The search term or query to match.",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Max results to return (default to 5).",
				},
			},
			"required": []string{"query"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				Query string `json:"query"`
				Limit int    `json:"limit"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("semantic_search_context: invalid args: %w", err)
			}
			if params.Limit <= 0 {
				params.Limit = 5
			}

			logs, err := store.QueryRelevantLogs(ctx, "", nil, 100)
			if err != nil {
				return "", fmt.Errorf("semantic_search_context: QueryRelevantLogs: %w", err)
			}

			type logMatch struct {
				log   adk.ContextLog
				score float32
			}

			queryEmb, embErr := getOllamaEmbedding(ctx, params.Query)
			hasQueryEmb := embErr == nil && len(queryEmb) > 0

			var matches []logMatch
			for _, log := range logs {
				var score float32
				if hasQueryEmb {
					logEmb := bytesToFloat32(log.Embedding)
					if len(logEmb) == 0 {
						freshEmb, freshErr := getOllamaEmbedding(ctx, log.Content)
						if freshErr == nil && len(freshEmb) > 0 {
							logEmb = freshEmb
							_ = store.UpdateLogEmbedding(ctx, log.ID, float32ToBytes(freshEmb))
						}
					}
					if len(logEmb) > 0 {
						score = cosineSimilarity(queryEmb, logEmb)
					} else {
						score = keywordRelevanceScore(log.Content, params.Query)
					}
				} else {
					score = keywordRelevanceScore(log.Content, params.Query)
				}
				if score > 0 {
					matches = append(matches, logMatch{log: log, score: score})
				}
			}

			for i := 0; i < len(matches); i++ {
				for j := i + 1; j < len(matches); j++ {
					if matches[j].score > matches[i].score {
						matches[i], matches[j] = matches[j], matches[i]
					}
				}
			}
			if len(matches) > params.Limit {
				matches = matches[:params.Limit]
			}

			if len(matches) == 0 {
				return "No relevant log entries found.", nil
			}

			var sb strings.Builder
			for _, m := range matches {
				sb.WriteString(fmt.Sprintf("[%s][%s] (Score: %.3f)\n%s\n---\n", m.log.AgentID, m.log.Role, m.score, m.log.Content))
			}
			return sb.String(), nil
		},
	}
}
