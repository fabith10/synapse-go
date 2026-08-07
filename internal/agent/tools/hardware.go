package agenttools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/fabith10/synapse-go/adk"
)

// GetInspectHostHardwareTool returns the tool to query underlying host hardware & GPU acceleration availability.
func GetInspectHostHardwareTool() adk.Tool {
	return adk.Tool{
		Name:        "inspect_host_hardware",
		Description: "Queries host OS infrastructure details: CPU core count, RAM, GPU acceleration (Apple Silicon Metal / NVIDIA CUDA), Docker daemon availability, and Ollama host setup.",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			numCPU := runtime.NumCPU()
			goOS := runtime.GOOS
			goArch := runtime.GOARCH

			var memStats runtime.MemStats
			runtime.ReadMemStats(&memStats)

			gpuStatus := "Unknown"
			if goOS == "darwin" && (goArch == "arm64" || goArch == "amd64") {
				gpuStatus = "Apple Silicon Metal Acceleration Active (Unified Memory)"
			} else {
				if _, err := exec.LookPath("nvidia-smi"); err == nil {
					gpuStatus = "NVIDIA CUDA Acceleration Available (nvidia-smi detected)"
				} else {
					gpuStatus = "CPU Only (No NVIDIA GPU detected on PATH)"
				}
			}

			dockerStatus := "Available"
			if _, err := exec.LookPath("docker"); err != nil {
				dockerStatus = "Unavailable (Docker CLI not found on PATH)"
			}

			ollamaHost := os.Getenv("OLLAMA_HOST")
			if ollamaHost == "" {
				ollamaHost = "http://localhost:11434 (Default Local Host)"
			}

			result := map[string]interface{}{
				"os":               goOS,
				"arch":             goArch,
				"cpu_cores":        numCPU,
				"gpu_acceleration": gpuStatus,
				"docker_status":    dockerStatus,
				"memory_alloc_mb":  memStats.Alloc / (1024 * 1024),
				"sys_memory_mb":    memStats.Sys / (1024 * 1024),
				"ollama_host":      ollamaHost,
			}
			data, _ := json.MarshalIndent(result, "", "  ")
			return string(data), nil
		},
	}
}

// GetInspectEnvVarsTool returns a native tool to inspect environment variables and runtime settings safely.
func GetInspectEnvVarsTool() adk.Tool {
	return adk.Tool{
		Name:        "inspect_env_vars",
		Description: "Inspects system environment variables and framework runtime settings with automatic credential masking.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"filter": map[string]interface{}{
					"type":        "string",
					"description": "Optional search term to filter environment variable names.",
				},
			},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				Filter string `json:"filter"`
			}
			if len(args) > 0 {
				_ = json.Unmarshal(args, &params)
			}

			filterLower := strings.ToLower(params.Filter)
			envMap := make(map[string]string)

			for _, env := range os.Environ() {
				parts := strings.SplitN(env, "=", 2)
				if len(parts) == 2 {
					key := parts[0]
					val := parts[1]

					if filterLower != "" && !strings.Contains(strings.ToLower(key), filterLower) {
						continue
					}

					keyUpper := strings.ToUpper(key)
					// Mask sensitive API keys, secrets, passwords
					if strings.Contains(keyUpper, "KEY") ||
						strings.Contains(keyUpper, "SECRET") ||
						strings.Contains(keyUpper, "PASSWORD") ||
						strings.Contains(keyUpper, "TOKEN") ||
						strings.Contains(keyUpper, "AUTH") {
						if len(val) > 8 {
							val = val[:4] + "...[MASKED]..." + val[len(val)-4:]
						} else if val != "" {
							val = "[MASKED]"
						}
					}
					envMap[key] = val
				}
			}

			outBytes, _ := json.MarshalIndent(map[string]interface{}{
				"env_count": len(envMap),
				"env_vars":  envMap,
			}, "", "  ")
			return string(outBytes), nil
		},
	}
}
