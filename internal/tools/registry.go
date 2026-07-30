package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// ---------------------------------------------------------------------------
// ToolRegistry
// ---------------------------------------------------------------------------

// Registry is the central catalogue of all tools available to agents. It
// provides thread-safe registration and execution dispatch, and is responsible
// for generating the MCP JSON schemas that LLM backends consume.
//
// Concurrency model:
//   - mu guards the tools map; concurrent Register and Execute calls are safe.
//   - Tool.Execute functions are called without holding mu, so a slow tool
//     never blocks registration of another tool.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry constructs an empty, thread-safe Tool Registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry. If a tool with the same Name already
// exists, it is overwritten and an error is returned so the caller can log the
// collision. Register is safe to call after the agent event loop has started.
func (r *Registry) Register(tool Tool) error {
	if tool.Name == "" {
		return fmt.Errorf("registry: tool name must not be empty")
	}
	if tool.Execute == nil {
		return fmt.Errorf("registry: tool %q has a nil Execute func", tool.Name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[tool.Name]; exists {
		r.tools[tool.Name] = tool
		return fmt.Errorf("registry: tool %q already registered; overwriting", tool.Name)
	}

	r.tools[tool.Name] = tool
	return nil
}

// Execute looks up the tool by name and calls its Execute function with the
// provided JSON-encoded arguments. Returns ErrToolNotFound if no tool is
// registered with that name.
//
// Context-first: the ctx deadline is forwarded to tool.Execute so every tool
// automatically inherits the agent's task-level timeout.
func (r *Registry) Execute(ctx context.Context, name string, args []byte) (string, error) {
	r.mu.RLock()
	tool, ok := r.tools[name]
	r.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("%w: %q", ErrToolNotFound, name)
	}

	// Validate arguments against tool schema
	if len(args) > 0 && string(args) != "{}" && string(args) != "null" {
		if err := ValidateArguments(tool, args); err != nil {
			return "", fmt.Errorf("input validation failed: %w", err)
		}
	}

	return tool.Execute(ctx, args)
}

// Get returns a copy of the named tool. Returns ErrToolNotFound if not
// registered. Useful for inspection without triggering execution.
func (r *Registry) Get(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, ok := r.tools[name]
	if !ok {
		return Tool{}, fmt.Errorf("%w: %q", ErrToolNotFound, name)
	}
	return tool, nil
}

// Names returns the sorted list of all registered tool names.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// ---------------------------------------------------------------------------
// MCP Schema generation (PDR-004 §3)
// ---------------------------------------------------------------------------

// Schema returns the MCP JSON schema struct for a single tool. Returns
// ErrToolNotFound if the tool is not registered.
func (r *Registry) Schema(name string) (MCPSchema, error) {
	tool, err := r.Get(name)
	if err != nil {
		return MCPSchema{}, err
	}
	return buildSchema(tool), nil
}

// AllSchemas returns the MCP schema for every registered tool. The slice is
// passed directly to LLMProvider.FormatPrompt so the LLM can select the
// correct tool at inference time.
func (r *Registry) AllSchemas() []MCPSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()

	schemas := make([]MCPSchema, 0, len(r.tools))
	for _, tool := range r.tools {
		schemas = append(schemas, buildSchema(tool))
	}
	return schemas
}

// AllSchemasJSON returns all tool schemas serialised as a JSON array. This is
// the wire format used by providers that accept a raw JSON tool list.
func (r *Registry) AllSchemasJSON() ([]byte, error) {
	schemas := r.AllSchemas()
	data, err := json.Marshal(schemas)
	if err != nil {
		return nil, fmt.Errorf("registry: marshal schemas: %w", err)
	}
	return data, nil
}

// buildSchema converts a Tool definition into the MCP wire schema.
//
// Required parameters are extracted from the "required" key in Parameters
// if present (as []string or []interface{}). All other entries in Parameters
// are treated as property definitions and placed under "properties".
func buildSchema(tool Tool) MCPSchema {
	props := make(map[string]interface{})
	var required []string

	for key, val := range tool.Parameters {
		if key == "required" {
			// Extract required field list stored by convention in Parameters.
			switch v := val.(type) {
			case []string:
				required = v
			case []interface{}:
				for _, item := range v {
					if s, ok := item.(string); ok {
						required = append(required, s)
					}
				}
			}
			continue
		}
		props[key] = val
	}

	return MCPSchema{
		Name:        tool.Name,
		Description: tool.Description,
		InputSchema: MCPInputSchema{
			Type:       "object",
			Properties: props,
			Required:   required,
		},
	}
}

// ValidateArguments verifies that the provided JSON-encoded arguments match the tool's Parameter schemas.
func ValidateArguments(tool Tool, args []byte) error {
	var input map[string]interface{}
	if err := json.Unmarshal(args, &input); err != nil {
		repaired := RepairJSON(string(args))
		if errRep := json.Unmarshal([]byte(repaired), &input); errRep != nil {
			return fmt.Errorf("invalid json payload: %w", err)
		}
	}

	// Extract required properties
	var required []string
	if reqVal, exists := tool.Parameters["required"]; exists {
		switch v := reqVal.(type) {
		case []string:
			required = v
		case []interface{}:
			for _, item := range v {
				if s, ok := item.(string); ok {
					required = append(required, s)
				}
			}
		}
	}

	// Normalize common parameter aliases before checking required parameters
	normalizeInputAliases(input)

	for _, reqName := range required {
		val, exists := input[reqName]
		if !exists || val == nil {
			return fmt.Errorf("missing required parameter: %q", reqName)
		}
		if strVal, ok := val.(string); ok && strings.TrimSpace(strVal) == "" {
			return fmt.Errorf("missing required parameter: %q", reqName)
		}
	}

	// Validate parameter types
	if propsVal, exists := tool.Parameters["properties"]; exists {
		if props, ok := propsVal.(map[string]interface{}); ok {
			for key, val := range input {
				if propSchemaVal, ok := props[key]; ok {
					if propSchema, ok := propSchemaVal.(map[string]interface{}); ok {
						if expectedType, ok := propSchema["type"].(string); ok {
							if err := validateType(key, val, expectedType); err != nil {
								return err
							}
						}
					}
				}
			}
		}
	}

	return nil
}

