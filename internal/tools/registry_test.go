package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/fabith10/agent-framework/internal/tools"
)

// ---------------------------------------------------------------------------
// Registry — Tier 1 (Native) tests
// ---------------------------------------------------------------------------

func TestRegistry_Register_And_Execute_Tier1(t *testing.T) {
	reg := tools.NewRegistry()

	called := false
	err := reg.Register(tools.Tool{
		Name:        "echo",
		Description: "Returns the input as-is.",
		Tier:        tools.TierNative,
		Parameters: map[string]interface{}{
			"text": map[string]string{"type": "string", "description": "text to echo"},
		},
		Execute: func(ctx context.Context, args []byte) (string, error) {
			called = true
			return string(args), nil
		},
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	result, err := reg.Execute(context.Background(), "echo", []byte(`{"text":"hello"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !called {
		t.Error("Execute func was not called")
	}
	if result != `{"text":"hello"}` {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestRegistry_Execute_NotFound(t *testing.T) {
	reg := tools.NewRegistry()

	_, err := reg.Execute(context.Background(), "nonexistent_tool", nil)
	if err == nil {
		t.Fatal("expected error for unknown tool, got nil")
	}
}

func TestRegistry_Register_Overwrite_Returns_Error(t *testing.T) {
	reg := tools.NewRegistry()

	noop := func(ctx context.Context, args []byte) (string, error) { return "", nil }
	_ = reg.Register(tools.Tool{Name: "my_tool", Execute: noop})

	// Second registration with the same name should return an error.
	err := reg.Register(tools.Tool{Name: "my_tool", Execute: noop})
	if err == nil {
		t.Fatal("expected collision error on duplicate registration")
	}
}

func TestRegistry_Register_NilExecute_Returns_Error(t *testing.T) {
	reg := tools.NewRegistry()
	err := reg.Register(tools.Tool{Name: "bad_tool", Execute: nil})
	if err == nil {
		t.Fatal("expected error for nil Execute func")
	}
}

func TestRegistry_Register_EmptyName_Returns_Error(t *testing.T) {
	reg := tools.NewRegistry()
	err := reg.Register(tools.Tool{
		Name:    "",
		Execute: func(_ context.Context, _ []byte) (string, error) { return "", nil },
	})
	if err == nil {
		t.Fatal("expected error for empty tool name")
	}
}

func TestRegistry_Get(t *testing.T) {
	reg := tools.NewRegistry()
	_ = reg.Register(tools.Tool{
		Name: "lookup",
		Execute: func(_ context.Context, _ []byte) (string, error) { return "", nil },
	})

	tool, err := reg.Get("lookup")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if tool.Name != "lookup" {
		t.Errorf("unexpected name: %q", tool.Name)
	}
}

func TestRegistry_Names(t *testing.T) {
	reg := tools.NewRegistry()
	noop := func(_ context.Context, _ []byte) (string, error) { return "", nil }
	_ = reg.Register(tools.Tool{Name: "tool_a", Execute: noop})
	_ = reg.Register(tools.Tool{Name: "tool_b", Execute: noop})

	names := reg.Names()
	if len(names) != 2 {
		t.Errorf("expected 2 names, got %d", len(names))
	}
}

// ---------------------------------------------------------------------------
// MCP Schema generation (PDR-004 §3)
// ---------------------------------------------------------------------------

func TestRegistry_Schema_SingleTool(t *testing.T) {
	reg := tools.NewRegistry()
	_ = reg.Register(tools.Tool{
		Name:        "calculate_cost",
		Description: "Calculates estimated API cost from token count.",
		Tier:        tools.TierNative,
		Parameters: map[string]interface{}{
			"tokens":   map[string]string{"type": "integer", "description": "estimated token count"},
			"provider": map[string]string{"type": "string", "description": "provider name"},
			"required": []string{"tokens", "provider"},
		},
		Execute: func(_ context.Context, _ []byte) (string, error) { return "0.01", nil },
	})

	schema, err := reg.Schema("calculate_cost")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	if schema.Name != "calculate_cost" {
		t.Errorf("wrong name: %q", schema.Name)
	}
	if schema.Description != "Calculates estimated API cost from token count." {
		t.Errorf("wrong description: %q", schema.Description)
	}
	if schema.InputSchema.Type != "object" {
		t.Errorf("expected type=object, got %q", schema.InputSchema.Type)
	}
	if _, ok := schema.InputSchema.Properties["tokens"]; !ok {
		t.Error("expected 'tokens' property in schema")
	}
	if len(schema.InputSchema.Required) == 0 {
		t.Error("expected required fields to be extracted")
	}
	// The 'required' key itself must not appear in Properties.
	if _, ok := schema.InputSchema.Properties["required"]; ok {
		t.Error("'required' should not appear as a property; it belongs in InputSchema.Required")
	}
}

func TestRegistry_Schema_NotFound(t *testing.T) {
	reg := tools.NewRegistry()
	_, err := reg.Schema("ghost_tool")
	if err == nil {
		t.Fatal("expected ErrToolNotFound")
	}
}

func TestRegistry_AllSchemas(t *testing.T) {
	reg := tools.NewRegistry()
	noop := func(_ context.Context, _ []byte) (string, error) { return "", nil }
	_ = reg.Register(tools.Tool{Name: "tool_x", Execute: noop})
	_ = reg.Register(tools.Tool{Name: "tool_y", Execute: noop})

	schemas := reg.AllSchemas()
	if len(schemas) != 2 {
		t.Errorf("expected 2 schemas, got %d", len(schemas))
	}
}

func TestRegistry_AllSchemasJSON_Valid(t *testing.T) {
	reg := tools.NewRegistry()
	_ = reg.Register(tools.Tool{
		Name:        "ping",
		Description: "Checks if a host is reachable.",
		Execute:     func(_ context.Context, _ []byte) (string, error) { return "pong", nil },
	})

	raw, err := reg.AllSchemasJSON()
	if err != nil {
		t.Fatalf("AllSchemasJSON: %v", err)
	}

	var schemas []tools.MCPSchema
	if err := json.Unmarshal(raw, &schemas); err != nil {
		t.Fatalf("unmarshal: %v — raw: %s", err, raw)
	}
	if len(schemas) != 1 || schemas[0].Name != "ping" {
		t.Errorf("unexpected schemas: %+v", schemas)
	}
}

// ---------------------------------------------------------------------------
// Concurrency safety
// ---------------------------------------------------------------------------

func TestRegistry_ConcurrentRegisterAndExecute(t *testing.T) {
	reg := tools.NewRegistry()
	const n = 50

	var wg sync.WaitGroup
	wg.Add(n * 2)

	// Concurrent registrations.
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			_ = reg.Register(tools.Tool{
				Name: fmt.Sprintf("concurrent_tool_%d", i),
				Execute: func(_ context.Context, _ []byte) (string, error) {
					return fmt.Sprintf("result_%d", i), nil
				},
			})
		}()
	}

	// Concurrent executions on a pre-registered tool — must not data-race.
	_ = reg.Register(tools.Tool{
		Name:    "stable_tool",
		Execute: func(_ context.Context, _ []byte) (string, error) { return "ok", nil },
	})

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, _ = reg.Execute(context.Background(), "stable_tool", nil)
		}()
	}

	wg.Wait()
}

// ---------------------------------------------------------------------------
// Wasm sandbox — basic unit tests (no real WASM daemon required)
// ---------------------------------------------------------------------------

func TestWasmSandbox_EmptyBytes_Returns_Error(t *testing.T) {
	sb := tools.NewWasmSandbox()
	result := sb.Execute(context.Background(), tools.ExecutionRequest{
		Language: "wasm",
		RawBytes: nil,
	})
	if result.Error == nil {
		t.Fatal("expected error for empty RawBytes")
	}
}

func TestWasmSandbox_InvalidBytes_Returns_Error(t *testing.T) {
	sb := tools.NewWasmSandbox()
	result := sb.Execute(context.Background(), tools.ExecutionRequest{
		Language: "wasm",
		RawBytes: []byte("this is not valid wasm"),
	})
	if result.Error == nil {
		t.Fatal("expected error for invalid WASM bytes")
	}
	if result.ExitCode == 0 {
		t.Error("expected non-zero exit code for invalid module")
	}
}

// TestWasmSandbox_MinimalModule verifies that a bare, valid WASM module
// (magic bytes + version, no sections, no _start) instantiates cleanly.
// This confirms the sandbox infrastructure works without requiring an OS process.
func TestWasmSandbox_MinimalModule(t *testing.T) {
	// Minimal valid WASM: magic + version only (no type/func/export sections).
	// wazero instantiates this module successfully and returns immediately.
	minimalWasm := []byte{
		0x00, 0x61, 0x73, 0x6d, // \0asm  — WASM magic
		0x01, 0x00, 0x00, 0x00, // 0x01   — WASM version 1
	}

	sb := tools.NewWasmSandbox()
	result := sb.Execute(context.Background(), tools.ExecutionRequest{
		Language:       "wasm",
		RawBytes:       minimalWasm,
		TimeoutSeconds: 5,
	})

	// A module without _start exits with code 0 and no error.
	if result.Error != nil {
		t.Fatalf("unexpected error for minimal WASM module: %v", result.Error)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected ExitCode 0, got %d", result.ExitCode)
	}
}

// ---------------------------------------------------------------------------
// Tier 2 tool — end-to-end via registry (no real WASM daemon required)
// ---------------------------------------------------------------------------

func TestRegistry_Tier2_ToolRegistration(t *testing.T) {
	reg := tools.NewRegistry()
	sb := tools.NewWasmSandbox()

	// Minimal WASM binary that instantiates without errors.
	minimalWasm := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}

	err := reg.Register(tools.Tool{
		Name:        "wasm_identity",
		Description: "Passes stdin through a WASM module.",
		Tier:        tools.TierWasm,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			res := sb.Execute(ctx, tools.ExecutionRequest{
				Language:       "wasm",
				RawBytes:       minimalWasm,
				Stdin:          args,
				TimeoutSeconds: 5,
			})
			if res.Error != nil {
				return "", res.Error
			}
			return res.Stdout, nil
		},
	})
	if err != nil {
		t.Fatalf("Register tier2 tool: %v", err)
	}

	out, err := reg.Execute(context.Background(), "wasm_identity", []byte(`{"key":"value"}`))
	if err != nil {
		t.Fatalf("Execute tier2 tool: %v", err)
	}
	// The minimal module has no stdout; we expect an empty string, not an error.
	_ = out
}
