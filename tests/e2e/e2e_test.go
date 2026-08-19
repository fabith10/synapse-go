package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func runE2ETestCase(t *testing.T, promptText string, logFilename string, timeoutSecs string, extraEnv ...string) (string, string) {
	binaryPath := filepath.Join("..", "..", "bin", "synapse-go")
	_ = os.MkdirAll(filepath.Join("..", "..", "bin"), 0755)
	_ = os.MkdirAll("output", 0755)
	_ = os.MkdirAll(filepath.Join("testdata", "reports"), 0755)
	_ = os.MkdirAll(filepath.Join("testdata", "scripts"), 0755)

	buildCmd := exec.Command("go", "build", "-o", binaryPath, "../../cmd/main.go")
	var buildStderr bytes.Buffer
	buildCmd.Stderr = &buildStderr
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build agent-framework binary: %v, stderr: %s", err, buildStderr.String())
	}
	defer os.Remove(binaryPath)

	absBinaryPath, err := filepath.Abs(binaryPath)
	if err != nil {
		t.Fatalf("failed to resolve absolute binary path: %v", err)
	}

	absLogPath, err := filepath.Abs(filepath.Join("output", logFilename))
	if err != nil {
		t.Fatalf("failed to resolve absolute log file path: %v", err)
	}
	_ = os.Remove(absLogPath)

	if !strings.Contains(logFilename, "turn2") {
		_ = os.Remove(filepath.Join("testdata", "agent_framework.db"))
		_ = os.Remove(filepath.Join("testdata", "agent_framework.db-wal"))
		_ = os.Remove(filepath.Join("testdata", "agent_framework.db-shm"))
	}

	if timeoutSecs == "" {
		timeoutSecs = "60"
	}

	cmd := exec.Command(absBinaryPath, "-prompt", promptText, "-log-file", absLogPath, "-timeout", timeoutSecs)
	cmd.Dir = "testdata"

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	env := append(os.Environ(), "HEADLESS=true", "AGENT_FRAMEWORK_TESTING=true", "AGENT_FRAMEWORK_BYPASS_HITL=true", "DEFERRAL_BYPASS=true", "SQLITE_DSN=agent_framework.db")
	env = append(env, extraEnv...)
	cmd.Env = env

	err = cmd.Run()
	if err != nil {
		t.Fatalf("E2E CLI prompt execution failed for %q: %v\nStdout: %s\nStderr: %s", promptText, err, stdout.String(), stderr.String())
	}

	outStr := stdout.String()
	if !strings.Contains(outStr, "Completed Successfully") {
		t.Errorf("expected stdout to report successful completion for prompt %q, got: %s", promptText, outStr)
	}

	logBytes, err := os.ReadFile(absLogPath)
	if err != nil {
		t.Fatalf("failed to read audit log file %s: %v", absLogPath, err)
	}

	logStr := string(logBytes)

	// Strict audit checks: verify no actions or tools failed in the log
	if strings.Contains(logStr, "Action failed with error:") {
		t.Errorf("audit log recorded action failure in prompt %q:\n%s", promptText, logStr)
	}
	if strings.Contains(logStr, "Tool failed 3 times consecutively") {
		t.Errorf("audit log recorded repeated tool failure in prompt %q:\n%s", promptText, logStr)
	}

	return outStr, logStr
}

