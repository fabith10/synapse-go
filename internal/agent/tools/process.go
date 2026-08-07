package agenttools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/fabith10/synapse-go/adk"
)

// GetInspectSystemProcessesTool returns a tool to list active system processes or signal/terminate specific processes.
func GetInspectSystemProcessesTool() adk.Tool {
	return adk.Tool{
		Name:        "inspect_system_processes",
		Description: "Inspects running system processes, CPU/RAM utilization, or signals/terminates background processes.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"action": map[string]interface{}{
					"type":        "string",
					"description": "Action: 'list', 'inspect', 'kill'",
				},
				"pid": map[string]interface{}{
					"type":        "integer",
					"description": "Process ID to inspect or kill",
				},
				"search": map[string]interface{}{
					"type":        "string",
					"description": "Optional search term to filter process name/command",
				},
			},
			"required": []interface{}{"action"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				Action string `json:"action"`
				PID    int    `json:"pid"`
				Search string `json:"search"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}

			action := strings.ToLower(strings.TrimSpace(params.Action))

			execCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			switch action {
			case "list", "inspect":
				goOS := runtime.GOOS
				var cmd *exec.Cmd

				if goOS == "windows" {
					cmd = exec.CommandContext(execCtx, "tasklist", "/FO", "CSV")
				} else {
					cmd = exec.CommandContext(execCtx, "ps", "-eo", "pid,user,%cpu,%mem,comm")
				}

				var outBuf bytes.Buffer
				cmd.Stdout = &outBuf
				if err := cmd.Run(); err != nil {
					return "", fmt.Errorf("process listing failed: %w", err)
				}

				lines := strings.Split(outBuf.String(), "\n")
				var processList []map[string]string

				for idx, line := range lines {
					if idx == 0 || strings.TrimSpace(line) == "" {
						continue
					}

					fields := strings.Fields(line)
					if len(fields) >= 5 {
						pidStr := fields[0]
						userStr := fields[1]
						cpuStr := fields[2]
						memStr := fields[3]
						commStr := strings.Join(fields[4:], " ")

						if params.PID > 0 && pidStr != strconv.Itoa(params.PID) {
							continue
						}

						if params.Search != "" && !strings.Contains(strings.ToLower(commStr), strings.ToLower(params.Search)) {
							continue
						}

						processList = append(processList, map[string]string{
							"pid":     pidStr,
							"user":    userStr,
							"cpu_pct": cpuStr,
							"mem_pct": memStr,
							"command": commStr,
						})
					}
				}

				if len(processList) > 50 {
					processList = processList[:50]
				}

				outBytes, _ := json.MarshalIndent(map[string]interface{}{
					"os":             goOS,
					"process_count":  len(processList),
					"processes":      processList,
				}, "", "  ")
				return string(outBytes), nil

			case "kill":
				if params.PID <= 0 {
					return "", fmt.Errorf("pid parameter is required for kill action")
				}

				// Prevent killing our own process or PID 1
				if params.PID == os.Getpid() || params.PID == 1 {
					return "", fmt.Errorf("cannot kill self PID %d or init PID 1", params.PID)
				}

				var cmd *exec.Cmd
				if runtime.GOOS == "windows" {
					cmd = exec.CommandContext(execCtx, "taskkill", "/F", "/PID", strconv.Itoa(params.PID))
				} else {
					cmd = exec.CommandContext(execCtx, "kill", "-9", strconv.Itoa(params.PID))
				}

				if err := cmd.Run(); err != nil {
					return "", fmt.Errorf("failed to kill PID %d: %w", params.PID, err)
				}

				return fmt.Sprintf("Successfully terminated process PID %d", params.PID), nil

			default:
				return "", fmt.Errorf("unsupported action %q. Use list, inspect, kill", action)
			}
		},
	}
}
