package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	dockerclient "github.com/docker/docker/client"
	"github.com/fabith10/synapse-go/adk"
	agenttools "github.com/fabith10/synapse-go/internal/agent/tools"
	"github.com/fabith10/synapse-go/internal/tools"
	"github.com/fabith10/synapse-go/pkg/logger"
)

// MultiTierSandbox acts as the central router for tool sandboxes.
type MultiTierSandbox struct {
	wasm   *tools.WasmSandbox
	docker *tools.DockerSandbox
}

func NewMultiTierSandbox() *MultiTierSandbox {
	return NewMultiTierSandboxWithMemory(0)
}

func NewMultiTierSandboxWithMemory(containerMemMB int64) *MultiTierSandbox {
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	var dockerSb *tools.DockerSandbox
	if err == nil && cli != nil {
		pingCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		if _, pingErr := cli.Ping(pingCtx); pingErr == nil {
			dockerSb, _ = tools.NewDockerSandboxWithMemory(cli, containerMemMB)
		}
	}
	return &MultiTierSandbox{
		wasm:   tools.NewWasmSandbox(),
		docker: dockerSb,
	}
}

// Execute dispatches the request to either WebAssembly or Docker.
func (m *MultiTierSandbox) Execute(ctx context.Context, req adk.ExecutionRequest) adk.ExecutionResult {
	if req.Language == "wasm" {
		return m.wasm.Execute(ctx, req)
	}
	if m.docker == nil {
		return adk.ExecutionResult{Error: tools.ErrDockerUnavailable}
	}
	return m.docker.Execute(ctx, req)
}

