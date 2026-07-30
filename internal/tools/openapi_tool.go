package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// OpenAPISpecMinimal represents a subset of an OpenAPI 3.0 specification for tool generation.
type OpenAPISpecMinimal struct {
	Paths map[string]map[string]OpenAPIOperation `json:"paths"`
}

// OpenAPIOperation represents an HTTP operation in an OpenAPI specification.
type OpenAPIOperation struct {
	OperationID string                 `json:"operationId"`
	Summary     string                 `json:"summary"`
	Description string                 `json:"description"`
	Parameters  []OpenAPIParameter     `json:"parameters"`
}

// OpenAPIParameter represents a parameter in an OpenAPI operation.
type OpenAPIParameter struct {
	Name        string `json:"name"`
	In          string `json:"in"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// GenerateToolsFromOpenAPI parses an OpenAPI 3.0 JSON specification and generates Tool definitions.
func GenerateToolsFromOpenAPI(specJSON []byte, httpExecutor func(ctx context.Context, method, path string, args map[string]interface{}) (string, error)) ([]Tool, error) {
	var spec OpenAPISpecMinimal
	if err := json.Unmarshal(specJSON, &spec); err != nil {
		return nil, fmt.Errorf("openapi: unmarshal spec: %w", err)
	}

	var generatedTools []Tool

	for path, methods := range spec.Paths {
		for method, op := range methods {
			name := op.OperationID
			if name == "" {
				name = fmt.Sprintf("%s_%s", method, path)
			}

			desc := op.Summary
			if desc == "" {
				desc = op.Description
			}

			paramsMap := make(map[string]interface{})
			var requiredFields []string

			for _, p := range op.Parameters {
				paramsMap[p.Name] = map[string]interface{}{
					"type":        "string",
					"description": p.Description,
				}
				if p.Required {
					requiredFields = append(requiredFields, p.Name)
				}
			}
			if len(requiredFields) > 0 {
				paramsMap["required"] = requiredFields
			}

			currentPath := path
			currentMethod := method

			tool := Tool{
				Name:        name,
				Description: desc,
				Parameters:  paramsMap,
				Execute: func(ctx context.Context, args []byte) (string, error) {
					var argMap map[string]interface{}
					if len(args) > 0 {
						_ = json.Unmarshal(args, &argMap)
					}
					if httpExecutor != nil {
						return httpExecutor(ctx, currentMethod, currentPath, argMap)
					}
					return fmt.Sprintf("Simulated %s request to %s with args: %s", currentMethod, currentPath, string(args)), nil
				},
			}

			generatedTools = append(generatedTools, tool)
		}
	}

	return generatedTools, nil
}
