package tools

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	markdownFenceRegex = regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)\\s*```")
	trailingCommaRegex = regexp.MustCompile(`,(\s*[\}\]])`)
)

// RepairJSON takes a raw LLM output string that may contain malformed JSON,
// single quotes, unescaped newlines inside strings, or markdown code fences,
// and returns a sanitized string that parses cleanly as valid JSON.
func RepairJSON(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "{}"
	}

	// 1. Extract JSON from markdown fences if present
	if matches := markdownFenceRegex.FindStringSubmatch(trimmed); len(matches) > 1 {
		trimmed = strings.TrimSpace(matches[1])
	}

	// If already valid JSON, return as-is
	var js interface{}
	if json.Unmarshal([]byte(trimmed), &js) == nil {
		return trimmed
	}

	// 2. Locate first '{' or '[' and last '}' or ']'
	startIdx := strings.IndexAny(trimmed, "{[")
	if startIdx != -1 {
		endChar := "}"
		if trimmed[startIdx] == '[' {
			endChar = "]"
		}
		endIdx := strings.LastIndex(trimmed, endChar)
		if endIdx > startIdx {
			trimmed = trimmed[startIdx : endIdx+1]
		}
	}

	// Re-check validity after slicing
	if json.Unmarshal([]byte(trimmed), &js) == nil {
		return trimmed
	}

	// 3. Fix trailing commas before } or ]
	trimmed = trailingCommaRegex.ReplaceAllString(trimmed, "$1")
	if json.Unmarshal([]byte(trimmed), &js) == nil {
		return trimmed
	}

	// 4. Convert single-quoted keys/strings to double quotes
	repaired := repairSingleQuotesAndNewlines(trimmed)
	if json.Unmarshal([]byte(repaired), &js) == nil {
		return repaired
	}

	// 5. Final fallback: sanitize trailing commas again after quote repair
	repaired = trailingCommaRegex.ReplaceAllString(repaired, "$1")
	return repaired
}

// repairSingleQuotesAndNewlines parses statefully to replace single quotes with double quotes
// and escape unescaped raw newlines inside multiline JSON string values.
func repairSingleQuotesAndNewlines(input string) string {
	var buf bytes.Buffer
	inDoubleQuote := false
	inSingleQuote := false
	escaped := false

	for i := 0; i < len(input); i++ {
		ch := input[i]

		if escaped {
			buf.WriteByte(ch)
			escaped = false
			continue
		}

		if ch == '\\' {
			buf.WriteByte(ch)
			escaped = true
			continue
		}

		if ch == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
			buf.WriteByte(ch)
			continue
		}

		if ch == '\'' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
			buf.WriteByte('"')
			continue
		}

		if (ch == '\n' || ch == '\r') && (inDoubleQuote || inSingleQuote) {
			switch ch {
			case '\n':
				buf.WriteString("\\n")
			case '\r':
				buf.WriteString("\\r")
			}
			continue
		}

		buf.WriteByte(ch)
	}

	return buf.String()
}

// ExtractJSON strips markdown code fences and surrounding prose from a string,
// returning the first JSON object or array found. Used by the broker, orchestrator,
// and agent packages to normalise LLM responses.
func ExtractJSON(input string) string {
	trimmed := strings.TrimSpace(input)

	// Strip markdown code fences first
	trimmed = StripMarkdownFences(trimmed)

	// Find the first brace `{` or `[` and the matching closing brace
	firstBrace := strings.Index(trimmed, "{")
	firstBracket := strings.Index(trimmed, "[")

	start := -1
	end := -1

	if firstBrace != -1 && (firstBracket == -1 || firstBrace < firstBracket) {
		start = firstBrace
		end = strings.LastIndex(trimmed, "}")
	} else if firstBracket != -1 {
		start = firstBracket
		end = strings.LastIndex(trimmed, "]")
	}

	if start == -1 || end == -1 || end <= start {
		return trimmed
	}
	return trimmed[start : end+1]
}

// StripMarkdownFences removes ``` or ```json code fences from LLM output,
// returning the inner content. If no fences are found, returns input unchanged.
func StripMarkdownFences(input string) string {
	if idx := strings.Index(input, "```json"); idx != -1 {
		content := input[idx+7:]
		if endIdx := strings.Index(content, "```"); endIdx != -1 {
			return strings.TrimSpace(content[:endIdx])
		}
	} else if idx := strings.Index(input, "```"); idx != -1 {
		content := input[idx+3:]
		if endIdx := strings.Index(content, "```"); endIdx != -1 {
			return strings.TrimSpace(content[:endIdx])
		}
	}
	return input
}

// CopyMeta returns a shallow clone of a string map. Nil input returns an empty map.
func CopyMeta(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// FindConfigPath walks parent directories from cwd looking for a file or directory
// named `name`. Returns the first match, or `name` unchanged if nothing is found.
func FindConfigPath(name string) string {
	wd, err := os.Getwd()
	if err != nil {
		return name
	}
	curr := wd
	for {
		candidate := filepath.Join(curr, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}
	return name
}
