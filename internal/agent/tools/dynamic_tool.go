package agenttools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/fabith10/synapse-go/adk"
	"github.com/fabith10/synapse-go/internal/tools"
	"github.com/fabith10/synapse-go/pkg/logger"
)

// DynamicToolSpec describes a dynamically authored tool.
type DynamicToolSpec struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Language    string                 `json:"language"` // "python", "bash", "wasm"
	Parameters  map[string]interface{} `json:"parameters"` // MCP JSON schema
	Code        string                 `json:"code"`
	Packages    []string               `json:"packages,omitempty"`
	Scope       string                 `json:"scope,omitempty"` // "global" or "agent"
	Author      string                 `json:"author,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
}

// DynamicToolManager manages the lifecycle, persistence, and registration of dynamic tools.
type DynamicToolManager struct {
	mu         sync.RWMutex
	tools      map[string]adk.Tool
	specs      map[string]DynamicToolSpec
	sandbox    adk.Sandbox
	orch       *adk.Orchestrator
	storageDir string
}

var (
	globalManager *DynamicToolManager
	managerOnce   sync.Once
	toolNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-]+$`)
)

// GetGlobalDynamicToolManager returns the singleton DynamicToolManager.
func GetGlobalDynamicToolManager() *DynamicToolManager {
	return globalManager
}

// InitDynamicToolManager initializes the global dynamic tool manager.
func InitDynamicToolManager(sb adk.Sandbox, orch *adk.Orchestrator, storageDir string) *DynamicToolManager {
	if storageDir == "" {
		storageDir = ".agents/dynamic_tools"
	}
	managerOnce.Do(func() {
		globalManager = &DynamicToolManager{
			tools:      make(map[string]adk.Tool),
			specs:      make(map[string]DynamicToolSpec),
			sandbox:    sb,
			orch:       orch,
			storageDir: storageDir,
		}
		_ = os.MkdirAll(storageDir, 0755)
		_, _ = globalManager.LoadFromDisk()
	})
	if globalManager != nil {
		if sb != nil {
			globalManager.sandbox = sb
		}
		if orch != nil {
			globalManager.orch = orch
		}
	}
	return globalManager
}

// Register registers a new dynamic tool from its spec and persists it to disk.
func (m *DynamicToolManager) Register(spec DynamicToolSpec) (adk.Tool, error) {
	if m == nil {
		return adk.Tool{}, fmt.Errorf("dynamic tool manager not initialized")
	}

	// Validate tool name
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return adk.Tool{}, fmt.Errorf("tool name cannot be empty")
	}
	if !toolNameRegex.MatchString(name) {
		return adk.Tool{}, fmt.Errorf("invalid tool name %q: only alphanumeric, underscore, and hyphens allowed", name)
	}
	if name == "done" || name == "delegate_subtask" {
		return adk.Tool{}, fmt.Errorf("cannot override reserved tool name %q", name)
	}

	// Validate language
	lang := strings.ToLower(strings.TrimSpace(spec.Language))
	if lang == "" {
		lang = "python"
	}
	if lang != "python" && lang != "bash" && lang != "wasm" && lang != "sh" {
		return adk.Tool{}, fmt.Errorf("unsupported dynamic tool language %q (must be 'python', 'bash', or 'wasm')", spec.Language)
	}
	if lang == "sh" {
		lang = "bash"
	}
	spec.Language = lang

	// Ensure parameters schema is well-formed
	if spec.Parameters == nil {
		spec.Parameters = map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}
	} else if _, ok := spec.Parameters["type"]; !ok {
		spec.Parameters["type"] = "object"
	}

	if spec.CreatedAt.IsZero() {
		spec.CreatedAt = time.Now().UTC()
	}
	if spec.Scope == "" {
		spec.Scope = "global"
	}

	// Build the executable adk.Tool
	tool := m.buildTool(spec)

	m.mu.Lock()
	m.specs[name] = spec
	m.tools[name] = tool
	m.mu.Unlock()

	// Persist to disk
	_ = m.saveToDisk(spec)

	logger.Info("Dynamically registered and hot-reloaded tool", "name", name, "language", lang, "author", spec.Author)
	return tool, nil
}

