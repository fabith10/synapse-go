# Hybrid Compute Broker & Arbitrage Specification

## 1. Core Philosophy
The Hybrid Compute Broker decouples the framework's reasoning logic from underlying infrastructure. The compiled Go orchestrator runs entirely locally. When a task requires execution, the broker dynamically arbitrates between three distinct compute tiers: Free Local Models, Token-Priced Commercial APIs, and Hardware-Priced GPU Spot Instances.

## 2. The Universal Interfaces
Because the framework interacts with APIs (text in/out) and Data Centers (container execution), the Go framework defines two distinct but unified interfaces.

```go
// For Commercial APIs (OpenAI, Anthropic) and Local Inference (Ollama)
type LLMProvider interface {
	FormatPrompt(messages []Message, tools []Tool) (interface{}, error)
	GenerateResponse(ctx context.Context, payload interface{}) (Message, error)
}

// For Hardware Spot Instances (AWS, DeepInfra, GPU clusters)
type ComputeProvider interface {
	ProvisionInstance(ctx context.Context, hardwareTier string) (Instance, error)
	ExecuteContainer(ctx context.Context, inst Instance, payload []byte) ([]byte, error)
	TerminateInstance(inst Instance) error
}
```

## 3. The Multi-Dimensional Pricing Oracle
The in-memory Oracle maintains a thread-safe matrix that tracks three entirely different pricing models simultaneously, calculating an "Estimated Task Cost" to normalize them.
* **Tier 0 (Local Models):** Cost is statically $0. Used via Ollama for triage routing, basic summarization, and local file reading.
* **Tier 1 (Commercial APIs):** Tracks static/dynamic Input, Output, and Cached token costs. Used for heavy reasoning and complex coding where speed is paramount.
* **Tier 2 (Data Center Spot GPUs):** Ingests live spot-pricing for raw hardware (e.g., H100s) and references CME/Silicon Data forward curves to predict price spikes. Used for heavy quantitative tasks, executing Docker containers, or parallel mathematical arrays.

## 4. The Normalized Penalty Scoring Algorithm
To compare a token-priced API against a time-priced GPU, the broker calculates a dynamic "Penalty Score."

**Normalization:**
* Estimated Token Cost = (Est. Input * Rate) + (Est. Output * Rate)
* Estimated Hardware Cost = (Est. Execution Seconds * Spot Rate per Second)
* Formula: Penalty = (Normalized_Est_Cost * Weight_Cost) + (Average_LatencyMs * Weight_Latency)

## 5. Failover and Rate-Limit State Machine
The broker implements a multi-paradigm failover strategy to ensure 100% uptime.
* **API Rate Limits (HTTP 429):** If Anthropic returns a 429 error, the broker instantly circuit-breaks the node and re-routes to OpenAI or Google.
* **Spot Preemption:** If a cloud provider abruptly terminates a Spot GPU instance due to a market price spike, the broker intercepts the infrastructure failure and queries the Oracle for the next cheapest GPU provider to restart the containerized task.
* **Model Escalation:** If Local Ollama (Tier 0) fails to output valid JSON, the broker upgrades the task requirement and routes it to a flagship Commercial API (Tier 1) to resolve the complexity.