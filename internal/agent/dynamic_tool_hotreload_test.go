package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabith10/synapse-go/adk"
	agenttools "github.com/fabith10/synapse-go/internal/agent/tools"
	"github.com/fabith10/synapse-go/internal/orchestrator"
)

type mockReActLLM struct {
	responses []string
	callIdx   int
}

func (m *mockReActLLM) Generate(ctx context.Context, req adk.TaskRequest, msgs []adk.Message) (adk.Message, error) {
	if m.callIdx >= len(m.responses) {
		return adk.Message{Sender: "assistant", Content: `{"action": "done", "summary": "Finished execution."}`}, nil
	}
	resp := m.responses[m.callIdx]
	m.callIdx++
	return adk.Message{Sender: "assistant", Content: resp}, nil
}

type mockDirectSandbox struct{}

func (m *mockDirectSandbox) Execute(ctx context.Context, req adk.ExecutionRequest) adk.ExecutionResult {
	if req.Language == "python" {
		return adk.ExecutionResult{
			Stdout: `{"status": "success", "result": "custom_computed_val_99"}`,
		}
	}
	return adk.ExecutionResult{
		Stdout: "ok",
	}
}

func TestDynamicToolHotReloadInReActLoop(t *testing.T) {
	tempDir := t.TempDir()
	sb := &mockDirectSandbox{}
	orch := orchestrator.NewOrchestrator()

	// Initialize DynamicToolManager
	agenttools.InitDynamicToolManager(sb, orch, tempDir)

	// Step 1: create_dynamic_tool
	// Step 2: call newly created dynamic tool "calc_custom_alpha"
	// Step 3: done
	mockLLM := &mockReActLLM{
		responses: []string{
			`{"action": "create_dynamic_tool", "name": "calc_custom_alpha", "description": "Calculates custom alpha factor", "language": "python", "code": "import sys, json; print('custom_computed_val_99')"}`,
			`{"action": "calc_custom_alpha", "asset": "H100", "window": 30}`,
			`{"action": "done", "summary": "Successfully authored tool and computed alpha metric."}`,
		},
	}

	baseTools := []adk.Tool{
		agenttools.GetCreateDynamicTool(sb, orch, "developer-agent"),
		agenttools.GetListDynamicToolsTool(),
	}

	msg := adk.Message{
		Sender:    "USER",
		Recipient: "developer-agent",
		Content:   "Create a custom tool calc_custom_alpha and compute the metric for H100.",
	}

	result, err := RunGenericReActLoop(context.Background(), mockLLM, orch, "developer-agent", "You are developer-agent.", baseTools, msg, 500, 0.10)
	if err != nil {
		t.Fatalf("RunGenericReActLoop failed: %v", err)
	}

	if !strings.Contains(result.FinalOutput, "Successfully authored tool") {
		t.Fatalf("Expected final output containing success summary, got %q", result.FinalOutput)
	}

	// Verify tool was registered and can be looked up dynamically
	tool, found := agenttools.GetGlobalDynamicTool("calc_custom_alpha")
	if !found {
		t.Fatalf("Expected 'calc_custom_alpha' to be registered in GlobalDynamicToolManager")
	}
	if tool.Name != "calc_custom_alpha" {
		t.Fatalf("Expected tool name 'calc_custom_alpha', got %q", tool.Name)
	}

	// Verify persistence to disk
	persisted := filepath.Join(tempDir, "calc_custom_alpha.json")
	if _, err := os.Stat(persisted); os.IsNotExist(err) {
		t.Fatalf("Expected persisted spec file at %s", persisted)
	}
}
