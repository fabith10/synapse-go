package agenttools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/fabith10/synapse-go/adk"
	"github.com/fabith10/synapse-go/pkg/logger"
)

// GetScheduleTaskTool returns a tool to schedule tasks.
func GetScheduleTaskTool() adk.Tool {
	return adk.Tool{
		Name:        "schedule_task",
		Description: "Schedules a recurring or delayed task via a 5-field cron expression. Returns the schedule ID.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "A human-readable name/label for the schedule.",
				},
				"cron": map[string]interface{}{
					"type":        "string",
					"description": "5-field cron pattern (e.g. '0 9 * * 1-5' for weekdays at 9am, or '*/5 * * * *' for every 5 minutes).",
				},
				"task": map[string]interface{}{
					"type":        "string",
					"description": "The exact user task description to dispatch to triage-agent when the cron triggers.",
				},
			},
			"required": []string{"name", "cron", "task"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				Name string `json:"name"`
				Cron string `json:"cron"`
				Task string `json:"task"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("failed to parse arguments: %w", err)
			}
			if GlobalScheduler == nil {
				return "", fmt.Errorf("scheduler is not initialized")
			}
			s := &Schedule{
				Name:    params.Name,
				Cron:    params.Cron,
				Task:    params.Task,
				Enabled: true,
			}
			if err := GlobalScheduler.AddSchedule(s); err != nil {
				return "", err
			}
			return fmt.Sprintf("Successfully scheduled task %q (ID: %s, Cron: %s)", s.Name, s.ID, s.Cron), nil
		},
	}
}

// GetListSchedulesTool returns a tool to view all active schedules.
func GetListSchedulesTool() adk.Tool {
	return adk.Tool{
		Name:        "list_schedules",
		Description: "Lists all active cron schedules.",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			if GlobalScheduler == nil {
				return "", fmt.Errorf("scheduler is not initialized")
			}
			scheds, nextTimes := GlobalScheduler.ListSchedules()
			type item struct {
				Schedule
				NextFire string `json:"next_fire"`
			}
			var items []item
			for _, s := range scheds {
				nf := "disabled"
				if t, ok := nextTimes[s.ID]; ok {
					nf = t.Format(time.RFC3339)
				}
				items = append(items, item{Schedule: *s, NextFire: nf})
			}
			out, err := json.MarshalIndent(items, "", "  ")
			if err != nil {
				return "", err
			}
			return string(out), nil
		},
	}
}

// GetCancelScheduleTool returns a tool to cancel/delete a schedule.
func GetCancelScheduleTool() adk.Tool {
	return adk.Tool{
		Name:        "cancel_schedule",
		Description: "Cancels and deletes a registered schedule by its ID.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"schedule_id": map[string]interface{}{
					"type":        "string",
					"description": "The unique ID of the schedule to cancel.",
				},
			},
			"required": []string{"schedule_id"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var raw map[string]interface{}
			_ = json.Unmarshal(args, &raw)
			scheduleID := extractStringAlias(raw, "schedule_id", "id", "name")
			if scheduleID == "" {
				return "", fmt.Errorf("cancel_schedule: missing required parameter 'schedule_id'")
			}
			if GlobalScheduler == nil {
				return "", fmt.Errorf("scheduler is not initialized")
			}
			if err := GlobalScheduler.RemoveSchedule(scheduleID); err != nil {
				return "", err
			}
			return fmt.Sprintf("Successfully cancelled schedule %s", scheduleID), nil
		},
	}
}

// GetWaitSecondsTool returns a native tool allowing agents to pause execution for a specified number of seconds.
func GetWaitSecondsTool() adk.Tool {
	return adk.Tool{
		Name:        "wait_seconds",
		Description: "Pauses execution for a specified number of seconds (1 to 60 seconds) to wait for background tasks, scheduled jobs, external processes, or asynchronous operations to complete.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"seconds": map[string]interface{}{
					"type":        "number",
					"description": "Number of seconds to pause execution (1 to 60 seconds).",
				},
				"reason": map[string]interface{}{
					"type":        "string",
					"description": "Optional reason for waiting (e.g. 'Waiting for scheduled job to write output file').",
				},
			},
			"required": []string{"seconds"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var raw map[string]interface{}
			_ = json.Unmarshal(args, &raw)

			secVal := extractFloatAlias(raw, "seconds", "sec", "duration", "wait", "delay", "time")
			if secVal <= 0 {
				secVal = 3.0
			}
			if secVal > 60 {
				secVal = 60
			}
			reason := extractStringAlias(raw, "reason", "purpose", "why", "message")
			if reason == "" {
				reason = "asynchronous process completion"
			}

			logger.Info("Agent pausing execution", "seconds", secVal, "reason", reason)

			select {
			case <-time.After(time.Duration(secVal * float64(time.Second))):
				return fmt.Sprintf("Finished waiting for %.1f seconds (%s). Proceeding with next step.", secVal, reason), nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		},
	}
}