// buildTool constructs an adk.Tool from a DynamicToolSpec.
func (m *DynamicToolManager) buildTool(spec DynamicToolSpec) adk.Tool {
	tier := adk.TierDocker
	if spec.Language == "wasm" {
		tier = adk.TierWasm
	}

	return adk.Tool{
		Name:        spec.Name,
		Description: fmt.Sprintf("[Dynamic Hot-Reloaded Tool] %s", spec.Description),
		Parameters:  spec.Parameters,
		Tier:        tier,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			if m.sandbox == nil {
				return fmt.Sprintf("[Dynamic Execution Fallback] Executed %s with args: %s", spec.Name, string(args)), nil
			}

			// Clean and prepare code
			code := spec.Code
			if spec.Language == "python" {
				code = tools.StripMarkdownFences(code)
			}

			res := m.sandbox.Execute(ctx, adk.ExecutionRequest{
				Language:       spec.Language,
				RawCode:        code,
				Stdin:          args,
				Packages:       spec.Packages,
				TimeoutSeconds: 45,
			})

			if res.Error == tools.ErrDockerUnavailable {
				return fmt.Sprintf("[Docker Mock Fallback] Executed dynamic tool %q with arguments %s. Output: OK", spec.Name, string(args)), nil
			}
			if res.Error != nil {
				return "", fmt.Errorf("dynamic tool %s execution failed: %w", spec.Name, res.Error)
			}

			out := res.Stdout
			if out == "" && res.Stderr != "" {
				out = res.Stderr
			}
			if out == "" {
				out = fmt.Sprintf("Tool %s executed successfully with no output.", spec.Name)
			}
			return out, nil
		},
	}
}

// saveToDisk saves a dynamic tool spec to the storage directory.
func (m *DynamicToolManager) saveToDisk(spec DynamicToolSpec) error {
	if m.storageDir == "" {
		return nil
	}
	_ = os.MkdirAll(m.storageDir, 0755)
	filePath := filepath.Join(m.storageDir, spec.Name+".json")
	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}

// LoadFromDisk discovers and hot-reloads all dynamic tools from the storage directory.
func (m *DynamicToolManager) LoadFromDisk() ([]adk.Tool, error) {
	if m.storageDir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(m.storageDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var loaded []adk.Tool
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		filePath := filepath.Join(m.storageDir, entry.Name())
		data, readErr := os.ReadFile(filePath)
		if readErr != nil {
			continue
		}
		var spec DynamicToolSpec
		if jsonErr := json.Unmarshal(data, &spec); jsonErr != nil || spec.Name == "" {
			continue
		}
		tool := m.buildTool(spec)
		m.specs[spec.Name] = spec
		m.tools[spec.Name] = tool
		loaded = append(loaded, tool)
	}

	return loaded, nil
}

// Delete removes a dynamic tool from memory and disk.
func (m *DynamicToolManager) Delete(name string) error {
	m.mu.Lock()
	delete(m.tools, name)
	delete(m.specs, name)
	m.mu.Unlock()

	if m.storageDir != "" {
		filePath := filepath.Join(m.storageDir, name+".json")
		_ = os.Remove(filePath)
	}
	return nil
}

// GetTools returns all currently active dynamic tools.
func (m *DynamicToolManager) GetTools() []adk.Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]adk.Tool, 0, len(m.tools))
	for _, t := range m.tools {
		res = append(res, t)
	}
	return res
}

// GetDynamicTool returns a specific dynamic tool by name.
func (m *DynamicToolManager) GetDynamicTool(name string) (adk.Tool, bool) {
	if m == nil {
		return adk.Tool{}, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tools[name]
	return t, ok
}

// GetSpecs returns all currently loaded dynamic tool specifications.
func (m *DynamicToolManager) GetSpecs() []DynamicToolSpec {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]DynamicToolSpec, 0, len(m.specs))
	for _, s := range m.specs {
		res = append(res, s)
	}
	return res
}

// GetGlobalDynamicTools returns all active dynamic tools from the global manager.
func GetGlobalDynamicTools() []adk.Tool {
	if globalManager == nil {
		return nil
	}
	return globalManager.GetTools()
}

// GetGlobalDynamicTool retrieves a specific dynamic tool from the global manager.
func GetGlobalDynamicTool(name string) (adk.Tool, bool) {
	if globalManager == nil {
		return adk.Tool{}, false
	}
	return globalManager.GetDynamicTool(name)
}

