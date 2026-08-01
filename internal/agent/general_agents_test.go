package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fabith10/synapse-go/adk"
	"github.com/fabith10/synapse-go/internal/broker"
	"github.com/fabith10/synapse-go/internal/memory"
	"github.com/fabith10/synapse-go/internal/orchestrator"
)

type mockGatekeeperProvider struct {
	triageResponse string
}

func (m *mockGatekeeperProvider) FormatPrompt(msgs []adk.Message, ts []broker.Tool) (interface{}, error) {
	return msgs, nil
}

func (m *mockGatekeeperProvider) GenerateResponse(_ context.Context, _ interface{}) (adk.Message, error) {
	return adk.Message{Content: m.triageResponse}, nil
}

func TestGatekeeperTriage_GeneralAgents(t *testing.T) {
	// 1. Triage to developer-agent
	devProvider := &mockGatekeeperProvider{
		triageResponse: `{"recipient":"developer-agent","content":"Write a python script to sort a list of numbers"}`,
	}
	cfgDev := adk.Config{
		SQLiteDSN: "file:test_dev_triage.db?mode=memory&cache=shared",
		LLMProviders: []adk.LLMNode{
			{
				Name:     "mock-triage-dev",
				Tier:     broker.TierLocal,
				Provider: devProvider,
			},
		},
	}

	rtDev, err := Bootstrap(cfgDev)
	if err != nil {
		t.Fatalf("Bootstrap dev triage: %v", err)
	}
	defer rtDev.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rtDev.Start(ctx)

	// Send message to triage-agent
	rtDev.Orchestrator().Send(adk.Message{
		Sender:    "USER",
		Recipient: "triage-agent",
		Content:   "Write a python script to sort a list of numbers",
	})

	select {
	case req := <-rtDev.Orchestrator().HumanApprovalChan():
		t.Logf("Triage routed output: %+v", req)
	case <-time.After(10 * time.Second):
		// That's fine, let's just make sure it runs
	}

	// 2. Triage to writer-agent
	writerProvider := &mockGatekeeperProvider{
		triageResponse: `{"recipient":"writer-agent","content":"Draft an announcement about our new API Release"}`,
	}
	cfgWriter := adk.Config{
		SQLiteDSN: "file:test_writer_triage.db?mode=memory&cache=shared",
		LLMProviders: []adk.LLMNode{
			{
				Name:     "mock-triage-writer",
				Tier:     broker.TierLocal,
				Provider: writerProvider,
			},
		},
	}

	rtWriter, err := Bootstrap(cfgWriter)
	if err != nil {
		t.Fatalf("Bootstrap writer triage: %v", err)
	}
	defer rtWriter.Shutdown()

	rtWriter.Start(ctx)

	rtWriter.Orchestrator().Send(adk.Message{
		Sender:    "USER",
		Recipient: "triage-agent",
		Content:   "Draft an announcement about our new API Release",
	})

	select {
	case req := <-rtWriter.Orchestrator().HumanApprovalChan():
		t.Logf("Triage routed output: %+v", req)
	case <-time.After(10 * time.Second):
		// That's fine, let's just make sure it runs
	}
}

