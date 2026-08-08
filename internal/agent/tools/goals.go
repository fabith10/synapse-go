package agenttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fabith10/synapse-go/adk"
	"github.com/fabith10/synapse-go/internal/memory"
)

// GetManageLongTermGoalsTool returns the tool to create, track, and complete long-term goals.
func GetManageLongTermGoalsTool(store memory.CheckpointStore) adk.Tool {
	return adk.Tool{
		Name:        "manage_long_term_goals",
		Description: "Creates, updates, or lists active long-term goals and milestones for autonomous background execution.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"operation": map[string]interface{}{
					"type":        "string",
					"description": "Operation: 'create_goal', 'list_goals', 'add_milestone', 'update_milestone', 'complete_goal'",
				},
				"title": map[string]interface{}{
					"type":        "string",
					"description": "Goal or milestone title",
				},
				"description": map[string]interface{}{
					"type":        "string",
					"description": "Detailed description or success criteria",
				},
				"goal_id": map[string]interface{}{
					"type":        "string",
					"description": "Target goal ID for milestones or status updates",
				},
				"milestone_id": map[string]interface{}{
					"type":        "string",
					"description": "Target milestone ID",
				},
				"agent_id": map[string]interface{}{
					"type":        "string",
					"description": "Assigned agent ID for milestone execution (e.g. 'generalist-agent')",
				},
				"status": map[string]interface{}{
					"type":        "string",
					"description": "Status string: 'ACTIVE', 'COMPLETED', 'PAUSED', 'PENDING', 'IN_PROGRESS', 'DONE'",
				},
			},
			"required": []string{"operation"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				Operation   string `json:"operation"`
				Title       string `json:"title"`
				Description string `json:"description"`
				GoalID      string `json:"goal_id"`
				MilestoneID string `json:"milestone_id"`
				AgentID     string `json:"agent_id"`
				Status      string `json:"status"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}

			switch strings.ToLower(params.Operation) {
			case "create_goal":
				if params.Title == "" {
					return "", fmt.Errorf("title is required for create_goal")
				}
				g, err := store.CreateGoal(ctx, params.Title, params.Description, "{}")
				if err != nil {
					return "", err
				}
				out, _ := json.MarshalIndent(g, "", "  ")
				return string(out), nil

			case "list_goals":
				goals, err := store.ListActiveGoals(ctx)
				if err != nil {
					return "", err
				}
				out, _ := json.MarshalIndent(map[string]interface{}{
					"active_goal_count": len(goals),
					"goals":             goals,
				}, "", "  ")
				return string(out), nil

			case "add_milestone":
				if params.GoalID == "" || params.Title == "" {
					return "", fmt.Errorf("goal_id and title are required for add_milestone")
				}
				m, err := store.AddGoalMilestone(ctx, params.GoalID, params.Title, params.AgentID)
				if err != nil {
					return "", err
				}
				out, _ := json.MarshalIndent(m, "", "  ")
				return string(out), nil

			case "update_milestone":
				if params.MilestoneID == "" || params.Status == "" {
					return "", fmt.Errorf("milestone_id and status are required for update_milestone")
				}
				err := store.UpdateMilestoneStatus(ctx, params.MilestoneID, strings.ToUpper(params.Status))
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("Milestone %s updated to status %s", params.MilestoneID, params.Status), nil

			case "complete_goal":
				if params.GoalID == "" {
					return "", fmt.Errorf("goal_id is required for complete_goal")
				}
				err := store.UpdateGoalStatus(ctx, params.GoalID, "COMPLETED")
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("Goal %s successfully marked as COMPLETED", params.GoalID), nil

			default:
				return "", fmt.Errorf("unsupported operation %q", params.Operation)
			}
		},
	}
}

// GetEvaluateGoalProgressTool returns the tool to evaluate active goals against criteria.
func GetEvaluateGoalProgressTool(store memory.CheckpointStore) adk.Tool {
	return adk.Tool{
		Name:        "evaluate_goal_progress",
		Description: "Evaluates current active goals and milestone progress stored in RAG memory and SQLite.",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			goals, err := store.ListActiveGoals(ctx)
			if err != nil {
				return "", err
			}

			if len(goals) == 0 {
				return "No active long-term goals currently registered.", nil
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("🎯 Found %d Active Long-Term Goal(s):\n\n", len(goals)))

			for i, g := range goals {
				sb.WriteString(fmt.Sprintf("%d. Goal ID: %s | Title: %q\n", i+1, g.ID, g.Title))
				if g.Description != "" {
					sb.WriteString(fmt.Sprintf("   Description: %s\n", g.Description))
				}
				sb.WriteString(fmt.Sprintf("   Milestones (%d):\n", len(g.Milestones)))
				for _, m := range g.Milestones {
					sb.WriteString(fmt.Sprintf("     - [%s] %s (Assigned: %s, ID: %s)\n", m.Status, m.Title, m.AgentID, m.ID))
				}
				sb.WriteString("\n")
			}
			return sb.String(), nil
		},
	}
}
