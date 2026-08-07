package agenttools

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

func TestComprehensiveTools(t *testing.T) {
	ctx := context.Background()

	// 1. Test extract_web_tables
	t.Run("extract_web_tables", func(t *testing.T) {
		tool := GetExtractWebTablesTool()
		html := `<html><body><table><tr><th>Name</th><th>Price</th></tr><tr><td>Widget</td><td>$10</td></tr></table></body></html>`
		args, _ := json.Marshal(map[string]interface{}{
			"html_content": html,
		})
		out, err := tool.Execute(ctx, args)
		if err != nil {
			t.Fatalf("extract_web_tables failed: %v", err)
		}
		if out == "" {
			t.Errorf("expected non-empty table output")
		}
	})

	// 2. Test inspect_env_vars
	t.Run("inspect_env_vars", func(t *testing.T) {
		tool := GetInspectEnvVarsTool()
		os.Setenv("TEST_SECRET_KEY", "super-secret-pass-12345")
		defer os.Unsetenv("TEST_SECRET_KEY")

		args, _ := json.Marshal(map[string]interface{}{
			"filter": "TEST_SECRET",
		})
		out, err := tool.Execute(ctx, args)
		if err != nil {
			t.Fatalf("inspect_env_vars failed: %v", err)
		}
		if out == "" {
			t.Errorf("expected non-empty env vars output")
		}
	})

	// 3. Test validate_json_schema
	t.Run("validate_json_schema", func(t *testing.T) {
		tool := GetValidateJSONSchemaTool()
		jsonStr := `{"name": "SynapseGo", "status": "active"}`

		args, _ := json.Marshal(map[string]interface{}{
			"json_string":   jsonStr,
			"required_keys": []string{"name", "status"},
		})
		out, err := tool.Execute(ctx, args)
		if err != nil {
			t.Fatalf("validate_json_schema failed: %v", err)
		}
		if out == "" {
			t.Errorf("expected validation result")
		}
	})
}
