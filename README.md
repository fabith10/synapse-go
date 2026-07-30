# Go Agent-Framework

A secure, multi-agent orchestrator framework built strictly in Go. This framework implements a three-tier sandbox execution strategy (Native Go, WebAssembly, and Docker), centralized orchestrator event-loop routing, real-time HTMX-powered mobile steering dashboard, and robust semantic prompt-injection firewalls.

---

## Architecture Overview

```
                      +-----------------------------+
                      |        HTTP Dashboard       |
                      |   (Go Templates + HTMX)     |
                      +--------------+--------------+
                                     | Event Logs & HITL Replies
                                     v
                      +-----------------------------+
                      |    Orchestrator Router      | <--- Middleware Pipeline
                      |      (Central Event Loop)   |      (Logging, Auth, Tracing,
                      +--------------+--------------+       InjectionGuardrail, CostLimit)
                                     |
         +---------------------------+---------------------------+
         |                           |                           |
         v                           v                           v
+------------------+       +------------------+       +------------------+
|  triage-agent    |       |  etl-agent       |       |  quant-agent     |
|  (Gatekeeper)    |       |  (Data Harvester)|       |  (Math Sandbox)  |
+--------+---------+       +--------+---------+       +--------+---------+
         |                          |                          |
         | (Tier 1 Native)          | (Tier 2 WASM)            | (Tier 3 Docker)
         | - [Pricing Oracle]       | - [wazero Sandbox]       | - [Docker Container]
         |                          | - [wasm_json_mapper]     | - [execute_python]
         |                                                     |
         +---------------------------+-------------------------+
                                     |
                                     v
                  +-------------------------------------+
                  | excel-agent & researcher-agent      |
                  | (Excel Hero & Deep Researcher)      |
                  +------------------+------------------+
                                     |
                                     | (Tier 1 Native Extensions)
                                     | - [modify_excel_workbook]
                                     | - [web_search_and_extract]
                                     | - [generate_pdf_report]
```

### 1. Central Event Loop & Orchestrator
All communication—User-to-Agent (U2A) and Agent-to-Agent (A2A)—flows through the Go Orchestrator's central `MessageBus`. A unified dispatcher compiles and executes a chain of composeable middlewares before delivering payloads to the target agent mailboxes.
- **Human-in-the-Loop (HITL):** Messages targeting the `"USER"` recipient park execution and block the orchestrator goroutine on an unbuffered approval channel until operators submit a steering reply (`APPROVED`/`DENIED`).

### 2. Multi-Agent Blueprints
- **triage-agent (Compute Gatekeeper):** Routes prompts to appropriate downstream agents, querying the `query_pricing_oracle` Tier 1 native tool to identify the most cost-effective compute nodes.
- **etl-agent (Data Harvester):** Scrapes unstructured website HTML and cleans the data securely.
- **quant-agent (Quantitative Analyst):** Conducts high-performance regressions and options calculations in Python.
- **excel-agent (Excel Hero):** Natively parses and updates cash flow sheets.
- **researcher-agent (Deep Researcher):** Gathers real-time web context using clean Markdown extraction via the `web_search_and_extract` tool, bypassing browser bloat. Automatically compiles beautiful, board-ready summaries using the `generate_pdf_report` native Go compiler.

### 3. Three-Tier Sandbox Strategy
- **Tier 1 (Native Go):** Fast execution in Go memory space (e.g. `excelize/v2` spreadsheet adjustments, native oracle check).
- **Tier 2 (WebAssembly):** Safe computation of dynamic mapping code via `wazero`. Completely sandboxed from network/filesystem.
- **Tier 3 (Docker SDK):** Ephemeral Alpine Python containers running heavy financial equations.

---

## Secure Prompt Injection Firewalls

1. **Input Firewall (`InjectionGuardrail`):** Outermost middleware that scans all bus messages. Immediate drops occur for known heuristics (`"ignore previous"`, `"system override"`, etc.).
2. **Local Semantic Classifier (`isMalicious`):** Communicates with a local Llama instance via the official Ollama client SDK to analyze Role Hijacking. Fails-open if the daemon is offline; fails-closed if parser format errors occur.
3. **XML Delimiter Boundary Hardening:** Wraps raw prompts in structural `<user_data>` tags to clearly demarcate untrusted content for LLMs.
4. **Scraper HTML Sanitizer:** Raw crawls are processed by a strict `microcosm-cc/bluemonday` policy to strip hidden scripts and injection blocks.

---

## Model & Compute Node Specifications

In this framework, **all model providers and spot hardware are represented as node entries** registered in the in-memory `PricingOracle`.

