package sanitizer

import (
	"testing"
)

func TestSanitizeSecrets_APIKeyRedaction(t *testing.T) {
	input := "api_key = \"sk-proj-1234567890abcdef12345\"\nDATABASE_URL = postgres://user:pass@localhost:5432/db"
	got, found := SanitizeSecrets(input)

	if !found {
		t.Errorf("expected secret detection, got none")
	}
	if !testingContains(got, "[REDACTED]") {
		t.Errorf("expected API key redaction, got: %s", got)
	}
	if testingContains(got, "postgres://user:pass") {
		t.Errorf("expected database URL redaction, got: %s", got)
	}
}

func TestSanitizeSecrets_PrivateKeyRedaction(t *testing.T) {
	input := "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA...\n-----END RSA PRIVATE KEY-----"
	got, found := SanitizeSecrets(input)

	if !found || !testingContains(got, "[REDACTED_PRIVATE_KEY]") {
		t.Errorf("expected RSA private key redaction, got: %s", got)
	}
}

func testingContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && containsSubstring(s, substr)))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
