package dag

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/fabith10/synapse-go/internal/memory"
	syslog "github.com/fabith10/synapse-go/pkg/logger"
)

// State represents the runtime context passed between nodes in a DAG workflow graph.
type State struct {
	mu           sync.RWMutex
	WorkflowID   string                 `json:"workflow_id"`
	CurrentNode  string                 `json:"current_node"`
	Data         map[string]interface{} `json:"data"`
	Completed    bool                   `json:"completed"`
	NodeOutputs  map[string]string      `json:"node_outputs"`
	Error        string                 `json:"error,omitempty"`
}

// NewState constructs a initialized State instance.
func NewState(workflowID string) *State {
	return &State{
		WorkflowID:  workflowID,
		Data:        make(map[string]interface{}),
		NodeOutputs: make(map[string]string),
	}
}

func (s *State) Get(key string) (interface{}, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.Data[key]
	return val, ok
}

func (s *State) Set(key string, val interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Data == nil {
		s.Data = make(map[string]interface{})
	}
	s.Data[key] = val
}

// NodeFunc represents a execution function associated with a DAG graph node.
type NodeFunc func(ctx context.Context, state *State) (string, error)

// ConditionFunc evaluates the state and returns the next target node ID.
type ConditionFunc func(state *State) string

// Node represents a step within the DAG workflow graph.
type Node struct {
	ID      string
	AgentID string
	Handler NodeFunc
}

// Graph represents a declarative multi-agent state machine and DAG workflow builder.
type Graph struct {
	nodes            map[string]*Node
	edges            map[string][]string
	conditionalEdges map[string]ConditionFunc
	entryNode        string
	finishNode       string
	checkpointStore  memory.CheckpointStore
}

// NewGraph constructs a empty Graph.
func NewGraph() *Graph {
	return &Graph{
		nodes:            make(map[string]*Node),
		edges:            make(map[string][]string),
		conditionalEdges: make(map[string]ConditionFunc),
	}
}

// SetCheckpointStore enables step-level SQLite persistence for pause & resume workflows.
func (g *Graph) SetCheckpointStore(store memory.CheckpointStore) {
	g.checkpointStore = store
}

// AddNode registers a step node in the graph with a custom handler or agent ID.
func (g *Graph) AddNode(id, agentID string, handler NodeFunc) {
	g.nodes[id] = &Node{
		ID:      id,
		AgentID: agentID,
		Handler: handler,
	}
}

// SetEntryPoint defines the starting node ID for workflow execution.
func (g *Graph) SetEntryPoint(nodeID string) {
	g.entryNode = nodeID
}

// SetFinishPoint defines the terminal node ID for workflow execution.
func (g *Graph) SetFinishPoint(nodeID string) {
	g.finishNode = nodeID
}

// AddEdge connects a source node to a target node.
func (g *Graph) AddEdge(fromNode, toNode string) {
	g.edges[fromNode] = append(g.edges[fromNode], toNode)
}

// AddConditionalEdge attaches a dynamic condition function to route from a node based on state.
func (g *Graph) AddConditionalEdge(fromNode string, condition ConditionFunc) {
	g.conditionalEdges[fromNode] = condition
}

// Execute runs the DAG workflow graph starting from entryPoint until reaching finishNode or completing.
func (g *Graph) Execute(ctx context.Context, state *State) (*State, error) {
	if g.entryNode == "" {
		return nil, fmt.Errorf("dag: entry point node is not set")
	}

	currentNodeID := g.entryNode
	if state.CurrentNode != "" {
		currentNodeID = state.CurrentNode // Resume from checkpoint if present
	}

	for currentNodeID != "" {
		select {
		case <-ctx.Done():
			return state, ctx.Err()
		default:
		}

		node, exists := g.nodes[currentNodeID]
		if !exists {
			return nil, fmt.Errorf("dag: node %q not found in graph", currentNodeID)
		}

		state.CurrentNode = currentNodeID
		syslog.Info("Executing DAG node", "workflow_id", state.WorkflowID, "node_id", currentNodeID)

		// Execute node handler
		if node.Handler != nil {
			output, err := node.Handler(ctx, state)
			if err != nil {
				state.Error = err.Error()
				g.saveCheckpoint(ctx, state)
				return state, fmt.Errorf("dag: node %q failed: %w", currentNodeID, err)
			}
			state.NodeOutputs[currentNodeID] = output
		}

		// Persist step checkpoint to SQLite if checkpoint store is attached
		g.saveCheckpoint(ctx, state)

		if currentNodeID == g.finishNode {
			state.Completed = true
			syslog.Info("DAG workflow completed successfully", "workflow_id", state.WorkflowID)
			break
		}

		// Determine next node: conditional edge takes precedence over static edge
		if condFn, ok := g.conditionalEdges[currentNodeID]; ok {
			currentNodeID = condFn(state)
		} else if targets, ok := g.edges[currentNodeID]; ok && len(targets) > 0 {
			currentNodeID = targets[0] // Primary next edge
		} else {
			// No outgoing edges
			state.Completed = true
			break
		}
	}

	return state, nil
}

func (g *Graph) saveCheckpoint(ctx context.Context, state *State) {
	if g.checkpointStore == nil {
		return
	}

	dataBytes, err := json.Marshal(state)
	if err != nil {
		return
	}

	_ = g.checkpointStore.SaveLongTermMemory(ctx, "dag-engine", state.WorkflowID+":"+state.CurrentNode, string(dataBytes), nil)
}
