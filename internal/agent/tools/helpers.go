package agenttools

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/fabith10/synapse-go/internal/sanitizer"
)

// hitlCorrCounter is an atomic counter used to generate unique HITL correlation IDs.
var hitlCorrCounter uint64

// nextHITLCorrID returns a unique correlation ID for human-in-the-loop approval requests.
func nextHITLCorrID() uint64 {
	return atomic.AddUint64(&hitlCorrCounter, 1)
}

// MinimalWasmBinary represents a minimal valid WebAssembly module used in Wasm sandbox tests.
var MinimalWasmBinary = []byte{
	0x00, 0x61, 0x73, 0x6d, // \0asm
	0x01, 0x00, 0x00, 0x00, // version 1
}

// workspaceRootKeyType is the unexported key type for WorkspaceRootKey.
type workspaceRootKeyType struct{}

// WorkspaceRootKey is the context.Context key holding the active target workspace root path.
var WorkspaceRootKey = workspaceRootKeyType{}

// isFrameworkSourcePath detects framework core engine source directories and files
// so workspace agent tasks never read or tamper with the host engine code.
func isFrameworkSourcePath(path string) bool {
	if os.Getenv("AGENT_FRAMEWORK_TESTING") == "true" {
		return false
	}
	cleaned := strings.ToLower(filepath.Clean(path))
	parts := strings.Split(cleaned, string(filepath.Separator))
	for i, part := range parts {
		if part == "internal" || part == "adk" || part == "cmd" || part == "pkg" || part == "vendor" {
			return true
		}
		if i == len(parts)-1 {
			if part == "go.mod" || part == "go.sum" || part == "main.go" || strings.HasSuffix(part, ".go") {
				return true
			}
		}
	}
	return false
}

// isSystemMetadataPath detects framework metadata files and sensitive credentials
// so workspace LLM tools cannot read or leak them.
func isSystemMetadataPath(path string) bool {
	if isFrameworkSourcePath(path) {
		return true
	}
	if sanitizer.IsGitIgnoredPath(path) {
		return true
	}
	cleaned := strings.ToLower(filepath.Clean(path))
	parts := strings.Split(cleaned, string(filepath.Separator))
	for _, part := range parts {
		if part == ".agents" || part == ".gemini" || part == ".git" || part == ".aws" || part == ".ssh" || part == ".kube" || part == ".docker" ||
			part == "agents.md" || part == "gemini.md" || part == "agent.md" ||
			part == ".env" || strings.HasPrefix(part, ".env.") ||
			part == "id_rsa" || part == "id_ed25519" || strings.HasSuffix(part, ".pem") || strings.HasSuffix(part, ".key") {
			return true
		}
	}
	return false
}

// resolveSafeWorkspacePathWithCtx cleans a path and enforces context-aware workspace boundary checks.
func resolveSafeWorkspacePathWithCtx(ctx context.Context, targetPath string) (string, error) {
	cwd, _ := os.Getwd()
	activeRoot := cwd

	if ctx != nil {
		if rootVal, ok := ctx.Value(WorkspaceRootKey).(string); ok && strings.TrimSpace(rootVal) != "" {
			activeRoot = filepath.Clean(rootVal)
		}
	}

	isTest := os.Getenv("AGENT_FRAMEWORK_TESTING") == "true" || strings.HasSuffix(os.Args[0], ".test") || flag.Lookup("test.v") != nil
	cleaned := filepath.Clean(targetPath)

	var resolved string
	if filepath.IsAbs(targetPath) {
		resolved = cleaned
	} else {
		rel := strings.TrimLeft(targetPath, "/\\")
		resolved = filepath.Clean(filepath.Join(activeRoot, rel))
	}

	if realRoot, err := filepath.EvalSymlinks(activeRoot); err == nil {
		activeRoot = realRoot
	}
	if realResolved, err := filepath.EvalSymlinks(resolved); err == nil {
		resolved = realResolved
	}

	isInsideActiveRoot := strings.HasPrefix(resolved, activeRoot)
	isInsideCwd := strings.HasPrefix(resolved, cwd)

	isDevWorkspace := false
	if filepath.IsAbs(resolved) {
		parentDev := filepath.Dir(cwd)
		if strings.HasPrefix(resolved, parentDev) && !isSystemMetadataPath(resolved) {
			isDevWorkspace = true
		}
	}

	tempDir := os.TempDir()
	if realTemp, err := filepath.EvalSymlinks(tempDir); err == nil {
		tempDir = realTemp
	}
	isTemp := strings.HasPrefix(resolved, os.TempDir()) || strings.HasPrefix(resolved, tempDir)

	isSafe := isInsideActiveRoot || isInsideCwd || isDevWorkspace || (isTest && isTemp)
	if !isSafe {
		return "", fmt.Errorf("permission denied: path %q must remain inside active workspace (%q)", targetPath, activeRoot)
	}
	return resolved, nil
}

// resolveSafeWorkspacePath cleans a path and enforces workspace boundary check.
func resolveSafeWorkspacePath(targetPath string) (string, error) {
	return resolveSafeWorkspacePathWithCtx(context.Background(), targetPath)
}

// extractStringAlias looks for a string value under any of the given key names in a map.
// It also handles nested "action_input" / "parameters" containers and raw JSON strings.
func extractStringAlias(m map[string]interface{}, keys ...string) string {
	if m == nil {
		return ""
	}
	for _, k := range keys {
		if val, ok := m[k]; ok && val != nil {
			if s, ok := val.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	for _, containerKey := range []string{"action_input", "parameters", "args", "input", "payload", "data"} {
		if containerVal, ok := m[containerKey]; ok && containerVal != nil {
			if nestedMap, isMap := containerVal.(map[string]interface{}); isMap {
				for _, k := range keys {
					if val, ok := nestedMap[k]; ok && val != nil {
						if s, ok := val.(string); ok && strings.TrimSpace(s) != "" {
							return strings.TrimSpace(s)
						}
					}
				}
			} else if strVal, isStr := containerVal.(string); isStr {
				trimmedStr := strings.TrimSpace(strVal)
				if trimmedStr != "" {
					if strings.HasPrefix(trimmedStr, "{") && strings.HasSuffix(trimmedStr, "}") {
						var parsed map[string]interface{}
						if err := json.Unmarshal([]byte(trimmedStr), &parsed); err == nil {
							for _, k := range keys {
								if val, ok := parsed[k]; ok && val != nil {
									if s, ok := val.(string); ok && strings.TrimSpace(s) != "" {
										return strings.TrimSpace(s)
									}
								}
							}
						}
					}
					return trimmedStr
				}
			}
		}
	}
	return ""
}

// extractIntAlias looks for an integer value under any of the given key names.
func extractIntAlias(m map[string]interface{}, keys ...string) int {
	if m == nil {
		return 0
	}
	for _, k := range keys {
		if val, ok := m[k]; ok && val != nil {
			switch v := val.(type) {
			case float64:
				return int(v)
			case int:
				return v
			case string:
				if n, err := strconv.Atoi(v); err == nil {
					return n
				}
			}
		}
	}
	return 0
}

// extractFloatAlias looks for a float64 value under any of the given key names.
func extractFloatAlias(m map[string]interface{}, keys ...string) float64 {
	if m == nil {
		return 0
	}
	for _, k := range keys {
		if val, ok := m[k]; ok && val != nil {
			switch v := val.(type) {
			case float64:
				return v
			case float32:
				return float64(v)
			case int:
				return float64(v)
			case int64:
				return float64(v)
			case string:
				if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
					return f
				}
			}
		}
	}
	return 0
}
