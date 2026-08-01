package agent

import (
	"strings"
	"testing"
)

func TestDynamicAgentRouting(t *testing.T) {
	// Mock a custom user-defined agent loaded from a markdown file (e.g. legal-agent)
	loadedAgentConfigsMu.Lock()
	origConfigs := loadedAgentConfigs
	loadedAgentConfigs = map[string]AgentJSONConfig{
		"legal-compliance-agent": {
			SystemPrompt: "You are the Legal & Compliance Officer. Review contracts, NDAs, and licensing agreements.",
			Description:  "Legal advisor and contract auditor",
			Capabilities: []string{"contract review", "compliance auditing", "legal risk evaluation"},
			Tools:        []string{"read_file", "write_file"},
		},
		"generalist-agent": {
			SystemPrompt: "General assistant",
			Description:  "General tasks",
			Capabilities: []string{"general"},
		},
	}
	loadedAgentConfigsMu.Unlock()
	defer func() {
		loadedAgentConfigsMu.Lock()
		loadedAgentConfigs = origConfigs
		loadedAgentConfigsMu.Unlock()
	}()

	// 1. Direct match on custom target ID
	target := ResolveTargetAgent("legal-compliance-agent", "Please review this NDA.")
	if target != "legal-compliance-agent" {
		t.Errorf("expected legal-compliance-agent, got: %s", target)
	}

	// 2. Dynamic matching via custom capability terms without any hardcoded switch
	target = ResolveTargetAgent("", "Perform a compliance auditing check on our supplier contract.")
	if target != "legal-compliance-agent" {
		t.Errorf("expected dynamic routing to match legal-compliance-agent, got: %s", target)
	}
}

func TestSystemWarningsDependencyFiltering(t *testing.T) {
	// 1. Mock loaded agent configs without Docker tools
	loadedAgentConfigsMu.Lock()
	origConfigs := loadedAgentConfigs
	loadedAgentConfigs = map[string]AgentJSONConfig{
		"writer-agent": {
			Tools: []string{"read_file", "write_file"},
		},
	}
	loadedAgentConfigsMu.Unlock()
	defer func() {
		loadedAgentConfigsMu.Lock()
		loadedAgentConfigs = origConfigs
		loadedAgentConfigsMu.Unlock()
	}()

	// Since writer-agent does not use execute_python_docker or execute_bash_docker or web_search_and_extract,
	// CheckSystemWarnings should NOT warn about Docker or Tavily API key!
	warnings := CheckSystemWarnings()
	for _, w := range warnings {
		if strings.Contains(w, "Docker") {
			t.Errorf("unexpected Docker warning when no active agent depends on Docker: %s", w)
		}
		if strings.Contains(w, "TAVILY") {
			t.Errorf("unexpected Tavily warning when no active agent depends on web search: %s", w)
		}
	}

	// 2. Test warning suppression via config
	SetSystemWarningsConfig(SystemWarningsConfig{
		DisabledChecks: []string{"tavily", "docker"},
		CustomChecks: []CustomWarningCheck{
			{
				ID:      "custom_db",
				EnvVar:  "NON_EXISTENT_CUSTOM_DB_URL",
				Warning: "Custom DB URL is missing.",
			},
		},
	})

	warningsWithCustom := CheckSystemWarnings()
	foundCustom := false
	for _, w := range warningsWithCustom {
		if strings.Contains(w, "Custom DB URL is missing") {
			foundCustom = true
		}
	}
	if !foundCustom {
		t.Errorf("expected custom warning to be triggered")
	}

	// Reset config
	SetSystemWarningsConfig(SystemWarningsConfig{})
}
