package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMockComputePricesHandler(t *testing.T) {
	srv := NewServer(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/mock/compute/prices?provider=runpod&gpu_type=h100", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	if resp["status"] != "success" {
		t.Errorf("expected status success, got %v", resp["status"])
	}

	prices, ok := resp["prices"].([]interface{})
	if !ok || len(prices) == 0 {
		t.Fatalf("expected non-empty prices list, got %v", resp["prices"])
	}

	first := prices[0].(map[string]interface{})
	if !strings.Contains(strings.ToLower(first["provider"].(string)), "runpod") {
		t.Errorf("expected provider runpod, got %v", first["provider"])
	}
}

func TestMockForwardCurvesHandler(t *testing.T) {
	srv := NewServer(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/mock/compute/forward-curves?asset=H100_SXM", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	if resp["asset"] != "H100_SXM" {
		t.Errorf("expected asset H100_SXM, got %v", resp["asset"])
	}

	curve, ok := resp["curve"].([]interface{})
	if !ok || len(curve) == 0 {
		t.Fatalf("expected non-empty forward curve points, got %v", resp["curve"])
	}
}

func TestMockOptionsHandler(t *testing.T) {
	srv := NewServer(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/mock/compute/options?asset=H100_SXM&expiration=30d", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	options, ok := resp["options"].([]interface{})
	if !ok || len(options) == 0 {
		t.Fatalf("expected non-empty options chain, got %v", resp["options"])
	}
}

func TestMockTaskLifecycle(t *testing.T) {
	srv := NewServer(nil)

	// 1. Create task
	body := map[string]interface{}{
		"task_type": "compute_optimization",
		"payload": map[string]interface{}{
			"model": "deepseek-r1",
			"nodes": 4,
		},
		"priority": "high",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/mock/tasks", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", rec.Code)
	}

	var createResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &createResp)
	taskMap, ok := createResp["task"].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid task payload returned: %v", createResp)
	}
	taskID := taskMap["id"].(string)

	// 2. Fetch task status (queued or running)
	reqGet := httptest.NewRequest(http.MethodGet, "/api/mock/tasks/"+taskID, nil)
	recGet := httptest.NewRecorder()
	srv.ServeHTTP(recGet, reqGet)

	if recGet.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for GET task, got %d", recGet.Code)
	}

	// 3. Wait for execution completion
	time.Sleep(1200 * time.Millisecond)

	reqDone := httptest.NewRequest(http.MethodGet, "/api/mock/tasks/"+taskID, nil)
	recDone := httptest.NewRecorder()
	srv.ServeHTTP(recDone, reqDone)

	var doneResp map[string]interface{}
	json.Unmarshal(recDone.Body.Bytes(), &doneResp)
	doneTask := doneResp["task"].(map[string]interface{})
	if doneTask["status"] != "completed" {
		t.Errorf("expected status completed after sleep, got %v", doneTask["status"])
	}

	// 4. List tasks
	reqList := httptest.NewRequest(http.MethodGet, "/api/mock/tasks?status=completed", nil)
	recList := httptest.NewRecorder()
	srv.ServeHTTP(recList, reqList)

	var listResp map[string]interface{}
	json.Unmarshal(recList.Body.Bytes(), &listResp)
	tasksList := listResp["tasks"].([]interface{})
	if len(tasksList) == 0 {
		t.Errorf("expected list to contain completed task")
	}
}

func TestMockTaskCancel(t *testing.T) {
	srv := NewServer(nil)

	// Create task
	bodyBytes, _ := json.Marshal(map[string]interface{}{"task_type": "long_job"})
	req := httptest.NewRequest(http.MethodPost, "/api/mock/tasks", bytes.NewReader(bodyBytes))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	var createResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &createResp)
	taskMap := createResp["task"].(map[string]interface{})
	taskID := taskMap["id"].(string)

	// Immediately cancel
	reqCancel := httptest.NewRequest(http.MethodPost, "/api/mock/tasks/"+taskID+"/cancel", nil)
	recCancel := httptest.NewRecorder()
	srv.ServeHTTP(recCancel, reqCancel)

	if recCancel.Code != http.StatusOK {
		t.Fatalf("expected status 200 on cancel, got %d", recCancel.Code)
	}

	var cancelResp map[string]interface{}
	json.Unmarshal(recCancel.Body.Bytes(), &cancelResp)
	if cancelResp["status"] != "cancelled" {
		t.Errorf("expected status cancelled, got %v", cancelResp["status"])
	}
}

