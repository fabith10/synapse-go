package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

// ---------------------------------------------------------------------------
// WasmSandbox — Tier 2 isolation via wazero (pure-Go WebAssembly runtime)
// ---------------------------------------------------------------------------

// WasmSandbox executes pre-compiled WebAssembly modules in a hermetic sandbox.
// It enforces PDR-004 §2 security guarantees:
//   - Zero access to the host filesystem (no WithFS, no WASI preopens)
//   - Zero access to host environment variables (WithSysWalltime only)
//   - Zero network access (no WASI sock imports)
//
// A compilation cache is shared across invocations so the same WASM binary
// is JIT-compiled only once per process lifetime — subsequent calls reuse the
// cached artefact for near-zero startup overhead.
//
// Usage pattern (ADK §3.2):
//
//	sb := tools.NewWasmSandbox()
//	result := sb.Execute(ctx, tools.ExecutionRequest{
//	    Language:       "wasm",
//	    RawBytes:       myCompiledModule,  // must be a valid .wasm binary
//	    Stdin:          jsonArgs,
//	    TimeoutSeconds: 5,
//	})
type WasmSandbox struct {
	cache wazero.CompilationCache
}

// NewWasmSandbox constructs a WasmSandbox with a process-scoped compilation
// cache. The sandbox is safe for concurrent use across goroutines.
func NewWasmSandbox() *WasmSandbox {
	return &WasmSandbox{
		cache: wazero.NewCompilationCache(),
	}
}

// Execute compiles (or retrieves from cache) and runs the WASM module provided
// in req.RawBytes. The module's stdin, stdout, and stderr are wired to
// req.Stdin and the returned ExecutionResult respectively.
//
// Execution stops when:
//   - The WASM module calls proc_exit (WASI) — ExitCode is captured.
//   - The module's _start function returns normally — ExitCode 0.
//   - ctx or the TimeoutSeconds deadline expires — Error is set.
func (s *WasmSandbox) Execute(ctx context.Context, req ExecutionRequest) ExecutionResult {
	if len(req.RawBytes) == 0 {
		return ExecutionResult{ExitCode: 1, Error: ErrNoWasmBytes}
	}

	// Apply the per-request timeout on top of the caller's context deadline.
	execCtx := ctx
	if req.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	// A new wazero Runtime is created per execution for strict isolation.
	// The shared CompilationCache means module compilation is only paid once.
	rt := wazero.NewRuntimeWithConfig(execCtx,
		wazero.NewRuntimeConfig().WithCompilationCache(s.cache))
	defer rt.Close(execCtx)

	// Instantiate WASI preview 1 so modules can perform I/O via proc_exit,
	// fd_write, etc. No preopened directories → zero filesystem access.
	if _, err := wasi_snapshot_preview1.Instantiate(execCtx, rt); err != nil {
		return ExecutionResult{
			ExitCode: 1,
			Error:    fmt.Errorf("wasm: wasi init: %w", err),
		}
	}

	// Capture stdout and stderr in memory buffers.
	var stdout, stderr bytes.Buffer

	var stdinReader io.Reader = strings.NewReader("")
	if len(req.Stdin) > 0 {
		stdinReader = bytes.NewReader(req.Stdin)
	}

	// ModuleConfig wires the I/O descriptors and deliberately omits:
	//   - WithFS         → no filesystem
	//   - WithEnv        → no environment variables
	//   - WithArgs       → no argv (module reads stdin instead)
	//   - WithSysNano... → wall-clock available for legitimate use cases
	modCfg := wazero.NewModuleConfig().
		WithStdout(&stdout).
		WithStderr(&stderr).
		WithStdin(stdinReader)

	// InstantiateWithConfig compiles, links, and runs the _start entry point.
	// For WASI modules, _start is automatically invoked after instantiation.
	_, err := rt.InstantiateWithConfig(execCtx, req.RawBytes, modCfg)
	if err != nil {
		// wazero wraps a WASI proc_exit call as sys.ExitError. This is a
		// normal exit path (including exit code != 0) — not a sandbox failure.
		var exitErr *sys.ExitError
		if errors.As(err, &exitErr) {
			return ExecutionResult{
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
				ExitCode: int(exitErr.ExitCode()),
			}
		}

		// Any other error is a genuine sandbox failure.
		return ExecutionResult{
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
			ExitCode: 1,
			Error:    fmt.Errorf("wasm: execute: %w", err),
		}
	}

	return ExecutionResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}
}
