package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fabith10/agent-framework/adk"
)

// GetWriteStateVariableTool returns the Tier 1 native tool to write blackboard state.
func GetWriteStateVariableTool(orch *adk.Orchestrator) adk.Tool {
	return adk.Tool{
		Name:        "write_state_variable",
		Description: "Persists a variable (key-value pair) on the shared blackboard state memory for downstream agents to read.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"key": map[string]interface{}{
					"type":        "string",
					"description": "The state key name to store the value under.",
				},
				"value": map[string]interface{}{
					"type":        "string",
					"description": "The value to store (can be a plain string, number, or JSON string).",
				},
			},
			"required": []string{"key", "value"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				Key   string      `json:"key"`
				Value interface{} `json:"value"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("write_state_variable: invalid JSON: %w", err)
			}
			sessionID, _ := ctx.Value("session_id").(string)
			if sessionID == "" {
				return "", fmt.Errorf("write_state_variable: no session context found")
			}
			orch.SetBlackboard(sessionID, params.Key, params.Value)
			return fmt.Sprintf("Successfully wrote key %q to state blackboard.", params.Key), nil
		},
	}
}

// GetReadStateVariableTool returns the Tier 1 native tool to read blackboard state.
func GetReadStateVariableTool(orch *adk.Orchestrator) adk.Tool {
	return adk.Tool{
		Name:        "read_state_variable",
		Description: "Reads a variable by key from the shared blackboard state memory.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"key": map[string]interface{}{
					"type":        "string",
					"description": "The key name of the variable to retrieve.",
				},
			},
			"required": []string{"key"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				Key string `json:"key"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("read_state_variable: invalid JSON: %w", err)
			}
			sessionID, _ := ctx.Value("session_id").(string)
			if sessionID == "" {
				return "", fmt.Errorf("read_state_variable: no session context found")
			}
			state := orch.GetBlackboard(sessionID)
			val, ok := state[params.Key]
			if !ok {
				return fmt.Sprintf("Key %q not found in blackboard.", params.Key), nil
			}
			valBytes, _ := json.Marshal(val)
			return string(valBytes), nil
		},
	}
}
