# Multi-Agent Loop, Real Tools, and HITL Integration Specification

This document defines the architecture, data flow, and verification plan for:
1. A collaborative point-to-point multi-agent loop between `CoderAgent` and `WriterAgent`.
2. Real-world execution boundary tools (Tier 1 File Editor with path jailing, Tier 2 WASM text casing, and Tier 3 Docker Python evaluator with memory bounds).
3. Human-in-the-Loop (HITL) approval controls for high-risk tool operations.

---

## 1. Multi-Agent Collaborative Loop

The workflow is triggered by sending a task to `CoderAgent` with routing hints indicating that `WriterAgent` is the downstream consumer.

```
 USER ──▶ [CoderAgent] ──(A2A Hand-off)──▶ [WriterAgent] ──▶ [USER (HITL Approval)]
```

### Protocol Mechanics
1. The **User** seeds the pipeline by sending a query to `coder-agent`. The message contains:
   - `msg.Content`: The code requirements.
   - `msg.Metadata["reply_to"] = "writer-agent"`: Routing hint directing the Coder's output.
   - `msg.Metadata["correlation_id"]`: Session trace token.
2. **CoderAgent** writes the code. Upon completion, instead of returning the result to the sender, it inspects `Metadata["reply_to"]` and forwards the output to `writer-agent`.
3. **WriterAgent** receives the code, appends markdown documentation, and routes the final result back to the `USER` (triggering terminal HITL display).

---

## 2. Real-World Execution Boundary Tools

To enforce security boundaries, we implement real-world logic across all three tiers:

### Tier 1 (Native Go): Jailed File Editor
- **Name:** `edit_file`
- **Capability:** Writes content to a local file.
- **Security Check:** Resolves the absolute path of the target file using `filepath.Abs`. If the path does not have the prefix of the current workspace directory, the operation is blocked and returns an unauthorized access error.

### Tier 2 (WebAssembly): Wazero Text Case Formatter
- **Name:** `wasm_transform`
- **Capability:** Evaluates a compiled WASM text transformation module.
- **Security Check:** Runs in a sandbox with zero preopened host paths, environment variables, or sockets.

### Tier 3 (Docker): Ephemeral Python Sandbox
- **Name:** `python_run`
- **Capability:** Runs arbitrary Python calculations.
- **Security Check:** Memory limits are set via container configs (`HostConfig.Resources.Memory = 512 * 1024 * 1024` for 512MB RAM cap). CPU and execution timeouts are enforced.

---

## 3. Human-in-the-Loop (HITL) Integration

High-risk tool invocations (such as writing to a file via Tier 1 `edit_file` or executing arbitrary code via Tier 3 `python_run`) require manual operator sign-off before execution.

```
 [Agent] ─▶ Send(Recipient: "USER", Type: "HITL_APPROVAL") ─▶ (Orchestrator parks router)
                                                                       │
 [Agent] ◀─ Mailbox read APPROVED/REJECTED ◀─ Send(Type: "HITL_RESPONSE") ─┘
```

### Step-by-Step Approval Protocol
1. **Checkpoint State:** Before sending the request, the agent registers a checkpoint indicating it is waiting for approval.
2. **Blocking Request:** The agent sends a message with `Recipient: "USER"`, `Metadata["Type"] = "HITL_APPROVAL"`, and details of the action.
3. **Bus Blocking:** The orchestrator router goroutine performs an unbuffered send to `humanApproval` and blocks, parking the router thread.
4. **Terminal Input:** The CLI handler prints the approval prompt, reads user input (`y/N`), and pushes the response onto the message bus.
5. **Resume Execution:** The agent reads the response from its mailbox. If `Content == "APPROVED"`, it executes the tool. If rejected, it aborts.
