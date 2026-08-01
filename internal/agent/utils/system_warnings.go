package utils

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// CustomWarningCheck defines a user-configurable environment requirement check.
type CustomWarningCheck struct {
	ID           string `json:"id"`
	EnvVar       string `json:"env_var,omitempty"`
	Warning      string `json:"warning"`
	RequiredTool string `json:"required_tool,omitempty"`
}

// SystemWarningsConfig holds warning suppression and custom check configurations.
type SystemWarningsConfig struct {
	DisabledChecks []string             `json:"disabled_checks"`
	CustomChecks   []CustomWarningCheck `json:"custom_checks"`
}

var (
	warningsConfigMu sync.RWMutex
	warningsConfig   SystemWarningsConfig
)

// LoadSystemWarningsConfig reads and parses a warnings configuration JSON file.
func LoadSystemWarningsConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var cfg SystemWarningsConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to parse system warnings config at %s: %w", path, err)
	}

	warningsConfigMu.Lock()
	warningsConfig = cfg
	warningsConfigMu.Unlock()
	return nil
}

// SetSystemWarningsConfig programmatically sets the warnings configuration.
func SetSystemWarningsConfig(cfg SystemWarningsConfig) {
	warningsConfigMu.Lock()
	warningsConfig = cfg
	warningsConfigMu.Unlock()
}

// GetSystemWarningsConfig returns a copy of the current system warnings configuration.
func GetSystemWarningsConfig() SystemWarningsConfig {
	warningsConfigMu.RLock()
	defer warningsConfigMu.RUnlock()
	return warningsConfig
}

// ActiveToolsResolver returns the active tools map and alias resolver function.
type ActiveToolsResolver func() (map[string]bool, func(string) string)

// CheckSystemWarnings evaluates environment dependencies dynamically based on active agent tool requirements.
func CheckSystemWarnings(getToolsAndAlias ActiveToolsResolver) []string {
	warningsConfigMu.RLock()
	cfg := warningsConfig
	warningsConfigMu.RUnlock()

	disabledMap := make(map[string]bool)
	for _, d := range cfg.DisabledChecks {
		disabledMap[strings.ToLower(strings.TrimSpace(d))] = true
	}
	if envSuppress := os.Getenv("SUPPRESS_WARNINGS"); envSuppress != "" {
		for _, part := range strings.Split(envSuppress, ",") {
			disabledMap[strings.ToLower(strings.TrimSpace(part))] = true
		}
	}

	activeTools := make(map[string]bool)
	var resolveAlias func(string) string
	if getToolsAndAlias != nil {
		activeTools, resolveAlias = getToolsAndAlias()
	}
	if resolveAlias == nil {
		resolveAlias = func(s string) string { return s }
	}

	var warnings []string

	// 2. Dependency Check: Web Search (Tavily API)
	dependsOnSearch := activeTools["web_search_and_extract"] || activeTools["fetch_html"]
	if dependsOnSearch && !disabledMap["tavily"] && !disabledMap["search"] {
		if os.Getenv("TAVILY_API_KEY") == "" {
			warnings = append(warnings, "TAVILY_API_KEY is not set. Agents using web search will fallback to simulated mock results.")
		}
	}

	// 3. Dependency Check: Ollama / Local Model Inference
	dependsOnOllama := false
	if os.Getenv("OLLAMA_HOST") != "" {
		dependsOnOllama = true
	} else if data, err := os.ReadFile("models.json"); err == nil {
		if strings.Contains(strings.ToLower(string(data)), "ollama") || strings.Contains(string(data), "11434") {
			dependsOnOllama = true
		}
	}

	if dependsOnOllama && !disabledMap["ollama"] && !disabledMap["local_llm"] {
		ollamaHost := os.Getenv("OLLAMA_HOST")
		if ollamaHost == "" {
			ollamaHost = "127.0.0.1:11434"
		} else {
			ollamaHost = strings.TrimPrefix(ollamaHost, "http://")
			ollamaHost = strings.TrimPrefix(ollamaHost, "https://")
		}
		conn, err := net.DialTimeout("tcp", ollamaHost, 100*time.Millisecond)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("Local Ollama daemon is unreachable on http://%s.", ollamaHost))
		} else {
			conn.Close()
		}
	}

	// 4. Dependency Check: Docker Socket Execution
	dependsOnDocker := activeTools["execute_python_docker"] || activeTools["execute_bash_docker"]
	if dependsOnDocker && !disabledMap["docker"] {
		conn, err := net.DialTimeout("unix", "/var/run/docker.sock", 100*time.Millisecond)
		if err != nil {
			warnings = append(warnings, "Docker daemon socket (/var/run/docker.sock) is unreachable. Docker container sandbox execution is unavailable.")
		} else {
			conn.Close()
		}
	}

	// 5. Mock Mode & Environment Settings Checks
	if os.Getenv("USE_MOCK_PRICING") == "true" || os.Getenv("MOCK_PRICING") == "true" || os.Getenv("PRICING_PROVIDER") == "mock" {
		warnings = append(warnings, "⚠️ Mock Pricing Mode Active: Pricing Oracle is returning synthetic compute & option pricing data.")
	}

	if os.Getenv("USE_MOCK_MODELS") == "true" || os.Getenv("MOCK_MODELS") == "true" || os.Getenv("MOCK_LLM") == "true" {
		warnings = append(warnings, "⚠️ Mock Models Mode Active: LLM inference is using fallback simulated model responses.")
	}

	if os.Getenv("USE_MOCK_TOOLS") == "true" || os.Getenv("MOCK_TOOLS") == "true" || os.Getenv("MOCK_COMPUTE") == "true" || activeTools["submit_mock_task"] || activeTools["check_mock_task"] || activeTools["query_compute_prices"] || activeTools["query_forward_curves"] || activeTools["query_options_chain"] || activeTools["query_vol_surface"] {
		warnings = append(warnings, "⚠️ Mock Compute Tools Connected: Specialist agents are interacting with simulated mock API endpoints (/api/mock/*).")
	}

	// 6. Evaluate User-Defined Custom Warning Checks
	for _, custom := range cfg.CustomChecks {
		checkID := strings.ToLower(strings.TrimSpace(custom.ID))
		if checkID != "" && disabledMap[checkID] {
			continue
		}

		if custom.RequiredTool != "" && !activeTools[resolveAlias(custom.RequiredTool)] {
			continue
		}

		if custom.EnvVar != "" && os.Getenv(custom.EnvVar) == "" {
			warnings = append(warnings, custom.Warning)
		}
	}

	return warnings
}
