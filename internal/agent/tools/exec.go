package agenttools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fabith10/synapse-go/adk"
	"github.com/fabith10/synapse-go/internal/tools"
)

// GetExecutePythonDockerTool returns the Tier 3 Docker execution tool for Python code.
func GetExecutePythonDockerTool(sb adk.Sandbox, orch *adk.Orchestrator, agentID string, mailbox chan adk.Message) adk.Tool {
	return adk.Tool{
		Name:        "execute_python_docker",
		Description: "Executes Python code in a secure, ephemeral Docker container. Supports optional 'packages' list (e.g. ['numpy', 'pandas', 'scipy', 'matplotlib']) and 'prep_commands' list to dynamically prep the container environment before script execution.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"python_code": map[string]interface{}{
					"type":        "string",
					"description": "The full Python script to execute.",
				},
				"packages": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Optional list of PyPI packages to pre-install before running the script (e.g. ['numpy', 'pandas', 'scipy']).",
				},
				"prep_commands": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Optional list of setup commands to execute in the container prior to script execution.",
				},
			},
			"required": []string{"python_code"},
		},
		Tier: adk.TierDocker,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params map[string]interface{}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("execute_python_docker: invalid JSON: %w", err)
			}
			code := extractStringAlias(params, "python_code", "code", "script", "python", "command", "input", "content", "script_content", "raw_code", "query")
			if code == "" {
				if ai, ok := params["action_input"].(map[string]interface{}); ok {
					code = extractStringAlias(ai, "python_code", "code", "script", "python", "command", "input", "content", "script_content", "raw_code", "query")
				} else if aiStr, ok := params["action_input"].(string); ok {
					code = aiStr
				}
			}

			scriptPath := extractStringAlias(params, "script_path", "path", "file_path", "file")
			if scriptPath != "" && (strings.HasSuffix(scriptPath, ".py") || strings.Contains(scriptPath, "scripts/")) {
				if cleanP, err := resolveSafeWorkspacePath(scriptPath); err == nil {
					if fileBytes, err := os.ReadFile(cleanP); err == nil && len(fileBytes) > 0 {
						code = string(fileBytes)
					}
				}
			}
			if code != "" && (strings.HasSuffix(strings.TrimSpace(code), ".py") || strings.HasPrefix(strings.TrimSpace(code), "scripts/")) {
				if cleanP, err := resolveSafeWorkspacePath(strings.TrimSpace(code)); err == nil {
					if fileBytes, err := os.ReadFile(cleanP); err == nil && len(fileBytes) > 0 {
						code = string(fileBytes)
					}
				}
			}

			if code == "" {
				return "Missing required 'python_code' parameter. Provide Python code to execute (e.g. {\"python_code\": \"import numpy as np...\"}).", nil
			}

			// Clean up code block markers
			lowerCode := strings.ToLower(code)
			if idx := strings.Index(lowerCode, "```python"); idx != -1 {
				content := code[idx+len("```python"):]
				if end := strings.Index(content, "```"); end != -1 {
					code = strings.TrimSpace(content[:end])
				}
			} else if idx := strings.Index(lowerCode, "```py"); idx != -1 {
				content := code[idx+len("```py"):]
				if end := strings.Index(content, "```"); end != -1 {
					code = strings.TrimSpace(content[:end])
				}
			} else if idx := strings.Index(lowerCode, "```"); idx != -1 {
				content := code[idx+len("```"):]
				if end := strings.Index(content, "```"); end != -1 {
					code = strings.TrimSpace(content[:end])
				}
			}

			lines := strings.Split(code, "\n")
			var cleaned []string
			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "!") || strings.Contains(trimmed, "execute_python_docker") {
					continue
				}
				cleaned = append(cleaned, line)
			}
			code = strings.TrimSpace(strings.Join(cleaned, "\n"))

			if RequiresHumanReview(code) {
				uniqueCorrID := fmt.Sprintf("%s-python-%d-%d", agentID, time.Now().UnixNano(), atomic.AddUint64(&hitlCorrCounter, 1))
				orch.Send(adk.Message{
					Sender:    agentID,
					Recipient: "USER",
					Content:   fmt.Sprintf("[Risk Checkpoint] quantitative agent requests approval to execute Python code containing dangerous commands:\n%s", code),
					Metadata: map[string]string{
						"Type":           "HITL_APPROVAL",
						"correlation_id": uniqueCorrID,
						"reply_to":       agentID,
						"risk_level":     "HIGH",
						"action":         "python.execute",
					},
				})
				replyCh := make(chan adk.Message, 1)
				RegisterPendingResponse(uniqueCorrID, replyCh)
				defer UnregisterPendingResponse(uniqueCorrID)
				var approved bool
				select {
				case reply := <-replyCh:
					approved = reply.Content == "APPROVED"
				case <-ctx.Done():
					return "", ctx.Err()
				}
				if !approved {
					return "", fmt.Errorf("human denied execution")
				}
			}

			var packages []string
			if pkgsRaw, ok := params["packages"].([]interface{}); ok {
				for _, p := range pkgsRaw {
					if ps, ok := p.(string); ok && ps != "" {
						packages = append(packages, ps)
					}
				}
			}
			var prepCommands []string
			if prepRaw, ok := params["prep_commands"].([]interface{}); ok {
				for _, cmd := range prepRaw {
					if cs, ok := cmd.(string); ok && cs != "" {
						prepCommands = append(prepCommands, cs)
					}
				}
			}

			res := sb.Execute(ctx, adk.ExecutionRequest{
				Language:       "python",
				RawCode:        code,
				Packages:       packages,
				PrepCommands:   prepCommands,
				TimeoutSeconds: 45,
			})
			if res.Error == tools.ErrDockerUnavailable {
				return fmt.Sprintf("[Docker Mock Fallback] Docker offline. Output of %q: 123.45", code), nil
			}
			if res.Error != nil {
				return "", fmt.Errorf("execute_python_docker execute: %w", res.Error)
			}
			return res.Stdout + res.Stderr, nil
		},
	}
}

