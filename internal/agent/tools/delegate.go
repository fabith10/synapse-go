package agenttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/fabith10/synapse-go/adk"
)

// GetDelegateSubtaskTool returns a tool allowing an active agent to delegate subtasks to another specialist agent.
func GetDelegateSubtaskTool(orch *adk.Orchestrator, parentAgentID string, mb chan adk.Message) adk.Tool {
	return adk.Tool{
		Name:        "delegate_subtask",
		Description: "Delegate a focused subtask to another registered specialist agent and receive its final result.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"target_agent_id": map[string]interface{}{
					"type":        "string",
					"description": "ID of the target specialist agent to delegate to (as registered in workspace configuration).",
				},
				"subtask_prompt": map[string]interface{}{
					"type":        "string",
					"description": "Focused subtask instructions and goal for the delegated specialist agent.",
				},
			},
			"required": []string{"target_agent_id", "subtask_prompt"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var raw map[string]interface{}
			_ = json.Unmarshal(args, &raw)

			targetAgentID := extractStringAlias(raw, "target_agent_id", "agent_id", "target_agent", "recipient")
			subtaskPrompt := extractStringAlias(raw, "subtask_prompt", "prompt", "task", "instructions")

			targetAgentID = strings.TrimSpace(targetAgentID)
			subtaskPrompt = strings.TrimSpace(subtaskPrompt)

			if targetAgentID == "" || subtaskPrompt == "" {
				return "", fmt.Errorf("delegate_subtask: target_agent_id and subtask_prompt are required")
			}
			if targetAgentID == parentAgentID {
				return "", fmt.Errorf("delegate_subtask: cannot delegate subtask to self (%s)", parentAgentID)
			}
			if orch == nil {
				return "", fmt.Errorf("delegate_subtask: orchestrator not available")
			}

			subtaskID := fmt.Sprintf("subtask-%d", time.Now().UnixNano())
			subMsg := adk.Message{
				Sender:    parentAgentID,
				Recipient: targetAgentID,
				Content:   subtaskPrompt,
				Metadata: map[string]string{
					"is_subtask":        "true",
					"parent_agent_id":   parentAgentID,
					"subtask_id":        subtaskID,
					"planner_bypass":    "true",
					"supervisor_bypass": "true",
				},
			}
			orch.Send(subMsg)

			if mb == nil {
				return fmt.Sprintf("[Subtask Delegated] Sent subtask %s to agent %s.", subtaskID, targetAgentID), nil
			}

			subCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
			defer cancel()

			for {
				select {
				case <-subCtx.Done():
					return fmt.Sprintf("[Subtask Dispatched] Delegated subtask to %s (ID: %s).", targetAgentID, subtaskID), nil
				case reply, ok := <-mb:
					if !ok {
						return fmt.Sprintf("[Subtask Dispatched] Subtask %s sent to %s.", subtaskID, targetAgentID), nil
					}
					if reply.Sender == targetAgentID || (reply.Metadata != nil && (reply.Metadata["subtask_id"] == subtaskID || reply.Metadata["parent_agent_id"] == parentAgentID)) {
						return fmt.Sprintf("DELEGATED SUBAGENT (%s) RESULT:\n%s", targetAgentID, reply.Content), nil
					}
				}
			}
		},
	}
}