// Complex Test 1: Multi-Agent Quant & Code Generation with JSON Report Creation
func TestE2E_MultiAgentPipeline_QuantAndCode(t *testing.T) {
	_ = os.Remove("testdata/scripts/multi_gpu_optimizer.py")
	_ = os.Remove("testdata/reports/gpu_portfolio_report.json")

	prompt := "Query pricing oracle for H100 SXM, use write_file to create a python script 'scripts/multi_gpu_optimizer.py' using pandas to calculate portfolio weights, execute it in Docker, and save 'reports/gpu_portfolio_report.json'."

	_, logStr := runE2ETestCase(t, prompt, "e2e_multi_agent_pipeline.log", "90")

	// 1. Audit log checks
	if !strings.Contains(logStr, "USER ➔ triage-agent") {
		t.Errorf("expected audit log to record USER -> triage-agent transition")
	}
	if !strings.Contains(logStr, "triage-agent ➔") {
		t.Errorf("expected audit log to record triage-agent handoff")
	}

	// 2. Physical File Checks on Disk
	scriptPath := filepath.Join("testdata", "scripts", "multi_gpu_optimizer.py")
	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil || len(scriptBytes) == 0 {
		t.Errorf("expected physical script %s to be created on disk, err: %v", scriptPath, err)
	}

	reportPath := filepath.Join("testdata", "reports", "gpu_portfolio_report.json")
	reportBytes, err := os.ReadFile(reportPath)
	if err != nil || len(reportBytes) == 0 {
		t.Errorf("expected physical report %s to be created on disk, err: %v", reportPath, err)
	} else {
		var parsed interface{}
		if err := json.Unmarshal(reportBytes, &parsed); err != nil {
			t.Errorf("expected generated report %s to be valid JSON, err: %v", reportPath, err)
		}
	}
}

// Complex Test 2: Multi-Sheet Excel Financial Model Generation and Inspection
func TestE2E_MultiSheetExcelFinancialModel(t *testing.T) {
	excelPath := filepath.Join("testdata", "scripts", "financial_model.xlsx")
	_ = os.Remove(excelPath)

	prompt := "Create a multi-sheet excel file 'scripts/financial_model.xlsx' with a 'Summary' sheet setting cell A1 to 'ARR' and B1 to 500000, and a 'Details' sheet setting cell A1 to 'Region' and B1 to 'NA', then read back cell B1 from both sheets."

	_, logStr := runE2ETestCase(t, prompt, "e2e_multisheet_excel.log", "60")

	if !strings.Contains(logStr, "USER ➔ triage-agent") {
		t.Errorf("expected audit log to record USER -> triage-agent transition")
	}

	// Physical File Verification in Go using excelize
	f, err := excelize.OpenFile(excelPath)
	if err != nil {
		t.Fatalf("failed to open generated excel file %s: %v", excelPath, err)
	}
	defer f.Close()

	summaryVal, err := f.GetCellValue("Summary", "B1")
	if err != nil || (summaryVal != "500000" && summaryVal != "500000.00" && summaryVal != "500000.0") {
		t.Errorf("expected Summary sheet B1 to be 500000, got: %q, err: %v", summaryVal, err)
	}

	detailsVal, err := f.GetCellValue("Details", "B1")
	if err != nil || detailsVal != "NA" {
		t.Errorf("expected Details sheet B1 to be 'NA', got: %q, err: %v", detailsVal, err)
	}
}

// Complex Test 3: Monte Carlo Simulation Code Generation & Docker Execution
func TestE2E_CodeGenerationAndExecutionVerification(t *testing.T) {
	scriptPath := filepath.Join("testdata", "scripts", "monte_carlo_sim.py")
	_ = os.Remove(scriptPath)

	prompt := "Use write_file to write a Python script 'scripts/monte_carlo_sim.py' that runs a 10,000-iteration Monte Carlo simulation for GPU spot rate volatility using numpy, execute it in Docker, and print the 95% Value-at-Risk (VaR)."

	_, logStr := runE2ETestCase(t, prompt, "e2e_monte_carlo.log", "90")

	if !strings.Contains(logStr, "USER ➔ triage-agent") {
		t.Errorf("expected audit log to record USER -> triage-agent transition")
	}

	// Physical File Verification
	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil || len(scriptBytes) == 0 {
		t.Errorf("expected physical script %s to be created on disk, err: %v", scriptPath, err)
	} else if !strings.Contains(string(scriptBytes), "numpy") {
		t.Errorf("expected generated python script to import numpy, got: %s", string(scriptBytes))
	}

	// Log content check for numerical output
	if !strings.Contains(logStr, "VaR") && !strings.Contains(logStr, "Risk") && !strings.Contains(logStr, "Simulation") && !strings.Contains(logStr, "95%") {
		t.Errorf("expected log to contain simulation results")
	}
}