### 1. LLM Model Specification (Tiers 0 and 1)
LLM providers (such as local Ollama instances or commercial APIs) are defined as `adk.LLMNode` configurations:
```go
type LLMNode struct {
    Name          string        // e.g., "mock-ollama", "openai-gpt-4o"
    Tier          ProviderTier  // TierLocal (0) or TierCommercial (1)
    Provider      LLMProvider   // Driver implementing FormatPrompt and GenerateResponse
    InputRateUSD  float64       // USD cost per 1,000 input tokens (0.0 for Tier 0)
    OutputRateUSD float64       // USD cost per 1,000 output tokens
}
```

### 2. Spot Compute Specification (Tier 2 Spot GPUs)
Hardware nodes (such as Spot GPUs or execution servers) are defined as `adk.ComputeNode` configurations:
```go
type ComputeNode struct {
    Name                 string          // e.g., "aws-us-east-1-h100"
    Provider             ComputeProvider // Driver wrapping Tier 3 sandboxed executions
    SpotRatePerSecondUSD float64         // Hourly lease rate translated to seconds
}
```

### 3. Model Registration & Configuration File
To configure the available local and commercial LLM models in a user-friendly way without modifying Go source code, place a `models.json` file in the project's root folder:

```json
{
  "llm_providers": [
    {
      "name": "demo-mock-llm",
      "provider": "mock",
      "tier": "local",
      "input_rate_usd": 0.0,
      "output_rate_usd": 0.0
    },
    {
      "name": "local-ollama-llama3",
      "provider": "ollama",
      "tier": "local",
      "input_rate_usd": 0.0,
      "output_rate_usd": 0.0
    },
    {
      "name": "gpt-4o",
      "provider": "openai",
      "tier": "commercial",
      "input_rate_usd": 0.000005,
      "output_rate_usd": 0.000015,
      "api_url": "https://api.openai.com/v1/chat/completions",
      "api_key_env": "OPENAI_API_KEY"
    }
  ]
}
```

At startup, the launcher automatically parses `models.json` to configure the `LLMProviders` list, dynamically linking specified driver handlers (e.g. `"mock"`, `"ollama"`, or commercial `"openai"` bridges). If `models.json` is not present, it falls back to the default demo mock LLM setup.

### 4. Agent System Prompt Configuration File
To dynamically update or override agent system prompts without recompiling Go code, place an `agents.json` file in the workspace root directory:

```json
{
  "triage-agent": {
    "system_prompt": "You are the System Gatekeeper. Your job is triage..."
  },
  "etl-agent": {
    "system_prompt": "You are the Data Harvester..."
  }
}
```

The bootstrap routine loads these variables and applies them directly to the mutable system prompts of the corresponding registered agents (`triage-agent`, `etl-agent`, `quant-agent`, `excel-agent`, `researcher-agent`). If missing, the framework defaults to the built-in blueprint prompts.

### 5. Secrets & Environment Configuration (`.env`)
To prevent checking sensitive API keys into public repositories, the framework uses environment-specific parameters.

1. **Local Development (.env):** Rename `.env.example` to `.env` in the root folder and add your credentials:
   ```env
   TAVILY_API_KEY=your_key_here
   OPENAI_API_KEY=your_key_here
   ```
   At boot time, `godotenv` automatically loads `.env` variables into system environment memory.
2. **Cloud/Staging Environments:** Since `.env` is listed in `.gitignore`, you should inject environment variables natively via your hosting provider (e.g. Kubernetes, AWS ECS, GCP Cloud Run). The framework fails-open/gracefully handles the absence of a local `.env` file and reads environment properties directly.

---

## Project Layout

- `adk/` — Public ADK framework interfaces (Runtime, Agent, Middleware pipelines, LLMClient).
- `internal/agent/` — Concrete agent implementations (Gatekeeper, ETL, Quant, Excel) and MCP tool bindings.
- `internal/broker/` — Pricing oracle logic, latency maps, and model routing brokers.
- `internal/memory/` — SQLite task logs, auditing, and idempotency step locks.
- `internal/orchestrator/` — Core event loop, message delivery, and topic subscribers.
- `internal/tools/` — Wazero WASM sandboxes and Docker host configuration controls.
- `internal/web/` — Minimalist SSE logs broker and control dashboard.
- `cmd/main.go` — Launcher bootstrap script.

---

## Requirements & Dependencies

To execute the full multi-sandbox execution suite, ensure your machine satisfies the following hardware and runtime constraints:

### 1. System Requirements
- **Go Version:** `1.26+` (required for modern compiler features and standard library structures).
- **Docker Daemon:** Active local daemon (e.g. Docker Desktop) to execute Tier 3 container math scripts.
- **Ollama Client Daemon:** Local Ollama runner (listening on default `http://127.0.0.1:11434`) with the `llama3` model pulled (`ollama pull llama3`) to support semantic prompt injection classifications.

