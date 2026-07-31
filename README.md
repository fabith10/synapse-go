# ⚡ SynapseGo (`synapse-go`)

[![Go Version](https://img.shields.io/badge/go-1.26+-00ADD8?style=flat-square&logo=go)](https://go.dev)
[![Sandboxing](https://img.shields.io/badge/Sandboxing-WASM%20%7C%20Docker-blueviolet?style=flat-square)]()
[![Build Status](https://img.shields.io/badge/tests-passing-brightgreen?style=flat-square)]()

A high-speed, secure multi-agent orchestrator framework built strictly in Go. This framework implements a three-tier sandbox execution strategy (Native Go, WebAssembly, and Docker), centralized orchestrator event-loop routing, real-time HTMX-powered mobile steering dashboard, and robust semantic prompt-injection firewalls.

---
## Architecture Overview

```
                          +-------------------------------------------------------+
                          |        HTTP Control Dashboard (HTMX + SSE)            |
                          |  - Real-Time Live Logs & Interactive Steering Panel   |
                          |  - 🕸️ Live Force-Directed Agent Topology Canvas       |
                          +--------------------------+----------------------------+
                                                     | Real-Time SSE Telemetry & Steering
                                                     v
                          +-------------------------------------------------------+
                          |            SynapseGo Orchestrator Engine              |
                          |        (Central MessageBus Event-Loop Dispatcher)     |
                          +--------------------------+----------------------------+
                                                     | Middleware Pipeline
                                                     | (Tracing, Guardrails, InjectionFilter, CostLimits)
                                                     v
                          +-------------------------------------------------------+
                          |               triage-agent (Gatekeeper)               |
                          |      (Dynamic Capability Matching & Intent Router)    |
                          +----+---------------------+----------------------+-----+
                               |                     |                      |
            +------------------+                     |                      +------------------+
            |                                        v                                         |
            v                               +------------------+                               v
 +--------------------+                     |  planner-agent   |                     +--------------------+
 |  supervisor-agent  |                     | (DAG Task Dissect|                     | Dynamic Specialist |
 | (Goal QA & Verifier|                     +--------+---------+                     |   Agents Pool      |
 +--------------------+                              |                               +---------+----------+
                                                     v                                         |
                                            +------------------+                               |
                                            | developer-agent  |                               |
                                            | researcher-agent |<------------------------------+
                                            |   quant-agent    |  Dynamic Agent Blueprints
                                            |   writer-agent   |  (Loaded via GetLoadedAgentConfigs)
                                            |   browser-agent  |
                                            |   excel-agent    |
                                            |   email-agent    |
                                            +--------+---------+
                                                     |
             +---------------------------------------+---------------------------------------+
             |                                       |                                       |
             v                                       v                                       v
+------------------------+              +------------------------+              +------------------------+
| Tier 1: Native Go      |              | Tier 2: WASM Sandbox   |              | Tier 3: Docker SDK     |
| - Excelize (XML)       |              | - Wazero Engine        |              | - Ephemeral Python     |
| - Chromedp (DOM)       |              | - Isolated Code Exec   |              | - Ephemeral Bash Exec  |
| - SQLite Ledger        |              | - Zero Network Access  |              | - Resource Capped      |
+------------------------+              +------------------------+              +------------------------+
`````

### 1. Central Event Loop & Orchestrator
All communication—User-to-Agent (U2A) and Agent-to-Agent (A2A)—flows through the Go Orchestrator's central `MessageBus`. A unified dispatcher compiles and executes a chain of composeable middlewares before delivering payloads to the target agent mailboxes.
- **Human-in-the-Loop (HITL):** Messages targeting the `"USER"` recipient park execution and block the orchestrator goroutine on an unbuffered approval channel until operators submit a steering reply (`APPROVED`/`DENIED`).

### 2. Built-in Base Agents

SynapseGo ships out-of-the-box with **Core Orchestrator Agents** and **Specialist Domain Agents**:

#### A. Core Orchestrator Agents
| Agent ID | Role | Description & Primary Responsibilities |
| :--- | :--- | :--- |
| **`triage-agent`** | **System Gatekeeper** | Outermost entry point. Classifies incoming prompts, enriches underspecified user requests, queries `query_pricing_oracle` to select cost-effective compute nodes, and routes tasks. |
| **`planner-agent`** | **Task Dissector & Planner** | Dissects complex multi-step goals into directed task graphs with explicit dependencies and targeted agent assignments. |
| **`supervisor-agent`** | **Goal Supervisor & QA** | Evaluates specialist output against the original user goal (`DONE`, `RETRY`, `ESCALATE`), enforcing strict validation rules (e.g. script execution for math vs LLM mental math). |

#### B. Specialist Domain Agents
| Agent ID | Role | Tier & Primary Capabilities |
| :--- | :--- | :--- |
| **`etl-agent`** | **Data Harvester** | Tier 2 (WASM): Unstructured web scraping, raw HTML cleaning via `bluemonday`, and JSON mapping. |
| **`quant-agent`** | **Quantitative Analyst** | Tier 3 (Docker SDK): Monte Carlo simulations, options chain pricing, forward curves, and Python math regressions. |
| **`excel-agent`** | **Spreadsheet Hero** | Tier 1 (Native Go): Direct XML parsing, updating, and formula calculation across Excel workbooks via `excelize/v2`. |
| **`researcher-agent`** | **Deep Researcher** | Tier 1 (Native Go): Real-time web search/extraction and automated board-ready PDF generation via `generate_pdf_report`. |
| **`developer-agent`** | **Software Engineer** | Tier 3 (Docker SDK): Code generation, complexity analysis, refactoring verification, and script execution. |
| **`writer-agent`** | **Technical Copywriter** | Tier 1 (Native Go): Executive summaries, technical documentation synthesis, and report drafting. |
| **`browser-agent`** | **Browser Automation** | Tier 1 (Native Go): Headed/headless navigation, DOM inspection, form filling, and screenshot capture. |
| **`email-agent`** | **Communication Assistant**| Tier 1 (Native Go): Writing draft emails, user notifications, and Human-in-the-Loop approval workflows. |
| **`sales-agent`** | **Sales & Lead Scoring** | Tier 1 (Native Go): Analyzing lead metrics, scoring prospect data via `check_lead_score`, and outreach drafting. |
| **`generalist-agent`**| **Generalist Specialist** | Tier 1 (Native Go): Fallback agent for general knowledge queries, text transformation, and unmapped tasks. |

---

### 3. How to Add Custom Agents

Adding custom agents to SynapseGo is designed to be modular and zero-friction. You can register custom agents using any of the 3 approaches below:

#### Approach A: Markdown System Prompts (`agents/` Directory)
Place a markdown file inside the `agents/` directory (e.g. `agents/security-auditor-agent.md`). SynapseGo automatically discovers and loads the system prompt:

```markdown
# Security Auditor Agent
You are a senior security engineer. Your job is to analyze code snippets for vulnerabilities, secrets leakage, and injection risks.

## Guidelines
1. Always check user inputs against strict validation regex.
2. Flag hardcoded secret keys immediately.
```

#### Approach B: Dynamic JSON Configuration (`agents.json`)
Define or override agent prompts without re-compiling Go code by placing an `agents.json` file in your workspace root:

```json
{
  "custom-security-agent": {
    "system_prompt": "You are a specialized security agent auditing infrastructure scripts...",
    "tools": ["read_file", "execute_python_docker"]
  }
}
```

#### Approach C: Programmatic Go ADK API
Register custom agents programmatically in Go source code using `adk.AgentBlueprint`:

```go
package main

import "github.com/fabith10/synapse-go/adk"

func main() {
    rt, _ := adk.NewRuntime(cfg)
    
    // Register custom agent blueprint
    rt.RegisterAgent(adk.AgentBlueprint{
        ID:           "custom-risk-agent",
        Name:         "Custom Risk Analyst",
        SystemPrompt: "You evaluate portfolio Value-at-Risk using Monte Carlo math...",
        Tools:        []string{"execute_python_docker", "write_file"},
    })
}
```

---

### 4. Three-Tier Sandbox Strategy
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
   GEMINI_API_KEY=your_key_here
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
- `Makefile` — Build, test, and cleanup task runner.

---

## Build & Development Commands

A top-level `Makefile` is provided for standard developer commands:

```bash
make build       # Compiles the binary to bin/synapse-go
make test        # Runs unit and package integration tests
make test-e2e    # Runs the end-to-end test suite
make clean       # Removes temporary build files, test databases, and binaries
make help        # Displays available Makefile commands
```

---

## Requirements & Dependencies

To execute the full multi-sandbox execution suite, ensure your machine satisfies the following hardware and runtime constraints:

### 1. System Requirements
- **Go Version:** `1.26+` (required for modern compiler features and standard library structures).
- **Docker Daemon:** Active local daemon (e.g. Docker Desktop) to execute Tier 3 container math scripts.
- **Ollama Client Daemon:** Local Ollama runner (listening on default `http://127.0.0.1:11434`) with the `llama3` model pulled (`ollama pull llama3`) to support semantic prompt injection classifications.

### 2. Core Go Dependencies

SynapseGo relies on key open-source Go libraries for sandboxing, database persistence, security, and document parsing:

| Package | Version | Purpose & Usage |
| :--- | :--- | :--- |
| **`github.com/tetratelabs/wazero`** | `v1.12.0` | Pure Go WebAssembly runtime for Tier 2 sandboxed code execution. |
| **`github.com/docker/docker`** | `v28.5.2` | Official Docker SDK client managing Tier 3 ephemeral Python/Bash container sandboxes. |
| **`modernc.org/sqlite`** | `v1.53.0` | Pure Go (CGo-free) SQLite engine for checkpoint persistence, memory logs, and auditing. |
| **`github.com/xuri/excelize/v2`** | `v2.11.0` | Native XML spreadsheet parser and calculation engine (`excel-agent`). |
| **`github.com/chromedp/chromedp`** | `v0.16.0` | Headless & headed Chrome browser automation driver (`browser-agent`). |
| **`github.com/microcosm-cc/bluemonday`** | `v1.0.27` | Strict HTML sanitizer for prompt injection defense on scraped web pages (`etl-agent`). |
| **`github.com/ollama/ollama`** | `v0.32.1` | Official Ollama SDK for local Llama-based semantic injection classification. |
| **`github.com/ledongthuc/pdf`** | `v0.0.0` | Native Go PDF text extraction engine (`extract_pdf_text` tool). |
| **`go.opentelemetry.io/otel`** | `v1.44.0` | OpenTelemetry distributed tracing and observability. |
| **`github.com/joho/godotenv`** | `v1.5.1` | Automated environment variable loading from local `.env` files. |
| **`github.com/google/uuid`** | `v1.6.0` | Cryptographically safe UUID generation for agent sessions and execution trace IDs. |
| **`github.com/pkg/browser`** | `v0.0.0` | Native browser launcher for the HTMX steering dashboard. |

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
make test
```

#### Step 4: Boot the Control Center
Start the web dashboard server natively using Go:
```bash
go run ./cmd/main.go
```
Open **`http://localhost:8080`** in your browser.

---

### Mode B: Standalone Compilation (No Go Installation Required)

Go compiles down to a **single, standalone, statically linked binary** containing all its dependencies. Target host machines do not require Go installed.

To build and run the compiled binary:

1. **Compile for your host architecture:**
   ```bash
   make build
   ```
2. **Execute the compiled binary directly:**
   ```bash
   ./bin/synapse-go
   ```

#### Cross-Compilation Mappings
You can cross-compile binaries from your local machine to any other operating system and CPU architecture targets by using environment flags:

* **macOS (Apple Silicon M-series):**
  ```bash
  GOOS=darwin GOARCH=arm64 go build -o bin/synapse-go ./cmd/main.go
  ```
* **macOS (Intel):**
  ```bash
  GOOS=darwin GOARCH=amd64 go build -o bin/synapse-go ./cmd/main.go
  ```
* **Linux (Statically linked, zero external runtime dependency):**
  ```bash
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/synapse-go-linux ./cmd/main.go
  ```
* **Windows:**
  ```bash
  GOOS=windows GOARCH=amd64 go build -o bin/synapse-go.exe ./cmd/main.go
  ```

---

### Mode C: Cloud / Production Deployment

When deploying to a cloud environment (e.g., AWS, GCP, or a private VPS), the system should be configured for high availability, security containment, and persistence.

#### Step 1: Configure Environment Variables
Set the routing configurations to separate the runtime environment from local assumptions:
```bash
# Point to a persistent directory for SQLite database logs
export SQLITE_DSN="/var/lib/synapse-go/data.db"

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
synapse.yourdomain.com {
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
Access the dashboard securely on your phone via `https://synapse.yourdomain.com` to steer workflows anywhere.

---

## 🏁 Making Your First Run

Follow this step-by-step walkthrough to run your first multi-agent workflow and explore the framework tools:

### Step 1: Launch the Control Dashboard
Start the native Web Control Server:
```bash
go run ./cmd/main.go
```
*Alternatively, if compiled, run `./bin/synapse-go` or execute `./start.sh`.*

Open **`http://localhost:8080`** in your browser.

---

### Step 2: Dispatch Your First Task

1. On the **⚡ Control Center** tab, choose one of the preset task templates or type a custom prompt:
   - **Single-step Task**: *"Calculate 30-day hosting costs for an H100 GPU cluster on Akash vs AWS."*
   - **Multi-step Research & Synthesis**: *"Perform web research on current AI market trends, synthesize a summary, and generate an executive PDF report."*
2. Click **⚡ Launch Task Workflow**.

---

### Step 3: Monitor Execution & Visual Insights

- **📜 Live Log Console**: Watch real-time Server-Sent Events (SSE) stream detailed progress logs from `triage-agent`, `planner-agent`, specialist agents, and supervisor checks.
- **🕸️ Agent Network Topology Graph**: Click the **🕸️ Network Graph** tab to view an interactive, real-time force-directed canvas. Built dynamically from registered agent configurations (`GetLoadedAgentConfigs()`), active firing agent nodes expand, glow, and emit animated ripple rings in real time as SSE log stream events fire.
- **📊 Cost & Telemetry Ledger**: Click the **📊 Cost & Audit** tab to inspect token consumption per model, USD cost breakdowns, and export downloadable CSV execution reports.

---

### Step 4: Run via Command Line Interface (CLI Mode)

You can also execute tasks headlessly straight from your terminal:
```bash
go run ./cmd/main.go -prompt "Research options volatility trends and save a summary report"
```

---

## 🛠️ Making the Framework Yours

This framework is built for maximum extensibility. Here is how you can customize agents, tools, LLM providers, and UI workflows:

### 1. Adding Custom Specialist Agents

You can add custom agents either through the visual UI or by dropping markdown files into the repository:

#### Option A: Via Agent Studio (Visual UI)
1. Open `http://localhost:8080` and switch to the **🎨 Agent Studio** tab.
2. Fill in the **Agent ID** (e.g. `market-analyst`), **Role Description**, **System Prompt**, and check the tools you want to grant.
3. Click **🚀 Deploy Agent to Framework**. The agent is instantly compiled and ready for routing!

#### Option B: Via Markdown Blueprints (Code Base)
Add a markdown file to `agents/my-specialist.md`:
```markdown
---
description: "Custom Financial Analyst specialist for quantitative reporting"
capabilities:
  - "financial_modeling"
  - "options_pricing"
tools:
  - "query_options_chain"
  - "execute_python_docker"
  - "write_file"
---
You are a quantitative financial analyst. Your job is to process financial datasets, calculate greeks, and produce structured summaries.

CRITICAL RULES:
1. Calculations MUST be computed via script tools ('execute_python_docker' or native tools).
2. Deliver clear markdown tables summarizing key metrics.
```
*The framework's dynamic discovery engine automatically registers your markdown agent into the Gatekeeper intent router and Planner DAG scheduler at boot—no code changes required!*

---

### 2. Registering Custom Tools

To grant agents new capabilities (e.g., querying internal databases, calling external APIs, or executing local scripts):

1. **Define the Tool in Go** (`adk/tools.go` or `internal/agent/tool_alias.go`):
   ```go
   func MyCustomAPITool(ctx context.Context, payload string) (string, error) {
       // Your API logic or custom computation
       return "Processed data result", nil
   }
   ```
2. **Register the Tool Schema**:
   Add the tool metadata to `GetAvailableToolsList()`:
   ```go
   ToolMetadata{
       Name:        "query_custom_api",
       Description: "Fetches live analytical data from internal enterprise endpoint",
       Category:    "API",
   }
   ```

---

### 3. Configuring LLM Providers & Cost Rates

The framework supports hybrid multi-provider LLM setups (Ollama, OpenAI, Anthropic, DeepSeek, Azure):

1. **Environment Configuration** (`.env` or system environment):
   ```bash
   export OPENAI_API_KEY="sk-..."
   export ANTHROPIC_API_KEY="sk-ant-..."
   export OLLAMA_HOST="http://localhost:11434"
   ```
2. **Custom Pricing Oracle Rates**:
   Modify pricing tiers in `internal/agent/pricing_oracle.go` to match your enterprise LLM discount rates or custom local cluster costs.

---

### 4. Customizing Dashboard Themes & Control Views

- **Theme Preference**: Toggle between Dark Mode and high-contrast Light Mode via the header ☀️/🌙 toggle. Preferences are saved automatically in local browser storage.
- **Extending Web UI**: Add new tabs, metrics, or custom HTMX endpoints in `internal/web/templates.go` and `internal/web/server.go`.

---

## License

This project is licensed under the Apache 2.0 License - see the [LICENSE](LICENSE) file for details.

