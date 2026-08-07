package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/api/types/container"
	dockerimage "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/pkg/stdcopy"
)

// ---------------------------------------------------------------------------
// DockerSandbox — Tier 3 isolation via ephemeral Docker containers
// ---------------------------------------------------------------------------

// DockerSandbox executes arbitrary code inside a short-lived Docker container
// that is created, started, awaited, and immediately destroyed per invocation.
// This enforces PDR-004 §2.3 guarantees:
//
//   - Container is always removed after stdout/stderr capture (deferred call).
//   - GPU pass-through via nvidia-container-runtime when RequiresGPU is true.
//   - Execution is wrapped in context.WithTimeout to prevent infinite loops.
//
// The caller is responsible for constructing a *dockerclient.Client and
// ensuring the Docker daemon is reachable before registration. Use
// dockerclient.NewClientWithOpts(dockerclient.FromEnv) for the standard setup.
//
// Language → image mapping (PDR-004 §2.3):
//
//	"python"     → python:3.12-slim   (or pytorch/pytorch if RequiresGPU)
//	"javascript" → node:22-slim
//	"bash"       → alpine:latest
type DockerSandbox struct {
	client          *dockerclient.Client
	containerMemMB  int64 // per-container RAM hard-cap; 0 → 2048 MB default
}

// NewDockerSandbox wraps an existing Docker client with the default 2 GB container memory cap.
func NewDockerSandbox(client *dockerclient.Client) (*DockerSandbox, error) {
	return NewDockerSandboxWithMemory(client, 0)
}

// NewDockerSandboxWithMemory creates a DockerSandbox whose containers are
// hard-capped at containerMemMB megabytes of RAM. Zero uses the 2 GB default.
func NewDockerSandboxWithMemory(client *dockerclient.Client, containerMemMB int64) (*DockerSandbox, error) {
	if client == nil {
		return nil, ErrDockerUnavailable
	}
	if containerMemMB <= 0 {
		containerMemMB = 2048
	}
	return &DockerSandbox{client: client, containerMemMB: containerMemMB}, nil
}

