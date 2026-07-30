# Memory and State Persistence Specification

## 1. Core Philosophy
This framework rejects flat Markdown files for long-term agent memory. Relying on flat files for ongoing context is a guaranteed recipe for massive token bloat and API bankruptcy. Instead, all conversational context, tool execution results, and agent reasoning logs are persisted in a fast, embedded database running locally on the host machine. The storage layer must be abstracted behind Go interfaces (e.g., `CheckpointStore`) to allow seamless migration from local embedded databases to distributed caches like Redis if horizontal scaling is required later.

## 2. The Default Engine: SQLite (WAL Mode)
The default, single-binary implementation utilizes `sqlite`. Running inside the Go process effectively zeroes out state-retrieval latency. To solve SQLite's traditional concurrency limitations, the database connection MUST initialize with Write-Ahead Logging (WAL) enabled. 
* **Pragma Requirement:** `PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL;`
* **Concurrency:** Readers do not block writers, allowing dozens of lightweight agent goroutines to query context simultaneously without locking the database.

## 3. Data Schemas
The database will maintain three primary tables to handle hybrid routing and context:

### A. Context Logs (`context_logs`)
Stores the raw inputs, reasoning steps, and outputs of the agents.
* `id` (UUID, Primary Key)
* `agent_id` (String, Index)
* `session_id` (String, Index)
* `role` (Enum: USER, SYSTEM, AGENT, TOOL)
* `content` (Text)
* `embedding` (Blob/Vector - for semantic search retrieval)
* `created_at` (Timestamp)

### B. Tool Execution Checkpoints (`step_checkpoints`)
Critical for idempotency and preventing double-execution during API rate limits or network retries.
* `task_id` (String, Index)
* `tool_name` (String)
* `status` (Enum: PENDING, SUCCESS, FAILED)
* `output_payload` (JSON)
* `executed_at` (Timestamp)

### C. Active Routing State (`active_tasks`)
Tracks the lifecycle of a task currently traversing the Hybrid Compute Broker.
* `task_id` (String, Primary Key)
* `current_agent` (String)
* `status` (Enum: IN_PROGRESS, AWAITING_HUMAN, RESOLVED)
* `compute_tier_used` (Enum: TIER_0_LOCAL, TIER_1_API, TIER_2_SPOT)

## 4. Context Injection Pipeline (RAG)
When an agent requires historical context, the framework must not dump the entire `context_logs` table into the prompt.
1. The framework uses the Model Context Protocol (MCP) to standardize how agents access tools and data.
2. It converts the current task into a vector embedding.
3. It queries the `context_logs` table for the top 3-5 most semantically relevant prior interactions.
4. Only those specific interactions are formatted into Markdown and injected into the MCP payload. This slashes token usage and speeds up reasoning.

## 5. Security & Idempotency Rules
* **Pre-Execution Locks:** Before a tool executes (especially Tier 3 Docker tools or financial transactions), a `PENDING` record must be written to `step_checkpoints`. 
* **Retry Safety:** If the Hybrid Broker triggers an escalation (e.g., Ollama fails and routes to GPT-4o) or a retry loop due to a rate limit, the orchestrator MUST check `step_checkpoints` first. If a successful payload exists, it feeds the cached JSON directly to the agent instead of physically re-running the tool.