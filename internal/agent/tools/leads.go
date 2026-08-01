package agenttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fabith10/synapse-go/adk"
)

// GetCheckLeadScoreTool returns a native tool to evaluate sales leads.
func GetCheckLeadScoreTool() adk.Tool {
	return adk.Tool{
		Name:        "check_lead_score",
		Description: "Scores a sales lead based on company profile (size, industry, budget) and returns standard JSON analysis.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"company_name": map[string]interface{}{
					"type":        "string",
					"description": "Name of the target company.",
				},
				"company_size": map[string]interface{}{
					"type":        "integer",
					"description": "Number of employees (e.g. 150).",
				},
				"budget_usd": map[string]interface{}{
					"type":        "number",
					"description": "Estimated budget in USD (e.g. 25000.0).",
				},
				"industry": map[string]interface{}{
					"type":        "string",
					"description": "Company industry (e.g. 'finance', 'tech', 'retail').",
				},
			},
			"required": []string{"company_name", "company_size", "budget_usd"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				CompanyName string  `json:"company_name"`
				CompanySize int     `json:"company_size"`
				BudgetUSD   float64 `json:"budget_usd"`
				Industry    string  `json:"industry"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("failed to parse lead scoring arguments: %w", err)
			}

			score := 50
			if params.BudgetUSD >= 50000 {
				score += 25
			} else if params.BudgetUSD >= 10000 {
				score += 15
			} else if params.BudgetUSD < 2000 {
				score -= 10
			}
			if params.CompanySize >= 500 {
				score += 15
			} else if params.CompanySize >= 100 {
				score += 10
			}
			switch strings.ToLower(params.Industry) {
			case "finance", "fintech", "banking":
				score += 10
			case "tech", "saas", "software":
				score += 10
			}
			if score > 100 {
				score = 100
			} else if score < 0 {
				score = 0
			}

			status := "Cold"
			if score >= 80 {
				status = "SQL (Sales Qualified)"
			} else if score >= 60 {
				status = "MQL (Marketing Qualified)"
			}

			prob := float64(score) / 100.0
			res := map[string]interface{}{
				"company":            params.CompanyName,
				"lead_score":         score,
				"status":             status,
				"conversion_prob":    fmt.Sprintf("%.1f%%", prob*100),
				"recommended_action": getRecommendedAction(status),
			}
			jsonBytes, _ := json.Marshal(res)
			return string(jsonBytes), nil
		},
	}
}

func getRecommendedAction(status string) string {
	switch status {
	case "SQL (Sales Qualified)":
		return "Immediate 1-on-1 direct outreach from Account Executive."
	case "MQL (Marketing Qualified)":
		return "Add to automated email nurture sequence and monitor engagement."
	default:
		return "Keep in low-priority marketing broadcast list."
	}
}