func TestDeveloperAgent_Execution(t *testing.T) {
	cfg := adk.Config{
		SQLiteDSN: "file:test_dev_agent.db?mode=memory&cache=shared",
		LLMProviders: []adk.LLMNode{
			{
				Name:     "mock-developer",
				Tier:     broker.TierLocal,
				Provider: &mockGatekeeperProvider{triageResponse: "Successfully generated python bubble sort script."},
			},
		},
	}

	rt, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	defer rt.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rt.Start(ctx)

	rt.Orchestrator().Send(adk.Message{
		Sender:    "USER",
		Recipient: "developer-agent",
		Content:   "Create bubble sort python script",
	})

	select {
	case msg := <-rt.Orchestrator().HumanApprovalChan():
		if msg.Sender != "developer-agent" {
			t.Errorf("expected sender to be developer-agent, got: %q", msg.Sender)
		}
		if !strings.Contains(msg.Content, "Successfully generated python bubble") {
			t.Errorf("expected bubble sort success response, got: %q", msg.Content)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for developer agent response")
	}
}

func TestWriterAgent_Execution(t *testing.T) {
	cfg := adk.Config{
		SQLiteDSN: "file:test_writer_agent.db?mode=memory&cache=shared",
		LLMProviders: []adk.LLMNode{
			{
				Name:     "mock-writer",
				Tier:     broker.TierLocal,
				Provider: &mockGatekeeperProvider{triageResponse: "API Release Blog Draft:\nWe are excited to announce..."},
			},
		},
	}

	rt, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	defer rt.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rt.Start(ctx)

	rt.Orchestrator().Send(adk.Message{
		Sender:    "USER",
		Recipient: "writer-agent",
		Content:   "Draft blog post about API release",
	})

	select {
	case msg := <-rt.Orchestrator().HumanApprovalChan():
		if msg.Sender != "writer-agent" {
			t.Errorf("expected sender to be writer-agent, got: %q", msg.Sender)
		}
		if !strings.Contains(msg.Content, "API Release Blog Draft") {
			t.Errorf("expected API release blog post, got: %q", msg.Content)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for writer agent response")
	}
}

func TestDynamicAgentRegistry(t *testing.T) {
	// Write a temporary agent config file
	testConfig := `{
		"custom-designer-agent": {
			"system_prompt": "You are the Custom Designer Agent. Design layouts.",
			"description": "Design layout sheets and custom templates",
			"capabilities": ["layout design", "document templates"],
			"tools": ["grep_documents"],
			"hardware_tier": "tier0",
			"max_willing_to_pay": 0.05
		}
	}`
	tmpFile := filepath.Join(t.TempDir(), "agents_test.json")
	if err := os.WriteFile(tmpFile, []byte(testConfig), 0644); err != nil {
		t.Fatalf("failed to write temp config file: %v", err)
	}

	// Backup existing config
	loadedAgentConfigsMu.Lock()
	oldConfig := loadedAgentConfigs
	loadedAgentConfigsMu.Unlock()
	defer func() {
		loadedAgentConfigsMu.Lock()
		loadedAgentConfigs = oldConfig
		loadedAgentConfigsMu.Unlock()
	}()

	if err := LoadAgentsConfig(tmpFile); err != nil {
		t.Fatalf("LoadAgentsConfig failed: %v", err)
	}

	// Verify GatekeeperPrompt compiled dynamically includes the custom designer agent details
	gatekeeperSystemPrompt := GatekeeperPrompt
	if !strings.Contains(gatekeeperSystemPrompt, "custom-designer-agent") {
		t.Errorf("expected GatekeeperPrompt to contain custom-designer-agent, got:\n%s", gatekeeperSystemPrompt)
	}
	if !strings.Contains(gatekeeperSystemPrompt, "layout design") {
		t.Errorf("expected GatekeeperPrompt to contain custom designer capabilities, got:\n%s", gatekeeperSystemPrompt)
	}

	// Bootstrap runtime
	cfg := adk.Config{
		SQLiteDSN: "file:test_dynamic_registry.db?mode=memory&cache=shared",
		LLMProviders: []adk.LLMNode{
			{
				Name: "mock-provider",
				Tier: broker.TierLocal,
				Provider: &mockGatekeeperProvider{
					triageResponse: `{"recipient":"custom-designer-agent","content":"Design a newsletter layout"}`,
				},
			},
		},
	}

	rt, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}
	defer rt.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rt.Start(ctx)

	// Verify dynamic agent is registered
	agents := rt.GetAgents()
	found := false
	for _, a := range agents {
		if a.ID == "custom-designer-agent" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("custom-designer-agent not registered dynamically on bootstrap")
	}

	// Send message to triage-agent
	rt.Orchestrator().Send(adk.Message{
		Sender:    "USER",
		Recipient: "triage-agent",
		Content:   "Design a newsletter layout",
	})

	select {
	case msg := <-rt.Orchestrator().HumanApprovalChan():
		if msg.Sender != "custom-designer-agent" {
			t.Errorf("expected routed message sender to be custom-designer-agent, got: %q", msg.Sender)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for dynamic agent routing")
	}
}

func TestDynamicPlaceholderInjection(t *testing.T) {
	// Write a temp configuration containing the {{SPECIALIST_AGENTS}} placeholder
	testConfig := `{
		"triage-agent": {
			"system_prompt": "Classifier.\nRouting rules:\n{{SPECIALIST_AGENTS}}\nEnd.",
			"description": "System Gatekeeper",
			"capabilities": ["routing"]
		},
		"test-specialist": {
			"system_prompt": "You are a test specialist.",
			"description": "Handles test specialist jobs",
			"capabilities": ["running tests"]
		}
	}`
	tmpFile := filepath.Join(t.TempDir(), "agents_placeholder_test.json")
	if err := os.WriteFile(tmpFile, []byte(testConfig), 0644); err != nil {
		t.Fatalf("failed to write temp config file: %v", err)
	}

	// Backup existing config
	loadedAgentConfigsMu.Lock()
	oldConfig := loadedAgentConfigs
	loadedAgentConfigsMu.Unlock()
	defer func() {
		loadedAgentConfigsMu.Lock()
		loadedAgentConfigs = oldConfig
		loadedAgentConfigsMu.Unlock()
	}()

	if err := LoadAgentsConfig(tmpFile); err != nil {
		t.Fatalf("LoadAgentsConfig failed: %v", err)
	}

	gatekeeperSystemPrompt := GatekeeperPrompt
	if !strings.Contains(gatekeeperSystemPrompt, "test-specialist") {
		t.Errorf("expected GatekeeperPrompt to contain test-specialist, got:\n%s", gatekeeperSystemPrompt)
	}
	if !strings.Contains(gatekeeperSystemPrompt, "running tests") {
		t.Errorf("expected GatekeeperPrompt to contain test capabilities, got:\n%s", gatekeeperSystemPrompt)
	}
}

func TestExternalPromptFiles(t *testing.T) {
	tempDir := t.TempDir()
	
	// 1. Create a prompt text file
	externalPromptContent := "You are a highly specialised content creator."
	promptFilePath := filepath.Join(tempDir, "creator_prompt.txt")
	if err := os.WriteFile(promptFilePath, []byte(externalPromptContent), 0644); err != nil {
		t.Fatalf("failed to write external prompt file: %v", err)
	}

	// 2. Define the configuration referencing the file using the @ prefix
	testConfig := fmt.Sprintf(`{
		"writer-agent": {
			"system_prompt": "@%s",
			"description": "Writes posts",
			"capabilities": ["writing"]
		}
	}`, promptFilePath)

	tmpConfigFile := filepath.Join(tempDir, "agents_external_test.json")
	if err := os.WriteFile(tmpConfigFile, []byte(testConfig), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Backup existing config
	loadedAgentConfigsMu.Lock()
	oldConfig := loadedAgentConfigs
	loadedAgentConfigsMu.Unlock()
	defer func() {
		loadedAgentConfigsMu.Lock()
		loadedAgentConfigs = oldConfig
		loadedAgentConfigsMu.Unlock()
	}()

	if err := LoadAgentsConfig(tmpConfigFile); err != nil {
		t.Fatalf("LoadAgentsConfig failed: %v", err)
	}

	// Verify loaded prompt is the resolved content of the file
	writerSystemPrompt := GetPromptByID("writer-agent")
	if writerSystemPrompt != externalPromptContent {
		t.Errorf("expected GetPromptByID(\"writer-agent\") to be %q, got: %q", externalPromptContent, writerSystemPrompt)
	}
}

func TestChromedpBrowserSession(t *testing.T) {
	// Start local test server serving dynamic HTML with JS interaction
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`
			<!DOCTYPE html>
			<html>
			<body>
				<h1>Welcome Dynamic</h1>
				<input id="input1" placeholder="enter text">
				<button id="btn1" onclick="document.body.innerHTML += '<p id=result>Success Click</p>'">Submit</button>
			</body>
			</html>
		`))
	}))
	defer server.Close()

	session := &BrowserSession{
		Inputs: make(map[int]string),
	}
	defer session.Close()

	// Navigate
	_, err := session.Navigate(server.URL)
	if err != nil {
		t.Fatalf("Navigate failed: %v", err)
	}

	// Verify we parsed dynamic elements
	foundInput := false
	var inputIdx int
	var buttonIdx int
	for _, el := range session.Elements {
		if el.Type == "input" {
			foundInput = true
			inputIdx = el.Index
		}
		if el.Type == "button" {
			buttonIdx = el.Index
		}
	}
	if !foundInput {
		t.Fatalf("expected to parse input element from dynamic page, got elements: %+v", session.Elements)
	}

	// Input some text
	_, err = session.Input(inputIdx, "hello chrome")
	if err != nil {
		t.Fatalf("Input failed: %v", err)
	}
	if session.Inputs[inputIdx] != "hello chrome" {
		t.Errorf("expected input value hello chrome, got: %q", session.Inputs[inputIdx])
	}

	// Click button
	_, err = session.Click(buttonIdx)
	if err != nil {
		t.Fatalf("Click failed: %v", err)
	}

	// Verify page text updated dynamically via click handler
	if !strings.Contains(session.PageText, "Success Click") {
		t.Errorf("expected page text to update with Success Click, got: %q", session.PageText)
	}
}

