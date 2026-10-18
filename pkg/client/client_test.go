package client_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/fabith10/synapse-go/pkg/client"
)

func TestSDKClient_EndToEnd(t *testing.T) {
	var mu sync.Mutex
	var receivedAuth string
	var dispatchedTasks []client.TaskRequest
	var resolvedApprovals []map[string]string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedAuth = r.Header.Get("Authorization")
		mu.Unlock()

		switch r.URL.Path {
		case "/api/v1/task", "/api/task":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var req client.TaskRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			dispatchedTasks = append(dispatchedTasks, req)
			mu.Unlock()

			if req.Wait {
				// Simulate synchronous completion
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(client.TaskResponse{
					Success:  true,
					TaskID:   "task-sync-123",
					Status:   "COMPLETED",
					Sender:   "planner-agent",
					Response: "Synthesized outcome: audit complete.",
				})
			} else {
				// Async fire-and-forget
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(client.TaskResponse{
					Success: true,
					TaskID:  "task-async-456",
					Message: "Dispatched",
				})
			}

		case "/stream/events":
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")

			flusher, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "flusher unsupported", http.StatusInternalServerError)
				return
			}

			evt := client.TelemetryEvent{
				Time:       "13:30:00",
				Sender:     "quant-agent",
				Recipient:  "planner-agent",
				EventType:  "TOOL_CALL",
				Category:   "tool_call",
				ActiveTool: "execute_python_docker",
				Content:    "Running risk calculation script",
			}
			data, _ := json.Marshal(evt)
			fmt.Fprintf(w, "event: telemetry\ndata: %s\n\n", string(data))
			flusher.Flush()

		case "/api/v1/hitl/pending":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]client.PendingApproval{
				{
					ID:            "app-1",
					CorrelationID: "corr-999",
					AgentID:       "developer-agent",
					Action:        "execute_bash_docker",
					Details:       "rm -rf /tmp/cache",
					Status:        "PENDING",
				},
			})

		case "/api/v1/hitl/respond":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			resolvedApprovals = append(resolvedApprovals, body)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})

		case "/api/system/health":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "healthy", "agents_online": 17})

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	c := client.New(server.URL, client.WithToken("test-secret-token"))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Synchronous Execution Test
	syncResp, err := c.Execute(ctx, client.TaskRequest{
		Prompt: "Run portfolio risk analysis",
		Mode:   client.ModeAutonomous,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !syncResp.Success || syncResp.Status != "COMPLETED" || syncResp.Response != "Synthesized outcome: audit complete." {
		t.Fatalf("Unexpected sync response: %+v", syncResp)
	}

	// 2. Asynchronous Dispatch Test
	asyncResp, err := c.Dispatch(ctx, client.TaskRequest{
		Prompt: "Background scheduled job",
		Mode:   client.ModeSteered,
	})
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if !asyncResp.Success || asyncResp.TaskID != "task-async-456" {
		t.Fatalf("Unexpected async response: %+v", asyncResp)
	}

	// 3. Structured JSON Streaming Test
	streamCtx, streamCancel := context.WithTimeout(ctx, 2*time.Second)
	defer streamCancel()
	eventsCh, errCh, err := c.Stream(streamCtx)
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	select {
	case evt := <-eventsCh:
		if evt.EventType != "TOOL_CALL" || evt.ActiveTool != "execute_python_docker" || evt.Sender != "quant-agent" {
			t.Fatalf("Unexpected streamed event: %+v", evt)
		}
	case err := <-errCh:
		t.Fatalf("Stream error: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for streamed telemetry event")
	}

	// 4. HITL Approvals Query and Resolution Test
	pending, err := c.ListPendingApprovals(ctx)
	if err != nil {
		t.Fatalf("ListPendingApprovals failed: %v", err)
	}
	if len(pending) != 1 || pending[0].CorrelationID != "corr-999" {
		t.Fatalf("Unexpected pending approvals: %+v", pending)
	}

	err = c.ResolveApproval(ctx, "corr-999", client.DecisionApprove)
	if err != nil {
		t.Fatalf("ResolveApproval failed: %v", err)
	}

	// 5. Health Check Test
	health, err := c.CheckHealth(ctx)
	if err != nil {
		t.Fatalf("CheckHealth failed: %v", err)
	}
	if health["status"] != "healthy" {
		t.Fatalf("Unexpected health status: %+v", health)
	}

	mu.Lock()
	defer mu.Unlock()
	if receivedAuth != "Bearer test-secret-token" {
		t.Fatalf("Expected Bearer token in request header, got %q", receivedAuth)
	}
	if len(resolvedApprovals) != 1 || resolvedApprovals[0]["decision"] != "APPROVED" {
		t.Fatalf("Expected 1 approved decision recorded, got: %+v", resolvedApprovals)
	}
}
