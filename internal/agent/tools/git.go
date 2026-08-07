package agenttools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fabith10/synapse-go/adk"
)

// GetGitOperationsTool returns a tool to perform git status, diff, log, commit, branch, and checkout.
func GetGitOperationsTool() adk.Tool {
	return adk.Tool{
		Name:        "git_operations",
		Description: "Executes git repository operations: status, diff, log, commit, branch, checkout, or add. Safe execution on target workspace.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"operation": map[string]interface{}{
					"type":        "string",
					"description": "Git subcommand: 'status', 'diff', 'log', 'commit', 'branch', 'checkout', 'add'",
				},
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Target repository directory (defaults to current workspace)",
				},
				"target": map[string]interface{}{
					"type":        "string",
					"description": "File path, branch name, or commit reference",
				},
				"message": map[string]interface{}{
					"type":        "string",
					"description": "Commit message (required for 'commit')",
				},
			},
			"required": []interface{}{"operation"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				Operation string `json:"operation"`
				Path      string `json:"path"`
				Target    string `json:"target"`
				Message   string `json:"message"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}

			repoDir := params.Path
			if root := ctx.Value(WorkspaceRootKey); root != nil {
				if rStr, ok := root.(string); ok && rStr != "" && repoDir == "" {
					repoDir = rStr
				}
			}
			if repoDir == "" {
				wd, _ := os.Getwd()
				repoDir = wd
			}
			repoDir = filepath.Clean(repoDir)

			op := strings.ToLower(strings.TrimSpace(params.Operation))
			var gitArgs []string

			switch op {
			case "status":
				gitArgs = []string{"status", "--short"}
			case "diff":
				gitArgs = []string{"diff"}
				if params.Target != "" {
					gitArgs = append(gitArgs, params.Target)
				}
			case "log":
				gitArgs = []string{"log", "-n", "10", "--oneline"}
				if params.Target != "" {
					gitArgs = append(gitArgs, params.Target)
				}
			case "branch":
				gitArgs = []string{"branch", "-a"}
			case "checkout":
				if params.Target == "" {
					return "", fmt.Errorf("'checkout' operation requires 'target' branch or commit")
				}
				gitArgs = []string{"checkout", params.Target}
			case "add":
				target := params.Target
				if target == "" {
					target = "."
				}
				gitArgs = []string{"add", target}
			case "commit":
				if params.Message == "" {
					return "", fmt.Errorf("'commit' operation requires 'message'")
				}
				gitArgs = []string{"commit", "-m", params.Message}
			case "show":
				gitArgs = []string{"show", "--stat"}
				if params.Target != "" {
					gitArgs = append(gitArgs, params.Target)
				}
			case "stash":
				target := params.Target
				if target == "" {
					target = "list"
				}
				gitArgs = []string{"stash", target}
			default:
				return "", fmt.Errorf("unsupported git operation %q. Use status, diff, log, show, branch, checkout, add, commit, stash", op)
			}

			execCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()

			cmd := exec.CommandContext(execCtx, "git", gitArgs...)
			cmd.Dir = repoDir

			var outBuf, errBuf bytes.Buffer
			cmd.Stdout = &outBuf
			cmd.Stderr = &errBuf

			err := cmd.Run()
			output := strings.TrimSpace(outBuf.String())
			errStr := strings.TrimSpace(errBuf.String())

			if err != nil {
				if errStr != "" {
					return "", fmt.Errorf("git %s failed: %s (%w)", op, errStr, err)
				}
				return "", fmt.Errorf("git %s failed: %w", op, err)
			}

			if output == "" {
				if errStr != "" {
					output = errStr
				} else {
					output = fmt.Sprintf("git %s completed cleanly (no output)", op)
				}
			}

			return output, nil
		},
	}
}
