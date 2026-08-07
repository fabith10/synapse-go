package agenttools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fabith10/synapse-go/adk"
)

func TestSendNtfyNotificationTool_Success(t *testing.T) {
	var receivedBody map[string]interface{}
	var receivedHeader http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header
		bodyBytes, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(bodyBytes, &receivedBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"test-msg-1","code":20001}`))
	}))
	defer server.Close()

	tool := GetSendNtfyNotificationTool()
	args, err := json.Marshal(map[string]interface{}{
		"topic":      "alerts",
		"message":    "System backup completed successfully",
		"title":      "Backup Status",
		"priority":   4,
		"tags":       []string{"backup", "success"},
		"click_url":  "https://example.com/logs",
		"server_url": server.URL,
		"auth_token": "secret-token",
	})
	if err != nil {
		t.Fatalf("failed to marshal args: %v", err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(result, "Successfully sent ntfy notification") {
		t.Errorf("unexpected output: %s", result)
	}

	if receivedBody["topic"] != "alerts" {
		t.Errorf("expected topic alerts, got %v", receivedBody["topic"])
	}
	if receivedBody["message"] != "System backup completed successfully" {
		t.Errorf("expected message body, got %v", receivedBody["message"])
	}
	if receivedBody["title"] != "Backup Status" {
		t.Errorf("expected title Backup Status, got %v", receivedBody["title"])
	}
	if receivedHeader.Get("Authorization") != "Bearer secret-token" {
		t.Errorf("expected Bearer secret-token authorization, got %s", receivedHeader.Get("Authorization"))
	}
}

func TestSendNtfyNotificationTool_MissingTopic(t *testing.T) {
	_ = os.Unsetenv("NTFY_TOPIC")
	tool := GetSendNtfyNotificationTool()
	args, _ := json.Marshal(map[string]interface{}{
		"message": "Hello world",
	})

	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatalf("expected error for missing topic, got nil")
	}
}

func TestNtfyListener_StreamAndHITL(t *testing.T) {
	corrID := "corr-test-hitl-123"
	hitlCh := make(chan adk.Message, 1)

	RegisterPendingResponse(corrID, hitlCh)
	defer UnregisterPendingResponse(corrID)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		msg := NtfyIncomingMessage{
			ID:            "msg-101",
			Event:         "message",
			Topic:         "testtopic",
			Message:       "APPROVED correlation_id: " + corrID,
			CorrelationID: corrID,
		}
		jsonBytes, _ := json.Marshal(msg)
		_, _ = w.Write(append(jsonBytes, '\n'))
		flusher.Flush()
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	cfg := NtfyConfig{
		ServerURL:     server.URL,
		Topic:         "testtopic",
		TargetAgentID: "triage-agent",
	}

	listener := NewNtfyListener(nil, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := listener.Start(ctx); err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer listener.Stop()

	select {
	case received := <-hitlCh:
		if !strings.Contains(received.Content, "APPROVED") {
			t.Errorf("expected APPROVED content, got %s", received.Content)
		}
		if received.Metadata["correlation_id"] != corrID {
			t.Errorf("expected correlation_id %s, got %s", corrID, received.Metadata["correlation_id"])
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for HITL message dispatch")
	}
}
