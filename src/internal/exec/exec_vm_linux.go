//go:build linux

package exec

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dsmmcken/dh-cli/src/internal/output"
	"github.com/dsmmcken/dh-cli/src/internal/vm"
)

// runVM executes code via a Firecracker VM snapshot restore.
// The VM contains a pre-connected runner daemon, so no host-side Python is needed.
func runVM(cfg *ExecConfig, userCode, version, dhgHome string) (int, map[string]any, error) {
	entryTime := time.Now()
	vmPaths := vm.NewVMPaths(dhgHome)

	// Start page cache warming ASAP — overlaps with prereq checks,
	// Firecracker startup, and the beginning of VM execution.
	vm.WarmSnapshotPageCacheAsync(vmPaths, version)

	// Run prereqs, snapshot check, and stale cleanup concurrently
	var prereqErrs []*vm.PrereqError
	var snapErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); prereqErrs = vm.CheckPrerequisites(vmPaths) }()
	go func() { defer wg.Done(); snapErr = vm.CheckSnapshot(vmPaths, version) }()
	go vm.CleanupStaleInstances(vmPaths) // fire-and-forget
	wg.Wait()

	if len(prereqErrs) > 0 {
		var msgs []string
		for _, e := range prereqErrs {
			msgs = append(msgs, e.Error())
		}
		return output.ExitError, nil, fmt.Errorf("VM prerequisites not met:\n  %s", strings.Join(msgs, "\n  "))
	}
	if snapErr != nil {
		return output.ExitError, nil, snapErr
	}

	prereqMs := time.Since(entryTime).Milliseconds()

	// Set up context with optional timeout
	ctx := context.Background()
	var cancel context.CancelFunc
	if cfg.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(cfg.Timeout)*time.Second)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	useUffd := os.Getenv("DHG_VM_NO_UFFD") != "1"
	vmCfg := &vm.VMConfig{
		DHGHome: dhgHome,
		Version: version,
		Verbose: cfg.Verbose,
		UseUffd: useUffd,
	}

	if cfg.Verbose {
		if !cfg.ProcessStart.IsZero() {
			fmt.Fprintf(cfg.Stderr, "Go startup: %dms (process_start→entry=%dms)\n",
				time.Since(cfg.ProcessStart).Milliseconds(),
				entryTime.Sub(cfg.ProcessStart).Milliseconds())
		}
		backend := "UFFD"
		if !useUffd {
			backend = "File"
		}
		fmt.Fprintf(cfg.Stderr, "Restoring VM from snapshot for version %s (backend=%s)...\n", version, backend)
	}

	// Start host file server before VM restore. The guest LD_PRELOAD library
	// will connect to this server to fetch workspace files on demand.
	cwd, _ := os.Getwd()
	vsockPath := filepath.Join(vmPaths.SnapshotDirForVersion(version), "vsock.sock")
	fileServer, err := vm.StartFileServer(vsockPath, cwd)
	if err != nil && cfg.Verbose {
		fmt.Fprintf(cfg.Stderr, "Warning: file server: %v\n", err)
	}
	if fileServer != nil {
		defer fileServer.Close()
	}

	start := time.Now()

	info, machine, uffdCloser, err := vm.RestoreFromSnapshot(ctx, vmCfg, vmPaths, cfg.Stderr)
	if err != nil {
		return output.ExitError, nil, fmt.Errorf("restoring VM: %w", err)
	}
	defer func() {
		vm.DestroyInstance(machine, info, vmPaths)
		if uffdCloser != nil {
			uffdCloser.Close()
		}
	}()

	restoreMs := time.Since(start).Milliseconds()
	if cfg.Verbose {
		fmt.Fprintf(cfg.Stderr, "VM restored in %dms (instance %s)\n",
			restoreMs, info.ID)
	}

	// Register signal handler for cleanup
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		if sig == nil {
			return // channel closed on normal exit
		}
		vm.DestroyInstance(machine, info, vmPaths)
		if uffdCloser != nil {
			uffdCloser.Close()
		}
		os.Exit(130)
	}()
	defer func() { signal.Stop(sigCh); close(sigCh) }()

	// Execute via vsock — no host-side Python needed
	req := &vm.VsockRequest{
		Code:          userCode,
		ShowTables:    cfg.ShowTables,
		ShowTableMeta: cfg.ShowTableMeta,
	}

	// Run vsock request with context-aware timeout
	type vsockResult struct {
		resp *vm.VsockResponse
		err  error
	}
	resultCh := make(chan vsockResult, 1)
	go func() {
		resp, err := vm.ExecuteViaVsock(info.VsockPath, vm.VsockPort, req)
		resultCh <- vsockResult{resp, err}
	}()

	var resp *vm.VsockResponse
	select {
	case r := <-resultCh:
		if r.err != nil {
			return output.ExitError, nil, fmt.Errorf("executing via vsock: %w", r.err)
		}
		resp = r.resp
	case <-ctx.Done():
		elapsed := time.Since(start).Seconds()
		if cfg.JSONMode {
			jsonResult := map[string]any{
				"exit_code":       output.ExitTimeout,
				"stdout":          "",
				"stderr":          "",
				"result_repr":     nil,
				"error":           fmt.Sprintf("Execution timed out after %d seconds", cfg.Timeout),
				"tables":          []any{},
				"version":         version,
				"vm_mode":         true,
				"elapsed_seconds": elapsed,
			}
			return output.ExitTimeout, jsonResult, nil
		}
		fmt.Fprintf(cfg.Stderr, "Error: Execution timed out after %d seconds\n", cfg.Timeout)
		return output.ExitTimeout, nil, nil
	}

	elapsed := time.Since(start).Seconds()
	exitCode := resp.ExitCode

	vsockMs := time.Since(start).Milliseconds() - restoreMs
	if cfg.Verbose && resp.Timing != nil {
		fmt.Fprintf(cfg.Stderr, "VM timing: prereqs=%dms restore=%dms vsock=%dms", prereqMs, restoreMs, vsockMs)
		for _, key := range []string{"build_wrapper_ms", "run_script_ms", "read_result_ms"} {
			if v, ok := resp.Timing[key]; ok {
				fmt.Fprintf(cfg.Stderr, " %s=%v", key, v)
			}
		}
		fmt.Fprintf(cfg.Stderr, " total=%.0fms (since entry=%.0fms)\n", elapsed*1000, float64(time.Since(entryTime).Milliseconds()))
	}

	if cfg.JSONMode {
		jsonResult := map[string]any{
			"exit_code":       exitCode,
			"stdout":          resp.Stdout,
			"stderr":          resp.Stderr,
			"result_repr":     resp.ResultRepr,
			"error":           resp.Error,
			"tables":          resp.Tables,
			"version":         version,
			"vm_mode":         true,
			"elapsed_seconds": elapsed,
		}
		if resp.Timing != nil {
			jsonResult["_timing"] = resp.Timing
		}
		return exitCode, jsonResult, nil
	}

	// Normal mode: print output directly
	if resp.Stdout != "" {
		fmt.Fprint(cfg.Stdout, resp.Stdout)
		if !strings.HasSuffix(resp.Stdout, "\n") {
			fmt.Fprintln(cfg.Stdout)
		}
	}

	if resp.Stderr != "" {
		fmt.Fprint(cfg.Stderr, resp.Stderr)
		if !strings.HasSuffix(resp.Stderr, "\n") {
			fmt.Fprintln(cfg.Stderr)
		}
	}

	if resp.ResultRepr != nil && *resp.ResultRepr != "None" {
		fmt.Fprintln(cfg.Stdout, *resp.ResultRepr)
	}

	if resp.Error != nil && *resp.Error != "" {
		fmt.Fprintln(cfg.Stderr, *resp.Error)
		return 1, nil, nil
	}

	// Print table previews
	if resp.Tables != nil {
		for _, t := range resp.Tables {
			tableMap, ok := t.(map[string]any)
			if !ok {
				continue
			}
			name, _ := tableMap["name"].(string)
			preview, _ := tableMap["preview"].(string)
			if cfg.ShowTableMeta {
				rowCount, _ := tableMap["row_count"].(float64)
				isRefreshing, _ := tableMap["is_refreshing"].(bool)
				status := "static"
				if isRefreshing {
					status = "refreshing"
				}
				fmt.Fprintf(cfg.Stdout, "\n=== Table: %s (%.0f rows, %s) ===\n", name, rowCount, status)
			} else {
				fmt.Fprintf(cfg.Stdout, "\n=== Table: %s ===\n", name)
			}
			fmt.Fprintln(cfg.Stdout, preview)
		}
	}

	return exitCode, nil, nil
}
