package agenttools

import (
	"testing"

	"github.com/fabith10/synapse-go/adk"
)

func TestBrowserEnhancements(t *testing.T) {
	session := &BrowserSession{
		Inputs: make(map[int]string),
	}

	// 1. Test self-healing ensureContextLocked
	session.ensureContextLocked()
	if session.Ctx == nil {
		t.Fatalf("expected session.Ctx to be initialized")
	}

	// 2. Test Close cleanup
	session.Close()
	if session.Ctx != nil {
		t.Errorf("expected session.Ctx to be nil after Close")
	}

	// 3. Test re-initialization after Close (self-healing recovery)
	session.ensureContextLocked()
	if session.Ctx == nil {
		t.Fatalf("expected session.Ctx to self-heal after Close")
	}

	// Clean up after test
	session.Close()
}

func TestBrowserToolsConstructors(t *testing.T) {
	tools := []func() adk.Tool{
		GetBrowserBackTool,
		GetBrowserReloadTool,
		GetBrowserSaveCookiesTool,
		GetBrowserLoadCookiesTool,
	}

	for _, fn := range tools {
		tool := fn()
		if tool.Name == "" {
			t.Errorf("expected non-empty tool name")
		}
		if tool.Tier != adk.TierNative {
			t.Errorf("expected TierNative for %s", tool.Name)
		}
	}
}