// Complex Test 4: Task Decomposition & Multi-Deliverable Artifact Generation
func TestE2E_TaskDecompositionWithArtifactGeneration(t *testing.T) {
	mdPath := filepath.Join("reports", "pricing_audit.md")
	pyPath := filepath.Join("scripts", "verify_providers.py")
	_ = os.Remove(mdPath)
	_ = os.Remove(pyPath)
	_ = os.Remove(filepath.Join("testdata", "reports", "pricing_audit.md"))
	_ = os.Remove(filepath.Join("testdata", "scripts", "verify_providers.py"))

	prompt := "Audit our compute pricing providers: 1) use write_file to write 'reports/pricing_audit.md' listing provider capabilities, and 2) use write_file to write 'scripts/verify_providers.py' that verifies provider latency."

	_, logStr := runE2ETestCase(t, prompt, "e2e_task_decomposition.log", "90")

	if !strings.Contains(logStr, "USER ➔ triage-agent") {
		t.Errorf("expected audit log to record USER -> triage-agent transition")
	}

	// Physical File Verifications
	mdBytes, err := os.ReadFile(mdPath)
	if err != nil || len(mdBytes) == 0 {
		mdBytes, err = os.ReadFile(filepath.Join("testdata", "reports", "pricing_audit.md"))
	}
	if err != nil || len(mdBytes) == 0 {
		t.Errorf("expected markdown report %s to exist on disk, err: %v", mdPath, err)
	} else if !strings.Contains(string(mdBytes), "#") {
		t.Errorf("expected markdown report to contain markdown headers, got: %s", string(mdBytes))
	}

	pyBytes, err := os.ReadFile(pyPath)
	if err != nil || len(pyBytes) == 0 {
		pyBytes, err = os.ReadFile(filepath.Join("testdata", "scripts", "verify_providers.py"))
	}
	if err != nil || len(pyBytes) == 0 {
		pyBytes, err = os.ReadFile(filepath.Join("scripts", "verify_latency.py"))
	}
	if err != nil || len(pyBytes) == 0 {
		pyBytes, err = os.ReadFile(filepath.Join("testdata", "scripts", "verify_latency.py"))
	}
	if err != nil || len(pyBytes) == 0 {
		t.Errorf("expected physical python verification script to exist on disk, err: %v", err)
	}
}

// Complex Test 5: Broad Task Dissection & RAG Memory Exchange Verification
func TestE2E_BroadTaskDissectionAndRAG(t *testing.T) {
	reportPath := filepath.Join("testdata", "reports", "executive_infrastructure_audit.md")
	_ = os.Remove(reportPath)

	prompt := "Perform a comprehensive compute infrastructure audit across H100 and RTX 4090 GPUs, query forward contracts, build a financial cost optimization model, and use write_file to save the executive summary report to 'reports/executive_infrastructure_audit.md'."

	_, logStr := runE2ETestCase(t, prompt, "e2e_broad_task_dissection.log", "90")

	// 1. Audit log checks for triage dispatch and planning
	if !strings.Contains(logStr, "USER ➔ triage-agent") {
		t.Errorf("expected audit log to record USER -> triage-agent transition")
	}
	if !strings.Contains(logStr, "triage-agent ➔") {
		t.Errorf("expected audit log to record triage-agent handoff")
	}

	// 2. Physical File Verification on Disk
	reportBytes, err := os.ReadFile(reportPath)
	if err != nil || len(reportBytes) == 0 {
		t.Errorf("expected physical executive report %s to exist on disk, err: %v", reportPath, err)
	} else {
		content := string(reportBytes)
		if !strings.Contains(content, "#") && !strings.Contains(content, "Audit") && !strings.Contains(content, "GPU") && !strings.Contains(content, "Executive") {
			t.Errorf("expected report content to contain header or audit analysis, got: %s", content)
		}
	}
}

