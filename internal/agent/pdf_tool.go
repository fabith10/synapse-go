package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fabith10/synapse-go/adk"
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

// GetGeneratePDFReportTool returns the Tier 1 native tool for compiling research summaries into PDF reports.
func GetGeneratePDFReportTool() adk.Tool {
	return adk.Tool{
		Name:        "generate_pdf_report",
		Description: "Natively compiles research summaries into a PDF report file and returns the file path.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"content": map[string]interface{}{
					"type":        "string",
					"description": "The text summary content to write inside the PDF report.",
				},
				"title": map[string]interface{}{
					"type":        "string",
					"description": "The title of the PDF report.",
				},
			},
			"required": []string{"content", "title"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				Content string `json:"content"`
				Title   string `json:"title"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("failed to parse PDF arguments: %w", err)
			}

			// Strip markdown decoration so the PDF shows readable plain text.
			cleanMD := func(s string) string {
				s = strings.ReplaceAll(s, "**", "")
				s = strings.ReplaceAll(s, "__", "")
				s = strings.ReplaceAll(s, "*", "")
				s = strings.ReplaceAll(s, "_", " ")
				for strings.Contains(s, "#") {
					s = strings.ReplaceAll(s, "#", "")
				}
				s = strings.ReplaceAll(s, "\\", "")
				s = strings.ReplaceAll(s, "(", "[")
				s = strings.ReplaceAll(s, ")", "]")
				return strings.TrimSpace(s)
			}

			// Word-wrap text into lines fitting within the page width.
			// A4: 595pt wide, left+right margins 50pt → usable 495pt.
			const charsPerLine = 82
			wrapText := func(text string) []string {
				var lines []string
				for _, paragraph := range strings.Split(text, "\n") {
					paragraph = cleanMD(paragraph)
					if paragraph == "" {
						lines = append(lines, "")
						continue
					}
					words := strings.Fields(paragraph)
					current := ""
					for _, word := range words {
						if current == "" {
							current = word
						} else if len(current)+1+len(word) <= charsPerLine {
							current += " " + word
						} else {
							lines = append(lines, current)
							current = word
						}
					}
					if current != "" {
						lines = append(lines, current)
					}
				}
				return lines
			}

			// Build page content streams — one A4 page per ~50 lines.
			const (
				lineHeight   = 16
				topMargin    = 800
				bottomMargin = 50
				leftMargin   = 50
				titleFontSz  = 16
				bodyFontSz   = 11
			)

			bodyLines := wrapText(params.Content)
			titleClean := cleanMD(params.Title)

			type pageStream struct{ content string }
			var pages []pageStream
			var sb strings.Builder
			y := topMargin

			sb.WriteString("BT\n")
			sb.WriteString(fmt.Sprintf("/F1 %d Tf\n", titleFontSz))
			sb.WriteString(fmt.Sprintf("%d %d Td\n", leftMargin, y))
			sb.WriteString(fmt.Sprintf("(%s) Tj\n", titleClean))
			y -= titleFontSz + 10
			sb.WriteString(fmt.Sprintf("/F1 %d Tf\n", bodyFontSz))
			sb.WriteString(fmt.Sprintf("%d %d Td\n", leftMargin, y))

			for _, line := range bodyLines {
				if y < bottomMargin {
					sb.WriteString("ET\n")
					pages = append(pages, pageStream{content: sb.String()})
					sb.Reset()
					y = topMargin
					sb.WriteString("BT\n")
					sb.WriteString(fmt.Sprintf("/F1 %d Tf\n", bodyFontSz))
					sb.WriteString(fmt.Sprintf("%d %d Td\n", leftMargin, y))
				}
				if line == "" {
					sb.WriteString(fmt.Sprintf("0 -%d Td\n", lineHeight/2))
					y -= lineHeight / 2
				} else {
					sb.WriteString(fmt.Sprintf("(%s) Tj\n", line))
					sb.WriteString(fmt.Sprintf("0 -%d Td\n", lineHeight))
					y -= lineHeight
				}
			}
			sb.WriteString("ET\n")
			pages = append(pages, pageStream{content: sb.String()})

			if len(pages) == 0 {
				pages = append(pages, pageStream{content: "BT /F1 11 Tf 50 800 Td (Empty report) Tj ET\n"})
			}

			// Assemble a valid PDF with accurate xref byte offsets.
			nPages := len(pages)
			fontObjID := 3
			pageBase := 4
			contentBase := pageBase + nPages
			totalObjs := contentBase + nPages

			type pdfObj struct {
				offset int
				body   string
			}
			objs := make([]pdfObj, totalObjs+1)

			kids := make([]string, nPages)
			for i := 0; i < nPages; i++ {
				kids[i] = fmt.Sprintf("%d 0 R", pageBase+i)
			}
			objs[1].body = "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
			objs[2].body = fmt.Sprintf("2 0 obj\n<< /Type /Pages /Kids [%s] /Count %d >>\nendobj\n",
				strings.Join(kids, " "), nPages)
			objs[fontObjID].body = "3 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n"

			for i := 0; i < nPages; i++ {
				pid := pageBase + i
				cid := contentBase + i
				objs[pid].body = fmt.Sprintf(
					"%d 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] "+
						"/Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>\nendobj\n",
					pid, cid)
				stream := pages[i].content
				objs[cid].body = fmt.Sprintf(
					"%d 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n",
					cid, len(stream), stream)
			}

			var buf bytes.Buffer
			buf.WriteString("%PDF-1.4\n")
			for id := 1; id <= totalObjs; id++ {
				objs[id].offset = buf.Len()
				buf.WriteString(objs[id].body)
			}
			xrefOffset := buf.Len()
			buf.WriteString(fmt.Sprintf("xref\n0 %d\n", totalObjs+1))
			buf.WriteString("0000000000 65535 f\r\n")
			for id := 1; id <= totalObjs; id++ {
				buf.WriteString(fmt.Sprintf("%010d 00000 n\r\n", objs[id].offset))
			}
			buf.WriteString(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",
				totalObjs+1, xrefOffset))

			reportsDir := "reports"
			if err := os.MkdirAll(reportsDir, 0755); err != nil {
				return "", fmt.Errorf("failed to create reports directory: %w", err)
			}
			filePath := fmt.Sprintf("%s/research_report_%d.pdf", reportsDir, time.Now().UnixNano())
			if err := os.WriteFile(filePath, buf.Bytes(), 0644); err != nil {
				return "", fmt.Errorf("failed to save PDF file: %w", err)
			}
			return filePath, nil
		},
	}
}

