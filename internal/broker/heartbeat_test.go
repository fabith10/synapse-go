package broker

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fabith10/synapse-go/internal/memory"
)

func TestAutonomousHeartbeatEngine(t *testing.T) {
	ctx := context.Background()

	dbPath := "/tmp/test_heartbeat.db"
	_ = os.Remove(dbPath)
	defer os.Remove(dbPath)

	store, err := memory.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

	// 1. Create a goal with a pending milestone
	goal, err := store.CreateGoal(ctx, "Automate Server Backups", "Run nightly backup", "{}")
	if err != nil {
		t.Fatalf("failed to create goal: %v", err)
	}

	_, err = store.AddGoalMilestone(ctx, goal.ID, "Backup MySQL DB", "generalist-agent")
	if err != nil {
		t.Fatalf("failed to add milestone: %v", err)
	}

	var dispatchCount int32
	dispatchFn := func(ctx context.Context, agentID, taskPrompt string) error {
		atomic.AddInt32(&dispatchCount, 1)
		return nil
	}

	// 2. Initialize heartbeat engine
	engine := NewAutonomousHeartbeatEngine(store, 100*time.Millisecond, dispatchFn)

	// 3. Trigger evaluation cycle
	err = engine.TriggerOnce(ctx)
	if err != nil {
		t.Fatalf("TriggerOnce failed: %v", err)
	}

	if atomic.LoadInt32(&dispatchCount) != 1 {
		t.Errorf("expected 1 milestone dispatch, got %d", atomic.LoadInt32(&dispatchCount))
	}

	// 4. Verify milestone status transitioned to DONE
	goals, err := store.ListActiveGoals(ctx)
	if err != nil || len(goals) == 0 {
		t.Fatalf("expected active goal: %v", err)
	}
	if len(goals[0].Milestones) == 0 || goals[0].Milestones[0].Status != "DONE" {
		t.Errorf("expected milestone status DONE, got: %v", goals[0].Milestones)
	}
}
