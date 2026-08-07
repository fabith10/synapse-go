// Package tools implements the three-tier execution sandbox and the Tool
// Registry for the agent framework. All tool execution follows the isolation
// boundary defined in PDR-004:
//
//	Tier 1 (TierNative)  — pre-compiled, strictly typed Go functions
//	Tier 2 (TierWasm)    — wazero WebAssembly sandbox (zero host access)
//	Tier 3 (TierDocker)  — ephemeral Docker containers (destroyed after use)
//
// All tools, regardless of tier, are exposed to LLM backends through the
// standardised Model Context Protocol (MCP) JSON schema.
package tools

import "context"

// ---------------------------------------------------------------------------
// Tier constants (PDR-004 §4)
// ---------------------------------------------------------------------------

// ToolTier identifies which execution sandbox a tool runs in.
type ToolTier int

const (
	// TierNative runs pre-compiled Go functions directly in the host process.
	// Fastest and safest: the agent cannot inject malicious logic at runtime.
	TierNative ToolTier = iota

	// TierWasm runs compiled WebAssembly binaries via wazero. The sandbox has
	// zero access to the host filesystem, network, or environment variables.
	TierWasm

	// TierDocker spins up an ephemeral container per invocation and destroys
	// it immediately after stdout/stderr are captured. GPU pass-through is
	// enabled when ExecutionRequest.RequiresGPU is true.
	TierDocker
)

// ---------------------------------------------------------------------------
// ExecutionRequest / ExecutionResult (PDR-004 §4 Sandbox Interface)
// ---------------------------------------------------------------------------

// ExecutionRequest carries everything a Sandbox needs to execute one unit of
// work. The Language field drives tier-specific routing inside each sandbox.
type ExecutionRequest struct {
	// Language determines how the sandbox interprets RawCode and RawBytes.
	//   TierNative  — field unused; Execute func is called directly.
	//   TierWasm    — "wasm" (binary in RawBytes) or reserved for future WASI-JS.
	//   TierDocker  — "python" | "javascript" | "bash" (maps to a Docker image).
	Language string

	// RawCode is the source code string for Tier 3 (Docker) executions.
	// For Python: passed as `python3 -c <RawCode>`.
	// For JavaScript: passed as `node -e <RawCode>`.
	// For Bash: passed as `sh -c <RawCode>`.
	RawCode string

	// RawBytes is the compiled WebAssembly binary for Tier 2 (wazero) executions.
	// Must be a valid WASM module; wazero validates the magic bytes on load.
	RawBytes []byte

	// RequiresGPU signals to the Tier 3 Docker sandbox that the container must
	// be launched with `--gpus all` for CUDA hardware pass-through.
	RequiresGPU bool

	// TimeoutSeconds caps the maximum wall-clock duration of the execution.
	// The sandbox derives a context.WithTimeout from the calling context.
	// A value of 0 means no additional timeout beyond the caller's ctx.
	TimeoutSeconds int

	// Stdin is optionally forwarded to the sandboxed process as standard input.
	Stdin []byte

	// Packages specifies third-party libraries (e.g. "numpy", "pandas") to prep/install in the container.
	Packages []string

	// PrepCommands specifies arbitrary setup commands (e.g. "apt-get update && apt-get install -y build-essential") to run prior to execution.
	PrepCommands []string

	// AllowNetwork enables container internet access when explicitly granted (e.g., via HITL operator approval).
	AllowNetwork bool
}

// ExecutionResult holds the captured output of one sandboxed execution.
// ExitCode mirrors POSIX exit codes: 0 = success, non-zero = failure.
type ExecutionResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Error    error // non-nil only for sandbox-level errors (not process errors)
}

// ---------------------------------------------------------------------------
// Sandbox interface (PDR-004 §4)
// ---------------------------------------------------------------------------

// Sandbox is the execution abstraction for Tier 2 and Tier 3 tools.
// Tier 1 tools bypass this interface entirely — their Execute func IS the
// sandbox boundary (compiled Go code, no dynamic dispatch needed).
//
// All implementations must:
//   - Respect the context deadline.
//   - Never block the caller beyond TimeoutSeconds.
//   - Never retain references to RawCode / RawBytes after Execute returns.
type Sandbox interface {
	Execute(ctx context.Context, req ExecutionRequest) ExecutionResult
}

// ---------------------------------------------------------------------------
// Tool (PDR-004 §4 Tool Registry Interface)
// ---------------------------------------------------------------------------

// Tool is the universal descriptor for every capability the framework exposes
// to an LLM. The Parameters map is serialised directly into the MCP JSON
// schema that FormatPrompt sends to the provider.
//
// # Execute function contract
//
// For Tier 1 tools, Execute contains pure Go logic.
// For Tier 2/3 tools, Execute is a closure that captures a Sandbox reference:
//
//	// Example Tier 2 registration:
//	sb := tools.NewWasmSandbox()
//	registry.Register(tools.Tool{
//	    Name: "json_transform",
//	    Tier: tools.TierWasm,
//	    Execute: func(ctx context.Context, args []byte) (string, error) {
//	        res := sb.Execute(ctx, tools.ExecutionRequest{
//	            Language: "wasm",
//	            RawBytes: myWasmBinary,
//	            Stdin:    args,
//	        })
//	        return res.Stdout, res.Error
//	    },
//	})
type Tool struct {
	Name        string
	Description string
	Tier        ToolTier
	Parameters  map[string]interface{} // MCP JSON Schema "properties" mapping
	Execute     func(ctx context.Context, args []byte) (string, error)
}

// ---------------------------------------------------------------------------
// MCP Schema types (PDR-004 §3)
// ---------------------------------------------------------------------------

// MCPSchema is the wire format for a single tool exposed to an LLM via the
// Model Context Protocol. All tools in the registry can be serialised to this
// format and injected into a FormatPrompt call.
type MCPSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema MCPInputSchema `json:"input_schema"`
}

// MCPInputSchema carries the JSON Schema "object" definition for a tool's
// accepted arguments. The Required slice lists parameter names that must be
// present; omitting it means all parameters are optional.
type MCPInputSchema struct {
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties"`
	Required   []string               `json:"required,omitempty"`
}

// ---------------------------------------------------------------------------
// Sentinel errors
// ---------------------------------------------------------------------------

var (
	// ErrToolNotFound is returned by Registry.Execute when no tool matches name.
	ErrToolNotFound = toolError("tool not found in registry")

	// ErrNoWasmBytes is returned by WasmSandbox when RawBytes is empty.
	ErrNoWasmBytes = toolError("wasm: RawBytes must not be empty")

	// ErrDockerUnavailable is returned by DockerSandbox when the Docker daemon
	// cannot be reached (e.g., Docker Desktop is not running).
	ErrDockerUnavailable = toolError("docker: daemon unreachable")
)

type toolError string

func (e toolError) Error() string { return string(e) }
