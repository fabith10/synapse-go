package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabith10/synapse-go/pkg/logger"
)

func TestLogger_TextFormat(t *testing.T) {
	buf := &bytes.Buffer{}
	l, err := logger.New(logger.Config{
		Level:  slog.LevelInfo,
		Format: "text",
		Output: buf,
	})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	l.Info("server starting", "port", 8080)
	l.Debug("this should not be printed")

	out := buf.String()
	if !strings.Contains(out, "server starting") {
		t.Errorf("expected 'server starting' in output, got: %s", out)
	}
	if !strings.Contains(out, "port=8080") {
		t.Errorf("expected 'port=8080' in output, got: %s", out)
	}
	if strings.Contains(out, "this should not be printed") {
		t.Errorf("debug log printed when level was Info: %s", out)
	}
}

func TestLogger_JSONFormat(t *testing.T) {
	buf := &bytes.Buffer{}
	l, err := logger.New(logger.Config{
		Level:  slog.LevelDebug,
		Format: "json",
		Output: buf,
	})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	l.WithComponent("orchestrator").WithAgent("triage").Info("routed message", "recipient", "planner")

	out := buf.Bytes()
	var parsed map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(out), &parsed); err != nil {
		t.Fatalf("invalid json output: %v, output: %s", err, string(out))
	}

	if parsed["msg"] != "routed message" {
		t.Errorf("expected msg 'routed message', got %v", parsed["msg"])
	}
	if parsed["component"] != "orchestrator" {
		t.Errorf("expected component 'orchestrator', got %v", parsed["component"])
	}
	if parsed["agent_id"] != "triage" {
		t.Errorf("expected agent_id 'triage', got %v", parsed["agent_id"])
	}
	if parsed["recipient"] != "planner" {
		t.Errorf("expected recipient 'planner', got %v", parsed["recipient"])
	}
}

func TestLogger_ContextExtraction(t *testing.T) {
	buf := &bytes.Buffer{}
	l, err := logger.New(logger.Config{
		Level:  slog.LevelInfo,
		Format: "json",
		Output: buf,
	})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	ctx := context.WithValue(context.Background(), logger.SessionIDKey, "sess-123")
	ctx = context.WithValue(ctx, logger.AgentIDKey, "agent-abc")

	l.InfoContext(ctx, "task started")

	var parsed map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &parsed); err != nil {
		t.Fatalf("invalid json output: %v", err)
	}

	if parsed["session_id"] != "sess-123" {
		t.Errorf("expected session_id 'sess-123', got %v", parsed["session_id"])
	}
	if parsed["agent_id"] != "agent-abc" {
		t.Errorf("expected agent_id 'agent-abc', got %v", parsed["agent_id"])
	}
}

func TestLogger_FileOutput(t *testing.T) {
	tmpDir := t.TempDir()
	logFilePath := filepath.Join(tmpDir, "test.log")

	l, err := logger.New(logger.Config{
		Level:    slog.LevelWarn,
		Format:   "text",
		FilePath: logFilePath,
	})
	if err != nil {
		t.Fatalf("failed to create file logger: %v", err)
	}

	l.Warn("warning message", "code", 404)

	content, err := os.ReadFile(logFilePath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	out := string(content)
	if !strings.Contains(out, "warning message") || !strings.Contains(out, "code=404") {
		t.Errorf("file output missing expected data: %s", out)
	}
}

func TestLogger_GlobalDefault(t *testing.T) {
	buf := &bytes.Buffer{}
	l, _ := logger.New(logger.Config{Level: slog.LevelInfo, Format: "text", Output: buf})
	logger.SetDefault(l)

	logger.Info("global info message", "key", "val")
	if !strings.Contains(buf.String(), "global info message") {
		t.Errorf("global Info failed, got: %s", buf.String())
	}
}