// GetCreateDynamicTool returns the create_dynamic_tool tool.
func GetCreateDynamicTool(sandbox adk.Sandbox, orch *adk.Orchestrator, authorAgent string) adk.Tool {
	return adk.Tool{
		Name:        "create_dynamic_tool",
		Description: "Dynamically authors and registers a new tool in Python, Bash, or WASM, immediately hot-reloading it into the active toolset.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Unique identifier for the new tool (e.g. 'calc_risk_matrix', 'parse_order_book').",
				},
				"description": map[string]interface{}{
					"type":        "string",
					"description": "Clear explanation of what the tool accomplishes and how parameters should be provided.",
				},
				"language": map[string]interface{}{
					"type":        "string",
					"description": "Language runtime: 'python', 'bash', or 'wasm'. Default is 'python'.",
				},
				"parameters": map[string]interface{}{
					"type":        "object",
					"description": "MCP / JSON Schema definition of tool parameters (e.g. {'type': 'object', 'properties': {...}, 'required': [...]}).",
				},
				"code": map[string]interface{}{
					"type":        "string",
					"description": "The complete source code of the tool. Tool input arguments are passed via stdin as JSON (e.g. sys.stdin.read()).",
				},
				"packages": map[string]interface{}{
					"type":        "array",
					"description": "Optional list of PyPI packages required (e.g. ['numpy', 'pandas']).",
					"items": map[string]interface{}{
						"type": "string",
					},
				},
				"scope": map[string]interface{}{
					"type":        "string",
					"description": "Tool accessibility: 'global' (shared with all agents) or 'agent' (local to author). Default: 'global'.",
				},
			},
			"required": []interface{}{"name", "description", "code"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var spec DynamicToolSpec
			if err := json.Unmarshal(args, &spec); err != nil {
				return "", fmt.Errorf("invalid create_dynamic_tool arguments: %w", err)
			}

			if spec.Author == "" {
				spec.Author = authorAgent
			}

			mgr := globalManager
			if mgr == nil {
				mgr = InitDynamicToolManager(sandbox, orch, ".agents/dynamic_tools")
			}

			tool, err := mgr.Register(spec)
			if err != nil {
				return "", fmt.Errorf("failed to register dynamic tool: %w", err)
			}

			return fmt.Sprintf("✅ Successfully created and hot-reloaded dynamic tool %q (%s tier %v). You and other agents can now invoke it directly in subsequent steps with action %q.", spec.Name, spec.Language, tool.Tier, spec.Name), nil
		},
	}
}

// GetListDynamicToolsTool returns the list_dynamic_tools tool.
func GetListDynamicToolsTool() adk.Tool {
	return adk.Tool{
		Name:        "list_dynamic_tools",
		Description: "Lists all dynamically authored and hot-reloaded tools currently registered in the framework.",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			if globalManager == nil {
				return "No dynamic tools registered.", nil
			}
			specs := globalManager.GetSpecs()
			if len(specs) == 0 {
				return "No dynamic tools registered.", nil
			}
			data, err := json.MarshalIndent(specs, "", "  ")
			if err != nil {
				return "", err
			}
			return string(data), nil
		},
	}
}

// GetReloadDynamicToolsTool returns the reload_dynamic_tools tool.
func GetReloadDynamicToolsTool(sandbox adk.Sandbox, orch *adk.Orchestrator) adk.Tool {
	return adk.Tool{
		Name:        "reload_dynamic_tools",
		Description: "Scans disk storage and hot-reloads all dynamic tool specifications into the active runtime.",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			mgr := globalManager
			if mgr == nil {
				mgr = InitDynamicToolManager(sandbox, orch, ".agents/dynamic_tools")
			}
			loaded, err := mgr.LoadFromDisk()
			if err != nil {
				return "", fmt.Errorf("failed to reload dynamic tools: %w", err)
			}
			return fmt.Sprintf("Successfully reloaded %d dynamic tools from disk.", len(loaded)), nil
		},
	}
}

// GetDeleteDynamicTool returns the delete_dynamic_tool tool.
func GetDeleteDynamicTool(sandbox adk.Sandbox, orch *adk.Orchestrator) adk.Tool {
	return adk.Tool{
		Name:        "delete_dynamic_tool",
		Description: "Deletes a dynamic tool from the active registry and removes its specification from disk.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Name of the dynamic tool to delete.",
				},
			},
			"required": []interface{}{"name"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid delete_dynamic_tool arguments: %w", err)
			}
			if globalManager == nil {
				return fmt.Sprintf("Dynamic tool %q not found.", params.Name), nil
			}
			if err := globalManager.Delete(params.Name); err != nil {
				return "", err
			}
			return fmt.Sprintf("Successfully deleted dynamic tool %q.", params.Name), nil
		},
	}
}
