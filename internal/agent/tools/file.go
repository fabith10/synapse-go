package agenttools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/fabith10/synapse-go/adk"
	"github.com/fabith10/synapse-go/internal/sanitizer"
)

// GetGrepDocumentsTool returns the Tier 1 native tool to search files for text or regex patterns.
func GetGrepDocumentsTool() adk.Tool {
	return adk.Tool{
		Name:        "grep_documents",
		Description: "Recursively searches documents in a directory or a specific file for a text pattern or regex.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "The path to the file or directory to search. Relative to workspace root.",
				},
				"pattern": map[string]interface{}{
					"type":        "string",
					"description": "The text string or regular expression to search for.",
				},
				"is_regex": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, treats the pattern as a regular expression.",
				},
			},
			"required": []string{"path", "pattern"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				Path    string `json:"path"`
				Pattern string `json:"pattern"`
				IsRegex bool   `json:"is_regex"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("grep_documents: invalid JSON args: %w", err)
			}

			cleanPath, err := resolveSafeWorkspacePathWithCtx(ctx, params.Path)
			if err != nil {
				return "", fmt.Errorf("grep_documents: %w", err)
			}

			var matches []string
			var matchFn func(line string) bool
			if params.IsRegex {
				re, err := regexp.Compile(params.Pattern)
				if err != nil {
					return "", fmt.Errorf("grep_documents: invalid regex pattern: %w", err)
				}
				matchFn = func(line string) bool { return re.MatchString(line) }
			} else {
				lowerPat := strings.ToLower(params.Pattern)
				matchFn = func(line string) bool { return strings.Contains(strings.ToLower(line), lowerPat) }
			}

			searchFunc := func(filePath string) error {
				file, err := os.Open(filePath)
				if err != nil {
					return nil
				}
				defer file.Close()
				scanner := bufio.NewScanner(file)
				lineNum := 1
				for scanner.Scan() {
					line := scanner.Text()
					if matchFn(line) {
						if len(matches) >= 50 {
							break
						}
						dispLine := line
						if len(dispLine) > 200 {
							dispLine = dispLine[:197] + "..."
						}
						matches = append(matches, fmt.Sprintf("%s:%d: %s", filepath.Base(filePath), lineNum, dispLine))
					}
					lineNum++
				}
				return scanner.Err()
			}

			info, err := os.Stat(cleanPath)
			if err != nil {
				return "", fmt.Errorf("grep_documents: path not found: %w", err)
			}
			if info.IsDir() {
				_ = filepath.Walk(cleanPath, func(path string, info os.FileInfo, err error) error {
					if err != nil || info.IsDir() {
						return nil
					}
					ext := strings.ToLower(filepath.Ext(path))
					if ext == ".txt" || ext == ".md" || ext == ".csv" || ext == ".json" || ext == ".go" || ext == ".py" || ext == ".sh" {
						_ = searchFunc(path)
					}
					return nil
				})
			} else {
				_ = searchFunc(cleanPath)
			}

			if len(matches) == 0 {
				return "No matches found.", nil
			}
			return strings.Join(matches, "\n"), nil
		},
	}
}

// GetReadFileTool returns the Tier 1 native tool to read file content with line windowing.
func GetReadFileTool() adk.Tool {
	return adk.Tool{
		Name:        "read_file",
		Description: "Reads text contents of a specified file in the workspace, with optional start_line and end_line parameters.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the file to read (relative to workspace root or absolute path inside workspace).",
				},
				"start_line": map[string]interface{}{
					"type":        "integer",
					"description": "Optional 1-indexed starting line number.",
				},
				"end_line": map[string]interface{}{
					"type":        "integer",
					"description": "Optional 1-indexed ending line number.",
				},
			},
			"required": []string{"path"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var raw map[string]interface{}
			_ = json.Unmarshal(args, &raw)

			targetPath := extractStringAlias(raw, "path", "target_file", "file_path", "file")
			if targetPath == "" {
				return "Missing required parameter 'path'. Provide a workspace file path (e.g. 'scripts/gpu_cost_optimizer.py') or use 'web_search_and_extract' for web documentation.", nil
			}
			if isSystemMetadataPath(targetPath) {
				return "", fmt.Errorf("read_file: access denied: framework metadata file %q is protected and cannot be read by workspace tasks", targetPath)
			}

			cleanPath, err := resolveSafeWorkspacePathWithCtx(ctx, targetPath)
			if err != nil {
				return "", fmt.Errorf("read_file: %w", err)
			}

			content, err := os.ReadFile(cleanPath)
			if err != nil {
				if os.IsNotExist(err) {
					return fmt.Sprintf("File %q does not exist yet. Use write_file to create it.", targetPath), nil
				}
				return "", fmt.Errorf("read_file: failed to read file: %w", err)
			}

			sanitizedContent, _ := sanitizer.SanitizeSecrets(string(content))
			lines := strings.Split(sanitizedContent, "\n")
			startLine := extractIntAlias(raw, "start_line", "start")
			endLine := extractIntAlias(raw, "end_line", "end")

			if startLine < 1 {
				startLine = 1
			}
			if endLine <= 0 || endLine > len(lines) {
				endLine = len(lines)
			}
			if startLine > len(lines) {
				return fmt.Sprintf("File '%s' has %d lines (start_line %d out of range).", filepath.Base(cleanPath), len(lines), startLine), nil
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("--- File: %s (Lines %d-%d of %d) ---\n", filepath.Base(cleanPath), startLine, endLine, len(lines)))
			for i := startLine - 1; i < endLine; i++ {
				sb.WriteString(fmt.Sprintf("%4d | %s\n", i+1, lines[i]))
			}
			return sb.String(), nil
		},
	}
}

// GetWriteFileTool returns the Tier 1 native tool to create or overwrite files.
func GetWriteFileTool() adk.Tool {
	return adk.Tool{
		Name:        "write_file",
		Description: "Creates a new file or overwrites an existing file with complete string content. Automatically creates missing parent directories.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to target file (relative to workspace root or absolute path inside workspace).",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "Complete text or code content to write to the file.",
				},
			},
			"required": []string{"path", "content"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var raw map[string]interface{}
			_ = json.Unmarshal(args, &raw)

			targetPath := extractStringAlias(raw, "path", "target_file", "file_path", "file")
			if targetPath == "" {
				return "", fmt.Errorf("write_file: missing required parameter: 'path'")
			}
			content := extractStringAlias(raw, "content", "code_content", "text", "body")

			cleanPath, err := resolveSafeWorkspacePathWithCtx(ctx, targetPath)
			if err != nil {
				return "", fmt.Errorf("write_file: %w", err)
			}
			dir := filepath.Dir(cleanPath)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return "", fmt.Errorf("write_file: failed to create parent directories: %w", err)
			}

			// Atomic write pattern: write to temp file then rename
			tmpFile, err := os.CreateTemp(dir, ".tmp-write-*")
			if err != nil {
				// Fallback to direct write if temp file creation fails
				if err := os.WriteFile(cleanPath, []byte(content), 0644); err != nil {
					return "", fmt.Errorf("write_file: failed to write file: %w", err)
				}
				return fmt.Sprintf("Successfully wrote %d bytes to file '%s'.", len(content), targetPath), nil
			}
			tmpName := tmpFile.Name()

			if _, err := tmpFile.Write([]byte(content)); err != nil {
				tmpFile.Close()
				_ = os.Remove(tmpName)
				return "", fmt.Errorf("write_file: failed writing to temp file: %w", err)
			}
			tmpFile.Close()

			if err := os.Rename(tmpName, cleanPath); err != nil {
				_ = os.Remove(tmpName)
				// Fallback write
				if err := os.WriteFile(cleanPath, []byte(content), 0644); err != nil {
					return "", fmt.Errorf("write_file: failed to replace target file: %w", err)
				}
			}
			return fmt.Sprintf("Successfully wrote %d bytes atomically to file '%s'.", len(content), targetPath), nil
		},
	}
}

// GetReplaceFileContentTool returns the Tier 1 native tool for find and replace edits.
func GetReplaceFileContentTool() adk.Tool {
	return adk.Tool{
		Name:        "replace_file_content",
		Description: "Surgically replaces target text or code blocks inside a file with replacement content.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to target file to edit.",
				},
				"target_content": map[string]interface{}{
					"type":        "string",
					"description": "Exact text string or code block to search for and replace.",
				},
				"replacement_content": map[string]interface{}{
					"type":        "string",
					"description": "New replacement content to insert in place of target_content.",
				},
			},
			"required": []string{"path", "target_content", "replacement_content"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var raw map[string]interface{}
			_ = json.Unmarshal(args, &raw)

			targetPath := extractStringAlias(raw, "path", "target_file", "file_path", "file")
			if targetPath == "" {
				return "", fmt.Errorf("replace_file_content: missing required parameter: 'path'")
			}
			targetContent := extractStringAlias(raw, "target_content", "target", "search", "old_content", "find")
			replacementContent := extractStringAlias(raw, "replacement_content", "replacement", "replace", "new_content")
			if targetContent == "" {
				return "", fmt.Errorf("replace_file_content: missing required parameter: 'target_content'")
			}

			cleanPath, err := resolveSafeWorkspacePathWithCtx(ctx, targetPath)
			if err != nil {
				return "", fmt.Errorf("replace_file_content: %w", err)
			}

			fileBytes, err := os.ReadFile(cleanPath)
			if err != nil {
				if os.IsNotExist(err) || strings.Contains(err.Error(), "no such file") {
					_ = os.MkdirAll(filepath.Dir(cleanPath), 0755)
					if err := os.WriteFile(cleanPath, []byte(replacementContent), 0644); err != nil {
						return "", fmt.Errorf("replace_file_content: failed to create new file: %w", err)
					}
					return fmt.Sprintf("File '%s' did not exist; created new file with content.", filepath.Base(cleanPath)), nil
				}
				return "", fmt.Errorf("replace_file_content: failed to read file: %w", err)
			}

			origStr := string(fileBytes)
			if !strings.Contains(origStr, targetContent) {
				lines := strings.Split(origStr, "\n")
				firstFew := lines
				if len(firstFew) > 5 {
					firstFew = firstFew[:5]
				}
				preview := strings.Join(firstFew, "\n")
				return fmt.Sprintf("Error: target_content not found in file %q (%d lines total).\nPlease inspect file with 'read_file' to copy exact lines. First few lines:\n%s", filepath.Base(cleanPath), len(lines), preview), nil
			}

			newStr := strings.ReplaceAll(origStr, targetContent, replacementContent)
			replaced := strings.Count(origStr, targetContent)
			dir := filepath.Dir(cleanPath)
			tmpFile, err := os.CreateTemp(dir, ".tmp-replace-*")
			if err == nil {
				tmpName := tmpFile.Name()
				_, _ = tmpFile.Write([]byte(newStr))
				tmpFile.Close()
				if os.Rename(tmpName, cleanPath) == nil {
					return fmt.Sprintf("Successfully replaced %d occurrence(s) in %s", replaced, filepath.Base(cleanPath)), nil
				}
				_ = os.Remove(tmpName)
			}
			if err := os.WriteFile(cleanPath, []byte(newStr), 0644); err != nil {
				return "", fmt.Errorf("replace_file_content: failed to write updated content: %w", err)
			}
			return fmt.Sprintf("Successfully replaced %d occurrence(s) in %s", replaced, filepath.Base(cleanPath)), nil
		},
	}
}

// GetListDirectoryTool returns the Tier 1 native tool to list directory tree contents.
func GetListDirectoryTool() adk.Tool {
	return adk.Tool{
		Name:        "list_directory",
		Description: "Lists files and subdirectories contained within a directory path.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to directory (relative to workspace root or absolute path inside workspace).",
				},
			},
			"required": []string{"path"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var raw map[string]interface{}
			_ = json.Unmarshal(args, &raw)

			targetPath := extractStringAlias(raw, "path", "directory_path", "dir_path", "dir")
			if targetPath == "" {
				targetPath = "."
			}

			cleanPath, err := resolveSafeWorkspacePathWithCtx(ctx, targetPath)
			if err != nil {
				return "", fmt.Errorf("list_directory: %w", err)
			}

			entries, err := os.ReadDir(cleanPath)
			if err != nil {
				return "", fmt.Errorf("list_directory: failed to read directory: %w", err)
			}

			var filtered []os.DirEntry
			for _, entry := range entries {
				if !isSystemMetadataPath(entry.Name()) {
					filtered = append(filtered, entry)
				}
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("--- Directory Contents: %s (%d items) ---\n", targetPath, len(filtered)))
			for _, entry := range filtered {
				info, _ := entry.Info()
				sizeStr := ""
				if entry.IsDir() {
					sizeStr = "<DIR>"
				} else if info != nil {
					sizeStr = fmt.Sprintf("%d bytes", info.Size())
				}
				sb.WriteString(fmt.Sprintf("%-10s  %s\n", sizeStr, entry.Name()))
			}
			return sb.String(), nil
		},
	}
}
