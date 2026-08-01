package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fabith10/synapse-go/adk"
	"github.com/fabith10/synapse-go/internal/memory"
)

// GetSaveLongTermMemoryTool returns a tool that saves persistent learnings/facts.
func GetSaveLongTermMemoryTool(store memory.CheckpointStore) adk.Tool {
	return adk.Tool{
		Name:        "save_long_term_memory",
		Description: "Saves a persistent learning, rule, workaround, or fact that persists across sessions. Use this to remember important learnings from errors, code modifications, or domain discoveries.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"key": map[string]interface{}{
					"type":        "string",
					"description": "Unique key or topic tag (e.g. 'yfinance-pe-ratio-workaround').",
				},
				"value": map[string]interface{}{
					"type":        "string",
					"description": "The precise learning, instruction, or data value to persist.",
				},
			},
			"required": []string{"key", "value"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("save_long_term_memory: invalid args: %w", err)
			}

			agentID, _ := ctx.Value("executing_agent_id").(string)
			if agentID == "" {
				agentID = "global"
			}

			// Generate embedding if possible
			var embeddingBytes []byte
			emb, err := getOllamaEmbedding(ctx, params.Value)
			if err == nil && len(emb) > 0 {
				embeddingBytes = float32ToBytes(emb)
			}

			err = store.SaveLongTermMemory(ctx, agentID, params.Key, params.Value, embeddingBytes)
			if err != nil {
				return "", fmt.Errorf("failed to save memory: %w", err)
			}

			return fmt.Sprintf("Successfully saved long-term learning for %q with key %q.", agentID, params.Key), nil
		},
	}
}

// GetSearchLongTermMemoriesTool returns a tool that semantically queries long-term learnings.
func GetSearchLongTermMemoriesTool(store memory.CheckpointStore) adk.Tool {
	return adk.Tool{
		Name:        "search_long_term_memories",
		Description: "Semantically searches the agent's long-term learnings and persistent memories recorded across all sessions.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "The semantic query topic to match.",
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
				return "", fmt.Errorf("search_long_term_memories: invalid args: %w", err)
			}
			if params.Limit <= 0 {
				params.Limit = 5
			}

			agentID, _ := ctx.Value("executing_agent_id").(string)

			// Retrieve recent memories
			memories, err := store.QueryLongTermMemories(ctx, agentID, nil, 100)
			if err != nil {
				return "", fmt.Errorf("search_long_term_memories: QueryLongTermMemories: %w", err)
			}

			type memoryMatch struct {
				mem   memory.LongTermMemory
				score float32
			}

			// Generate query embedding if possible
			queryEmb, embErr := getOllamaEmbedding(ctx, params.Query)
			hasQueryEmb := embErr == nil && len(queryEmb) > 0

			var matches []memoryMatch
			for _, m := range memories {
				var score float32
				if hasQueryEmb {
					logEmb := bytesToFloat32(m.Embedding)
					if len(logEmb) == 0 {
						freshEmb, freshErr := getOllamaEmbedding(ctx, m.Value)
						if freshErr == nil && len(freshEmb) > 0 {
							logEmb = freshEmb
							// Update in DB background
							_ = store.SaveLongTermMemory(ctx, m.AgentID, m.Key, m.Value, float32ToBytes(freshEmb))
						}
					}
					if len(logEmb) > 0 {
						score = cosineSimilarity(queryEmb, logEmb)
					} else {
						score = keywordRelevanceScore(m.Value, params.Query)
					}
				} else {
					score = keywordRelevanceScore(m.Key+" "+m.Value, params.Query)
				}

				// If score is 0, check if key matches or provide minimal recency score
				if score == 0 {
					score = keywordRelevanceScore(m.Key+" "+m.Value, params.Query)
					if score == 0 && len(memories) > 0 {
						score = 0.01 // Recency fallback score
					}
				}

				if score > 0 {
					matches = append(matches, memoryMatch{mem: m, score: score})
				}
			}

			// Sort by score
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
				return "No relevant long-term memories found.", nil
			}

			var sb strings.Builder
			for _, m := range matches {
				sb.WriteString(fmt.Sprintf("[%s] Key: %q (Score: %.3f)\n%s\n---\n", m.mem.AgentID, m.mem.Key, m.score, m.mem.Value))
			}
			return sb.String(), nil
		},
	}
}
