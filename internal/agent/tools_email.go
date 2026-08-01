package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fabith10/synapse-go/adk"
)

// GetWriteEmailTool returns the Tier 1 native tool for writing and saving email drafts.
func GetWriteEmailTool(orch *adk.Orchestrator, agentID string, mailbox chan adk.Message) adk.Tool {
	return adk.Tool{
		Name:        "write_email",
		Description: "Formats and writes an email draft as a persistent file, returning the generated file path.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"recipient": map[string]interface{}{
					"type":        "string",
					"description": "The destination email address (e.g. client@company.com).",
				},
				"subject": map[string]interface{}{
					"type":        "string",
					"description": "The email subject line.",
				},
				"body": map[string]interface{}{
					"type":        "string",
					"description": "The rich content body text of the email.",
				},
			},
			"required": []string{"recipient", "subject", "body"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				Recipient string `json:"recipient"`
				Subject   string `json:"subject"`
				Body      string `json:"body"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("failed to parse email draft arguments: %w", err)
			}

			// HITL checkpoint for external recipients
			isExternal := true
			lowerRecip := strings.ToLower(params.Recipient)
			for _, domain := range CriticalActions.InternalEmailDomains {
				if strings.HasSuffix(lowerRecip, "@"+strings.ToLower(domain)) {
					isExternal = false
					break
				}
			}
			if isExternal && orch != nil && mailbox != nil {
				uniqueCorrID := fmt.Sprintf("%s-email-%d-%d", agentID, time.Now().UnixNano(), atomic.AddUint64(&hitlCorrCounter, 1))
				orch.Send(adk.Message{
					Sender:    agentID,
					Recipient: "USER",
					Content:   fmt.Sprintf("[Risk Checkpoint] email agent requests approval to write email to EXTERNAL recipient: %q.", params.Recipient),
					Metadata: map[string]string{
						"Type":           "HITL_APPROVAL",
						"correlation_id": uniqueCorrID,
						"reply_to":       agentID,
						"risk_level":     "HIGH",
						"action":         "email.write",
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
					return "", fmt.Errorf("human denied email writing to external recipient")
				}
			}

			emailsDir := "emails"
			if err := os.MkdirAll(emailsDir, 0755); err != nil {
				return "", fmt.Errorf("failed to create emails directory: %w", err)
			}

			filePath := fmt.Sprintf("%s/email_draft_%d.txt", emailsDir, time.Now().UnixNano())
			draftContent := fmt.Sprintf("To: %s\nSubject: %s\nDate: %s\n\n%s\n",
				params.Recipient,
				params.Subject,
				time.Now().Format(time.RFC1123),
				params.Body,
			)

			if err := os.WriteFile(filePath, []byte(draftContent), 0644); err != nil {
				return "", fmt.Errorf("failed to save email draft file: %w", err)
			}

			return fmt.Sprintf("Email draft created. Draft: %s\n\n--- DRAFTED EMAIL CONTENT ---\nTo: %s\nSubject: %s\n\nBody:\n%s\n--- END DRAFTED EMAIL CONTENT ---",
				filePath, params.Recipient, params.Subject, params.Body), nil
		},
	}
}
