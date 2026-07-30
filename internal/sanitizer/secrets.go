package sanitizer

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// High-confidence regex patterns for API keys, tokens, credentials, and private key blocks
	apiKeyRegex       = regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password|auth[_-]?token)\s*[:=]\s*["']?([a-zA-Z0-9_\-\.]{12,})["']?`)
	awsKeyRegex       = regexp.MustCompile(`(AKIA[0-9A-Z]{16})`)
	pemBlockRegex     = regexp.MustCompile(`-----BEGIN [A-Z ]+ PRIVATE KEY-----[\s\S]*?-----END [A-Z ]+ PRIVATE KEY-----`)
	connStrRegex      = regexp.MustCompile(`(?i)(postgres|mysql|mongodb|redis):\/\/[^\s"']+`)
	jwtRegex          = regexp.MustCompile(`eyJ[a-zA-Z0-9_-]{10,}\.eyJ[a-zA-Z0-9_-]{10,}\.[a-zA-Z0-9_-]{10,}`)
	genericSecretRegex = regexp.MustCompile(`(?i)(sk-[a-zA-Z0-9]{20,}|ghp_[a-zA-Z0-9]{20,}|glpat-[a-zA-Z0-9]{20,})`)
)

// SanitizeSecrets scans text content and redacts any detected API keys, passwords,
// JWT tokens, SSH private key blocks, or database connection strings.
func SanitizeSecrets(content string) (string, bool) {
	modified := content
	found := false

	if pemBlockRegex.MatchString(modified) {
		modified = pemBlockRegex.ReplaceAllString(modified, "[REDACTED_PRIVATE_KEY]")
		found = true
	}
	if awsKeyRegex.MatchString(modified) {
		modified = awsKeyRegex.ReplaceAllString(modified, "[REDACTED_AWS_KEY]")
		found = true
	}
	if genericSecretRegex.MatchString(modified) {
		modified = genericSecretRegex.ReplaceAllString(modified, "[REDACTED_SECRET_TOKEN]")
		found = true
	}
	if jwtRegex.MatchString(modified) {
		modified = jwtRegex.ReplaceAllString(modified, "[REDACTED_JWT_TOKEN]")
		found = true
	}
	if connStrRegex.MatchString(modified) {
		modified = connStrRegex.ReplaceAllString(modified, "[REDACTED_CONNECTION_STRING]")
		found = true
	}

	// Redact key=value pairs matching API key or password assignments
	modified = apiKeyRegex.ReplaceAllStringFunc(modified, func(match string) string {
		found = true
		parts := strings.SplitN(match, "=", 2)
		if len(parts) < 2 {
			parts = strings.SplitN(match, ":", 2)
		}
		if len(parts) == 2 {
			return parts[0] + " = [REDACTED]"
		}
		return "[REDACTED]"
	})

	return modified, found
}

// IsGitIgnoredPath checks if a file or directory path is listed in workspace .gitignore
func IsGitIgnoredPath(targetPath string) bool {
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}

	gitignorePath := filepath.Join(cwd, ".gitignore")
	file, err := os.Open(gitignorePath)
	if err != nil {
		return false
	}
	defer file.Close()

	rel, err := filepath.Rel(cwd, targetPath)
	if err != nil {
		rel = targetPath
	}
	rel = strings.TrimPrefix(filepath.Clean(rel), "./")
	lowerRel := strings.ToLower(rel)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		pattern := strings.TrimPrefix(strings.TrimSuffix(strings.ToLower(line), "/"), "/")
		if pattern == "" {
			continue
		}
		if lowerRel == pattern || strings.HasPrefix(lowerRel, pattern+"/") || strings.Contains(lowerRel, "/"+pattern+"/") {
			return true
		}
	}
	if err := scanner.Err(); err != nil {
		return false
	}
	return false
}