// Bootstrap instantiates the orchestrator and registers the concrete agent blueprints.
func Bootstrap(cfg adk.Config) (*adk.Runtime, error) {
	// 1. Initialize the Runtime
	rt, err := adk.NewRuntime(cfg)
	if err != nil {
		return nil, fmt.Errorf("agent bootstrap: NewRuntime: %w", err)
	}

	// 2. Setup Sandboxes — container memory cap comes from cfg
	sandbox := NewMultiTierSandboxWithMemory(cfg.ContainerMemoryMB)
	if cfg.ContainerMemoryMB > 0 {
		logger.Info("Docker container memory hard-capped", "memory_mb", cfg.ContainerMemoryMB)
	}

	// Initialize CronScheduler
	scheduler := agenttools.NewCronScheduler(rt.Orchestrator(), "schedules.json")
	scheduler.Start(context.Background())

	// Load agents directory if present, otherwise fallback to agents.json to override system prompts dynamically
	if len(GetLoadedAgentConfigs()) == 0 {
		if err := LoadAgentsConfig(findConfigPath("agents")); err != nil {
			if errJSON := LoadAgentsConfig(findConfigPath("agents.json")); errJSON != nil {
				logger.Warn("Could not load agent configs, falling back to static blueprints", "dir_error", err, "json_error", errJSON)
			}
		}
	}

	// 3. Register agents and their tools

	// Map of shared tools that don't need agent-specific state
	sharedTools := map[string]adk.Tool{
		"fetch_html":              agenttools.GetFetchHTMLTool(),
		"wasm_json_mapper":        agenttools.GetWasmJsonMapperTool(sandbox),
		"web_search_and_extract":  agenttools.GetWebSearchAndExtractTool(),
		"generate_pdf_report":     agenttools.GetGeneratePDFReportTool(),
		"check_lead_score":        agenttools.GetCheckLeadScoreTool(),
		"browser_navigate":        agenttools.GetBrowserNavigateTool(),
		"browser_input":           agenttools.GetBrowserInputTool(),
		"browser_click":           agenttools.GetBrowserClickTool(),
		"browser_scroll":          agenttools.GetBrowserScrollTool(),
		"browser_wait":            agenttools.GetBrowserWaitForTool(),
		"browser_extract_js":      agenttools.GetBrowserExtractJSTool(),
		"browser_screenshot":      agenttools.GetBrowserScreenshotTool(),
		"grep_documents":          agenttools.GetGrepDocumentsTool(),
		"read_file":               agenttools.GetReadFileTool(),
		"write_file":              agenttools.GetWriteFileTool(),
		"replace_file_content":    agenttools.GetReplaceFileContentTool(),
		"list_directory":          agenttools.GetListDirectoryTool(),
		"query_pricing_oracle":    agenttools.GetQueryPricingOracleTool(),
		"extract_pdf_text":        agenttools.GetExtractPDFTextTool(),
		"semantic_search_context": agenttools.GetSemanticSearchContextTool(rt.Store()),
		"read_state_variable":     GetReadStateVariableTool(rt.Orchestrator()),
		"write_state_variable":    GetWriteStateVariableTool(rt.Orchestrator()),
		"schedule_task":           agenttools.GetScheduleTaskTool(),
		"list_schedules":          agenttools.GetListSchedulesTool(),
		"cancel_schedule":         agenttools.GetCancelScheduleTool(),
		"save_long_term_memory":    agenttools.GetSaveLongTermMemoryTool(rt.Store()),
		"search_long_term_memories": agenttools.GetSearchLongTermMemoriesTool(rt.Store()),
		"inspect_host_hardware":   agenttools.GetInspectHostHardwareTool(),
		"query_compute_prices":    agenttools.GetQueryComputePricesTool(),
		"query_forward_curves":    agenttools.GetQueryForwardCurvesTool(),
		"query_options_chain":     agenttools.GetQueryOptionsChainTool(),
		"submit_mock_task":        agenttools.GetSubmitMockTaskTool(),
		"check_mock_task":         agenttools.GetCheckMockTaskTool(),
		"wait_seconds":            agenttools.GetWaitSecondsTool(),
		"wait":                    agenttools.GetWaitSecondsTool(),
	}

	// Map of specific tool builders that take (agentID, mailbox)
	agentSpecificBuilders := map[string]func(id string, mb chan adk.Message) adk.Tool{
		"execute_python_docker": func(id string, mb chan adk.Message) adk.Tool {
			return tools.SecureTool(agenttools.GetExecutePythonDockerTool(sandbox, rt.Orchestrator(), id, mb), []string{id})
		},
		"execute_bash_docker": func(id string, mb chan adk.Message) adk.Tool {
			return tools.SecureTool(agenttools.GetExecuteBashDockerTool(sandbox, rt.Orchestrator(), id, mb), []string{id})
		},
		"modify_excel_workbook": func(id string, mb chan adk.Message) adk.Tool {
			return tools.SecureTool(agenttools.GetModifyExcelWorkbookTool(rt.Orchestrator(), id, mb), []string{id})
		},
		"write_email": func(id string, mb chan adk.Message) adk.Tool {
			return tools.SecureTool(agenttools.GetWriteEmailTool(rt.Orchestrator(), id, mb), []string{id})
		},
		"delegate_subtask": func(id string, mb chan adk.Message) adk.Tool {
			return agenttools.GetDelegateSubtaskTool(rt.Orchestrator(), id, mb)
		},
	}

	// Helper to resolve dynamic or default tools list for an agent
	resolveTools := func(id string, mb chan adk.Message, defaults []string) []adk.Tool {
		configs := GetLoadedAgentConfigs()
		var toolNames []string
		if cfg, ok := configs[id]; ok && len(cfg.Tools) > 0 {
			toolNames = cfg.Tools
		} else {
			toolNames = defaults
		}

		var agentTools []adk.Tool
		for _, name := range toolNames {
			if builder, ok := agentSpecificBuilders[name]; ok {
				agentTools = append(agentTools, builder(id, mb))
			} else if t, ok := sharedTools[name]; ok {
				agentTools = append(agentTools, t)
			} else {
				fmt.Fprintf(os.Stderr, "Warning: Agent %q configured tool %q which is not present in registry!\n", id, name)
			}
		}
		return agentTools
	}

	// Auto-load critical actions configuration if present in workspace
	for _, cfgPath := range []string{".agents/critical_actions.json", "critical_actions.json"} {
		if _, err := os.Stat(cfgPath); err == nil {
			_ = agenttools.LoadCriticalActionsConfig(cfgPath)
			break
		}
	}

	// Auto-load tool aliases configuration if present in workspace
	for _, aliasPath := range []string{".agents/tool_aliases.json", "tool_aliases.json"} {
		if _, err := os.Stat(aliasPath); err == nil {
			_ = agenttools.LoadToolAliasesConfig(aliasPath)
			break
		}
	}

	// Auto-load system warnings configuration if present in workspace
	for _, warnPath := range []string{".agents/system_warnings.json", "system_warnings.json"} {
		if _, err := os.Stat(warnPath); err == nil {
			_ = LoadSystemWarningsConfig(warnPath)
			break
		}
	}

	// Configure dynamic fallback resolver on orchestrator for unmapped target agent IDs
	rt.Orchestrator().FallbackResolver = ResolveTargetAgent

	// A. Gatekeeper Agent (triage)
	gatekeeperTools := []adk.Tool{
		agenttools.GetPricingOracleTool(),
		agenttools.GetScheduleTaskTool(),
		agenttools.GetListSchedulesTool(),
		agenttools.GetCancelScheduleTool(),
		agenttools.GetWaitSecondsTool(),
	}
	gatekeeper := NewGatekeeperAgent("triage-agent", rt.LLMClient(), rt.Orchestrator(), gatekeeperTools)
	gatekeeperBase, err := rt.RegisterAgent(gatekeeper.ID, gatekeeper)
	if err != nil {
		rt.Shutdown()
		return nil, fmt.Errorf("agent bootstrap: register gatekeeper: %w", err)
	}
	gatekeeper.BaseAgent = gatekeeperBase

	// B. Supervisor Agent (Goal evaluation)
	supervisor := NewSupervisorAgent("supervisor-agent", rt.LLMClient(), rt.Orchestrator())
	supervisorBase, err := rt.RegisterAgent(supervisor.ID, supervisor)
	if err != nil {
		rt.Shutdown()
		return nil, fmt.Errorf("agent bootstrap: register supervisor: %w", err)
	}
	supervisor.BaseAgent = supervisorBase

	// C. Planner Agent
	planner := NewPlannerAgent("planner-agent", rt.LLMClient(), rt.Orchestrator(), nil)
	plannerBase, err := rt.RegisterAgent(planner.ID, planner)
	if err != nil {
		rt.Shutdown()
		return nil, fmt.Errorf("agent bootstrap: register planner: %w", err)
	}
	planner.BaseAgent = plannerBase

	// Load and dynamically register other specialist agents defined in configuration
	configs := GetLoadedAgentConfigs()
	customAgents := map[string]bool{
		"triage-agent":     true,
		"supervisor-agent": true,
		"planner-agent":    true,
	}

	for id, cfg := range configs {
		if customAgents[id] {
			continue
		}
		dynAgent := NewGenericSpecialistAgent(id, cfg.SystemPrompt, nil, rt.LLMClient(), rt.Orchestrator(), cfg.HardwareTier, cfg.MaxWillingToPay).
			WithStore(rt.Store()) // enables RAG (PDR-002 §4) and CLARIFICATION_REQUIRED flow
		dynBase, err := rt.RegisterAgent(id, dynAgent)
		if err != nil {
			rt.Shutdown()
			return nil, fmt.Errorf("agent bootstrap: register dynamic agent %s: %w", id, err)
		}
		dynAgent.BaseAgent = dynBase
		dynAgent.tools = resolveTools(id, dynBase.Mailbox, nil)
		for _, t := range dynAgent.tools {
			_ = rt.Registry().Register(t)
		}
	}

	return rt, nil
}

