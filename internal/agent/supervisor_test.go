package agent

import (
	"context"
	"testing"
	"time"

	"github.com/fabith10/synapse-go/adk"
	"github.com/fabith10/synapse-go/internal/orchestrator"
)

func TestIsFailureOutput(t *testing.T) {
	failures := []string{
		"failed: cannot open file",
		"python execution failed: traceback",
		"some technical error: syntax error",
		"[Task Failed] quant-agent failed",
		"excel operation failed: sheet not found",
	}
	for _, f := range failures {
		if !isFailureOutput(f) {
			t.Errorf("expected isFailureOutput to detect failure for: %q", f)
		}
	}

	successes := []string{
		"Successfully completed spreadsheet updates.",
		"Email draft created.",
		"Calculated result: 123.45",
		"No error detected.",
	}
	for _, s := range successes {
		if isFailureOutput(s) {
			t.Errorf("expected isFailureOutput to be false for: %q", s)
		}
	}
}

type dummyLLM struct {
	lastPayload interface{}
}

func (d *dummyLLM) Generate(ctx context.Context, req adk.TaskRequest, msgs []adk.Message) (adk.Message, error) {
	d.lastPayload = msgs
	return adk.Message{Content: `{"verdict":"DONE","reason":"test"}`}, nil
}

func TestSupervisorAgent_SubTaskFallback(t *testing.T) {
	llm := &dummyLLM{}
	orch := orchestrator.NewOrchestrator()
	s := NewSupervisorAgent("supervisor-agent", llm, orch)

	msg := adk.Message{
		Sender:    "quant-agent",
		Recipient: "USER",
		Content:   "result payload",
		Metadata: map[string]string{
			"subtask_description": "calculate option premium",
			"original_goal":       "run black scholes",
		},
	}

	err := s.Handle(context.Background(), msg)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	msgs, ok := llm.lastPayload.([]adk.Message)
	if !ok || len(msgs) != 2 {
		t.Fatalf("expected lastPayload to contain 2 messages, got: %v", llm.lastPayload)
	}

	// Message 1 is system prompt, Message 2 is evaluationPrompt
	userMsg := msgs[1]
	if userMsg.Sender != "USER" {
		t.Errorf("expected sender USER, got: %q", userMsg.Sender)
	}

	if !tContains(userMsg.Content, "calculate option premium") {
		t.Errorf("expected evaluationPrompt to evaluate subtask description, got: %q", userMsg.Content)
	}
}

func TestSupervisorResultRouting(t *testing.T) {
	llm := &dummyLLM{}
	orch := orchestrator.NewOrchestrator()
	s := NewSupervisorAgent("supervisor-agent", llm, orch)

	var sentMsgs []orchestrator.Message
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	orch.Start(ctx)

	go func() {
		ch := orch.HumanApprovalChan()
		for msg := range ch {
			sentMsgs = append(sentMsgs, msg)
		}
	}()

	msg := adk.Message{
		Sender:    "browser-agent",
		Recipient: "USER",
		Content:   "extracted hacker news top story",
		Metadata: map[string]string{
			"is_subtask":          "true",
			"subtask_id":          "task1",
			"subtask_description": "Scrape Hacker News",
			"correlation_id":      "corr-123",
		},
	}

	err := s.Handle(context.Background(), msg)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	// Give a tiny moment for routing delivery goroutine
	time.Sleep(50 * time.Millisecond)

	// Should have sent 2 messages to "USER":
	// 1. The progress/evaluation log message (with is_subtask deleted, and Type set to PROGRESS).
	// 2. The forwarded original message content (retaining is_subtask="true" and supervisor_bypass="true").
	if len(sentMsgs) != 2 {
		t.Fatalf("expected 2 messages sent to USER, got %d", len(sentMsgs))
	}

	// Message 1 is the progress log
	m1 := sentMsgs[0]
	if m1.Metadata["is_subtask"] != "" {
		t.Errorf("expected is_subtask to be deleted on progress log, got: %q", m1.Metadata["is_subtask"])
	}
	if m1.Metadata["Type"] != "PROGRESS" {
		t.Errorf("expected Type to be PROGRESS on progress log, got: %q", m1.Metadata["Type"])
	}

	// Message 2 is the forwarded output
	m2 := sentMsgs[1]
	if m2.Metadata["is_subtask"] != "true" {
		t.Errorf("expected is_subtask to be true on forwarded output, got: %q", m2.Metadata["is_subtask"])
	}
	if m2.Metadata["supervisor_bypass"] != "true" {
		t.Errorf("expected supervisor_bypass to be true on forwarded output, got: %q", m2.Metadata["supervisor_bypass"])
	}
	if m2.Content != "extracted hacker news top story" {
		t.Errorf("expected original content, got: %q", m2.Content)
	}
}

func tContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