// Execute spins up an ephemeral container, runs the code, captures output,
// and destroys the container. The full lifecycle is:
//
//  1. ContainerCreate  — configure image, command, and optional GPU resources
//  2. ContainerStart   — hand off to the Docker daemon
//  3. ContainerWait    — block until the process exits or ctx expires
//  4. ContainerLogs    — demux stdout and stderr via stdcopy
//  5. ContainerRemove  — always run, even on error (deferred)
func (s *DockerSandbox) Execute(ctx context.Context, req ExecutionRequest) ExecutionResult {
	// Apply the per-request timeout on top of the caller's context deadline.
	// PDR-004 §2.3 mandates context.WithTimeout for every container execution.
	execCtx := ctx
	if req.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	image, cmd := resolveImageAndCmd(req)

	// Build the container host config. GPU resources are injected when the
	// agent sets RequiresGPU = true (e.g., for CUDA PyTorch workloads).
	hostCfg := &container.HostConfig{}
	hostCfg.Resources.Memory = s.containerMemMB * 1024 * 1024
	if req.RequiresGPU {
		hostCfg.Resources.DeviceRequests = []container.DeviceRequest{
			{
				Driver:       "nvidia",
				Count:        -1, // all GPUs
				Capabilities: [][]string{{"gpu"}},
			},
		}
	}

	cwd, _ := os.Getwd()
	if cwd != "" {
		// Security (H-2): Mount workspace read-only to prevent sandboxed code
		// from modifying host files (.env, agents.json, DB, source code, etc.).
		// Containers that need write access should use their own ephemeral /tmp.
		hostCfg.Binds = append(hostCfg.Binds, fmt.Sprintf("%s:/workspace:ro", cwd))

		// Security (H-4): Mask sensitive credential and environment files (.env, DBs, MCP configs)
		// by bind-mounting /dev/null over them so sandboxed code cannot exfiltrate secret keys.
		sensitiveMasks := []string{
			".env", ".env.local", ".env.production", ".env.staging", ".env.development",
			"agent_framework.db", "agent_framework.db-wal", "agent_framework.db-shm",
			"mcp_config.json",
		}
		for _, maskFile := range sensitiveMasks {
			hostCfg.Binds = append(hostCfg.Binds, fmt.Sprintf("/dev/null:/workspace/%s:ro", maskFile))
		}
	}

	// Security (H-3): Disable container networking by default to prevent
	// data exfiltration, SSRF, and downloading of malicious payloads.
	// Set DOCKER_ALLOW_NETWORK=true only when the workload explicitly requires
	if os.Getenv("DOCKER_ALLOW_NETWORK") != "true" && !req.AllowNetwork {
		hostCfg.NetworkMode = "none"
	}

	// Step 1: Create the container with workspace mounted at /workspace
	resp, err := s.client.ContainerCreate(
		execCtx,
		&container.Config{
			Image:        image,
			Cmd:          cmd,
			WorkingDir:   "/workspace",
			AttachStdout: true,
			AttachStderr: true,
		},
		hostCfg,
		nil, // no network config
		nil, // no platform override
		"",  // auto-generated name
	)
	if err != nil && req.RequiresGPU {
		// Fallback to CPU container if host lacks NVIDIA container runtime
		hostCfg.Resources.DeviceRequests = nil
		resp, err = s.client.ContainerCreate(
			execCtx,
			&container.Config{
				Image:        image,
				Cmd:          cmd,
				WorkingDir:   "/workspace",
				AttachStdout: true,
				AttachStderr: true,
			},
			hostCfg,
			nil,
			nil,
			"",
		)
	}
	if err != nil {
		if strings.Contains(err.Error(), "No such image") {
			fmt.Printf("DockerSandbox: Image %q not found locally. Pulling image...\n", image)
			pullReader, pullErr := s.client.ImagePull(execCtx, image, dockerimage.PullOptions{})
			if pullErr == nil {
				// Read stream to completion to wait for the pull to complete
				_, _ = io.Copy(io.Discard, pullReader)
				pullReader.Close()

				// Retry ContainerCreate
				resp, err = s.client.ContainerCreate(
					execCtx,
					&container.Config{
						Image:        image,
						Cmd:          cmd,
						WorkingDir:   "/workspace",
						AttachStdout: true,
						AttachStderr: true,
					},
					hostCfg,
					nil,
					nil,
					"",
				)
			} else {
				return ExecutionResult{
					ExitCode: 1,
					Error:    fmt.Errorf("docker: image pull failed for %q: %w", image, pullErr),
				}
			}
		}
	}
	if err != nil {
		return ExecutionResult{
			ExitCode: 1,
			Error:    fmt.Errorf("docker: ContainerCreate: %w", err),
		}
	}

	// Step 5 (deferred): Always destroy the container regardless of outcome.
	// Use a background context so removal is not cancelled if execCtx expires.
	defer func() {
		_ = s.client.ContainerRemove(
			context.Background(),
			resp.ID,
			container.RemoveOptions{Force: true},
		)
	}()

	// Step 2: Start.
	if err := s.client.ContainerStart(execCtx, resp.ID, container.StartOptions{}); err != nil {
		return ExecutionResult{
			ExitCode: 1,
			Error:    fmt.Errorf("docker: ContainerStart: %w", err),
		}
	}

	// Step 3: Wait for the process to exit.
	statusCh, errCh := s.client.ContainerWait(execCtx, resp.ID, container.WaitConditionNotRunning)

	var exitCode int64
	select {
	case waitErr := <-errCh:
		if waitErr != nil {
			return ExecutionResult{
				ExitCode: 1,
				Error:    fmt.Errorf("docker: ContainerWait: %w", waitErr),
			}
		}
	case status := <-statusCh:
		exitCode = status.StatusCode
	}

	// Step 4: Retrieve multiplexed stdout/stderr log stream.
	logReader, err := s.client.ContainerLogs(
		execCtx,
		resp.ID,
		container.LogsOptions{ShowStdout: true, ShowStderr: true},
	)
	if err != nil {
		return ExecutionResult{
			ExitCode: int(exitCode),
			Error:    fmt.Errorf("docker: ContainerLogs: %w", err),
		}
	}
	defer logReader.Close()

	// stdcopy.StdCopy demultiplexes the Docker log stream into separate
	// stdout and stderr writers (Docker wraps both in a single stream).
	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, logReader); err != nil && err != io.EOF {
		return ExecutionResult{
			ExitCode: int(exitCode),
			Error:    fmt.Errorf("docker: stdcopy: %w", err),
		}
	}

	return ExecutionResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: int(exitCode),
	}
}

