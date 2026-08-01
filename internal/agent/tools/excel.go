package agenttools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/fabith10/synapse-go/adk"
	"github.com/xuri/excelize/v2"
)

// GetModifyExcelWorkbookTool returns the Tier 1 native tool to update Excel files.
func GetModifyExcelWorkbookTool(orch *adk.Orchestrator, agentID string, mailbox chan adk.Message) adk.Tool {
	return adk.Tool{
		Name:        "modify_excel_workbook",
		Description: "Natively opens an .xlsx file, applies cell updates, and reads back specified cells. Extremely fast, uses no external dependencies.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"file_path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the .xlsx file.",
				},
				"updates": map[string]interface{}{
					"type":        "array",
					"description": "List of updates: [{'sheet': 'DCF', 'cell': 'B4', 'value': 0.08}]",
					"items":       map[string]interface{}{"type": "object"},
				},
				"extract_cells": map[string]interface{}{
					"type":        "array",
					"description": "List of cells to read back after updates: [{'sheet': 'DCF', 'cell': 'D20'}]",
					"items":       map[string]interface{}{"type": "object"},
				},
			},
			"required": []string{"file_path", "updates"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var rawMap map[string]interface{}
			if err := json.Unmarshal(args, &rawMap); err != nil {
				return "", fmt.Errorf("modify_excel_workbook: invalid JSON args: %w", err)
			}

			filePath := extractStringAlias(rawMap, "file_path", "path", "file", "target_file")
			if filePath == "" {
				return "", fmt.Errorf("modify_excel_workbook: missing required parameter 'file_path'")
			}

			var updates []map[string]interface{}
			if updRaw, ok := rawMap["updates"].([]interface{}); ok {
				for _, u := range updRaw {
					if um, ok := u.(map[string]interface{}); ok {
						updates = append(updates, um)
					}
				}
			}

			var extractCells []map[string]string
			if extRaw, ok := rawMap["extract_cells"].([]interface{}); ok {
				for _, item := range extRaw {
					if em, ok := item.(map[string]interface{}); ok {
						sheet := extractStringAlias(em, "sheet")
						cell := extractStringAlias(em, "cell")
						if cell != "" {
							if sheet == "" {
								sheet = "Sheet1"
							}
							extractCells = append(extractCells, map[string]string{"sheet": sheet, "cell": cell})
						}
					} else if strCell, ok := item.(string); ok && strCell != "" {
						extractCells = append(extractCells, map[string]string{"sheet": "Sheet1", "cell": strCell})
					}
				}
			} else if extStr, ok := rawMap["extract_cells"].(string); ok && extStr != "" {
				extractCells = append(extractCells, map[string]string{"sheet": "Sheet1", "cell": extStr})
			}

			// Automated Checkpoint for Production files
			if IsProductionFile(filePath) {
				uniqueCorrID := fmt.Sprintf("%s-excel-%d-%d", agentID, time.Now().UnixNano(), atomic.AddUint64(&hitlCorrCounter, 1))
				orch.Send(adk.Message{
					Sender:    agentID,
					Recipient: "USER",
					Content:   fmt.Sprintf("[Risk Checkpoint] excel agent requests approval to modify PRODUCTION workbook: %q.", filePath),
					Metadata: map[string]string{
						"Type":           "HITL_APPROVAL",
						"correlation_id": uniqueCorrID,
						"reply_to":       agentID,
						"risk_level":     "CRITICAL",
						"action":         "excel.modify",
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
					return "", fmt.Errorf("human denied modification of production model")
				}
			}

			if cleanP, errResolve := resolveSafeWorkspacePath(filePath); errResolve == nil {
				filePath = cleanP
			}

			var f *excelize.File
			var err error
			f, err = excelize.OpenFile(filePath)
			if err != nil {
				_ = os.MkdirAll(filepath.Dir(filePath), 0755)
				f = excelize.NewFile()
			}
			defer f.Close()

			for _, u := range updates {
				sheet := extractStringAlias(u, "sheet")
				cell := extractStringAlias(u, "cell")
				val := u["value"]
				if cell == "" {
					continue
				}
				if sheet == "" {
					sheet = "Sheet1"
				}
				_, _ = f.NewSheet(sheet)
				if err := f.SetCellValue(sheet, cell, val); err != nil {
					return "", fmt.Errorf("excel SetCellValue: %w", err)
				}
			}

			if err := f.SaveAs(filePath); err != nil {
				return "", fmt.Errorf("excel SaveAs: %w", err)
			}

			results := make(map[string]string)
			for _, e := range extractCells {
				sheet := e["sheet"]
				cell := e["cell"]
				cellVal, _ := f.GetCellValue(sheet, cell)
				results[cell] = cellVal
			}

			jsonBytes, _ := json.Marshal(map[string]interface{}{
				"status":          "updated",
				"file_path":       filePath,
				"extracted_cells": results,
			})
			return string(jsonBytes), nil
		},
	}
}