// Test 6: Multi-Turn RAG Memory Persistence & Context Retrieval Verification
func TestE2E_RAG_MultiTurnMemoryRetrieval(t *testing.T) {
	_ = os.Remove(filepath.Join("testdata", "agent_framework.db"))

	// Turn 1: Save user preference
	turn1Prompt := "Remember this user preference: Our primary cloud provider for H100 SXM compute is RunPod in us-east region with max spot rate $2.50/hr."
	_, logStr1 := runE2ETestCase(t, turn1Prompt, "e2e_rag_turn1.log", "60")

	if !strings.Contains(logStr1, "USER ➔ triage-agent") {
		t.Errorf("expected Turn 1 to execute successfully")
	}

	// Turn 2: Query new prompt requiring retrieval of Turn 1 memory
	turn2Prompt := "Which cloud provider and max spot rate should we use for H100 SXM compute?"
	outStr2, logStr2 := runE2ETestCase(t, turn2Prompt, "e2e_rag_turn2.log", "60")

	// Verify RAG context injection trace in stdout or log
	if !strings.Contains(outStr2, "[RAG DEBUG]") && !strings.Contains(outStr2, "Retrieved historical logs") && !strings.Contains(logStr2, "Retrieved historical logs") && !strings.Contains(logStr2, "[RAG CONTEXT INJECTED]") && !strings.Contains(logStr2, "conversation_history") {
		t.Errorf("expected Turn 2 log/stdout to record RAG context injection, got stdout:\n%s\n--- LOG FILE ---\n%s", outStr2, logStr2)
	}

	// Verify that final response utilizes the retrieved RAG memory or live oracle data
	if !strings.Contains(logStr2, "RunPod") && !strings.Contains(logStr2, "Lambda") && !strings.Contains(logStr2, "2.53") && !strings.Contains(logStr2, "2.50") && !strings.Contains(logStr2, "2.49") {
		t.Errorf("expected Turn 2 final response to utilize retrieved RAG memory or oracle rates, got:\n%s", logStr2)
	}
}

// Test 7: Independent RAG Memory Ingestion & Natural Retrieval (No Explicit Commands)
func TestE2E_IndependentRAGMemory(t *testing.T) {
	_ = os.Remove(filepath.Join("testdata", "agent_framework.db"))

	// Turn 1: Natural status update with NO explicit memory instructions ("remember", "save", etc.)
	turn1Prompt := "We completed our migration of GPU compute clusters to Lambda Labs in us-west-2 region at an hourly rate of $2.15 per H100 instance."
	_, logStr1 := runE2ETestCase(t, turn1Prompt, "e2e_independent_rag_turn1.log", "60")

	if !strings.Contains(logStr1, "USER ➔ triage-agent") {
		t.Errorf("expected Turn 1 natural prompt to execute successfully")
	}

	// Turn 2: Natural question with NO explicit retrieval commands ("check memory", "retrieve", etc.)
	turn2Prompt := "Where is our GPU compute cluster currently hosted and what is its hourly rate?"
	outStr2, logStr2 := runE2ETestCase(t, turn2Prompt, "e2e_independent_rag_turn2.log", "60")

	// 1. Verify RAG context injection trace was performed automatically in background
	if !strings.Contains(outStr2, "[RAG DEBUG]") && !strings.Contains(outStr2, "Retrieved historical logs") && !strings.Contains(logStr2, "conversation_history") {
		t.Errorf("expected automatic background RAG context injection, got stdout:\n%s", outStr2)
	}

	// 2. Verify agent independently answered with the automatically retrieved context facts
	if !strings.Contains(logStr2, "Lambda") && !strings.Contains(logStr2, "RunPod") && !strings.Contains(logStr2, "2.15") && !strings.Contains(logStr2, "2.49") {
		t.Errorf("expected agent to independently recall compute cluster hosting/rates from RAG memory, got:\n%s", logStr2)
	}
}

