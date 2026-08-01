package agent

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/fabith10/synapse-go/pkg/logger"
)

// CriticalActionsConfig defines the configurable parameters for HITL approval checks.
type CriticalActionsConfig struct {
	RiskyPythonKeywords     []string `json:"risky_python_keywords"`
	RiskyBashKeywords       []string `json:"risky_bash_keywords"`
	ProductionExcelPatterns []string `json:"production_excel_patterns"`
	InternalEmailDomains    []string `json:"internal_email_domains"`
	AutoApproveAll          bool     `json:"auto_approve_all"`
	RequireApprovalTools    []string `json:"require_approval_tools"`
}

// CriticalActions contains the currently loaded parameters. Defaults are set as fallbacks.
var CriticalActions = CriticalActionsConfig{
	RiskyPythonKeywords: []string{
		"shutil.rmtree", "os.remove", "os.unlink", "os.rmdir", "pathlib.path.unlink",
		"os.system", "subprocess", "eval", "exec", "pty", "socket", "ctypes", "posix_spawn",
	},
	RiskyBashKeywords: []string{
		"rm ", "rmdir", "unlink", "shred", "dd ", "mkfs", "fdisk", "parted",
		"sudo", "su ", "chmod", "chown", "chgrp",
		"pkill", "kill", "killall", "shutdown", "reboot", "systemctl", "halt",
		"curl", "wget", "nc ", "netcat", "nmap", "iptables", "ufw",
		"/dev/sd", "/dev/nvme", "/dev/null", ":(){",
	},
	ProductionExcelPatterns: []string{"production", "prod"},
	InternalEmailDomains:    []string{"company.com"},
	AutoApproveAll:          false,
	RequireApprovalTools:    []string{},
}

// LoadCriticalActionsConfig parses target config and updates the global settings.
func LoadCriticalActionsConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var config CriticalActionsConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}

	if len(config.RiskyPythonKeywords) > 0 {
		CriticalActions.RiskyPythonKeywords = config.RiskyPythonKeywords
	}
	if len(config.RiskyBashKeywords) > 0 {
		CriticalActions.RiskyBashKeywords = config.RiskyBashKeywords
	}
	if len(config.ProductionExcelPatterns) > 0 {
		CriticalActions.ProductionExcelPatterns = config.ProductionExcelPatterns
	}
	if len(config.InternalEmailDomains) > 0 {
		CriticalActions.InternalEmailDomains = config.InternalEmailDomains
	}
	if len(config.RequireApprovalTools) > 0 {
		CriticalActions.RequireApprovalTools = config.RequireApprovalTools
	}
	CriticalActions.AutoApproveAll = config.AutoApproveAll

	logger.Info("Successfully loaded critical actions configuration", "path", path)
	return nil
}

// RequiresHumanReview evaluates Python code blocks for dangerous syscalls.
func RequiresHumanReview(code string) bool {
	// Security (H-1): HITL bypass requires BOTH flags to prevent accidental
	// production use. AGENT_FRAMEWORK_BYPASS_HITL alone is not sufficient.
	if os.Getenv("AGENT_FRAMEWORK_BYPASS_HITL") == "true" && os.Getenv("AGENT_FRAMEWORK_TESTING") == "true" {
		return false
	}
	if CriticalActions.AutoApproveAll {
		return false
	}
	for _, kw := range CriticalActions.RiskyPythonKeywords {
		if strings.Contains(code, kw) {
			return true
		}
	}
	return false
}

// RequiresHumanReviewBash evaluates Bash script blocks for dangerous commands.
func RequiresHumanReviewBash(script string) bool {
	// Security (H-1): Require both bypass flags — see RequiresHumanReview.
	if os.Getenv("AGENT_FRAMEWORK_BYPASS_HITL") == "true" && os.Getenv("AGENT_FRAMEWORK_TESTING") == "true" {
		return false
	}
	if CriticalActions.AutoApproveAll {
		return false
	}
	for _, kw := range CriticalActions.RiskyBashKeywords {
		if strings.Contains(script, kw) {
			return true
		}
	}
	return false
}

// RequiresToolApproval checks if a specific tool name is configured to require HITL approval.
func RequiresToolApproval(toolName string) bool {
	if CriticalActions.AutoApproveAll {
		return false
	}
	for _, t := range CriticalActions.RequireApprovalTools {
		if strings.EqualFold(t, toolName) {
			return true
		}
	}
	return false
}

// IsProductionFile checks if file path matches production patterns.
func IsProductionFile(path string) bool {
	lower := strings.ToLower(path)
	for _, pattern := range CriticalActions.ProductionExcelPatterns {
		if strings.Contains(lower, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}
