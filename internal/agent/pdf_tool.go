package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fabith10/agent-framework/adk"
	"github.com/ledongthuc/pdf"
)

// GetExtractPDFTextTool returns a native Go tool to extract plain text from PDF files.
func GetExtractPDFTextTool() adk.Tool {
	return adk.Tool{
		Name:        "extract_pdf_text",
		Description: "Extracts all plain text content from a PDF document (e.g. CV, resume, or report) at the given path.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the PDF file relative to the workspace root.",
				},
			},
			"required": []string{"path"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("extract_pdf_text: invalid JSON: %w", err)
			}

			// Clean path and prevent directory traversal
			cwd, _ := os.Getwd()
			var resolved string
			if filepath.IsAbs(params.Path) {
				resolved = filepath.Clean(params.Path)
			} else {
				resolved = filepath.Clean(filepath.Join(cwd, params.Path))
			}

			isTest := os.Getenv("AGENT_FRAMEWORK_TESTING") == "true"
			isSafe := strings.HasPrefix(resolved, cwd) || (isTest && strings.HasPrefix(resolved, os.TempDir()))
			if !isSafe {
				return "", fmt.Errorf("extract_pdf_text: permission denied: path must remain inside workspace")
			}

			// Open PDF file using pdf.Open directly
			f, r, err := pdf.Open(resolved)
			if err != nil {
				return "", fmt.Errorf("failed to parse PDF document: %w", err)
			}
			defer f.Close()

			var buf bytes.Buffer
			b, err := r.GetPlainText()
			if err != nil {
				return "", fmt.Errorf("failed to read PDF plain text: %w", err)
			}
			if _, err := buf.ReadFrom(b); err != nil {
				return "", fmt.Errorf("failed to read PDF stream: %w", err)
			}

			return buf.String(), nil
		},
	}
}
