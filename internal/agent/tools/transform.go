package agenttools

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fabith10/synapse-go/adk"
)

// GetCSVJSONTransformerTool returns a tool to transform, filter, and aggregate CSV/JSON data.
func GetCSVJSONTransformerTool() adk.Tool {
	return adk.Tool{
		Name:        "csv_json_transformer",
		Description: "Transforms, filters, and aggregates structured data between CSV and JSON formats.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"operation": map[string]interface{}{
					"type":        "string",
					"description": "Operation: 'csv_to_json', 'json_to_csv', 'filter', 'aggregate'",
				},
				"data": map[string]interface{}{
					"type":        "string",
					"description": "Raw data string or file path containing CSV or JSON",
				},
				"filter_key": map[string]interface{}{
					"type":        "string",
					"description": "Field/column name to filter by (for 'filter' or 'aggregate')",
				},
				"filter_value": map[string]interface{}{
					"type":        "string",
					"description": "Value to match for filter operation",
				},
				"aggregate_field": map[string]interface{}{
					"type":        "string",
					"description": "Field/column name to sum/average (for 'aggregate')",
				},
				"output_path": map[string]interface{}{
					"type":        "string",
					"description": "Optional file path to save transformed output",
				},
			},
			"required": []interface{}{"operation", "data"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				Operation      string `json:"operation"`
				Data           string `json:"data"`
				FilterKey      string `json:"filter_key"`
				FilterValue    string `json:"filter_value"`
				AggregateField string `json:"aggregate_field"`
				OutputPath     string `json:"output_path"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}

			rawContent := params.Data
			// Check if Data is a file path
			if len(rawContent) < 1000 && (strings.HasSuffix(rawContent, ".csv") || strings.HasSuffix(rawContent, ".json")) {
				filePath := rawContent
				if root := ctx.Value(WorkspaceRootKey); root != nil {
					if rStr, ok := root.(string); ok && rStr != "" && !filepath.IsAbs(filePath) {
						filePath = filepath.Join(rStr, filePath)
					}
				}
				if b, err := os.ReadFile(filePath); err == nil {
					rawContent = string(b)
				}
			}

			op := strings.ToLower(strings.TrimSpace(params.Operation))

			switch op {
			case "csv_to_json":
				r := csv.NewReader(strings.NewReader(rawContent))
				records, err := r.ReadAll()
				if err != nil {
					return "", fmt.Errorf("failed to parse CSV data: %w", err)
				}
				if len(records) < 1 {
					return "[]", nil
				}

				headers := records[0]
				var jsonRows []map[string]string
				for _, row := range records[1:] {
					rowMap := make(map[string]string)
					for i, h := range headers {
						if i < len(row) {
							rowMap[h] = row[i]
						}
					}
					jsonRows = append(jsonRows, rowMap)
				}

				outBytes, _ := json.MarshalIndent(jsonRows, "", "  ")
				outStr := string(outBytes)

				if params.OutputPath != "" {
					savePath := params.OutputPath
					if root := ctx.Value(WorkspaceRootKey); root != nil {
						if rStr, ok := root.(string); ok && rStr != "" && !filepath.IsAbs(savePath) {
							savePath = filepath.Join(rStr, savePath)
						}
					}
					_ = os.WriteFile(savePath, outBytes, 0644)
				}
				return outStr, nil

			case "json_to_csv":
				var rows []map[string]interface{}
				if err := json.Unmarshal([]byte(rawContent), &rows); err != nil {
					return "", fmt.Errorf("failed to parse JSON array: %w", err)
				}
				if len(rows) == 0 {
					return "", nil
				}

				// Collect unique header keys
				headerSet := make(map[string]bool)
				var headers []string
				for _, r := range rows {
					for k := range r {
						if !headerSet[k] {
							headerSet[k] = true
							headers = append(headers, k)
						}
					}
				}

				var csvBuf bytes.Buffer
				w := csv.NewWriter(&csvBuf)
				_ = w.Write(headers)

				for _, r := range rows {
					var record []string
					for _, h := range headers {
						val := r[h]
						if val == nil {
							record = append(record, "")
						} else {
							record = append(record, fmt.Sprintf("%v", val))
						}
					}
					_ = w.Write(record)
				}
				w.Flush()

				outStr := csvBuf.String()
				if params.OutputPath != "" {
					savePath := params.OutputPath
					if root := ctx.Value(WorkspaceRootKey); root != nil {
						if rStr, ok := root.(string); ok && rStr != "" && !filepath.IsAbs(savePath) {
							savePath = filepath.Join(rStr, savePath)
						}
					}
					_ = os.WriteFile(savePath, csvBuf.Bytes(), 0644)
				}
				return outStr, nil

			case "filter":
				var rows []map[string]interface{}
				if err := json.Unmarshal([]byte(rawContent), &rows); err != nil {
					return "", fmt.Errorf("filter requires JSON array input: %w", err)
				}
				var filtered []map[string]interface{}
				for _, r := range rows {
					if val, ok := r[params.FilterKey]; ok {
						if strings.EqualFold(fmt.Sprintf("%v", val), params.FilterValue) {
							filtered = append(filtered, r)
						}
					}
				}
				outBytes, _ := json.MarshalIndent(filtered, "", "  ")
				return string(outBytes), nil

			case "aggregate":
				var rows []map[string]interface{}
				if err := json.Unmarshal([]byte(rawContent), &rows); err != nil {
					return "", fmt.Errorf("aggregate requires JSON array input: %w", err)
				}
				field := params.AggregateField
				if field == "" {
					return "", fmt.Errorf("aggregate_field is required")
				}

				count := 0
				sum := 0.0
				minVal := 0.0
				maxVal := 0.0
				first := true

				for _, r := range rows {
					if val, ok := r[field]; ok {
						numStr := fmt.Sprintf("%v", val)
						if num, err := strconv.ParseFloat(numStr, 64); err == nil {
							sum += num
							count++
							if first {
								minVal = num
								maxVal = num
								first = false
							} else {
								if num < minVal {
									minVal = num
								}
								if num > maxVal {
									maxVal = num
								}
							}
						}
					}
				}

				avg := 0.0
				if count > 0 {
					avg = sum / float64(count)
				}

				res := map[string]interface{}{
					"field":      field,
					"row_count":  len(rows),
					"num_count":  count,
					"sum":        sum,
					"average":    avg,
					"min":        minVal,
					"max":        maxVal,
				}
				outBytes, _ := json.MarshalIndent(res, "", "  ")
				return string(outBytes), nil

			default:
				return "", fmt.Errorf("unsupported operation %q. Use csv_to_json, json_to_csv, filter, aggregate", op)
			}
		},
	}
}

// GetValidateJSONSchemaTool returns a native tool to validate JSON syntax and required structural keys.
func GetValidateJSONSchemaTool() adk.Tool {
	return adk.Tool{
		Name:        "validate_json_schema",
		Description: "Validates JSON string syntax and checks for required key fields before saving or sending JSON payloads.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"json_string": map[string]interface{}{
					"type":        "string",
					"description": "JSON text string or file path to validate",
				},
				"required_keys": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "string",
					},
					"description": "Optional list of top-level key names that must be present",
				},
			},
			"required": []string{"json_string"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				JSONString   string   `json:"json_string"`
				RequiredKeys []string `json:"required_keys"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}

			raw := strings.TrimSpace(params.JSONString)
			if len(raw) < 500 && (strings.HasSuffix(raw, ".json") || strings.HasSuffix(raw, ".js")) {
				if b, err := os.ReadFile(raw); err == nil {
					raw = string(b)
				}
			}

			var parsed interface{}
			if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
				return fmt.Sprintf("❌ INVALID JSON: Syntax error: %v", err), nil
			}

			var missing []string
			if objMap, ok := parsed.(map[string]interface{}); ok {
				for _, reqKey := range params.RequiredKeys {
					if _, exists := objMap[reqKey]; !exists {
						missing = append(missing, reqKey)
					}
				}
			}

			if len(missing) > 0 {
				return fmt.Sprintf("⚠️ VALID JSON syntax, but missing %d required key(s): %s", len(missing), strings.Join(missing, ", ")), nil
			}

			return "✅ VALID JSON: Syntax is clean and all required schema keys are present.", nil
		},
	}
}
