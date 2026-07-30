package tools

import (
	"bytes"
	"encoding/json"
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