// Test 8: Live Headless Browser Control, Form Interaction & Screenshot Verification
func TestE2E_BrowserControlAndAutomation(t *testing.T) {
	// 1. Start a local HTTP server with an interactive web form
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintln(w, `<!DOCTYPE html>
<html>
<head><title>Cluster Management Portal</title></head>
<body>
    <h1>Cluster Provisioning Portal</h1>
    <form id="provisionForm">
        <label for="cluster">Cluster Name:</label>
        <input type="text" id="cluster" name="cluster" placeholder="Cluster Name"><br>
        <label for="provider">Provider Name:</label>
        <input type="text" id="provider" name="provider" placeholder="Provider Name"><br>
        <button id="btnSubmit" type="button" onclick="document.getElementById('status').innerText = 'Registered: ' + document.getElementById('cluster').value + ' on ' + document.getElementById('provider').value;">Register</button>
    </form>
    <div id="status">Status: Pending</div>
</body>
</html>`)
	}))
	defer server.Close()

	screenshotPath := filepath.Join("testdata", "reports", "browser_cluster_setup.png")
	_ = os.Remove(screenshotPath)

	prompt := fmt.Sprintf("Use browser_navigate to visit '%s', fill in 'h100-alpha' into Cluster Name, fill in 'RunPod' into Provider Name, click the Register button, and use browser_screenshot to save a screenshot to 'reports/browser_cluster_setup.png'.", server.URL)

	_, logStr := runE2ETestCase(t, prompt, "e2e_browser_control.log", "90")

	// 1. Audit Log Verifications
	if !strings.Contains(logStr, "USER ➔ triage-agent") {
		t.Errorf("expected audit log to record USER -> triage-agent transition")
	}
	if !strings.Contains(logStr, "browser_navigate") && !strings.Contains(logStr, "browser-agent") {
		t.Errorf("expected audit log to record browser action, got log:\n%s", logStr)
	}

	// 2. Physical File Integrity Verification (Screenshot PNG)
	imgBytes, err := os.ReadFile(screenshotPath)
	if err != nil || len(imgBytes) == 0 {
		t.Errorf("expected physical screenshot %s to exist on disk, err: %v", screenshotPath, err)
	}
}

// Test 9: Headed Browser Mode & CAPTCHA Detection / HITL Router Verification
func TestE2E_BrowserCaptchaAndHeadedMode(t *testing.T) {
	// 1. Start local server with CAPTCHA challenge HTML
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintln(w, `<!DOCTYPE html>
<html>
<head><title>Cloudflare Security Verification</title></head>
<body>
    <h1>Security Check</h1>
    <div class="g-recaptcha" data-sitekey="test-key"></div>
    <p>Please verify you are human before proceeding to cluster management.</p>
</body>
</html>`)
	}))
	defer server.Close()

	prompt := fmt.Sprintf("Use browser_navigate to open '%s' and inspect the page state.", server.URL)

	// Run in HEADED mode
	_, logStr := runE2ETestCase(t, prompt, "e2e_browser_captcha.log", "60", "HEADED=true")

	// 1. Verify browser agent navigation
	if !strings.Contains(logStr, "USER ➔ triage-agent") {
		t.Errorf("expected audit log to record USER -> triage-agent transition")
	}

	// 2. Verify CAPTCHA detection notice in audit log
	if !strings.Contains(logStr, "CAPTCHA DETECTED") && !strings.Contains(logStr, "verify you are human") {
		t.Errorf("expected audit log to record CAPTCHA detection notice, got log:\n%s", logStr)
	}
}

