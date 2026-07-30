# Tooling & Sandbox Execution Standard

## 1. Core Philosophy
To maintain the security and speed of the Go orchestration engine, the framework enforces a strict, tiered execution boundary. Open-source models (especially smaller parameters) must never be given raw access to the host machine's terminal or filesystem. All tools are defined, registered, and accessed using the Model Context Protocol (MCP) standard.

## 2. The Three-Tier Execution Model
When an agent determines it needs to use a tool, the framework routes the request through one of three isolation tiers based on the tool's predefined requirements.

### Tier 1: Native Go Functions (Fastest & Safest)
* **Use Case:** Everyday agentic tasks (API calls, basic database queries, sending emails, web scraping).
* [cite_start]**Execution:** Pre-compiled, strictly typed pure Go functions. 
* **Security:** Perfectly safe. The agent cannot hallucinate malicious logic because the execution bounds are hardcoded into the Go binary.

### Tier 2: WebAssembly (`wazero`) (Lightweight Sandboxing)
* [cite_start]**Use Case:** Custom text manipulation, mapping complex JSON payloads, or running lightweight LLM-generated JavaScript/Python snippets[cite: 99].
* [cite_start]**Execution:** Routed through `wazero`, a pure-Go WebAssembly runtime[cite: 100].
* **Security:** Extremely high. [cite_start]WebAssembly runs in a strict memory sandbox with zero access to your host machine's filesystem, network, or environment variables by default[cite: 101]. [cite_start]It executes in microseconds, keeping your framework incredibly fast[cite: 102].

### Tier 3: Ephemeral Docker Containers (Heavy Isolation / Spot Compute)
* [cite_start]**Use Case:** Quantitative finance tools, CUDA/GPU backend calculations, running complex data-science libraries (e.g., pandas, PyTorch), or executing bash scripts[cite: 91].
* [cite_start]**Execution:** The Go orchestrator uses the Go Docker SDK (`github.com/docker/docker/client`) to dynamically spin up a pre-built Docker container configured with the necessary Python/C++ quant libraries[cite: 92].
* **Security:** High isolation with hardware pass-through. 
  * [cite_start]MUST include `--gpus all` if hardware acceleration is required on a Spot instance[cite: 93].
  * [cite_start]MUST be wrapped in a strict Go `context.WithTimeout` (e.g., 5 seconds) to prevent hallucinated infinite loops[cite: 94].
  * [cite_start]The container is immediately destroyed (`container.Stop` / `container.Remove`) after standard output is captured[cite: 94, 95].

## 3. Model Context Protocol (MCP) Integration
All tools, regardless of their execution tier, must be exposed to the LLM using a standardized MCP JSON schema. This allows the local Triage Agent to understand the available capabilities.

**Standardized Tool Schema:**
```json
{
  "name": "tool_name",
  "description": "Clear description of what the tool does and when to use it.",
  "input_schema": {
    "type": "object",
    "properties": {
      "param1": { "type": "string", "description": "..." }
    },
    "required": ["param1"]
  }
}
```

## 4. Go Interface Contracts
To ensure modularity within the Hybrid Compute Broker, the core orchestrator must not care *how* a tool is executed, only that it adheres to these strict Go interfaces.

### Sandbox Interface
Used for dynamically generated code (Tiers 2 & 3).
```go
type ExecutionRequest struct {
	Language       string // "python", "javascript"
	RawCode        string
	RequiresGPU    bool
	TimeoutSeconds int
}

type ExecutionResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Error    error
}

type Sandbox interface {
	Execute(ctx context.Context, req ExecutionRequest) ExecutionResult
}
```

### Tool Registry Interface
Used to register and route all tools (Tiers 1, 2, & 3).
```go
type ToolTier int

const (
	TierNative ToolTier = iota
	TierWasm
	TierDocker
)

type Tool struct {
	Name        string
	Description string
	Tier        ToolTier
	Parameters  map[string]interface{} // MCP JSON Schema mapping
	Execute     func(ctx context.Context, args []byte) (string, error)
}
```