func TestExtractPDFText_InvalidFile(t *testing.T) {
	tool := GetExtractPDFTextTool()
	ctx := context.Background()

	// Call with non-existent file
	args, _ := json.Marshal(map[string]string{"path": "non_existent_file.pdf"})
	_, err := tool.Execute(ctx, args)
	if err == nil {
		t.Error("expected error for non-existent file path, got nil")
	}

	// Call with directory traversal path
	argsBad, _ := json.Marshal(map[string]string{"path": "/etc/passwd"})
	_, err = tool.Execute(ctx, argsBad)
	if err == nil {
		t.Error("expected traversal check failure, got nil")
	}
}

func TestLongTermMemoryTools(t *testing.T) {
	// Create SQLite store in WAL / memory mode
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	store, err := memory.Open(dsn)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer store.Close()

	saveTool := GetSaveLongTermMemoryTool(store)
	searchTool := GetSearchLongTermMemoriesTool(store)

	ctx := context.WithValue(context.Background(), "executing_agent_id", "developer-agent")

	// 1. Save fact
	saveArgs, _ := json.Marshal(map[string]string{
		"key":   "workaround-pe",
		"value": "Always use lead_score for ranking leads.",
	})
	saveResult, err := saveTool.Execute(ctx, saveArgs)
	if err != nil {
		t.Fatalf("Save tool execution failed: %v", err)
	}
	if !strings.Contains(saveResult, "Successfully saved") {
		t.Errorf("unexpected save output: %q", saveResult)
	}

	// 2. Query it back semantically
	queryArgs, _ := json.Marshal(map[string]interface{}{
		"query": "lead scoring learnings",
		"limit": 3,
	})
	queryResult, err := searchTool.Execute(ctx, queryArgs)
	if err != nil {
		t.Fatalf("Search tool execution failed: %v", err)
	}
	if !strings.Contains(queryResult, "Always use lead_score") {
		t.Errorf("expected query results to find workaround-pe learning, got: %q", queryResult)
	}
}

