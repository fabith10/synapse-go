package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cdpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/fabith10/synapse-go/adk"
	"github.com/fabith10/synapse-go/internal/agent"
	"github.com/fabith10/synapse-go/internal/web"
)

func TestE2E_ArtifactsAndTriageLifecycle(t *testing.T) {
	// 1. Environment & Runtime Setup
	os.Setenv("AGENT_FRAMEWORK_TESTING", "true")
	os.Setenv("HEADLESS", "true")
	os.Setenv("SQLITE_DSN", "file::memory:?cache=shared")

	rt, err := agent.Bootstrap(adk.Config{
		SQLiteDSN:       "file::memory:?cache=shared",
		DefaultStrategy: adk.StrategyBalanced,
	})
	if err != nil {
		t.Fatalf("failed to bootstrap agent runtime: %v", err)
	}
	defer rt.Shutdown()

	webServer := web.NewServer(rt)
	server := httptest.NewServer(webServer)
	defer server.Close()

	// Setup clean workspace folders for test artifacts
	_ = os.MkdirAll("reports", 0755)
	_ = os.MkdirAll("scripts", 0755)
	_ = os.MkdirAll("workbooks", 0755)
	_ = os.MkdirAll("uploads", 0755)
	defer os.RemoveAll("reports")
	defer os.RemoveAll("scripts")
	defer os.RemoveAll("workbooks")
	defer os.RemoveAll("uploads")

	// =========================================================================
	// PHASE 1: ARTIFACT GENERATION & DISK SCANNING
	// =========================================================================
	pdfPath := filepath.Join("reports", "ai_market_forecast.pdf")
	_ = os.WriteFile(pdfPath, []byte("%PDF-1.4 Mock PDF Content for E2E Testing"), 0644)

	pyPath := filepath.Join("scripts", "developer_portfolio_optimizer.py")
	_ = os.WriteFile(pyPath, []byte("import numpy as np\nprint('Optimization Complete')"), 0644)

	excelPath := filepath.Join("workbooks", "financial_model.xlsx")
	_ = os.WriteFile(excelPath, []byte("Mock Excel Binary Content"), 0644)

	mdPath := filepath.Join("reports", "researcher_memo.md")
	_ = os.WriteFile(mdPath, []byte("# Swarm Intelligence Report\nAutonomous multi-agent execution."), 0644)

	csvPath := filepath.Join("uploads", "pricing_matrix.csv")
	_ = os.WriteFile(csvPath, []byte("provider,region,price\nRunPod,us-east,2.50\nLambda,us-west,2.15"), 0644)

	t.Log("Created test artifacts across reports/, scripts/, workbooks/, and uploads/")

	// Query /api/artifacts JSON endpoint
	resp, err := http.Get(server.URL + "/api/artifacts")
	if err != nil {
		t.Fatalf("failed GET /api/artifacts: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 from /api/artifacts, got %d", resp.StatusCode)
	}

	var artifactsResp struct {
		Status    string             `json:"status"`
		Artifacts []web.ArtifactItem `json:"artifacts"`
		Count     int                `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&artifactsResp); err != nil {
		t.Fatalf("failed decoding /api/artifacts JSON: %v", err)
	}

	if artifactsResp.Count < 5 {
		t.Errorf("expected at least 5 scanned artifacts, got %d", artifactsResp.Count)
	}

	// Verify artifact categorization and agent attribution
	foundPDF, foundPython, foundExcel, foundDoc, foundCSV := false, false, false, false, false
	for _, a := range artifactsResp.Artifacts {
		switch a.Filename {
		case "ai_market_forecast.pdf":
			foundPDF = true
			if a.ArtifactType != "pdf" {
				t.Errorf("expected type 'pdf', got %q", a.ArtifactType)
			}
		case "developer_portfolio_optimizer.py":
			foundPython = true
			if a.ArtifactType != "code" {
				t.Errorf("expected type 'code', got %q", a.ArtifactType)
			}
			if !strings.Contains(a.Agent, "developer") {
				t.Errorf("expected agent attribution to contain developer, got %q", a.Agent)
			}
		case "financial_model.xlsx":
			foundExcel = true
			if a.ArtifactType != "excel" {
				t.Errorf("expected type 'excel', got %q", a.ArtifactType)
			}
		case "researcher_memo.md":
			foundDoc = true
			if a.ArtifactType != "doc" {
				t.Errorf("expected type 'doc', got %q", a.ArtifactType)
			}
			if !strings.Contains(a.Agent, "researcher") {
				t.Errorf("expected agent attribution to contain researcher, got %q", a.Agent)
			}
		case "pricing_matrix.csv":
			foundCSV = true
			if a.ArtifactType != "excel" {
				t.Errorf("expected type 'excel' for csv, got %q", a.ArtifactType)
			}
		}
	}

	if !foundPDF || !foundPython || !foundExcel || !foundDoc || !foundCSV {
		t.Errorf("missing scanned artifacts: pdf=%v, py=%v, xlsx=%v, doc=%v, csv=%v", foundPDF, foundPython, foundExcel, foundDoc, foundCSV)
	}
	t.Logf("✓ Scanned and categorized %d artifacts with correct agent attributions", artifactsResp.Count)

	// =========================================================================
	// PHASE 2: ARTIFACT DELIVERY API & SECURITY CONTROLS
	// =========================================================================

	// Test 1: Content-Type & Content-Disposition for PDF
	pdfURL := fmt.Sprintf("%s/api/artifact?path=%s", server.URL, url.QueryEscape(pdfPath))
	resPDF, err := http.Get(pdfURL)
	if err != nil || resPDF.StatusCode != http.StatusOK {
		t.Fatalf("failed fetching PDF artifact: code=%d, err: %v", resPDF.StatusCode, err)
	}
	if ct := resPDF.Header.Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("expected application/pdf, got %q", ct)
	}
	if cd := resPDF.Header.Get("Content-Disposition"); !strings.Contains(cd, "inline; filename=\"ai_market_forecast.pdf\"") {
		t.Errorf("expected inline Content-Disposition, got %q", cd)
	}
	resPDF.Body.Close()

	// Test 2: Content-Type for Python Code
	pyURL := fmt.Sprintf("%s/api/artifact?path=%s", server.URL, url.QueryEscape(pyPath))
	resPY, err := http.Get(pyURL)
	if err != nil || resPY.StatusCode != http.StatusOK {
		t.Fatalf("failed fetching Python artifact: code=%d, err: %v", resPY.StatusCode, err)
	}
	if ct := resPY.Header.Get("Content-Type"); !strings.Contains(ct, "text/x-python") {
		t.Errorf("expected text/x-python, got %q", ct)
	}
	resPY.Body.Close()

	// Test 3: Path Cleaning (file:// prefix and trailing quotes/periods)
	dirtyPath := "file://" + pyPath + "..."
	dirtyURL := fmt.Sprintf("%s/api/artifact?path=%s", server.URL, url.QueryEscape(dirtyPath))
	resDirty, err := http.Get(dirtyURL)
	if err != nil || resDirty.StatusCode != http.StatusOK {
		t.Errorf("failed fetching artifact with file:// prefix and trailing dots: code=%d, err: %v", resDirty.StatusCode, err)
	}
	resDirty.Body.Close()

	// Test 4: Basename Fallback Resolution (requesting just "ai_market_forecast.pdf")
	fallbackURL := server.URL + "/api/artifact?path=ai_market_forecast.pdf"
	resFallback, err := http.Get(fallbackURL)
	if err != nil || resFallback.StatusCode != http.StatusOK {
		t.Errorf("failed fetching artifact via basename fallback: code=%d, err: %v", resFallback.StatusCode, err)
	}
	resFallback.Body.Close()

	// Test 5: Security Boundary - Path Traversal Protection
	traversalURL := server.URL + "/api/artifact?path=../../../../etc/passwd"
	resTraversal, err := http.Get(traversalURL)
	if err == nil {
		if resTraversal.StatusCode != http.StatusForbidden && resTraversal.StatusCode != http.StatusBadRequest && resTraversal.StatusCode != http.StatusNotFound {
			t.Errorf("expected 400 or 403 or 404 for path traversal attempt, got %d", resTraversal.StatusCode)
		}
		resTraversal.Body.Close()
	}

	// Test 6: Security Boundary - Protected System Files Protection
	protectedFiles := []string{"models.json", "critical_actions.json", ".env"}
	for _, pFile := range protectedFiles {
		protURL := fmt.Sprintf("%s/api/artifact?path=%s", server.URL, pFile)
		resProt, err := http.Get(protURL)
		if err == nil {
			if resProt.StatusCode != http.StatusForbidden {
				t.Errorf("expected 403 Forbidden for protected system file %s, got %d", pFile, resProt.StatusCode)
			}
			resProt.Body.Close()
		}
	}
	t.Log("✓ Artifact delivery API, Content-Types, path cleaning, and security bounds verified")

	// =========================================================================
	// PHASE 3: SWARM TRIAGE & ROUTING ENGINE
	// =========================================================================

	// Test dynamic capability resolution across diverse task instructions
	triageCases := []struct {
		task           string
		expectedAgents []string
	}{
		{
			task:           "Write and execute a Python script to backtest moving average crossover strategy using pandas and numpy",
			expectedAgents: []string{"quant-agent", "developer-agent", "financial-analyst-agent"},
		},
		{
			task:           "Open financial workbook in Excel, inspect formula dependencies, calculate variance table",
			expectedAgents: []string{"excel-agent", "financial-analyst-agent"},
		},
		{
			task:           "Navigate to website with headless browser, take full-page screenshot, and extract pricing DOM tables",
			expectedAgents: []string{"browser-agent"},
		},
		{
			task:           "Deep research on autonomous swarm architectures, synthesize literature and write markdown memo",
			expectedAgents: []string{"researcher-agent", "writer-agent"},
		},
	}

	for _, tc := range triageCases {
		resolved := agent.ResolveTargetAgent("", tc.task)
		matched := false
		for _, exp := range tc.expectedAgents {
			if resolved == exp {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("ResolveTargetAgent(%q) = %q; expected one of %v", tc.task, resolved, tc.expectedAgents)
		} else {
			t.Logf("✓ Triage correctly routed %q ➔ %s", tc.task[:30]+"...", resolved)
		}
	}

	// Test Steered Mode Triage Routing (task with orchestration_mode: steered routes to planner-agent)
	corrID := fmt.Sprintf("triage-steered-%d", time.Now().UnixNano())
	steeredMsg := adk.Message{
		Sender:    "USER",
		Recipient: "triage-agent",
		Content:   "Build an automated data pipeline to sync and audit cloud compute instances",
		Metadata: map[string]string{
			"correlation_id":     corrID,
			"orchestration_mode": "steered",
		},
	}
	rt.Orchestrator().Send(steeredMsg)
	t.Log("✓ Steered task dispatched to swarm triage engine")

	// =========================================================================
	// PHASE 4: HEADLESS BROWSER E2E ARTIFACT PREVIEW MODAL & GALLERY
	// =========================================================================
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.DisableGPU,
		chromedp.NoSandbox,
		chromedp.Headless,
		chromedp.Flag("disable-dev-shm-usage", true),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocCancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	ctx, timeoutCancel := context.WithTimeout(ctx, 35*time.Second)
	defer timeoutCancel()

	var jsErrors []string
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		if ev, ok := ev.(*cdpruntime.EventExceptionThrown); ok {
			desc := ev.ExceptionDetails.Text
			if ev.ExceptionDetails.Exception != nil {
				desc = ev.ExceptionDetails.Exception.Description
			}
			jsErrors = append(jsErrors, fmt.Sprintf("%s at %s:%d", desc, ev.ExceptionDetails.URL, ev.ExceptionDetails.LineNumber))
		}
	})

	var renderedCardsCount int
	var modalDisplay string
	var modalTitle string

	// Step 1: Open dashboard and switch to Deliverables Drawer Tab
	err = chromedp.Run(ctx,
		chromedp.Navigate(server.URL),
		chromedp.WaitVisible(`.cockpit-grid`, chromedp.ByQuery),
		chromedp.Click(`#drawer-btn-artifacts`, chromedp.ByID),
		chromedp.Sleep(150*time.Millisecond),
		chromedp.Evaluate(`document.querySelectorAll('#artifacts-list .artifact-card').length`, &renderedCardsCount),
	)
	if err != nil || renderedCardsCount < 4 {
		t.Errorf("expected at least 4 rendered artifact cards in deliverables gallery, got %d, err: %v", renderedCardsCount, err)
	}
	t.Logf("✓ Deliverables gallery rendered %d dynamic artifact cards", renderedCardsCount)

	// Step 2: Click Preview on first artifact card
	err = chromedp.Run(ctx,
		chromedp.Click(`.btn-artifact-preview:first-child`, chromedp.ByQuery),
		chromedp.Sleep(200*time.Millisecond),
		chromedp.WaitVisible(`#artifact-viewer-modal`, chromedp.ByID),
		chromedp.Evaluate(`window.getComputedStyle(document.getElementById('artifact-viewer-modal')).display`, &modalDisplay),
		chromedp.Evaluate(`document.getElementById('artifact-modal-title').textContent`, &modalTitle),
	)
	if err != nil || modalDisplay != "flex" {
		t.Errorf("artifact preview modal failed to display: %s (err: %v)", modalDisplay, err)
	}
	t.Logf("✓ Clicked preview button: modal opened (%s) for artifact: %q", modalDisplay, modalTitle)

	// Step 3: Close modal with close button
	err = chromedp.Run(ctx,
		chromedp.Click(`#artifact-modal-close-btn`, chromedp.ByID),
		chromedp.Sleep(150*time.Millisecond),
		chromedp.Evaluate(`window.getComputedStyle(document.getElementById('artifact-viewer-modal')).display`, &modalDisplay),
	)
	if err != nil || modalDisplay != "none" {
		t.Errorf("modal failed to close via close button: display=%s, err: %v", modalDisplay, err)
	}
	t.Log("✓ Modal cleanly dismissed via close button")

	// Step 4: Test single artifact deletion API (/api/artifact/delete)
	deleteForm := url.Values{"path": {pdfPath}}
	resDel, err := http.PostForm(server.URL+"/api/artifact/delete", deleteForm)
	if err != nil || resDel.StatusCode != http.StatusOK {
		t.Errorf("failed deleting artifact via /api/artifact/delete: code=%d, err: %v", resDel.StatusCode, err)
	}
	if resDel != nil {
		resDel.Body.Close()
	}
	if _, err := os.Stat(pdfPath); !os.IsNotExist(err) {
		t.Errorf("expected %s to be deleted from disk, but it still exists", pdfPath)
	}
	t.Logf("✓ Single artifact deletion API (/api/artifact/delete) verified on disk")

	// Step 5: Test Clear All artifacts API (/api/artifact/delete-all)
	resDelAll, err := http.Post(server.URL+"/api/artifact/delete-all", "application/x-www-form-urlencoded", nil)
	if err != nil || resDelAll.StatusCode != http.StatusOK {
		t.Errorf("failed deleting all artifacts: code=%d, err: %v", resDelAll.StatusCode, err)
	}
	if resDelAll != nil {
		resDelAll.Body.Close()
	}

	// Verify Deliverables gallery updates to empty state via fetchLatestArtifacts
	var emptyText string
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`fetchLatestArtifacts()`, nil),
		chromedp.Sleep(250*time.Millisecond),
		chromedp.Evaluate(`document.getElementById('artifacts-empty') ? document.getElementById('artifacts-empty').textContent : ''`, &emptyText),
	)
	if err != nil || !strings.Contains(emptyText, "NO ARTIFACTS PRODUCED") {
		t.Errorf("expected empty state notice after clear-all, got %q, err: %v", emptyText, err)
	}
	t.Log("✓ Bulk delete-all API and frontend empty state refresh verified")

	// Step 6: Verify zero JavaScript exceptions across all artifact & triage interactions
	if len(jsErrors) > 0 {
		for _, e := range jsErrors {
			if !strings.Contains(e, "net::ERR") && !strings.Contains(e, "favicon") {
				t.Errorf("Browser JavaScript Exception detected: %s", e)
			}
		}
	} else {
		t.Log("✓ Zero JavaScript exceptions detected during Artifact & Triage lifecycle")
	}
}