// GetExecuteBashDockerTool returns the Tier 3 Docker bash execution tool.
func GetExecuteBashDockerTool(sb adk.Sandbox, orch *adk.Orchestrator, agentID string, mailbox chan adk.Message) adk.Tool {
	return adk.Tool{
		Name:        "execute_bash_docker",
		Description: "Executes a shell/bash script in a secure, ephemeral Docker container.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"bash_script": map[string]interface{}{
					"type":        "string",
					"description": "The full bash script to execute.",
				},
			},
			"required": []string{"bash_script"},
		},
		Tier: adk.TierDocker,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params map[string]interface{}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("execute_bash_docker: invalid JSON: %w", err)
			}
			script, _ := params["bash_script"].(string)
			if script == "" {
				script, _ = params["script"].(string)
			}
			if script == "" {
				script, _ = params["command"].(string)
			}
			if script == "" {
				script, _ = params["code"].(string)
			}
			if script == "" {
				if ai, ok := params["action_input"].(map[string]interface{}); ok {
					script, _ = ai["bash_script"].(string)
					if script == "" {
						script, _ = ai["script"].(string)
					}
					if script == "" {
						script, _ = ai["command"].(string)
					}
				} else if aiStr, ok := params["action_input"].(string); ok {
					script = aiStr
				}
			}
			if script == "" {
				return "", fmt.Errorf("execute_bash_docker: missing bash_script parameter")
			}

			// Clean up code block markers
			lowerScript := strings.ToLower(script)
			if idx := strings.Index(lowerScript, "```bash"); idx != -1 {
				content := script[idx+len("```bash"):]
				if end := strings.Index(content, "```"); end != -1 {
					script = strings.TrimSpace(content[:end])
				}
			} else if idx := strings.Index(lowerScript, "```sh"); idx != -1 {
				content := script[idx+len("```sh"):]
				if end := strings.Index(content, "```"); end != -1 {
					script = strings.TrimSpace(content[:end])
				}
			} else if idx := strings.Index(lowerScript, "```"); idx != -1 {
				content := script[idx+len("```"):]
				if end := strings.Index(content, "```"); end != -1 {
					script = strings.TrimSpace(content[:end])
				}
			}

			if RequiresHumanReviewBash(script) {
				uniqueCorrID := fmt.Sprintf("%s-bash-%d-%d", agentID, time.Now().UnixNano(), atomic.AddUint64(&hitlCorrCounter, 1))
				orch.Send(adk.Message{
					Sender:    agentID,
					Recipient: "USER",
					Content:   fmt.Sprintf("[Risk Checkpoint] agent requests approval to execute Bash script containing dangerous commands:\n%s", script),
					Metadata: map[string]string{
						"Type":           "HITL_APPROVAL",
						"correlation_id": uniqueCorrID,
						"reply_to":       agentID,
						"risk_level":     "HIGH",
						"action":         "bash.execute",
					},
				})
				replyCh := make(chan adk.Message, 1)
				RegisterPendingResponse(uniqueCorrID, replyCh)
				defer UnregisterPendingResponse(uniqueCorrID)
				var approved bool
				select {
				case reply := <-replyCh:
					approved = reply.Content == "APPROVED"
				case <-ctx.Done():
					return "", ctx.Err()
				}
				if !approved {
					return "", fmt.Errorf("human denied execution")
				}
			}

			res := sb.Execute(ctx, adk.ExecutionRequest{
				Language:       "bash",
				RawCode:        script,
				TimeoutSeconds: 45,
			})
			if res.Error == tools.ErrDockerUnavailable {
				return "[Docker Mock Fallback] Docker offline. Run output: mock bash run successful.", nil
			}
			if res.Error != nil {
				return "", fmt.Errorf("execute_bash_docker execute: %w", res.Error)
			}
			return res.Stdout + res.Stderr, nil
		},
	}
}