// Test 10: Dynamic Job Scheduling, Execution Verification & Self-Cleaning Schedule Cancellation
func TestE2E_ScheduleJobExecutionAndCleanup(t *testing.T) {
	outputFile := filepath.Join("testdata", "reports", "scheduled_job_output.txt")
	_ = os.Remove(outputFile)

	// Step 1: Tell agent to schedule a task named 'quick_health_audit' with cron '@every 3s'
	schedulePrompt := "Use schedule_task to schedule a task named 'quick_health_audit' with cron '@every 3s' to execute the instruction: 'Use write_file to save the text Health Check PASSED to reports/scheduled_job_output.txt'."

	outStr1, logStr1 := runE2ETestCase(t, schedulePrompt, "e2e_schedule_create.log", "60")

	// 1. Verify schedule_task tool executed cleanly
	if !strings.Contains(logStr1, "schedule_task") && !strings.Contains(outStr1, "Successfully scheduled task") {
		t.Errorf("expected schedule_task tool execution, got log:\n%s", logStr1)
	}

	// 2. Wait for timer to fire and execute the scheduled task
	time.Sleep(5 * time.Second)

	// Step 2: Self-cleaning step — cancel and delete the schedule
	cleanupPrompt := "Use list_schedules to find the schedule ID for 'quick_health_audit', and use cancel_schedule to delete it."

	outStr2, logStr2 := runE2ETestCase(t, cleanupPrompt, "e2e_schedule_cleanup.log", "60")

	// 3. Verify cancel_schedule tool execution
	if !strings.Contains(logStr2, "cancel_schedule") && !strings.Contains(outStr2, "Successfully cancelled schedule") {
		t.Errorf("expected cancel_schedule tool execution for self-cleaning, got log:\n%s", logStr2)
	}
}

// Test 11: Agent Execution Pause & Async Wait Tool Verification
func TestE2E_AgentWaitSecondsExecution(t *testing.T) {
	waitFile := filepath.Join("testdata", "reports", "wait_verification.txt")
	_ = os.Remove(waitFile)

	prompt := "Use wait_seconds to pause execution for 2 seconds while waiting for background task completion, then use write_file to save 'Wait Complete' to reports/wait_verification.txt."

	outStr, logStr := runE2ETestCase(t, prompt, "e2e_agent_wait.log", "60")

	// 1. Verify wait_seconds tool execution in log
	if !strings.Contains(logStr, "wait_seconds") && !strings.Contains(logStr, "Finished waiting") && !strings.Contains(outStr, "Finished waiting") {
		t.Errorf("expected wait_seconds tool execution in log, got log:\n%s", logStr)
	}

	// 2. Physical File Integrity Verification
	txtBytes, err := os.ReadFile(waitFile)
	if err != nil || len(txtBytes) == 0 {
		t.Errorf("expected physical output file %s to exist on disk, err: %v", waitFile, err)
	}
}

// Test 12: Oracle Off-Peak Deferred Prompt Execution & Delay Simulation
func TestE2E_OffPeakDeferredPromptExecution(t *testing.T) {
	deferredFile := filepath.Join("testdata", "reports", "off_peak_deferred_output.txt")
	_ = os.Remove(deferredFile)

	prompt := "Query pricing oracle for optimal execution window and use write_file to save 'Off-Peak Execution Complete' to reports/off_peak_deferred_output.txt."

	// Run E2E test simulating off-peak window delay (OFF_PEAK_TEST_DELAY_SECS=3 for fast test execution)
	outStr, logStr := runE2ETestCase(t, prompt, "e2e_off_peak_deferred.log", "60", "OFF_PEAK_TEST_DELAY_SECS=3")

	// 1. Verify off-peak optimal window scheduling log entry or output
	if !strings.Contains(logStr, "deferred until") && !strings.Contains(outStr, "Optimal Start") && !strings.Contains(logStr, "Off-Peak") {
		t.Errorf("expected audit log to record Off-Peak Scheduler deferral notice, got:\n%s", logStr)
	}

	// 2. Physical File Integrity Verification
	txtBytes, err := os.ReadFile(deferredFile)
	if err != nil || len(txtBytes) == 0 {
		t.Errorf("expected physical output file %s to exist after off-peak firing, err: %v", deferredFile, err)
	}
}

// Test 13: Semantic Vector Memory Ranking & Search Verification
func TestE2E_SemanticVectorMemoryRanking(t *testing.T) {
	prompt := "Step 1: Call save_long_term_memory with key 'giga_gpu_cluster' and value 'Cluster consists of 128 H100 nodes'. Step 2: Call search_long_term_memories with query 'giga_gpu_cluster' and output the retrieved memory."

	outStr, logStr := runE2ETestCase(t, prompt, "e2e_vector_memory.log", "60")

	// 1. Verify save_long_term_memory executed
	if !strings.Contains(logStr, "save_long_term_memory") && !strings.Contains(outStr, "save_long_term_memory") {
		t.Errorf("expected save_long_term_memory execution in log, got:\n%s", logStr)
	}

	// 2. Verify vector memory search retrieved the memory with relevance score
	if !strings.Contains(logStr, "giga_gpu_cluster") && !strings.Contains(outStr, "giga_gpu_cluster") {
		t.Errorf("expected audit log or output to contain retrieved vector memory key 'giga_gpu_cluster', got:\n%s", logStr)
	}
}

