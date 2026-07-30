package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fabith10/synapse-go/adk"
	"github.com/fabith10/synapse-go/pkg/logger"
)

// Schedule represents a recurring agent task that fires on a cron expression.
type Schedule struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Cron    string `json:"cron"`    // 5-field cron: "min hour dom mon dow"
	Task    string `json:"task"`    // Task description sent to triage-agent on each fire
	Enabled bool   `json:"enabled"`
	Created string `json:"created"`
}

type scheduleFile struct {
	Schedules []*Schedule `json:"schedules"`
}

// CronScheduler manages recurring agent tasks using native Go timers.
// It has no external dependencies — the cron parser and timer management
// are implemented from scratch for full control.
type CronScheduler struct {
	mu        sync.Mutex
	schedules map[string]*Schedule
	orch      *adk.Orchestrator
	timers    map[string]*time.Timer
	path      string // path to schedules.json
	ctx       context.Context
	logFn     func(sender, recipient, content string) // SSE log callback
}

// GlobalScheduler is the package-level singleton created by Bootstrap.
// Agent tools reference it to add/remove/list schedules.
var GlobalScheduler *CronScheduler

// NewCronScheduler creates a CronScheduler wired to the given orchestrator.
func NewCronScheduler(orch *adk.Orchestrator, path string) *CronScheduler {
	cs := &CronScheduler{
		schedules: make(map[string]*Schedule),
		orch:      orch,
		timers:    make(map[string]*time.Timer),
		path:      path,
	}
	GlobalScheduler = cs
	return cs
}

// SetLogger registers an SSE broadcast callback called whenever a schedule fires.
func (cs *CronScheduler) SetLogger(fn func(sender, recipient, content string)) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.logFn = fn
}

// Start loads persisted schedules from disk and arms all enabled ones.
// Must be called after the orchestrator is started.
func (cs *CronScheduler) Start(ctx context.Context) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.ctx = ctx
	cs.loadLocked()
	for _, s := range cs.schedules {
		if s.Enabled {
			cs.armLocked(s)
		}
	}
	logger.WithComponent("scheduler").Info("CronScheduler started", "schedule_count", len(cs.schedules), "path", cs.path)
}

// Stop cancels all pending timers cleanly.
func (cs *CronScheduler) Stop() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for _, t := range cs.timers {
		t.Stop()
	}
	cs.timers = make(map[string]*time.Timer)
}

// AddSchedule registers a new schedule, persists it, and arms the timer.
func (cs *CronScheduler) AddSchedule(s *Schedule) error {
	if _, err := nextFireTime(s.Cron, time.Now()); err != nil {
		return fmt.Errorf("invalid cron expression %q: %w", s.Cron, err)
	}
	if s.ID == "" {
		s.ID = fmt.Sprintf("sched-%d", time.Now().UnixNano())
	}
	if s.Created == "" {
		s.Created = time.Now().Format(time.RFC3339)
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.schedules[s.ID] = s
	if err := cs.saveLocked(); err != nil {
		return err
	}
	if s.Enabled {
		cs.armLocked(s)
	}
	logger.WithComponent("scheduler").Info("Added schedule", "name", s.Name, "cron", s.Cron, "task", s.Task)
	return nil
}

// RemoveSchedule cancels and deletes a schedule by ID.
func (cs *CronScheduler) RemoveSchedule(id string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if t, ok := cs.timers[id]; ok {
		t.Stop()
		delete(cs.timers, id)
	}
	name := ""
	if s, ok := cs.schedules[id]; ok {
		name = s.Name
	}
	delete(cs.schedules, id)
	if err := cs.saveLocked(); err != nil {
		return err
	}
	logger.WithComponent("scheduler").Info("Removed schedule", "name", name, "id", id)
	return nil
}

// ListSchedules returns a snapshot of all schedules with computed next-fire times.
func (cs *CronScheduler) ListSchedules() ([]*Schedule, map[string]time.Time) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	out := make([]*Schedule, 0, len(cs.schedules))
	nextFires := make(map[string]time.Time)
	for _, s := range cs.schedules {
		sCopy := *s
		out = append(out, &sCopy)
		if s.Enabled {
			if t, err := nextFireTime(s.Cron, time.Now()); err == nil {
				nextFires[s.ID] = t
			}
		}
	}
	return out, nextFires
}