func validateType(name string, val interface{}, expected string) error {
	switch expected {
	case "string":
		// String validation passes for strings or stringifiable primitives
		return nil
	case "integer", "number":
		switch v := val.(type) {
		case float64, float32, int, int64, int32:
			return nil
		case string:
			var f float64
			if _, err := fmt.Sscanf(v, "%f", &f); err == nil {
				return nil
			}
			return fmt.Errorf("parameter %q expected number, got non-numeric string %q", name, v)
		default:
			return fmt.Errorf("parameter %q expected number, got %T", name, val)
		}
	case "boolean":
		switch v := val.(type) {
		case bool:
			return nil
		case string:
			lower := strings.ToLower(v)
			if lower == "true" || lower == "false" || lower == "1" || lower == "0" {
				return nil
			}
			return fmt.Errorf("parameter %q expected boolean, got %q", name, v)
		case float64, int:
			return nil
		default:
			return fmt.Errorf("parameter %q expected boolean, got %T", name, val)
		}
	case "array":
		if _, ok := val.([]interface{}); !ok {
			return fmt.Errorf("parameter %q expected array, got %T", name, val)
		}
	case "object":
		if _, ok := val.(map[string]interface{}); !ok {
			return fmt.Errorf("parameter %q expected object, got %T", name, val)
		}
	}
	return nil
}

// SecureTool wraps a Tool's Execute function with context-based RBAC checks and parameter input validation.
func SecureTool(tool Tool, allowedAgents []string) Tool {
	originalExecute := tool.Execute
	tool.Execute = func(ctx context.Context, args []byte) (string, error) {
		// 1. RBAC authorization check
		if len(allowedAgents) > 0 {
			agentID, _ := ctx.Value("executing_agent_id").(string)
			authorized := false
			for _, allowed := range allowedAgents {
				if allowed == agentID {
					authorized = true
					break
				}
			}
			if !authorized {
				return "", fmt.Errorf("security policy violation: agent %q is not authorized to run tool %q", agentID, tool.Name)
			}
		}

		// 2. Input validation check
		if len(args) > 0 && string(args) != "{}" && string(args) != "null" {
			if err := ValidateArguments(tool, args); err != nil {
				return "", fmt.Errorf("input validation failed: %w", err)
			}
		}

		return originalExecute(ctx, args)
	}
	return tool
}

func normalizeInputAliases(input map[string]interface{}) {
	// 1. Unpack nested containers (action_input, parameters, args, input, payload, data)
	for _, container := range []string{"action_input", "parameters", "args", "input", "payload", "data"} {
		if containerVal, ok := input[container]; ok && containerVal != nil {
			if inner, ok := containerVal.(map[string]interface{}); ok {
				for k, v := range inner {
					if existing, exists := input[k]; !exists || existing == nil || existing == "" {
						input[k] = v
					}
				}
			} else if strVal, ok := containerVal.(string); ok && strings.TrimSpace(strVal) != "" {
				trimmed := strings.TrimSpace(strVal)
				if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
					var parsed map[string]interface{}
					if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
						for k, v := range parsed {
							if existing, exists := input[k]; !exists || existing == nil || existing == "" {
								input[k] = v
							}
						}
					}
				} else {
					for _, targetKey := range []string{"python_code", "bash_script", "path", "expression", "query", "url"} {
						if existing, exists := input[targetKey]; !exists || existing == nil || existing == "" {
							input[targetKey] = trimmed
							break
						}
					}
				}
			}
		}
	}

	aliasMap := map[string][]string{
		"python_code":         {"script", "code", "python", "python_script", "code_script", "source"},
		"bash_script":         {"command", "cmd", "script", "bash", "bash_code", "shell"},
		"path":                {"target_file", "file_path", "file", "filepath", "filename", "path_to_file", "location"},
		"target_content":      {"target", "search", "old_content", "find"},
		"replacement_content": {"replacement", "replace", "new_content"},
		"recipient":           {"to", "email", "receiver"},
		"query":               {"search", "pattern", "term"},
		"element_index":       {"element_id", "index", "id"},
		"selector":            {"element", "target_selector", "css"},
	}

	for canonical, aliases := range aliasMap {
		existingVal, exists := input[canonical]
		isEmpty := !exists || existingVal == nil
		if strVal, ok := existingVal.(string); ok && strings.TrimSpace(strVal) == "" {
			isEmpty = true
		}
		if isEmpty {
			for _, alias := range aliases {
				if val, ok := input[alias]; ok && val != nil {
					if s, ok := val.(string); ok && strings.TrimSpace(s) != "" {
						input[canonical] = strings.TrimSpace(s)
						break
					} else {
						input[canonical] = val
						break
					}
				}
			}
		}
	}
}