func findConfigPath(name string) string {
	wd, err := os.Getwd()
	if err != nil {
		return name
	}
	curr := wd
	for {
		candidate := filepath.Join(curr, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}
	return name
}

// ToolDescriptor represents tool metadata for the UI configuration panel.
type ToolDescriptor struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"` // "Native Go (Tier 1)", "WASM (Tier 2)", "Docker (Tier 3)"
	IsGated     bool   `json:"is_gated"`
}

// GetAvailableToolsList returns all supported framework tools with metadata for the UI.
func GetAvailableToolsList() []ToolDescriptor {
	return []ToolDescriptor{
		{Name: "web_search_and_extract", Description: "Search the web and extract clean Markdown text context", Category: "Native Go (Tier 1)", IsGated: false},
		{Name: "generate_pdf_report", Description: "Compile structured summaries into executive PDF reports", Category: "Native Go (Tier 1)", IsGated: false},
		{Name: "modify_excel_workbook", Description: "Parse, modify, and update financial Excel workbooks", Category: "Native Go (Tier 1)", IsGated: false},
		{Name: "write_email", Description: "Draft and save outbound communication emails", Category: "Native Go (Tier 1)", IsGated: false},
		{Name: "check_lead_score", Description: "Calculate lead priority score based on CRM attributes", Category: "Native Go (Tier 1)", IsGated: false},
		{Name: "fetch_html", Description: "Fetch raw web page content and strip malicious scripts", Category: "Native Go (Tier 1)", IsGated: false},
		{Name: "grep_documents", Description: "Search document directories with pattern matching", Category: "Native Go (Tier 1)", IsGated: false},
		{Name: "read_file", Description: "Read content from workspace files", Category: "Native Go (Tier 1)", IsGated: false},
		{Name: "write_file", Description: "Write output content to workspace files", Category: "Native Go (Tier 1)", IsGated: false},
		{Name: "schedule_task", Description: "Schedule automated background tasks via cron expressions", Category: "Native Go (Tier 1)", IsGated: false},
		{Name: "inspect_host_hardware", Description: "Query host system CPU, RAM, and GPU specs", Category: "Native Go (Tier 1)", IsGated: false},
		{Name: "wasm_json_mapper", Description: "Transform JSON data inside Wazero WebAssembly sandbox", Category: "WASM (Tier 2)", IsGated: false},
		{Name: "execute_python_docker", Description: "Execute dynamic Python math & analytics in Docker container", Category: "Docker (Tier 3)", IsGated: true},
		{Name: "execute_bash_docker", Description: "Execute shell script pipeline inside Docker container", Category: "Docker (Tier 3)", IsGated: true},
		{Name: "delegate_subtask", Description: "Delegate a subtask to another specialist agent (developer-agent, researcher-agent, quant-agent, writer-agent)", Category: "Native Go (Tier 1)", IsGated: false},
	}
}

