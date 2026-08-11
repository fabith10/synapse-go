package hitl

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestHITLGateway(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_hitl.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	defer db.Close()

	gw, err := InitGlobalHITLGateway(db)
	if err != nil {
		t.Fatalf("failed to init HITL gateway: %v", err)
	}

	// 1. Test CreateApproval
	meta := map[string]string{"action": "test.action", "risk": "MEDIUM"}
	app, err := gw.CreateApproval("corr-123", "planner-agent", "test.action", "Test HITL Request", meta)
	if err != nil {
		t.Fatalf("CreateApproval failed: %v", err)
	}
	if app.Status != "PENDING" {
		t.Errorf("expected PENDING status, got %s", app.Status)
	}

	// 2. Test ListPendingApprovals
	pending, err := gw.ListPendingApprovals()
	if err != nil {
		t.Fatalf("ListPendingApprovals failed: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("expected 1 pending approval, got %d", len(pending))
	}

	// 3. Test SubmitDecision
	resolved, err := gw.SubmitDecision("corr-123", "approve", "Looks good")
	if err != nil {
		t.Fatalf("SubmitDecision failed: %v", err)
	}
	if resolved.Status != "APPROVED" {
		t.Errorf("expected APPROVED status, got %s", resolved.Status)
	}
	if resolved.Feedback != "Looks good" {
		t.Errorf("expected feedback 'Looks good', got '%s'", resolved.Feedback)
	}

	// 4. Test ListPendingApprovals empty after resolution
	pendingAfter, _ := gw.ListPendingApprovals()
	if len(pendingAfter) != 0 {
		t.Errorf("expected 0 pending approvals after resolution, got %d", len(pendingAfter))
	}

	_ = os.Remove(dbPath)
}
