package agenttools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabith10/synapse-go/adk"
	"github.com/fabith10/synapse-go/internal/orchestrator"
)

type mockTestSandbox struct {
	lastReq adk.ExecutionRequest
}

func (m *mockTestSandbox) Execute(ctx context.Context, req adk.ExecutionRequest) adk.ExecutionResult {
	m.lastReq = req
	if strings.Contains(req.RawCode, "error_trigger") {
		return adk.ExecutionResult{Stderr: "simulated script error"}
	}
	return adk.ExecutionResult{
		Stdout: `{"status": "ok", "computed": 42}`,
	}
}

func TestDynamicToolManager_RegisterAndExecute(t *testing.T) {
	tempDir := t.TempDir()
	sb := &mockTestSandbox{}
	orch := orchestrator.NewOrchestrator()

	mgr := &DynamicToolManager{
		tools:      make(map[string]adk.Tool),
		specs:      make(map[string]DynamicToolSpec),
		sandbox:    sb,
		orch:       orch,
		storageDir: tempDir,
	}

	spec := DynamicToolSpec{
		Name:        "calculate_risk_index",
		Description: "Calculates custom risk index from portfolio parameters",
		Language:    "python",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"portfolio_id": map[string]interface{}{"type": "string"},
				"volatility":   map[string]interface{}{"type": "number"},
			},
			"required": []interface{}{"portfolio_id"},
		},
		Code:   "import sys, json\nargs = json.loads(sys.stdin.read())\nprint(json.dumps({'result': 42}))",
		Author: "quant-agent",
	}

	tool, err := mgr.Register(spec)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if tool.Name != "calculate_risk_index" {
		t.Fatalf("Expected tool name 'calculate_risk_index', got %q", tool.Name)
	}

	// Verify disk persistence
	persistedPath := filepath.Join(tempDir, "calculate_risk_index.json")
	if _, err := os.Stat(persistedPath); os.IsNotExist(err) {
		t.Fatalf("Expected spec persisted to %s, file not found", persistedPath)
	}

	// Test tool execution
	argsJSON := []byte(`{"action": "calculate_risk_index", "portfolio_id": "port-001", "volatility": 0.25}`)
	out, err := tool.Execute(context.Background(), argsJSON)
	if err != nil {
		t.Fatalf("Tool Execute failed: %v", err)
	}

	if !strings.Contains(out, "42") {
		t.Fatalf("Expected output containing '42', got %q", out)
	}
	if sb.lastReq.Language != "python" {
		t.Fatalf("Expected language 'python', got %q", sb.lastReq.Language)
	}
}

func TestDynamicToolManager_LoadFromDiskAndDelete(t *testing.T) {
	tempDir := t.TempDir()
	sb := &mockTestSandbox{}
	orch := orchestrator.NewOrchestrator()

	spec := DynamicToolSpec{
		Name:        "custom_parser",
		Description: "Parses structured feeds",
		Language:    "bash",
		Code:        "cat | jq .",
		Author:      "developer-agent",
	}

	mgr1 := &DynamicToolManager{
		tools:      make(map[string]adk.Tool),
		specs:      make(map[string]DynamicToolSpec),
		sandbox:    sb,
		orch:       orch,
		storageDir: tempDir,
	}

	_, err := mgr1.Register(spec)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// New manager instance simulating process restart
	mgr2 := &DynamicToolManager{
		tools:      make(map[string]adk.Tool),
		specs:      make(map[string]DynamicToolSpec),
		sandbox:    sb,
		orch:       orch,
		storageDir: tempDir,
	}

	loaded, err := mgr2.LoadFromDisk()
	if err != nil {
		t.Fatalf("LoadFromDisk failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("Expected 1 tool loaded, got %d", len(loaded))
	}
	if loaded[0].Name != "custom_parser" {
		t.Fatalf("Expected tool name 'custom_parser', got %q", loaded[0].Name)
	}

	// Verify delete
	if err := mgr2.Delete("custom_parser"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if len(mgr2.GetTools()) != 0 {
		t.Fatalf("Expected 0 tools after delete, got %d", len(mgr2.GetTools()))
	}
	if _, err := os.Stat(filepath.Join(tempDir, "custom_parser.json")); !os.IsNotExist(err) {
		t.Fatalf("Expected JSON file deleted from disk")
	}
}

func TestDynamicTool_CreateToolHandler(t *testing.T) {
	tempDir := t.TempDir()
	sb := &mockTestSandbox{}
	orch := orchestrator.NewOrchestrator()
	InitDynamicToolManager(sb, orch, tempDir)

	createTool := GetCreateDynamicTool(sb, orch, "test-agent")
	payload := map[string]interface{}{
		"name":        "agent_created_tool",
		"description": "Tool created dynamically by an agent",
		"language":    "python",
		"code":        "print('hello from dynamic tool')",
	}
	payloadBytes, _ := json.Marshal(payload)

	res, err := createTool.Execute(context.Background(), payloadBytes)
	if err != nil {
		t.Fatalf("create_dynamic_tool failed: %v", err)
	}
	if !strings.Contains(res, "agent_created_tool") {
		t.Fatalf("Expected success message containing tool name, got %q", res)
	}

	listTool := GetListDynamicToolsTool()
	listRes, err := listTool.Execute(context.Background(), []byte("{}"))
	if err != nil {
		t.Fatalf("list_dynamic_tools failed: %v", err)
	}
	if !strings.Contains(listRes, "agent_created_tool") {
		t.Fatalf("Expected list output containing 'agent_created_tool', got %q", listRes)
	}
}