// RegisterDynamicAgent dynamically registers a new agent at runtime.
func RegisterDynamicAgent(rt *adk.Runtime, id string, cfg AgentJSONConfig) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("agent ID cannot be empty")
	}
	reserved := map[string]bool{"triage-agent": true, "supervisor-agent": true, "planner-agent": true}
	if reserved[id] {
		return fmt.Errorf("cannot override reserved system agent %q", id)
	}

	// Gate Tier 3 Docker execution tools behind administrator toggle
	dockerTools := map[string]bool{
		"execute_python_docker": true,
		"execute_bash_docker":   true,
	}
	for _, toolName := range cfg.Tools {
		if dockerTools[toolName] && !IsDockerToolsEnabled() {
			return fmt.Errorf("Tier 3 Docker tool %q is restricted: Docker tools are disabled by administrator policy", toolName)
		}
	}

	sandbox := NewMultiTierSandbox()

	// Build dynamic specialist agent
	dynAgent := NewGenericSpecialistAgent(id, cfg.SystemPrompt, nil, rt.LLMClient(), rt.Orchestrator(), cfg.HardwareTier, cfg.MaxWillingToPay).
		WithStore(rt.Store())

	dynBase, err := rt.RegisterAgent(id, dynAgent)
	if err != nil {
		return fmt.Errorf("failed to register agent %s in orchestrator: %w", id, err)
	}
	dynAgent.BaseAgent = dynBase

	// Build tools map
	sharedTools := map[string]adk.Tool{
		"fetch_html":              agenttools.GetFetchHTMLTool(),
		"wasm_json_mapper":        agenttools.GetWasmJsonMapperTool(sandbox),
		"web_search_and_extract":  agenttools.GetWebSearchAndExtractTool(),
		"generate_pdf_report":     agenttools.GetGeneratePDFReportTool(),
		"check_lead_score":        agenttools.GetCheckLeadScoreTool(),
		"grep_documents":          agenttools.GetGrepDocumentsTool(),
		"read_file":               agenttools.GetReadFileTool(),
		"write_file":              agenttools.GetWriteFileTool(),
		"replace_file_content":    agenttools.GetReplaceFileContentTool(),
		"list_directory":          agenttools.GetListDirectoryTool(),
		"query_pricing_oracle":    agenttools.GetQueryPricingOracleTool(),
		"extract_pdf_text":        agenttools.GetExtractPDFTextTool(),
		"semantic_search_context": agenttools.GetSemanticSearchContextTool(rt.Store()),
		"schedule_task":           agenttools.GetScheduleTaskTool(),
		"inspect_host_hardware":   agenttools.GetInspectHostHardwareTool(),
	}

	var agentTools []adk.Tool
	for _, name := range cfg.Tools {
		if name == "execute_python_docker" {
			agentTools = append(agentTools, tools.SecureTool(agenttools.GetExecutePythonDockerTool(sandbox, rt.Orchestrator(), id, dynBase.Mailbox), []string{id}))
		} else if name == "execute_bash_docker" {
			agentTools = append(agentTools, tools.SecureTool(agenttools.GetExecuteBashDockerTool(sandbox, rt.Orchestrator(), id, dynBase.Mailbox), []string{id}))
		} else if name == "modify_excel_workbook" {
			agentTools = append(agentTools, tools.SecureTool(agenttools.GetModifyExcelWorkbookTool(rt.Orchestrator(), id, dynBase.Mailbox), []string{id}))
		} else if name == "write_email" {
			agentTools = append(agentTools, tools.SecureTool(agenttools.GetWriteEmailTool(rt.Orchestrator(), id, dynBase.Mailbox), []string{id}))
		} else if t, ok := sharedTools[name]; ok {
			agentTools = append(agentTools, t)
		}
	}
	dynAgent.tools = agentTools
	for _, t := range agentTools {
		_ = rt.Registry().Register(t)
	}

	UpdateAgentConfig(id, cfg)
	_ = SaveAgentsConfig("agents.json")

	logger.Info("Dynamically registered new agent", "agent_id", id, "tools_count", len(agentTools))
	return nil
}