### 2. Core Go Dependencies
The framework utilizes the following specialized libraries:
- **`github.com/tetratelabs/wazero`:** Pure Go WebAssembly compilation sandbox (Tier 2).
- **`github.com/docker/docker`:** Official Docker SDK client wrapper (Tier 3).
- **`github.com/xuri/excelize/v2`:** Native binary XML spreadsheet reader and writer (Tier 1 Excel Hero).
- **`github.com/microcosm-cc/bluemonday`:** Strict HTML sanitizer protecting against indirect web injections.
- **`github.com/ollama/ollama`:** Official Ollama client SDK for local guardrail classifications.
- **`modernc.org/sqlite`:** Pure Go SQLite package managing database checkpoints and audit logs.

---

## Quick Start Guide

Follow the instructions below depending on whether you are running a local dev instance or deploying to a cloud infrastructure:

### Mode A: Local Development Setup

#### Step 1: Install Dependencies & Setup sumfile
```bash
go mod tidy
```

#### Step 2: Set Up Local Firewalls (Ollama)
Ensure the local Ollama daemon is running, then pull the required classifier model:
```bash
ollama pull llama3
```
*Note: If the Ollama service is unavailable, the InjectionGuardrail middleware logs a warning and automatically falls back to fail-open mode.*

#### Step 3: Run the Local Test Suite
Ensure your local Docker daemon (e.g. Docker Desktop) is online:
```bash
go test -v ./...
```

#### Step 4: Boot the Control Center
Start the web dashboard server natively using Go:
```bash
go run ./cmd/main.go
```
Open **`http://localhost:8080`** in your browser.

---

### Mode B: Standalone Compilation (No Go Installation Required)

Yes! Go compiles down to a **single, standalone, statically linked binary** that contains all its dependencies. A user does not need to have Go installed at all on their target host machine to run the compiled framework.

To build and run the compiled binary:

1. **Compile for your host architecture:**
   ```bash
   go build -o agent-framework ./cmd/main.go
   ```
2. **Execute the compiled binary directly:**
   ```bash
   ./agent-framework
   ```

#### Cross-Compilation Mappings
You can cross-compile binaries from your local machine to any other operating system and CPU architecture targets by using environment flags:

* **macOS (Apple Silicon M-series):**
  ```bash
  GOOS=darwin GOARCH=arm64 go build -o agent-framework ./cmd/main.go
  ```
* **macOS (Intel):**
  ```bash
  GOOS=darwin GOARCH=amd64 go build -o agent-framework ./cmd/main.go
  ```
* **Linux (Statically linked, zero external runtime dependency):**
  ```bash
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o agent-framework-linux ./cmd/main.go
  ```
* **Windows:**
  ```bash
  GOOS=windows GOARCH=amd64 go build -o agent-framework.exe ./cmd/main.go
  ```

---

### Mode C: Cloud / Production Deployment

When deploying to a cloud environment (e.g., AWS, GCP, or a private VPS), the system should be configured for high availability, security containment, and persistence.

#### Step 1: Configure Environment Variables
Set the routing configurations to separate the runtime environment from local assumptions:
```bash
# Point to a persistent directory for SQLite database logs
export SQLITE_DSN="/var/lib/agent-framework/data.db"

# Route semantic classifications to a shared or dedicated local Ollama container
export OLLAMA_HOST="http://ollama-service.internal:11434"

# Set the HTTP server listener port
export PORT="8080"
```

#### Step 2: Establish Sandbox Containment
- **Tier 2 (WASM):** Works out-of-the-box inside the Go process space; requires no system dependencies.
- **Tier 3 (Docker):** Ensure the cloud container running this application has access to the host's Docker socket or a remote Docker host by setting `DOCKER_HOST`. 
  - Ensure the Docker daemon has a memory limit cap (configured to `512MB` by default in host configs).

#### Step 3: Deployment Architecture & SSL Reverse Proxy
Since HITL steering requests are designed to be managed from mobile devices, the control panel must be served behind a secure reverse proxy (like **Nginx** or **Caddy**) handling SSL termination:

Example Caddyfile setup:
```caddy
agent-framework.yourdomain.com {
    reverse_proxy localhost:8080
}
```

#### Step 4: Run via Systemd or Docker Compose
To run as a persistent service, use docker-compose to orchestrate the runtime:
```yaml
version: '3.8'
services:
  app:
    build: .
    ports:
      - "8080:8080"
    environment:
      - SQLITE_DSN=/data/framework.db
      - OLLAMA_HOST=http://ollama:11434
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - data-volume:/data
  ollama:
    image: ollama/ollama:latest
    ports:
      - "11434:11434"

volumes:
  data-volume:
```
Launch the infrastructure:
```bash
docker-compose up -d
```
Access the dashboard securely on your phone via `https://agent-framework.yourdomain.com` to steer workflows anywhere.

---

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.


