//go:build linux

package exec

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dsmmcken/dh-cli/src/internal/output"
	"github.com/dsmmcken/dh-cli/src/internal/vm"
)

// runVM executes code via a Firecracker VM snapshot restore.
// The VM contains a pre-connected runner daemon, so no host-side Python is needed.
func runVM(cfg *ExecConfig, userCode, version, dhHome string) (int, map[string]any, error) {
	entryTime := time.Now()

	// Try pool first (fast path ~20ms vs ~700ms cold restore).
	// Skip pool if DH_VM_POOL=0 is set.
	if os.Getenv("DH_VM_POOL") != "0" {
		if exitCode, jsonResult, resp, err := tryPoolExec(cfg, userCode, version, dhHome, entryTime); err == nil {
			return formatVsockResponse(cfg, resp, version, entryTime, exitCode, jsonResult)
		}
	}

	// Cold restore path
	vmPaths := vm.NewVMPaths(dhHome)

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

	useUffd := os.Getenv("DH_VM_NO_UFFD") != "1"
	vmCfg := &vm.VMConfig{
		DHHome: dhHome,
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

	// Start host file server after VM restore. The guest LD_PRELOAD library
	// connects to this server to fetch workspace files on demand. We use the
	// instance's vsock path so pool and non-pool VMs both work correctly.
	cwd, _ := os.Getwd()
	fileServer, err := vm.StartFileServer(info.VsockPath, cwd)
	if err != nil && cfg.Verbose {
		fmt.Fprintf(cfg.Stderr, "Warning: file server: %v\n", err)
	}
	if fileServer != nil {
		defer fileServer.Close()
	}

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

	return formatVsockResponse(cfg, resp, version, entryTime, exitCode, nil)
}

// tryPoolExec attempts to execute code via the pool daemon.
// Returns (exitCode, jsonResult, resp, nil) on success, or (0, nil, nil, err) on failure.
// On failure, the caller should fall through to cold restore.
func tryPoolExec(cfg *ExecConfig, userCode, version, dhHome string, entryTime time.Time) (int, map[string]any, *vm.VsockResponse, error) {
	poolRunning := vm.PoolProbe()

	// Auto-start: if pool is not running, fork a daemon in the background
	// (fire-and-forget). The first exec pays cold-start cost; subsequent
	// execs benefit from the warm pool.
	if !poolRunning {
		vmPaths := vm.NewVMPaths(dhHome)
		if vm.CheckSnapshot(vmPaths, version) == nil {
			if cfg.Verbose {
				fmt.Fprintf(cfg.Stderr, "Auto-starting pool daemon (first exec uses cold restore)...\n")
			}
			autoStartPool(dhHome, version, cfg.Verbose)
		}
		return 0, nil, nil, fmt.Errorf("pool not running yet")
	}

	cwd, _ := os.Getwd()
	poolResp, err := vm.PoolExec(&vm.PoolRequest{
		Type:          "exec",
		Code:          userCode,
		CWD:           cwd,
		ShowTables:    cfg.ShowTables,
		ShowTableMeta: cfg.ShowTableMeta,
	})
	if err != nil {
		if cfg.Verbose {
			fmt.Fprintf(cfg.Stderr, "Pool exec failed: %v, falling back to cold restore\n", err)
		}
		return 0, nil, nil, err
	}

	if poolResp.Type == "error" {
		if cfg.Verbose {
			fmt.Fprintf(cfg.Stderr, "Pool error: %s, falling back to cold restore\n", poolResp.Error)
		}
		return 0, nil, nil, fmt.Errorf("pool error: %s", poolResp.Error)
	}

	// Verify version matches
	if poolResp.Version != version {
		if cfg.Verbose {
			fmt.Fprintf(cfg.Stderr, "Pool version mismatch (pool=%s, requested=%s), falling back to cold restore\n",
				poolResp.Version, version)
		}
		return 0, nil, nil, fmt.Errorf("version mismatch")
	}

	resp := poolResp.Exec
	if resp == nil {
		return 0, nil, nil, fmt.Errorf("no exec result from pool")
	}

	elapsed := time.Since(entryTime).Seconds()
	if cfg.Verbose {
		fmt.Fprintf(cfg.Stderr, "Pool exec completed in %.0fms\n", elapsed*1000)
	}

	if cfg.JSONMode {
		jsonResult := map[string]any{
			"exit_code":       resp.ExitCode,
			"stdout":          resp.Stdout,
			"stderr":          resp.Stderr,
			"result_repr":     resp.ResultRepr,
			"error":           resp.Error,
			"tables":          resp.Tables,
			"version":         version,
			"vm_mode":         true,
			"pool_mode":       true,
			"elapsed_seconds": elapsed,
		}
		if resp.Timing != nil {
			jsonResult["_timing"] = resp.Timing
		}
		return resp.ExitCode, jsonResult, resp, nil
	}

	return resp.ExitCode, nil, resp, nil
}

// autoStartPool forks a pool daemon in the background.
func autoStartPool(dhHome, version string, verbose bool) {
	exePath, err := os.Executable()
	if err != nil {
		return
	}

	args := []string{"vm", "pool", "start", "--background",
		"-n", "1",
		"--idle-timeout", "5m",
		"--version", version,
	}
	if verbose {
		args = append(args, "-v")
	}

	cmd := exec.Command(exePath, args...)
	cmd.Env = os.Environ()
	// Inherit DH_HOME if set via config dir
	if dhHome != "" {
		cmd.Env = append(cmd.Env, "DH_HOME="+dhHome)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	vmPaths := vm.NewVMPaths(dhHome)
	os.MkdirAll(vmPaths.Base, 0o755)
	logFile, err := os.OpenFile(fmt.Sprintf("%s/pool.log", vmPaths.Base), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Start()
	logFile.Close()
}

// formatVsockResponse formats and prints the VsockResponse output.
// Returns (exitCode, jsonResult, error) suitable for returning from runVM.
func formatVsockResponse(cfg *ExecConfig, resp *vm.VsockResponse, version string, entryTime time.Time, exitCode int, jsonResult map[string]any) (int, map[string]any, error) {
	if jsonResult != nil {
		return exitCode, jsonResult, nil
	}

	if cfg.JSONMode {
		elapsed := time.Since(entryTime).Seconds()
		jsonResult := map[string]any{
			"exit_code":       resp.ExitCode,
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
		return resp.ExitCode, jsonResult, nil
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