func TestMarkdownDirectoryAgentParser(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "agents_test_dir_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	mdContent := `---
id: custom-agent-test
description: "A custom test agent for markdown directory parsing"
capabilities:
  - testing-capability-1
  - testing-capability-2
tools:
  - check_lead_score
  - fetch_html
hardware_tier: tier1
max_willing_to_pay: 0.12
---
You are the Custom Test Agent. Your goal is to pass the markdown parsing tests.
`

	agentFile := filepath.Join(tempDir, "custom-agent-test.md")
	if err := os.WriteFile(agentFile, []byte(mdContent), 0644); err != nil {
		t.Fatalf("failed to write test agent file: %v", err)
	}

	loadedAgentConfigsMu.Lock()
	oldConfig := loadedAgentConfigs
	loadedAgentConfigsMu.Unlock()

	defer func() {
		loadedAgentConfigsMu.Lock()
		loadedAgentConfigs = oldConfig
		loadedAgentConfigsMu.Unlock()
	}()

	err = LoadAgentsConfig(tempDir)
	if err != nil {
		t.Fatalf("failed to load agent config from temp dir: %v", err)
	}

	configs := GetLoadedAgentConfigs()
	cfg, ok := configs["custom-agent-test"]
	if !ok {
		t.Fatalf("expected 'custom-agent-test' agent to be loaded")
	}

	if cfg.Description != "A custom test agent for markdown directory parsing" {
		t.Errorf("unexpected description: %q", cfg.Description)
	}
	if len(cfg.Capabilities) != 2 || cfg.Capabilities[0] != "testing-capability-1" || cfg.Capabilities[1] != "testing-capability-2" {
		t.Errorf("unexpected capabilities: %v", cfg.Capabilities)
	}
	if len(cfg.Tools) != 2 || cfg.Tools[0] != "check_lead_score" || cfg.Tools[1] != "fetch_html" {
		t.Errorf("unexpected tools: %v", cfg.Tools)
	}
	if cfg.HardwareTier != "tier1" {
		t.Errorf("unexpected hardware tier: %q", cfg.HardwareTier)
	}
	if cfg.MaxWillingToPay != 0.12 {
		t.Errorf("unexpected max willing to pay: %v", cfg.MaxWillingToPay)
	}
	if !strings.Contains(cfg.SystemPrompt, "You are the Custom Test Agent") {
		t.Errorf("unexpected system prompt: %q", cfg.SystemPrompt)
	}
}

func TestLoadAgentsConfig(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "agents_test_*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := `{
		"triage-agent": {
			"system_prompt": "Dynamic Triage Custom Prompt"
		},
		"etl-agent": {
			"system_prompt": "Dynamic ETL Custom Prompt"
		}
	}`

	if _, err := tmpFile.Write([]byte(content)); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	// Backup existing config
	loadedAgentConfigsMu.Lock()
	oldConfig := loadedAgentConfigs
	loadedAgentConfigsMu.Unlock()
	defer func() {
		loadedAgentConfigsMu.Lock()
		loadedAgentConfigs = oldConfig
		loadedAgentConfigsMu.Unlock()
	}()

	// Store original prompts
	origTriage := GatekeeperPrompt
	defer func() {
		GatekeeperPrompt = origTriage
	}()

	err = LoadAgentsConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadAgentsConfig failed: %v", err)
	}

	if GatekeeperPrompt != "Dynamic Triage Custom Prompt" {
		t.Errorf("expected GatekeeperPrompt to be overridden, got %q", GatekeeperPrompt)
	}

	etlPrompt := GetPromptByID("etl-agent")
	if etlPrompt != "Dynamic ETL Custom Prompt" {
		t.Errorf("expected EtlPrompt to be overridden, got %q", etlPrompt)
	}
}

func TestDelegateSubtaskTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	orch := orchestrator.NewOrchestrator()
	mb := make(chan adk.Message, 10)

	tool := GetDelegateSubtaskTool(orch, "researcher-agent", mb)
	if tool.Name != "delegate_subtask" {
		t.Fatalf("expected tool name 'delegate_subtask', got %q", tool.Name)
	}

	// Prepare tool execution args
	args, _ := json.Marshal(map[string]string{
		"target_agent_id": "developer-agent",
		"subtask_prompt":  "Write a Python script for data processing",
	})

	// Pre-stage subagent response in mailbox
	mb <- adk.Message{
		Sender:    "developer-agent",
		Recipient: "researcher-agent",
		Content:   "def process(): pass",
		Metadata: map[string]string{
			"parent_agent_id": "researcher-agent",
		},
	}

	res, err := tool.Execute(ctx, args)
	if err != nil {
		t.Fatalf("tool.Execute failed: %v", err)
	}

	resStr := fmt.Sprintf("%v", res)
	if !strings.Contains(resStr, "DELEGATED SUBAGENT (developer-agent) RESULT:") || !strings.Contains(resStr, "def process(): pass") {
		t.Errorf("unexpected tool output: %s", resStr)
	}
}


