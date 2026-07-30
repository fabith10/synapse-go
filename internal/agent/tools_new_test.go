package agent

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fabith10/synapse-go/adk"
	"github.com/fabith10/synapse-go/internal/orchestrator"
)

func TestGrepDocumentsTool(t *testing.T) {
	// Create temporary directory for searching
	tmpDir, err := ioutil.TempDir("", "grep_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	file1 := filepath.Join(tmpDir, "doc1.txt")
	err = ioutil.WriteFile(file1, []byte("Hello world!\nTarget keyword here.\nAnother line."), 0644)
	if err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	file2 := filepath.Join(tmpDir, "doc2.md")
	err = ioutil.WriteFile(file2, []byte("Some markdown\nno target\nend."), 0644)
	if err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	tool := GetGrepDocumentsTool()

	// 1. Test search matching
	args, _ := json.Marshal(map[string]interface{}{
		"path":    tmpDir,
		"pattern": "Target keyword",
	})
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Grep execute failed: %v", err)
	}
	if !strings.Contains(res, "Target keyword here") {
		t.Errorf("expected grep output to contain 'Target keyword here', got: %q", res)
	}

	// 2. Test directory traversal prevention
	badArgs, _ := json.Marshal(map[string]interface{}{
		"path":    "../../../../../../etc/passwd",
		"pattern": "root",
	})
	_, err = tool.Execute(context.Background(), badArgs)
	if err == nil {
		t.Error("expected directory traversal attempt to return an error, but got nil")
	}
}

func TestSemanticSearchContext_KeywordFallback(t *testing.T) {
	// Verify keyword relevance scoring
	score1 := keywordRelevanceScore("financial volatility calculation done", "volatility calculation")
	if score1 != 1.0 {
		t.Errorf("expected keyword score 1.0, got: %f", score1)
	}

	score2 := keywordRelevanceScore("some other text here", "volatility calculation")
	if score2 != 0.0 {
		t.Errorf("expected keyword score 0.0, got: %f", score2)
	}

	score3 := keywordRelevanceScore("calculation logic", "volatility calculation")
	if score3 != 0.5 {
		t.Errorf("expected keyword score 0.5, got: %f", score3)
	}
}

type mockSandbox struct{}

func (m *mockSandbox) Execute(ctx context.Context, req adk.ExecutionRequest) adk.ExecutionResult {
	return adk.ExecutionResult{
		Stdout:   "echoed " + req.RawCode,
		ExitCode: 0,
	}
}

func TestExecuteBashDocker_HITL(t *testing.T) {
	sb := &mockSandbox{}
	orch := orchestrator.NewOrchestrator()
	mailbox := make(chan adk.Message, 5)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	orch.Start(ctx)

	baseAgent := adk.NewBaseAgent("test-agent", nil)
	baseAgent.Mailbox = mailbox
	_ = orch.Register(&baseAgent.Agent)
	baseAgent.Run(ctx)

	tool := GetExecuteBashDockerTool(sb, orch, "test-agent", mailbox)

	// Verify dangerous script triggers HITL approval
	go func() {
		time.Sleep(100 * time.Millisecond)
		// Receive approval request
		select {
		case req := <-orch.HumanApprovalChan():
			if req.Metadata == nil || req.Metadata["Type"] != "HITL_APPROVAL" {
				t.Errorf("expected HITL_APPROVAL request, got: %+v", req)
			}
			// Send response back via orchestrator
			orch.Send(adk.Message{
				Sender:    "USER",
				Recipient: "test-agent",
				Content:   "APPROVED",
				Metadata: map[string]string{
					"correlation_id": req.Metadata["correlation_id"],
					"Type":           "HITL_RESPONSE",
				},
			})
		case <-time.After(10 * time.Second):
			t.Errorf("timeout waiting for approval request")
		}
	}()

	args, _ := json.Marshal(map[string]interface{}{
		"bash_script": "rm -rf /some/dir",
	})
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected bash execute error: %v", err)
	}
	if !strings.Contains(res, "echoed rm -rf /some/dir") {
		t.Errorf("expected executed script output, got: %q", res)
	}
}

