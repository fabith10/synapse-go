package orchestrator

import (
	"context"
	"testing"
)

func TestPlannerScheduler_MiddlewareProgressSuppression(t *testing.T) {
	ps := &PlannerScheduler{
		plans: make(map[string]*ExecutionPlan),
	}
	var sent []Message
	ps.Initialize(func(m Message) {
		sent = append(sent, m)
	})

	mw := ps.Middleware()

	nextCalled := false
	next := func(m Message) {
		nextCalled = true
	}

	// 1. Progress message from planner-agent to USER should be suppressed, next should NOT be called
	progressMsg := Message{
		Sender:    "planner-agent",
		Recipient: "USER",
		Content:   "[Plan Execution] Dispatched sub-task \"task1\"",
		Metadata: map[string]string{
			"Type":           "PROGRESS",
			"correlation_id": "test-123",
		},
	}
	mw(context.Background(), progressMsg, next)
	if nextCalled {
		t.Errorf("expected progress message to be suppressed, but next was called")
	}

	// 2. Final synthesis message from planner-agent to USER should NOT be intercepted as plan; next MUST be called
	nextCalled = false
	var passedMsg Message
	next = func(m Message) {
		nextCalled = true
		passedMsg = m
	}
	synthesisMsg := Message{
		Sender:    "planner-agent",
		Recipient: "USER",
		Content:   "# Final Synthesis Report\nAll tasks succeeded.",
		Metadata: map[string]string{
			"phase":          "synthesis",
			"correlation_id": "test-123",
		},
	}
	mw(context.Background(), synthesisMsg, next)
	if !nextCalled {
		t.Fatalf("expected synthesis message to pass downstream to next, but it was not")
	}
	if passedMsg.Content != synthesisMsg.Content {
		t.Errorf("expected %q, got %q", synthesisMsg.Content, passedMsg.Content)
	}

	// 3. Valid JSON plan from planner-agent to USER should be intercepted (next not called)
	nextCalled = false
	planJSON := `{
		"tasks": [
			{"id": "t1", "description": "do task 1", "dependencies": []}
		]
	}`
	planMsg := Message{
		Sender:    "planner-agent",
		Recipient: "USER",
		Content:   planJSON,
		Metadata: map[string]string{
			"correlation_id": "test-plan-456",
		},
	}
	mw(context.Background(), planMsg, next)
	if nextCalled {
		t.Errorf("expected valid plan to be intercepted and suppressed, but next was called")
	}

	// 4. Valid JSON plan with map tasks from planner-agent to USER
	nextCalled = false
	planMapJSON := `{
		"original_goal": "test goal",
		"tasks": {
			"t2": {"id": "t2", "description": "do task 2", "dependencies": []}
		}
	}`
	planMapMsg := Message{
		Sender:    "planner-agent",
		Recipient: "USER",
		Content:   planMapJSON,
		Metadata: map[string]string{
			"correlation_id": "test-plan-789",
		},
	}
	mw(context.Background(), planMapMsg, next)
	if nextCalled {
		t.Errorf("expected valid map plan to be intercepted and suppressed, but next was called")
	}

	// 5. Intercept completed subtask message
	nextCalled = false
	subtaskMsg := Message{
		Sender:    "generalist-agent",
		Recipient: "USER",
		Content:   "Task 2 completed successfully.",
		Metadata: map[string]string{
			"is_subtask":     "true",
			"subtask_id":     "t2",
			"correlation_id": "test-plan-789",
		},
	}
	mw(context.Background(), subtaskMsg, next)
	if nextCalled {
		t.Errorf("expected subtask completion to be intercepted, but next was called")
	}

	ps.mu.RLock()
	task2Status := ps.plans["test-plan-789"].Tasks["t2"].Status
	ps.mu.RUnlock()
	if task2Status != "completed" {
		t.Errorf("expected task t2 status to be completed, got %q", task2Status)
	}
}
