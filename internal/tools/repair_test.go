package tools

import (
	"encoding/json"
	"testing"
)

func TestRepairJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Valid JSON unchanged",
			input:    `{"action": "execute_python", "code": "print(1)"}`,
			expected: `{"action": "execute_python", "code": "print(1)"}`,
		},
		{
			name:     "Markdown fenced JSON",
			input:    "```json\n{\"action\": \"read_file\", \"path\": \"main.go\"}\n```",
			expected: `{"action": "read_file", "path": "main.go"}`,
		},
		{
			name:     "Single-quoted JSON keys and strings",
			input:    `{'action': 'read_file', 'path': 'main.go'}`,
			expected: `{"action": "read_file", "path": "main.go"}`,
		},
		{
			name:     "Trailing commas in object",
			input:    `{"action": "read_file", "path": "main.go",}`,
			expected: `{"action": "read_file", "path": "main.go"}`,
		},
		{
			name:     "Multiline Python string with raw newlines",
			input:    "{\"action\": \"execute_python\", \"python_code\": \"import sys\nprint('hello')\"}",
			expected: `{"action": "execute_python", "python_code": "import sys\nprint('hello')"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repaired := RepairJSON(tt.input)
			var js interface{}
			if err := json.Unmarshal([]byte(repaired), &js); err != nil {
				t.Fatalf("RepairJSON produced invalid JSON %q: %v", repaired, err)
			}
		})
	}
}