func TestModernCodingTools(t *testing.T) {
	os.Setenv("AGENT_FRAMEWORK_TESTING", "true")
	defer os.Unsetenv("AGENT_FRAMEWORK_TESTING")

	tmpDir, err := ioutil.TempDir("", "coding_tools_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	targetFile := filepath.Join(tmpDir, "sub", "test.txt")

	// 1. Test write_file
	writeTool := GetWriteFileTool()
	writeArgs, _ := json.Marshal(map[string]interface{}{
		"path":    targetFile,
		"content": "line 1: hello\nline 2: world\nline 3: foo",
	})
	res, err := writeTool.Execute(context.Background(), writeArgs)
	if err != nil {
		t.Fatalf("write_file failed: %v", err)
	}
	if !strings.Contains(res, "Successfully wrote") {
		t.Errorf("unexpected write_file result: %s", res)
	}

	// 2. Test read_file
	readTool := GetReadFileTool()
	readArgs, _ := json.Marshal(map[string]interface{}{
		"path":       targetFile,
		"start_line": 1,
		"end_line":   2,
	})
	res, err = readTool.Execute(context.Background(), readArgs)
	if err != nil {
		t.Fatalf("read_file failed: %v", err)
	}
	if !strings.Contains(res, "line 1: hello") || !strings.Contains(res, "line 2: world") {
		t.Errorf("unexpected read_file result: %s", res)
	}

	// 3. Test replace_file_content
	replaceTool := GetReplaceFileContentTool()
	replaceArgs, _ := json.Marshal(map[string]interface{}{
		"path":                targetFile,
		"target_content":      "line 2: world",
		"replacement_content": "line 2: universe",
	})
	res, err = replaceTool.Execute(context.Background(), replaceArgs)
	if err != nil {
		t.Fatalf("replace_file_content failed: %v", err)
	}
	if !strings.Contains(res, "Successfully replaced") {
		t.Errorf("unexpected replace_file_content result: %s", res)
	}

	// Read back to verify replacement
	contentBytes, _ := ioutil.ReadFile(targetFile)
	if !strings.Contains(string(contentBytes), "line 2: universe") {
		t.Errorf("replace_file_content did not update file content, got: %s", string(contentBytes))
	}

	// 4. Test list_directory
	listTool := GetListDirectoryTool()
	listArgs, _ := json.Marshal(map[string]interface{}{
		"path": tmpDir,
	})
	res, err = listTool.Execute(context.Background(), listArgs)
	if err != nil {
		t.Fatalf("list_directory failed: %v", err)
	}
	if !strings.Contains(res, "sub") {
		t.Errorf("expected list_directory to show 'sub', got: %s", res)
	}
}

func TestTargetDirScoping(t *testing.T) {
	os.Setenv("AGENT_FRAMEWORK_TESTING", "true")
	defer os.Unsetenv("AGENT_FRAMEWORK_TESTING")

	tmpDir, err := ioutil.TempDir("", "target_dir_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	targetFile := filepath.Join(tmpDir, "target_app_file.txt")
	_ = ioutil.WriteFile(targetFile, []byte("target project content"), 0644)

	// Create context with WorkspaceRootKey set to tmpDir
	ctx := context.WithValue(context.Background(), WorkspaceRootKey, tmpDir)

	// Test read_file using relative path
	readTool := GetReadFileTool()
	readArgs, _ := json.Marshal(map[string]interface{}{
		"path": "target_app_file.txt",
	})

	res, err := readTool.Execute(ctx, readArgs)
	if err != nil {
		t.Fatalf("read_file failed with target_dir context: %v", err)
	}
	if !strings.Contains(res, "target project content") {
		t.Errorf("expected target project content, got: %s", res)
	}

	// Test list_directory using "."
	listTool := GetListDirectoryTool()
	listArgs, _ := json.Marshal(map[string]interface{}{
		"path": ".",
	})
	res, err = listTool.Execute(ctx, listArgs)
	if err != nil {
		t.Fatalf("list_directory failed with target_dir context: %v", err)
	}
	if !strings.Contains(res, "target_app_file.txt") {
		t.Errorf("expected list_directory to show target_app_file.txt, got: %s", res)
	}
}

func TestMultipleQueuedApprovals(t *testing.T) {
	sb := &mockSandbox{}
	orch := orchestrator.NewOrchestrator()
	mailbox := make(chan adk.Message, 50)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	orch.Start(ctx)

	baseAgent := adk.NewBaseAgent("multi-approval-agent", nil)
	baseAgent.Mailbox = mailbox
	_ = orch.Register(&baseAgent.Agent)
	baseAgent.Run(ctx)

	tool := GetExecuteBashDockerTool(sb, orch, "multi-approval-agent", mailbox)

	// Launch two approval requests concurrently
	errCh1 := make(chan error, 1)
	errCh2 := make(chan error, 1)

	go func() {
		args, _ := json.Marshal(map[string]interface{}{
			"bash_script": "rm -rf /dir1",
		})
		_, err := tool.Execute(context.Background(), args)
		errCh1 <- err
	}()

	go func() {
		args, _ := json.Marshal(map[string]interface{}{
			"bash_script": "rm -rf /dir2",
		})
		_, err := tool.Execute(context.Background(), args)
		errCh2 <- err
	}()

	// Receive both approval requests from HumanApprovalChan
	reqs := make([]adk.Message, 2)
	for i := 0; i < 2; i++ {
		select {
		case reqs[i] = <-orch.HumanApprovalChan():
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout waiting for approval request %d", i+1)
		}
	}

	// Approve both requests (out of order to strictly verify correlation_id routing)
	for i := 1; i >= 0; i-- {
		req := reqs[i]
		corrID := req.Metadata["correlation_id"]
		orch.Send(adk.Message{
			Sender:    "USER",
			Recipient: "multi-approval-agent",
			Content:   "APPROVED",
			Metadata: map[string]string{
				"correlation_id": corrID,
				"Type":           "HITL_RESPONSE",
			},
		})
	}

	// Both executions should succeed cleanly
	select {
	case err := <-errCh1:
		if err != nil {
			t.Errorf("request 1 failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Errorf("timeout waiting for request 1 execution")
	}

	select {
	case err := <-errCh2:
		if err != nil {
			t.Errorf("request 2 failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Errorf("timeout waiting for request 2 execution")
	}
}

func TestConfigurableToolAliases(t *testing.T) {
	// Default alias resolution
	if resolved := ResolveToolAlias("read_page"); resolved != "fetch_html" {
		t.Errorf("expected read_page to resolve to fetch_html, got: %s", resolved)
	}

	// Register custom alias
	SetToolAlias("custom_web_fetcher", "fetch_html")
	if resolved := ResolveToolAlias("custom_web_fetcher"); resolved != "fetch_html" {
		t.Errorf("expected custom_web_fetcher to resolve to fetch_html, got: %s", resolved)
	}

	// Verify findTool resolves custom alias
	tools := []adk.Tool{GetFetchHTMLTool()}
	foundTool, ok := findTool(tools, "custom_web_fetcher")
	if !ok || foundTool.Name != "fetch_html" {
		t.Errorf("expected findTool to resolve custom_web_fetcher to fetch_html tool")
	}

	// Test loading JSON alias configuration file
	tmpFile, err := ioutil.TempFile("", "tool_aliases_*.json")
	if err != nil {
		t.Fatalf("failed to create temp json file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	jsonContent := `{"aliases": {"execute_python_script": "execute_python_docker"}}`
	_ = ioutil.WriteFile(tmpFile.Name(), []byte(jsonContent), 0644)

	if err := LoadToolAliasesConfig(tmpFile.Name()); err != nil {
		t.Fatalf("failed to load tool aliases config: %v", err)
	}

	if resolved := ResolveToolAlias("execute_python_script"); resolved != "execute_python_docker" {
		t.Errorf("expected execute_python_script to resolve to execute_python_docker, got: %s", resolved)
	}
}

func TestDynamicAgentRouting(t *testing.T) {
	// Mock a custom user-defined agent loaded from a markdown file (e.g. legal-agent)
	loadedAgentConfigsMu.Lock()
	origConfigs := loadedAgentConfigs
	loadedAgentConfigs = map[string]AgentJSONConfig{
		"legal-compliance-agent": {
			SystemPrompt: "You are the Legal & Compliance Officer. Review contracts, NDAs, and licensing agreements.",
			Description:  "Legal advisor and contract auditor",
			Capabilities: []string{"contract review", "compliance auditing", "legal risk evaluation"},
			Tools:        []string{"read_file", "write_file"},
		},
		"generalist-agent": {
			SystemPrompt: "General assistant",
			Description:  "General tasks",
			Capabilities: []string{"general"},
		},
	}
	loadedAgentConfigsMu.Unlock()
	defer func() {
		loadedAgentConfigsMu.Lock()
		loadedAgentConfigs = origConfigs
		loadedAgentConfigsMu.Unlock()
	}()

	// 1. Direct match on custom target ID
	target := ResolveTargetAgent("legal-compliance-agent", "Please review this NDA.")
	if target != "legal-compliance-agent" {
		t.Errorf("expected legal-compliance-agent, got: %s", target)
	}

	// 2. Dynamic matching via custom capability terms without any hardcoded switch
	target = ResolveTargetAgent("", "Perform a compliance auditing check on our supplier contract.")
	if target != "legal-compliance-agent" {
		t.Errorf("expected dynamic routing to match legal-compliance-agent, got: %s", target)
	}
}

func TestSystemWarningsDependencyFiltering(t *testing.T) {
	// 1. Mock loaded agent configs without Docker tools
	loadedAgentConfigsMu.Lock()
	origConfigs := loadedAgentConfigs
	loadedAgentConfigs = map[string]AgentJSONConfig{
		"writer-agent": {
			Tools: []string{"read_file", "write_file"},
		},
	}
	loadedAgentConfigsMu.Unlock()
	defer func() {
		loadedAgentConfigsMu.Lock()
		loadedAgentConfigs = origConfigs
		loadedAgentConfigsMu.Unlock()
	}()

	// Since writer-agent does not use execute_python_docker or execute_bash_docker or web_search_and_extract,
	// CheckSystemWarnings should NOT warn about Docker or Tavily API key!
	warnings := CheckSystemWarnings()
	for _, w := range warnings {
		if strings.Contains(w, "Docker") {
			t.Errorf("unexpected Docker warning when no active agent depends on Docker: %s", w)
		}
		if strings.Contains(w, "TAVILY") {
			t.Errorf("unexpected Tavily warning when no active agent depends on web search: %s", w)
		}
	}

	// 2. Test warning suppression via config
	SetSystemWarningsConfig(SystemWarningsConfig{
		DisabledChecks: []string{"tavily", "docker"},
		CustomChecks: []CustomWarningCheck{
			{
				ID:      "custom_db",
				EnvVar:  "NON_EXISTENT_CUSTOM_DB_URL",
				Warning: "Custom DB URL is missing.",
			},
		},
	})

	warningsWithCustom := CheckSystemWarnings()
	foundCustom := false
	for _, w := range warningsWithCustom {
		if strings.Contains(w, "Custom DB URL is missing") {
			foundCustom = true
		}
	}
	if !foundCustom {
		t.Errorf("expected custom warning to be triggered")
	}

	// Reset config
	SetSystemWarningsConfig(SystemWarningsConfig{})
}



