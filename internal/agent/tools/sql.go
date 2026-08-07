package agenttools

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fabith10/synapse-go/adk"
	_ "modernc.org/sqlite"
)

// GetQuerySQLiteDBTool returns a tool to inspect tables, schema, or execute SELECT queries on SQLite databases.
func GetQuerySQLiteDBTool() adk.Tool {
	return adk.Tool{
		Name:        "query_sqlite_db",
		Description: "Queries a SQLite database file: inspect tables, schema, or execute read-only SQL queries.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"db_path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the SQLite database file (.db or .sqlite)",
				},
				"query": map[string]interface{}{
					"type":        "string",
					"description": "SQL query string (or '.tables' / '.schema' for schema inspection)",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Max rows to return (default 50, max 500)",
				},
			},
			"required": []interface{}{"db_path", "query"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				DBPath string `json:"db_path"`
				Query  string `json:"query"`
				Limit  int    `json:"limit"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}

			if params.DBPath == "" {
				return "", fmt.Errorf("db_path is required")
			}

			dbPath := params.DBPath
			if root := ctx.Value(WorkspaceRootKey); root != nil {
				if rStr, ok := root.(string); ok && rStr != "" && !filepath.IsAbs(dbPath) {
					dbPath = filepath.Join(rStr, dbPath)
				}
			}
			dbPath = filepath.Clean(dbPath)

			if parentDir := filepath.Dir(dbPath); parentDir != "" {
				_ = os.MkdirAll(parentDir, 0755)
			}

			limit := params.Limit
			if limit <= 0 {
				limit = 50
			} else if limit > 500 {
				limit = 500
			}

			db, err := sql.Open("sqlite", dbPath)
			if err != nil {
				return "", fmt.Errorf("failed to open sqlite database: %w", err)
			}
			defer db.Close()

			query := strings.TrimSpace(params.Query)
			queryLower := strings.ToLower(query)

			execCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			if queryLower == ".tables" {
				query = "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name;"
			} else if queryLower == ".schema" || strings.HasPrefix(queryLower, ".schema ") {
				tableName := strings.TrimSpace(strings.TrimPrefix(queryLower, ".schema"))
				if tableName != "" {
					query = fmt.Sprintf("SELECT sql FROM sqlite_master WHERE type='table' AND name='%s';", tableName)
				} else {
					query = "SELECT sql FROM sqlite_master WHERE type='table' AND sql IS NOT NULL ORDER BY name;"
				}
			}

			rows, err := db.QueryContext(execCtx, query)
			if err != nil {
				return "", fmt.Errorf("SQL query failed: %w", err)
			}
			defer rows.Close()

			cols, err := rows.Columns()
			if err != nil {
				return "", fmt.Errorf("failed to get columns: %w", err)
			}

			var resultRows []map[string]interface{}
			rowCount := 0

			for rows.Next() && rowCount < limit {
				columns := make([]interface{}, len(cols))
				columnPointers := make([]interface{}, len(cols))
				for i := range columns {
					columnPointers[i] = &columns[i]
				}

				if err := rows.Scan(columnPointers...); err != nil {
					return "", fmt.Errorf("failed to scan row: %w", err)
				}

				rowMap := make(map[string]interface{})
				for i, colName := range cols {
					val := columnPointers[i].(*interface{})
					b, ok := (*val).([]byte)
					if ok {
						rowMap[colName] = string(b)
					} else {
						rowMap[colName] = *val
					}
				}
				resultRows = append(resultRows, rowMap)
				rowCount++
			}

			if err := rows.Err(); err != nil {
				return "", fmt.Errorf("error reading database rows: %w", err)
			}

			out := map[string]interface{}{
				"db_path":    dbPath,
				"query":      query,
				"row_count":  len(resultRows),
				"columns":    cols,
				"data_rows":  resultRows,
			}

			outBytes, _ := json.MarshalIndent(out, "", "  ")
			return string(outBytes), nil
		},
	}
}
