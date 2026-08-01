package agenttools

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

var (
	toolAliasMu sync.RWMutex
	toolAliases = map[string]string{
		"read_page":            "fetch_html",
		"search":               "web_search_and_extract",
		"execute_bash":         "execute_bash_docker",
		"grep_docs":            "grep_documents",
		"grep":                 "grep_documents",
		"search_files":         "grep_documents",
		"navigate":             "browser_navigate",
		"input":                "browser_input",
		"click":                "browser_click",
		"scroll":               "browser_scroll",
		"wait":                 "browser_wait",
		"extract_js":           "browser_extract_js",
		"screenshot":           "browser_screenshot",
		"view_file":            "read_file",
		"cat":                  "read_file",
		"read_file_content":    "read_file",
		"create_file":          "write_file",
		"write_to_file":        "write_file",
		"edit_file":            "replace_file_content",
		"replace_content":      "replace_file_content",
		"modify_file":          "replace_file_content",
		"ls":                   "list_directory",
		"list_dir":             "list_directory",
		"dir":                  "list_directory",
	}
)

// SetToolAlias registers or overrides a single tool alias mapping.
func SetToolAlias(alias, target string) {
	toolAliasMu.Lock()
	defer toolAliasMu.Unlock()
	toolAliases[strings.ToLower(alias)] = strings.ToLower(target)
}

// SetToolAliases replaces or extends the registered tool alias map.
func SetToolAliases(aliases map[string]string) {
	toolAliasMu.Lock()
	defer toolAliasMu.Unlock()
	for k, v := range aliases {
		toolAliases[strings.ToLower(k)] = strings.ToLower(v)
	}
}

// ResolveToolAlias looks up a target tool name for a given alias.
// Returns the resolved target tool name if an alias exists, or the original alias string.
func ResolveToolAlias(alias string) string {
	toolAliasMu.RLock()
	defer toolAliasMu.RUnlock()
	lower := strings.ToLower(alias)
	if target, ok := toolAliases[lower]; ok {
		return target
	}
	return lower
}

// GetToolAliases returns a snapshot copy of all configured tool aliases.
func GetToolAliases() map[string]string {
	toolAliasMu.RLock()
	defer toolAliasMu.RUnlock()
	snapshot := make(map[string]string, len(toolAliases))
	for k, v := range toolAliases {
		snapshot[k] = v
	}
	return snapshot
}

// LoadToolAliasesConfig loads custom tool aliases from a JSON file.
// The JSON file can be either a map of aliases `{"alias": "target"}`
// or an object `{"aliases": {"alias": "target"}}`.
func LoadToolAliasesConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read tool aliases file: %w", err)
	}

	// Try unmarshaling as map[string]string first
	var directMap map[string]string
	if err := json.Unmarshal(data, &directMap); err == nil && len(directMap) > 0 {
		SetToolAliases(directMap)
		return nil
	}

	// Try unmarshaling as struct {"aliases": map[string]string}
	var wrapper struct {
		Aliases map[string]string `json:"aliases"`
	}
	if err := json.Unmarshal(data, &wrapper); err == nil && len(wrapper.Aliases) > 0 {
		SetToolAliases(wrapper.Aliases)
		return nil
	}

	return fmt.Errorf("invalid tool aliases JSON schema in %s", path)
}
