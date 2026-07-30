package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/fabith10/synapse-go/internal/orchestrator"
	"github.com/fabith10/synapse-go/internal/tools"
)

func TestToolSchemaValidation(t *testing.T) {
	// Simple tool with required params and type schemas
	testTool := tools.Tool{
		Name: "test_validated_tool",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"username": map[string]interface{}{"type": "string"},
				"age":      map[string]interface{}{"type": "number"},
				"admin":    map[string]interface{}{"type": "boolean"},
			},
			"required": []string{"username", "age"},
		},
		Tier: tools.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			return "success", nil
		},
	}

	wrapped := tools.SecureTool(testTool, nil)

	// 1. Valid arguments
	goodArgs, _ := json.Marshal(map[string]interface{}{
		"username": "alice",
		"age":      30.0,
	})
	res, err := wrapped.Execute(context.Background(), goodArgs)
	if err != nil {
		t.Fatalf("expected valid arguments to pass, got error: %v", err)
	}
	if res != "success" {
		t.Errorf("expected 'success', got %q", res)
	}

	// 2. Missing required field
	badArgs1, _ := json.Marshal(map[string]interface{}{
		"username": "bob",
	})
	_, err = wrapped.Execute(context.Background(), badArgs1)
	if err == nil || !strings.Contains(err.Error(), "missing required parameter") {
		t.Errorf("expected error containing 'missing required parameter', got: %v", err)
	}

	// 3. Invalid field type
	badArgs2, _ := json.Marshal(map[string]interface{}{
		"username": "bob",
		"age":      "thirty", // should be number
	})
	_, err = wrapped.Execute(context.Background(), badArgs2)
	if err == nil || !strings.Contains(err.Error(), "expected number") {
		t.Errorf("expected error containing 'expected number', got: %v", err)
	}
}

func TestToolRBACPolicy(t *testing.T) {
	testTool := tools.Tool{
		Name: "test_rbac_tool",
		Tier: tools.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			return "success", nil
		},
	}

	// Only allow researcher-agent
	secured := tools.SecureTool(testTool, []string{"researcher-agent"})

	// 1. Authorized execution
	authCtx := context.WithValue(context.Background(), "executing_agent_id", "researcher-agent")
	res, err := secured.Execute(authCtx, nil)
	if err != nil {
		t.Fatalf("expected authorized execution to pass, got error: %v", err)
	}
	if res != "success" {
		t.Errorf("expected 'success', got %q", res)
	}

	// 2. Unauthorized execution
	unauthCtx := context.WithValue(context.Background(), "executing_agent_id", "etl-agent")
	_, err = secured.Execute(unauthCtx, nil)
	if err == nil || !strings.Contains(err.Error(), "is not authorized to run tool") {
		t.Errorf("expected authorization block error, got: %v", err)
	}
}

func TestBlackboardStateMemory(t *testing.T) {
	orch := orchestrator.NewOrchestrator()
	writeTool := GetWriteStateVariableTool(orch)
	readTool := GetReadStateVariableTool(orch)

	sessionCtx := context.WithValue(context.Background(), "session_id", "test-session-123")

	// 1. Write variable
	writeArgs, _ := json.Marshal(map[string]interface{}{
		"key":   "target_rate",
		"value": "0.085",
	})
	_, err := writeTool.Execute(sessionCtx, writeArgs)
	if err != nil {
		t.Fatalf("write state variable tool failed: %v", err)
	}

	// 2. Read variable
	readArgs, _ := json.Marshal(map[string]interface{}{
		"key": "target_rate",
	})
	res, err := readTool.Execute(sessionCtx, readArgs)
	if err != nil {
		t.Fatalf("read state variable tool failed: %v", err)
	}

	var parsed string
	if err := json.Unmarshal([]byte(res), &parsed); err != nil {
		t.Fatalf("failed to parse read tool output: %v", err)
	}
	if parsed != "0.085" {
		t.Errorf("expected retrieved value '0.085', got: %q", parsed)
	}

	// 3. Read missing key
	missingArgs, _ := json.Marshal(map[string]interface{}{
		"key": "nonexistent_key",
	})
	res2, err := readTool.Execute(sessionCtx, missingArgs)
	if err != nil {
		t.Fatalf("read missing key failed with execution error: %v", err)
	}
	if !strings.Contains(res2, "not found in blackboard") {
		t.Errorf("expected missing key message, got: %q", res2)
	}
}