func TestMockVolSurfaceHandler(t *testing.T) {

	srv := NewServer(nil)

	assets := []string{"H100_SXM", "A100_80GB", "RTX_4090"}
	for _, asset := range assets {
		t.Run("asset_"+asset, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/mock/compute/vol-surface?asset="+asset, nil)
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d", rec.Code)
			}

			var resp map[string]interface{}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to parse JSON response: %v", err)
			}

			if resp["status"] != "success" {
				t.Errorf("expected status success, got %v", resp["status"])
			}

			if resp["asset"] != asset {
				t.Errorf("expected asset %s, got %v", asset, resp["asset"])
			}

			surface, ok := resp["vol_surface"].([]interface{})
			if !ok || len(surface) == 0 {
				t.Fatal("expected non-empty vol_surface array")
			}

			// Validate structure of first tenor slice
			first, ok := surface[0].(map[string]interface{})
			if !ok {
				t.Fatal("vol_surface[0] not a map")
			}
			if first["tenor"] == nil {
				t.Error("expected tenor field in vol_surface slice")
			}
			strikes, ok := first["strikes"].([]interface{})
			if !ok || len(strikes) == 0 {
				t.Fatal("expected non-empty strikes array in vol_surface")
			}

			// Validate structure of a strike point
			sp, ok := strikes[0].(map[string]interface{})
			if !ok {
				t.Fatal("strike point not a map")
			}
			for _, field := range []string{"strike_usd", "moneyness", "implied_vol", "call_premium_usd", "put_premium_usd", "delta_call", "gamma", "vega", "theta_per_day"} {
				if sp[field] == nil {
					t.Errorf("missing field %q in strike point", field)
				}
			}
		})
	}
}

func TestStudioAndHealthAPIs(t *testing.T) {
	srv := NewServer(nil)
	t.Cleanup(func() {
		_ = os.Remove("agents.json")
	})

	// 1. Test GET /api/system/health
	t.Run("System Health API", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/system/health", nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}

		var health map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &health); err != nil {
			t.Fatalf("failed to parse health JSON: %v", err)
		}

		if _, ok := health["docker_online"]; !ok {
			t.Error("health response missing docker_online")
		}
		if _, ok := health["ollama_online"]; !ok {
			t.Error("health response missing ollama_online")
		}
	})

	// 2. Test GET /api/agents
	t.Run("Agents List API", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse agents JSON: %v", err)
		}

		tools, ok := resp["available_tools"].([]interface{})
		if !ok || len(tools) == 0 {
			t.Error("expected non-empty available_tools array")
		}
	})

	// 3. Test POST /api/settings/docker-toggle
	t.Run("Docker Admin Policy Toggle API", func(t *testing.T) {
		body := bytes.NewBufferString(`{"enabled": true}`)
		req := httptest.NewRequest(http.MethodPost, "/api/settings/docker-toggle", body)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
	})
}

func TestHandleArtifact_PathCleaningAndFallback(t *testing.T) {
	srv := NewServer(nil)
	_ = os.MkdirAll("reports", 0755)
	testFile := filepath.Join("reports", "artifact_test_sample.txt")
	_ = os.WriteFile(testFile, []byte("Artifact sample content"), 0644)
	defer os.Remove(testFile)

	absFile, _ := filepath.Abs(testFile)

	// Test case 1: Raw path with trailing dot
	t.Run("Trailing dot cleanup", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/artifact?path=reports/artifact_test_sample.txt.", nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200 for trailing dot path, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "Artifact sample content") {
			t.Errorf("unexpected body content: %s", rec.Body.String())
		}
	})

	// Test case 2: file:// URI prefix
	t.Run("file:// URI prefix", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/artifact?path=file://"+absFile, nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200 for file:// URI path, got %d", rec.Code)
		}
	})

	// Test case 3: Base name fallback resolution
	t.Run("Base name fallback", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/artifact?path=artifact_test_sample.txt", nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200 for basename fallback path, got %d", rec.Code)
		}
	})
}

func TestAuditAPIEndpoints(t *testing.T) {
	srv := NewServer(nil)

	// 1. GET /api/audit/summary
	t.Run("Audit Summary API", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/audit/summary", nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
	})

	// 2. GET /api/audit/records
	t.Run("Audit Records List API", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/audit/records?search=quant", nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
	})

	// 3. POST /api/audit/prune
	t.Run("Audit Prune API", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/audit/prune", nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
	})
}

func TestNetworkTopologyAPI(t *testing.T) {
	srv := NewServer(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/network/topology", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse network topology JSON: %v", err)
	}

	nodes, ok := resp["nodes"].([]interface{})
	if !ok || len(nodes) == 0 {
		t.Error("expected non-empty nodes array in network topology")
	}

	edges, ok := resp["edges"].([]interface{})
	if !ok || len(edges) == 0 {
		t.Error("expected non-empty edges array in network topology")
	}
}