func detectPythonImports(code string) []string {
	knownPackages := map[string]string{
		"numpy":        "numpy",
		"pandas":       "pandas",
		"scipy":        "scipy",
		"matplotlib":   "matplotlib",
		"requests":     "requests",
		"sklearn":      "scikit-learn",
		"scikit_learn": "scikit-learn",
		"sympy":        "sympy",
		"yfinance":     "yfinance",
		"bs4":          "beautifulsoup4",
	}

	var detected []string
	seen := make(map[string]bool)
	lines := strings.Split(code, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		for module, pkgName := range knownPackages {
			if (strings.HasPrefix(trimmed, "import "+module) || strings.HasPrefix(trimmed, "from "+module+" ")) && !seen[pkgName] {
				seen[pkgName] = true
				detected = append(detected, pkgName)
			}
		}
	}
	return detected
}

func formatPythonStringList(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = fmt.Sprintf("%q", item)
	}
	return strings.Join(quoted, ", ")
}

// resolveImageAndCmd maps the Language field in an ExecutionRequest to a
// Docker image name and the command slice that runs the raw code.
//
// Images are intentionally slim variants to minimise pull time and attack
// surface. GPU workloads override the Python image to pytorch/pytorch.
func resolveImageAndCmd(req ExecutionRequest) (image string, cmd []string) {
	switch req.Language {
	case "python":
		imageName := "python:3.12-slim"
		if req.RequiresGPU {
			imageName = "pytorch/pytorch:2.3.0-cuda12.1-cudnn8-runtime"
		}

		pkgs := req.Packages
		if len(pkgs) == 0 {
			pkgs = detectPythonImports(req.RawCode)
		}

		b64Code := base64.StdEncoding.EncodeToString([]byte(req.RawCode))

		if len(pkgs) > 0 || len(req.PrepCommands) > 0 {
			var prep []string
			prep = append(prep, req.PrepCommands...)
			if len(pkgs) > 0 {
				prep = append(prep, fmt.Sprintf("pip install --quiet --no-cache-dir %s || true", strings.Join(pkgs, " ")))
			}
			prep = append(prep, fmt.Sprintf("echo %s | base64 -d | python3 -", b64Code))
			fullCmd := strings.Join(prep, " && ")
			return imageName, []string{"sh", "-c", fullCmd}
		}

		return imageName, []string{"sh", "-c", fmt.Sprintf("echo %s | base64 -d | python3 -", b64Code)}

	case "javascript":
		return "node:22-slim", []string{"node", "-e", req.RawCode}

	case "bash":
		if len(req.PrepCommands) > 0 {
			var prep []string
			prep = append(prep, req.PrepCommands...)
			prep = append(prep, req.RawCode)
			fullCmd := strings.Join(prep, " && ")
			return "alpine:latest", []string{"sh", "-c", fullCmd}
		}
		return "alpine:latest", []string{"sh", "-c", req.RawCode}

	default:
		// Unknown language defaults to a bare Alpine shell.
		return "alpine:latest", []string{"sh", "-c", req.RawCode}
	}
}
