package dag

import (
	"context"
	"strings"
	"testing"
)

func TestDAGGraphExecution(t *testing.T) {
	g := NewGraph()

	g.AddNode("node1", "agent-1", func(ctx context.Context, s *State) (string, error) {
		s.Set("count", 1)
		return "node1_output", nil
	})

	g.AddNode("node2", "agent-2", func(ctx context.Context, s *State) (string, error) {
		val, _ := s.Get("count")
		if count, ok := val.(int); ok {
			s.Set("count", count+1)
		}
		return "node2_output", nil
	})

	g.AddNode("finishNode", "agent-3", func(ctx context.Context, s *State) (string, error) {
		val, _ := s.Get("count")
		return strings.Repeat("done", val.(int)), nil
	})

	g.SetEntryPoint("node1")
	g.AddEdge("node1", "node2")
	g.AddConditionalEdge("node2", func(s *State) string {
		val, _ := s.Get("count")
		if val.(int) >= 2 {
			return "finishNode"
		}
		return "node1"
	})
	g.SetFinishPoint("finishNode")

	state := NewState("test-workflow-123")
	finalState, err := g.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected DAG execution error: %v", err)
	}

	if !finalState.Completed {
		t.Errorf("expected state.Completed to be true")
	}

	if finalState.NodeOutputs["finishNode"] != "donedone" {
		t.Errorf("expected finishNode output 'donedone', got %q", finalState.NodeOutputs["finishNode"])
	}
}
