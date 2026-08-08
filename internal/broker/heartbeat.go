package broker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/fabith10/synapse-go/internal/memory"
	"github.com/fabith10/synapse-go/pkg/logger"
)

// AutonomousHeartbeatEngine runs a background ticker loop to evaluate active goals
// and autonomously dispatch pending milestones to specialist agents.
type AutonomousHeartbeatEngine struct {
	mu         sync.Mutex
	store      memory.CheckpointStore
	interval   time.Duration
	dispatchFn func(ctx context.Context, agentID, taskPrompt string) error
	cancel     context.CancelFunc
	isRunning  bool
}

// NewAutonomousHeartbeatEngine initializes the autonomous goal steering ticker engine.
func NewAutonomousHeartbeatEngine(store memory.CheckpointStore, interval time.Duration, dispatchFn func(ctx context.Context, agentID, taskPrompt string) error) *AutonomousHeartbeatEngine {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &AutonomousHeartbeatEngine{
		store:      store,
		interval:   interval,
		dispatchFn: dispatchFn,
	}
}

// Start launches the autonomous heartbeat ticker in a background goroutine.
func (h *AutonomousHeartbeatEngine) Start() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.isRunning {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	h.isRunning = true

	go h.loop(ctx)
	logger.Info("Autonomous Heartbeat Engine started", "interval", h.interval)
}

// Stop gracefully terminates the background heartbeat ticker.
func (h *AutonomousHeartbeatEngine) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.isRunning {
		return
	}
	if h.cancel != nil {
		h.cancel()
	}
	h.isRunning = false
	logger.Info("Autonomous Heartbeat Engine stopped")
}

// TriggerOnce manually triggers a single heartbeat evaluation cycle (useful for tests or immediate triggers).
func (h *AutonomousHeartbeatEngine) TriggerOnce(ctx context.Context) error {
	return h.evaluateActiveGoals(ctx)
}

func (h *AutonomousHeartbeatEngine) loop(ctx context.Context) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := h.evaluateActiveGoals(ctx); err != nil {
				logger.Warn("Autonomous Heartbeat evaluation encountered error", "error", err)
			}
		}
	}
}

func (h *AutonomousHeartbeatEngine) evaluateActiveGoals(ctx context.Context) error {
	goals, err := h.store.ListActiveGoals(ctx)
	if err != nil {
		return fmt.Errorf("heartbeat list active goals: %w", err)
	}

	for _, g := range goals {
		for _, m := range g.Milestones {
			if m.Status == "PENDING" {
				targetAgent := m.AgentID
				if targetAgent == "" {
					targetAgent = "generalist-agent"
				}

				prompt := fmt.Sprintf("Autonomous Milestone Execution for Goal %q (ID: %s): %s", g.Title, g.ID, m.Title)
				logger.Info("Autonomous Heartbeat triggering milestone task", "goal", g.Title, "milestone", m.Title, "agent", targetAgent)

				// Update status to IN_PROGRESS
				_ = h.store.UpdateMilestoneStatus(ctx, m.ID, "IN_PROGRESS")

				if h.dispatchFn != nil {
					if err := h.dispatchFn(ctx, targetAgent, prompt); err != nil {
						logger.Warn("Failed to dispatch autonomous milestone task", "milestone_id", m.ID, "error", err)
						_ = h.store.UpdateMilestoneStatus(ctx, m.ID, "PENDING")
					} else {
						_ = h.store.UpdateMilestoneStatus(ctx, m.ID, "DONE")
					}
				}
				break // Process 1 milestone per goal per heartbeat tick
			}
		}
	}
	return nil
}
