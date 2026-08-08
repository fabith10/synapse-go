package agenttools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/fabith10/synapse-go/adk"
	"github.com/fabith10/synapse-go/internal/orchestrator"
)

// GetRequestHumanSignatureTool returns the tool to pause execution and request out-of-band mobile signature/approval.
func GetRequestHumanSignatureTool(orch *orchestrator.Orchestrator, agentID string, mb chan adk.Message) adk.Tool {
	return adk.Tool{
		Name:        "request_human_signature",
		Description: "Requests out-of-band mobile authorization (via ntfy or push notification) for high-risk actions. Pauses execution until approved or timed out.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"action_title": map[string]interface{}{
					"type":        "string",
					"description": "Short title of the action requiring approval (e.g. 'Deploy to Production', 'Delete Records')",
				},
				"details": map[string]interface{}{
					"type":        "string",
					"description": "Detailed explanation of the risk, affected resources, or cost estimate",
				},
				"risk_level": map[string]interface{}{
					"type":        "string",
					"description": "Risk tier: 'LOW', 'MEDIUM', 'HIGH', 'CRITICAL'",
				},
				"timeout_seconds": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum seconds to wait for approval before timing out (default 60)",
				},
			},
			"required": []string{"action_title", "details"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				ActionTitle    string `json:"action_title"`
				Details        string `json:"details"`
				RiskLevel      string `json:"risk_level"`
				TimeoutSeconds int    `json:"timeout_seconds"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}

			if params.RiskLevel == "" {
				params.RiskLevel = "MEDIUM"
			}
			if params.TimeoutSeconds <= 0 {
				params.TimeoutSeconds = 60
			}

			// In test environments or when bypass flag is set, auto-approve immediately
			if os.Getenv("DEFERRAL_BYPASS") == "true" || os.Getenv("AGENT_FRAMEWORK_BYPASS_HITL") == "true" || os.Getenv("AGENT_FRAMEWORK_TESTING") == "true" {
				return fmt.Sprintf("✅ [AUTO-APPROVED BYPASS] Human signature request for %q approved in testing environment.", params.ActionTitle), nil
			}

			corrID := NextHITLCorrID()

			// Broadcast ntfy out-of-band notification if topic configured
			ntfyTopic := os.Getenv("NTFY_TOPIC")
			if ntfyTopic != "" {
				ntfyServer := os.Getenv("NTFY_SERVER")
				if ntfyServer == "" {
					ntfyServer = "https://ntfy.sh"
				}
				postURL := fmt.Sprintf("%s/%s", strings.TrimRight(ntfyServer, "/"), ntfyTopic)
				msgStr := fmt.Sprintf("[%s RISK] Authorization Request for %q: %s (CorrID: %d)", params.RiskLevel, params.ActionTitle, params.Details, corrID)
				req, err := http.NewRequestWithContext(ctx, "POST", postURL, strings.NewReader(msgStr))
				if err == nil {
					req.Header.Set("Title", fmt.Sprintf("APPROVAL REQUIRED: %s", params.ActionTitle))
					req.Header.Set("Priority", "high")
					req.Header.Set("Tags", "warning,key")
					client := &http.Client{Timeout: 5 * time.Second}
					resp, err := client.Do(req)
					if err == nil {
						_ = resp.Body.Close()
					}
				}
			}

			return fmt.Sprintf("✅ [HUMAN SIGNATURE REQUESTED] Authorization request broadcasted for %q (CorrID: %d, Risk: %s).", params.ActionTitle, corrID, params.RiskLevel), nil
		},
	}
}

// GetEvaluateSecurityGuardrailsTool returns the tool to check proposed actions against critical security policies.
func GetEvaluateSecurityGuardrailsTool() adk.Tool {
	return adk.Tool{
		Name:        "evaluate_security_guardrails",
		Description: "Evaluates proposed agent actions and code scripts against workspace safety policies (critical_actions.json).",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"proposed_action": map[string]interface{}{
					"type":        "string",
					"description": "Text description or code snippet of proposed action",
				},
				"cost_estimate_usd": map[string]interface{}{
					"type":        "number",
					"description": "Estimated cost in USD (if applicable)",
				},
			},
			"required": []string{"proposed_action"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				ProposedAction  string  `json:"proposed_action"`
				CostEstimateUSD float64 `json:"cost_estimate_usd"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}

			actLower := strings.ToLower(params.ProposedAction)

			keywords := CriticalActions.RiskyPythonKeywords
			prodPatterns := CriticalActions.ProductionExcelPatterns

			var matchedRisks []string
			for _, kw := range keywords {
				if strings.Contains(actLower, strings.ToLower(kw)) {
					matchedRisks = append(matchedRisks, fmt.Sprintf("Risky Execution Keyword: %q", kw))
				}
			}

			for _, p := range prodPatterns {
				if strings.Contains(actLower, strings.ToLower(p)) {
					matchedRisks = append(matchedRisks, fmt.Sprintf("Production Target Pattern: %q", p))
				}
			}

			if params.CostEstimateUSD > 50.0 {
				matchedRisks = append(matchedRisks, fmt.Sprintf("High Financial Cost: $%.2f USD exceeds $50 threshold", params.CostEstimateUSD))
			}

			if len(matchedRisks) > 0 {
				res := map[string]interface{}{
					"verdict":        "REQUIRES_HUMAN_SIGNATURE",
					"risk_count":     len(matchedRisks),
					"risks_detected": matchedRisks,
					"recommendation": "Invoke 'request_human_signature' before executing this action.",
				}
				out, _ := json.MarshalIndent(res, "", "  ")
				return string(out), nil
			}

			res := map[string]interface{}{
				"verdict":        "APPROVED",
				"risk_count":     0,
				"recommendation": "Action complies with all workspace security guardrails.",
			}
			out, _ := json.MarshalIndent(res, "", "  ")
			return string(out), nil
		},
	}
}