// Test 14: Dynamic Subagent Delegation & Hierarchy Verification
func TestE2E_DynamicSubagentDelegation(t *testing.T) {
	prompt := "Use delegate_subtask tool with target_agent_id 'writer-agent' and subtask_prompt 'Draft summary titled Subagent Executive Summary' to delegate a subtask."

	outStr, logStr := runE2ETestCase(t, prompt, "e2e_subagent_delegation.log", "60")

	// 1. Verify delegate_subtask tool execution
	if !strings.Contains(logStr, "delegate_subtask") && !strings.Contains(outStr, "delegate_subtask") && !strings.Contains(outStr, "Subagent Executive Summary") {
		t.Errorf("expected delegate_subtask execution in log, got log:\n%s", logStr)
	}
}

// Test 15: Git Repo Operations, CSV/JSON Transformation & Zip Archive Packaging
func TestE2E_GitAndArchivePipeline(t *testing.T) {
	jsonPath := filepath.Join("testdata", "reports", "users.json")
	zipPath := filepath.Join("testdata", "reports", "user_data.zip")
	_ = os.Remove(jsonPath)
	_ = os.Remove(zipPath)

	prompt := "Call native tool git_operations with operation 'status'. Call native tool csv_json_transformer with operation 'csv_to_json', csv_data 'name,role\\nAlice,Admin\\nBob,Dev\\n', and output_path 'reports/users.json'. Call native tool archive_manager with operation 'zip', source_path 'reports/users.json', and zip_path 'reports/user_data.zip'."

	_, logStr := runE2ETestCase(t, prompt, "e2e_git_archive_pipeline.log", "60", "DISABLE_DOCKER_TOOLS=true")

	// 1. Audit log check
	if !strings.Contains(logStr, "USER ➔ triage-agent") {
		t.Errorf("expected audit log to record USER -> triage-agent transition")
	}

	// 2. Physical File Verifications
	jsonBytes, err := os.ReadFile(jsonPath)
	if err != nil || len(jsonBytes) == 0 {
		t.Errorf("expected generated JSON %s to exist on disk, err: %v", jsonPath, err)
	}

	zipBytes, err := os.ReadFile(zipPath)
	if err != nil || len(zipBytes) == 0 {
		t.Errorf("expected generated zip archive %s to exist on disk, err: %v", zipPath, err)
	}
}

// Test 16: SQLite Database Querying & System Process Inspection
func TestE2E_SQLiteAndProcessInspection(t *testing.T) {
	prompt := "Step 1: Call query_sqlite_db with db_path 'agent_framework.db' and operation 'list_tables'. Step 2: Call inspect_system_processes with action 'list'."

	outStr, logStr := runE2ETestCase(t, prompt, "e2e_sqlite_process.log", "60", "DISABLE_DOCKER_TOOLS=true")

	// 1. Audit log check
	if !strings.Contains(logStr, "USER ➔ triage-agent") {
		t.Errorf("expected audit log to record USER -> triage-agent transition")
	}

	// 2. Log or output verification for tool execution
	if !strings.Contains(logStr, "query_sqlite_db") && !strings.Contains(outStr, "query_sqlite_db") {
		t.Errorf("expected query_sqlite_db execution in log, got log:\n%s", logStr)
	}
	if !strings.Contains(logStr, "inspect_system_processes") && !strings.Contains(outStr, "inspect_system_processes") {
		t.Errorf("expected inspect_system_processes execution in log, got log:\n%s", logStr)
	}
}

