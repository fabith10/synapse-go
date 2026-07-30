# General-Purpose Concurrent Multi-Agent Framework

## System Overview
[cite_start]This framework is a high-performance, cost-sensitive, general-purpose agentic system[cite: 79]. [cite_start]The core orchestrator is built entirely in Go (Golang) to solve the "Slower Than Scripts" and "Memory Bloat" issues common in off-the-shelf Python frameworks[cite: 16, 17]. [cite_start]The compiled Go framework runs completely locally on your machine, ensuring your tools, sandboxes, and file systems remain completely private[cite: 132].

## Hybrid Compute Routing
[cite_start]The orchestrator should be smart enough to look at a task and choose between three completely different paradigms[cite: 169]:

1. [cite_start]**Local Inference ($0 Cost):** Using Ollama for triage and simple tasks[cite: 170]. 
2. [cite_start]**Commercial APIs (Token-Priced):** Using OpenAI/Anthropic/Google for high-speed, general-purpose heavy reasoning[cite: 171].
3. [cite_start]**Data Center Spot Compute (Hardware-Priced):** Spinning up raw GPU instances based on CME forward curves for heavy quantitative finance calculations or custom model training[cite: 172]. 

## Memory and Context Management
[cite_start]The framework explicitly avoids relying on flat Markdown files to store your agent's ongoing memory[cite: 25]. 

* [cite_start]**Standardization:** You can use the Model Context Protocol (MCP) to standardize how your agents access tools, but back your memory with a fast, embedded database (like SQLite paired with vector embeddings)[cite: 96]. 
* [cite_start]**Injection:** When the agent needs context, you run a semantic search and only inject the top 3 most relevant paragraphs into the prompt as Markdown[cite: 28]. [cite_start]This slashes token usage and speeds up reasoning[cite: 29].
* [cite_start]**Abstraction:** You should abstract the storage layer using Go interfaces from day one[cite: 75]. [cite_start]By coding against a `CheckpointStore` interface, you build an `sqliteStore` implementation today but can swap it out for a `redisStore` later[cite: 76].

## Resilience and Escalation
[cite_start]The broker implements a multi-paradigm failover strategy to ensure 100% uptime[cite: 183]:

* [cite_start]**API Rate Limits (HTTP 429):** If Anthropic returns a 429 error, the broker instantly circuit-breaks the node and re-routes to OpenAI or Google[cite: 184].
* [cite_start]**Spot Preemption:** If a cloud provider abruptly terminates a Spot GPU instance due to a market price spike, the broker intercepts the infrastructure failure and queries the Oracle for the next cheapest GPU provider to restart the containerized task[cite: 185].
* [cite_start]**Model Escalation:** If Local Ollama (Tier 0) fails to output valid JSON, the broker upgrades the task requirement and routes it to a flagship Commercial API (Tier 1) to resolve the complexity[cite: 186].

## Security and Tool Execution
[cite_start]Security in agentic frameworks comes down to strict boundaries[cite: 30]. 

* [cite_start]**Native HITL:** Because Go relies on message channels, building a major Human-in-the-Loop component is as simple as routing high-risk intents to a `human_approval` channel[cite: 32]. [cite_start]The agent's thread securely pauses (blocks) until you hit "Y" in the terminal or click "Approve" on a UI[cite: 33].
* **Tier 1 (Native Go):** Fast and safe native Go functions for everyday tasks. 
* [cite_start]**Tier 2 (WebAssembly):** For executing lightweight, LLM-generated JavaScript or Python snippets[cite: 91]. [cite_start]You pass the generated code into `wazero` (a pure-Go WebAssembly runtime)[cite: 92]. [cite_start]WebAssembly runs in a strict memory sandbox with zero access to your host machine's filesystem, network, or environment variables by default[cite: 93].
* [cite_start]**Tier 3 (Ephemeral Docker):** For heavy quantitative finance tools[cite: 83]. [cite_start]You must use the Go Docker SDK to dynamically spin up a pre-built Docker container configured with the necessary Python/C++ quant libraries[cite: 84].