// armLocked computes the next fire time and sets a time.AfterFunc timer.
// Must be called with cs.mu held.
func (cs *CronScheduler) armLocked(s *Schedule) {
	next, err := nextFireTime(s.Cron, time.Now())
	if err != nil {
		logger.WithComponent("scheduler").Error("Cannot arm schedule", "name", s.Name, "error", err)
		return
	}
	delay := time.Until(next)
	id, name, task := s.ID, s.Name, s.Task
	ctx := cs.ctx

	timer := time.AfterFunc(delay, func() {
		if ctx != nil {
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
		cs.fire(id, name, task)
	})
	if old, ok := cs.timers[id]; ok {
		old.Stop()
	}
	cs.timers[id] = timer
}

// fire dispatches the scheduled task to triage-agent and re-arms for next occurrence.
func (cs *CronScheduler) fire(id, name, task string) {
	corrID := fmt.Sprintf("sched-%s-%d", id, time.Now().UnixNano())
	cs.mu.Lock()
	logFn := cs.logFn
	cs.mu.Unlock()

	if logFn != nil {
		logFn("SCHEDULER", "triage-agent", fmt.Sprintf("[Cron: %s] %s", name, task))
	}
	cs.orch.Send(adk.Message{
		Sender:    "SCHEDULER",
		Recipient: "triage-agent",
		Content:   task,
		Metadata: map[string]string{
			"correlation_id": corrID,
			"schedule_id":    id,
			"schedule_name":  name,
		},
	})

	// Re-arm for next occurrence.
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if sched, ok := cs.schedules[id]; ok && sched.Enabled {
		cs.armLocked(sched)
	}
}

// loadLocked reads schedules.json into cs.schedules. Must hold cs.mu.
func (cs *CronScheduler) loadLocked() {
	data, err := os.ReadFile(cs.path)
	if err != nil {
		return // no file yet, start fresh
	}
	var f scheduleFile
	if err := json.Unmarshal(data, &f); err != nil {
		logger.WithComponent("scheduler").Error("Failed to parse schedule file", "path", cs.path, "error", err)
		return
	}
	for _, s := range f.Schedules {
		cs.schedules[s.ID] = s
	}
}

// saveLocked writes cs.schedules to disk atomically. Must hold cs.mu.
func (cs *CronScheduler) saveLocked() error {
	list := make([]*Schedule, 0, len(cs.schedules))
	for _, s := range cs.schedules {
		list = append(list, s)
	}
	f := scheduleFile{Schedules: list}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cs.path, data, 0644)
}

// ---------------------------------------------------------------------------
// Minimal 5-field cron parser — no external dependency
// Fields: minute(0-59) hour(0-23) dom(1-31) month(1-12) dow(0-6, Sun=0)
// Supports: * */n n n,m n-m
// ---------------------------------------------------------------------------

func parseCronField(field string, min, max int) (map[int]struct{}, error) {
	result := make(map[int]struct{})

	if field == "*" {
		for i := min; i <= max; i++ {
			result[i] = struct{}{}
		}
		return result, nil
	}

	if strings.HasPrefix(field, "*/") {
		step, err := strconv.Atoi(field[2:])
		if err != nil || step <= 0 {
			return nil, fmt.Errorf("invalid step in %q", field)
		}
		for i := min; i <= max; i += step {
			result[i] = struct{}{}
		}
		return result, nil
	}

	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			lo, e1 := strconv.Atoi(bounds[0])
			hi, e2 := strconv.Atoi(bounds[1])
			if e1 != nil || e2 != nil || lo > hi || lo < min || hi > max {
				return nil, fmt.Errorf("invalid range %q (valid: %d-%d)", part, min, max)
			}
			for i := lo; i <= hi; i++ {
				result[i] = struct{}{}
			}
		} else {
			n, err := strconv.Atoi(part)
			if err != nil || n < min || n > max {
				return nil, fmt.Errorf("invalid value %q (valid: %d-%d)", part, min, max)
			}
			result[n] = struct{}{}
		}
	}
	return result, nil
}

// nextFireTime returns the earliest time after 'from' matching the cron expression.
func nextFireTime(cronExpr string, from time.Time) (time.Time, error) {
	if strings.HasPrefix(cronExpr, "@every ") {
		dur, err := time.ParseDuration(strings.TrimSpace(strings.TrimPrefix(cronExpr, "@every ")))
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid @every expression %q: %w", cronExpr, err)
		}
		return from.Add(dur), nil
	}

	fields := strings.Fields(cronExpr)
	if len(fields) == 6 {
		fields = fields[1:]
	}
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("cron must have 5 fields (got %d): %q", len(fields), cronExpr)
	}

	mins, err := parseCronField(fields[0], 0, 59)
	if err != nil {
		return time.Time{}, fmt.Errorf("minute field: %w", err)
	}
	hrs, err := parseCronField(fields[1], 0, 23)
	if err != nil {
		return time.Time{}, fmt.Errorf("hour field: %w", err)
	}
	doms, err := parseCronField(fields[2], 1, 31)
	if err != nil {
		return time.Time{}, fmt.Errorf("dom field: %w", err)
	}
	months, err := parseCronField(fields[3], 1, 12)
	if err != nil {
		return time.Time{}, fmt.Errorf("month field: %w", err)
	}
	dows, err := parseCronField(fields[4], 0, 6)
	if err != nil {
		return time.Time{}, fmt.Errorf("dow field: %w", err)
	}

	// Scan minute-by-minute up to 4 years.
	t := from.Add(time.Minute).Truncate(time.Minute)
	limit := t.Add(4 * 366 * 24 * time.Hour)
	for t.Before(limit) {
		_, okMonth := months[int(t.Month())]
		_, okDom := doms[t.Day()]
		_, okDow := dows[int(t.Weekday())]
		_, okHour := hrs[t.Hour()]
		_, okMin := mins[t.Minute()]
		if okMonth && okDom && okDow && okHour && okMin {
			return t, nil
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("no matching fire time within 4 years for %q", cronExpr)
}

// HumanDuration formats a duration for UI display (e.g., "2h 35m", "45s").
func HumanDuration(d time.Duration) string {
	if d <= 0 {
		return "now"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}