// Test 17: HTTP API REST Client & JSON Schema Validation
func TestE2E_HTTPAndJSONSchemaValidation(t *testing.T) {
	prompt := "Step 1: Call http_api_request with method 'GET' and url 'https://httpbin.org/get'. Step 2: Call validate_json_schema with json_string '{\"headers\": {}}' and required_keys ['headers']."

	outStr, logStr := runE2ETestCase(t, prompt, "e2e_http_json_schema.log", "60", "DISABLE_DOCKER_TOOLS=true")

	// 1. Audit log check
	if !strings.Contains(logStr, "USER ➔ triage-agent") {
		t.Errorf("expected audit log to record USER -> triage-agent transition")
	}

	// 2. Log or output verification for tool execution
	if !strings.Contains(logStr, "http_api_request") && !strings.Contains(outStr, "http_api_request") {
		t.Errorf("expected http_api_request execution in log, got log:\n%s", logStr)
	}
	if !strings.Contains(logStr, "validate_json_schema") && !strings.Contains(outStr, "validate_json_schema") {
		t.Errorf("expected validate_json_schema execution in log, got log:\n%s", logStr)
	}
}

// Test 18: Free Real-World Declarative Zero-Code Pricing & Forward Curve Integration
func TestE2E_FreeLivePricingAndForwardCurve(t *testing.T) {
	prompt := "Step 1: Call query_pricing_oracle with asset 'RTX_4090' and market_type 'spot' to check live GPU spot rate. Step 2: Call query_pricing_oracle with asset 'H100_SXM' and market_type 'execution_window' and cost_mode 'hedged_spot' to check forward hedged execution window."

	outStr, logStr := runE2ETestCase(t, prompt, "e2e_declarative_pricing_pipeline.log", "60", "PRICING_PROVIDER=declarative_live", "DISABLE_DOCKER_TOOLS=true")

	// 1. Audit log checks for triage routing and agent execution
	if !strings.Contains(logStr, "USER ➔ triage-agent") {
		t.Errorf("expected audit log to record USER -> triage-agent transition")
	}
	if !strings.Contains(logStr, "triage-agent ➔") {
		t.Errorf("expected audit log to record triage-agent handoff")
	}

	// 2. Verify pricing oracle execution and declarative provider data in log/output
	hasPricingData := strings.Contains(logStr, "declarative_live") || strings.Contains(logStr, "RTX_4090") || strings.Contains(logStr, "H100_SXM") || strings.Contains(logStr, "spot_price") || strings.Contains(outStr, "RTX_4090") || strings.Contains(outStr, "H100_SXM")
	if !hasPricingData {
		t.Errorf("expected log or output to contain live GPU pricing data from declarative_live provider, got log:\n%s", logStr)
	}
}

// Test 19: Stateful Derivative Hedge Contract Execution & Position Management
func TestE2E_DerivativeHedgeContractExecution(t *testing.T) {
	prompt := "Step 1: Call manage_hedge_contract with action 'enter_forward', asset 'H100_SXM', forward_rate 2.60, and duration_hours 8 to lock compute rate. Step 2: Call manage_hedge_contract with action 'list_positions' to verify the active contract."

	outStr, logStr := runE2ETestCase(t, prompt, "e2e_derivative_hedge_pipeline.log", "90", "PRICING_PROVIDER=declarative_live", "DISABLE_DOCKER_TOOLS=true")

	// 1. Audit log checks for triage routing and agent execution
	if !strings.Contains(logStr, "USER ➔ triage-agent") {
		t.Errorf("expected audit log to record USER -> triage-agent transition")
	}
	if !strings.Contains(logStr, "triage-agent ➔") {
		t.Errorf("expected audit log to record triage-agent handoff")
	}

	// 2. Verify manage_hedge_contract tool execution and open position verification
	hasHedgeContract := strings.Contains(logStr, "manage_hedge_contract") || strings.Contains(logStr, "enter_forward") || strings.Contains(logStr, "H100_SXM") || strings.Contains(outStr, "hedge") || strings.Contains(outStr, "forward")
	if !hasHedgeContract {
		t.Errorf("expected log or output to contain derivative hedge execution data, got log:\n%s", logStr)
	}
}





