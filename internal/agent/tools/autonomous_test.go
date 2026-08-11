package agenttools

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/fabith10/synapse-go/internal/memory"
)

func TestAutonomousGoalAndApprovalTools(t *testing.T) {
	ctx := context.Background()

	// 1. Initialize temporary SQLite store
	dbPath := "/tmp/test_autonomous_goals.db"
	_ = os.Remove(dbPath)
	defer os.Remove(dbPath)

	store, err := memory.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to initialize sqlite store: %v", err)
	}
	defer store.Close()

	// 2. Test manage_long_term_goals
	t.Run("manage_long_term_goals", func(t *testing.T) {
		tool := GetManageLongTermGoalsTool(store)

		// Create goal
		createArgs, _ := json.Marshal(map[string]interface{}{
			"operation":   "create_goal",
			"title":       "Maintain GPU Infrastructure Uptime",
			"description": "Ensure zero downtime across H100 clusters",
		})
		out, err := tool.Execute(ctx, createArgs)
		if err != nil {
			t.Fatalf("create_goal failed: %v", err)
		}

		var goal struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(out), &goal); err != nil || goal.ID == "" {
			t.Fatalf("expected valid goal JSON output with ID, got: %s", out)
		}

		// Add milestone
		addArgs, _ := json.Marshal(map[string]interface{}{
			"operation": "add_milestone",
			"goal_id":   goal.ID,
			"title":     "Run daily latency audit",
			"agent_id":  "generalist-agent",
		})
		out, err = tool.Execute(ctx, addArgs)
		if err != nil {
			t.Fatalf("add_milestone failed: %v", err)
		}

		// List goals
		listArgs, _ := json.Marshal(map[string]interface{}{
			"operation": "list_goals",
		})
		out, err = tool.Execute(ctx, listArgs)
		if err != nil {
			t.Fatalf("list_goals failed: %v", err)
		}
		if out == "" {
			t.Errorf("expected non-empty list_goals output")
		}
	})

	// 3. Test evaluate_goal_progress
	t.Run("evaluate_goal_progress", func(t *testing.T) {
		tool := GetEvaluateGoalProgressTool(store)
		out, err := tool.Execute(ctx, nil)
		if err != nil {
			t.Fatalf("evaluate_goal_progress failed: %v", err)
		}
		if out == "" {
			t.Errorf("expected evaluation output")
		}
	})

	// 4. Test evaluate_security_guardrails
	t.Run("evaluate_security_guardrails", func(t *testing.T) {
		tool := GetEvaluateSecurityGuardrailsTool()

		// Low risk action
		safeArgs, _ := json.Marshal(map[string]interface{}{
			"proposed_action": "read file reports/summary.md",
		})
		out, err := tool.Execute(ctx, safeArgs)
		if err != nil {
			t.Fatalf("evaluate_security_guardrails safe failed: %v", err)
		}
		if !containsString(out, "APPROVED") {
			t.Errorf("expected APPROVED verdict, got: %s", out)
		}

		// High risk action
		riskyArgs, _ := json.Marshal(map[string]interface{}{
			"proposed_action":   "subprocess.run('rm -rf /production')",
			"cost_estimate_usd": 150.0,
		})
		out, err = tool.Execute(ctx, riskyArgs)
		if err != nil {
			t.Fatalf("evaluate_security_guardrails risky failed: %v", err)
		}
		if !containsString(out, "REQUIRES_HUMAN_SIGNATURE") {
			t.Errorf("expected REQUIRES_HUMAN_SIGNATURE verdict, got: %s", out)
		}
	})

	// 5. Test request_human_signature (Bypass test mode)
	t.Run("request_human_signature", func(t *testing.T) {
		tool := GetRequestHumanSignatureTool(nil, "test-agent", nil)
		os.Setenv("AGENT_FRAMEWORK_TESTING", "true")
		defer os.Unsetenv("AGENT_FRAMEWORK_TESTING")

		args, _ := json.Marshal(map[string]interface{}{
			"action_title": "Deploy Hotfix",
			"details":      "Update memory allocation in production",
			"risk_level":   "HIGH",
		})
		out, err := tool.Execute(ctx, args)
		if err != nil {
			t.Fatalf("request_human_signature failed: %v", err)
		}
		if !containsString(out, "APPROVED") {
			t.Errorf("expected approved signature response in testing mode, got: %s", out)
		}
	})
}

func containsString(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || findSubstr(s, substr)))
}

func findSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
