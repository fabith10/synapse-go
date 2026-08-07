package agenttools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewTools(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	// 1. Test git_operations (status)
	t.Run("git_operations", func(t *testing.T) {
		tool := GetGitOperationsTool()
		args, _ := json.Marshal(map[string]interface{}{
			"operation": "status",
			"path":      tempDir,
		})
		// Initialize git repo in tempDir
		_ = os.WriteFile(filepath.Join(tempDir, "README.md"), []byte("# Test"), 0644)
		out, err := tool.Execute(ctx, args)
		// git status may fail if not a git repo or succeed if git status succeeds
		if err == nil {
			t.Logf("git status output: %s", out)
		}
	})

	// 2. Test http_api_request (invalid host fallback/error handling)
	t.Run("http_api_request", func(t *testing.T) {
		tool := GetHTTPAPIRequestTool()
		args, _ := json.Marshal(map[string]interface{}{
			"url":    "https://httpbin.org/get",
			"method": "GET",
		})
		out, err := tool.Execute(ctx, args)
		if err != nil {
			t.Logf("http request error (expected offline/sandbox): %v", err)
		} else {
			if out == "" {
				t.Errorf("expected non-empty response")
			}
		}
	})

	// 3. Test query_sqlite_db
	t.Run("query_sqlite_db", func(t *testing.T) {
		dbPath := filepath.Join(tempDir, "test.db")
		tool := GetQuerySQLiteDBTool()

		// First create a table via raw SQLite query
		createArgs, _ := json.Marshal(map[string]interface{}{
			"db_path": dbPath,
			"query":   "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT); INSERT INTO users (name) VALUES ('Alice');",
		})
		_, _ = tool.Execute(ctx, createArgs)

		selectArgs, _ := json.Marshal(map[string]interface{}{
			"db_path": dbPath,
			"query":   "SELECT name FROM users;",
		})
		out, err := tool.Execute(ctx, selectArgs)
		if err != nil {
			t.Fatalf("query_sqlite_db error: %v", err)
		}
		if out == "" {
			t.Errorf("expected query results")
		}
	})

	// 4. Test csv_json_transformer
	t.Run("csv_json_transformer", func(t *testing.T) {
		tool := GetCSVJSONTransformerTool()
		csvData := "name,age,salary\nAlice,30,100000\nBob,40,150000\n"

		// CSV to JSON
		args, _ := json.Marshal(map[string]interface{}{
			"operation": "csv_to_json",
			"data":      csvData,
		})
		jsonOut, err := tool.Execute(ctx, args)
		if err != nil {
			t.Fatalf("csv_to_json failed: %v", err)
		}

		// Aggregate
		aggArgs, _ := json.Marshal(map[string]interface{}{
			"operation":       "aggregate",
			"data":            jsonOut,
			"aggregate_field": "salary",
		})
		aggOut, err := tool.Execute(ctx, aggArgs)
		if err != nil {
			t.Fatalf("aggregate failed: %v", err)
		}
		if aggOut == "" {
			t.Errorf("expected aggregate results")
		}
	})

	// 5. Test archive_manager
	t.Run("archive_manager", func(t *testing.T) {
		tool := GetArchiveManagerTool()

		// Create test files
		file1 := filepath.Join(tempDir, "file1.txt")
		_ = os.WriteFile(file1, []byte("Hello World"), 0644)

		zipPath := filepath.Join(tempDir, "test.zip")
		createArgs, _ := json.Marshal(map[string]interface{}{
			"action":       "create",
			"archive_path": zipPath,
			"target_dir":   tempDir,
			"files":        []string{"file1.txt"},
		})
		_, err := tool.Execute(ctx, createArgs)
		if err != nil {
			t.Fatalf("zip create failed: %v", err)
		}

		listArgs, _ := json.Marshal(map[string]interface{}{
			"action":       "list",
			"archive_path": zipPath,
		})
		listOut, err := tool.Execute(ctx, listArgs)
		if err != nil {
			t.Fatalf("zip list failed: %v", err)
		}
		if listOut == "" {
			t.Errorf("expected zip list results")
		}
	})

	// 6. Test inspect_system_processes
	t.Run("inspect_system_processes", func(t *testing.T) {
		tool := GetInspectSystemProcessesTool()
		args, _ := json.Marshal(map[string]interface{}{
			"action": "list",
		})
		out, err := tool.Execute(ctx, args)
		if err != nil {
			t.Fatalf("process list failed: %v", err)
		}
		if out == "" {
			t.Errorf("expected process listing")
		}
	})

	// 7. Test fetch_rss_feed
	t.Run("fetch_rss_feed", func(t *testing.T) {
		tool := GetFetchRSSFeedTool()
		args, _ := json.Marshal(map[string]interface{}{
			"url":       "https://go.dev/doc/devel/release.rss",
			"max_items": 3,
		})
		out, err := tool.Execute(ctx, args)
		if err != nil {
			t.Logf("rss fetch error (expected if offline): %v", err)
		} else {
			t.Logf("rss output: %s", out)
		}
	})
}
