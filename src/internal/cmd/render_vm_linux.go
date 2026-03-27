//go:build linux

package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dsmmcken/dh-cli/src/internal/config"
	dhexec "github.com/dsmmcken/dh-cli/src/internal/exec"
	"github.com/dsmmcken/dh-cli/src/internal/output"
	"github.com/dsmmcken/dh-cli/src/internal/vm"
	"github.com/spf13/cobra"
)

func runRenderVM(cmd *cobra.Command, args []string, diagnose bool) error {
	scriptPath := args[0]

	// Read script file
	scriptContent, err := os.ReadFile(scriptPath)
	if err != nil {
		return fmt.Errorf("reading script file %s: %w", scriptPath, err)
	}

	// Resolve version: flag → env → config → latest snapshot
	config.SetConfigDir(ConfigDir)
	dhHome := config.DHHome()
	vmPaths := vm.NewVMPaths(dhHome)

	version, err := config.ResolveVersion(renderVersionFlag, os.Getenv("DH_VERSION"))
	if err != nil {
		// Try latest snapshot as last resort
		snapVersions, snapErr := listSnapshotVersions(vmPaths)
		if snapErr != nil || len(snapVersions) == 0 {
			return fmt.Errorf("resolving version: %w", err)
		}
		version = snapVersions[0]
	}

	if output.IsVerbose() {
		fmt.Fprintf(cmd.ErrOrStderr(), "Resolved version: %s\n", version)
	}

	// Try pool checkout first (fast path)
	var info *vm.InstanceInfo
	if checkout := tryRenderPoolCheckout(cmd, version, dhHome); checkout != nil {
		version = checkout.Version
		info = &vm.InstanceInfo{
			ID:            checkout.InstanceID,
			PID:           checkout.PID,
			VsockPath:     checkout.VsockPath,
			SnapVsockPath: checkout.SnapVsockPath,
		}
		defer vm.DestroyCheckedOutVM(checkout)

		if output.IsVerbose() {
			fmt.Fprintf(cmd.ErrOrStderr(), "Using pool VM %s (PID %d)\n",
				checkout.InstanceID, checkout.PID)
		}
	} else {
		// Cold restore path — start page cache warming ASAP so the render
		// daemon's memory pages are in cache by the time we send the request.
		vm.WarmSnapshotPageCacheAsync(vmPaths, version)

		var prereqErrs []*vm.PrereqError
		var snapErr error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); prereqErrs = vm.CheckPrerequisites(vmPaths) }()
		go func() { defer wg.Done(); snapErr = vm.CheckSnapshot(vmPaths, version) }()
		go vm.CleanupStaleInstances(vmPaths)
		wg.Wait()

		if len(prereqErrs) > 0 {
			var msgs []string
			for _, e := range prereqErrs {
				msgs = append(msgs, e.Error())
			}
			return fmt.Errorf("VM prerequisites not met:\n  %s", strings.Join(msgs, "\n  "))
		}
		if snapErr != nil {
			return snapErr
		}

		useUffd := os.Getenv("DH_VM_NO_UFFD") != "1"
		vmCfg := &vm.VMConfig{
			DHHome:  dhHome,
			Version: version,
			Verbose: output.IsVerbose(),
			UseUffd: useUffd,
		}

		if !output.IsQuiet() {
			fmt.Fprintln(cmd.ErrOrStderr(), "Starting Deephaven VM...")
		}

		start := time.Now()
		restoreInfo, machine, uffdCloser, restoreErr := vm.RestoreFromSnapshot(cmd.Context(), vmCfg, vmPaths, cmd.ErrOrStderr())
		if restoreErr != nil {
			return fmt.Errorf("restoring VM: %w", restoreErr)
		}
		info = restoreInfo
		defer func() {
			vm.DestroyInstance(machine, info, vmPaths)
			if uffdCloser != nil {
				uffdCloser.Close()
			}
		}()

		if output.IsVerbose() {
			fmt.Fprintf(cmd.ErrOrStderr(), "VM restored in %dms (instance %s)\n",
				time.Since(start).Milliseconds(), info.ID)
		}
	}

	// Start host file server for workspace file access
	cwd, _ := os.Getwd()
	fileServer, err := vm.StartFileServer(info.SnapVsockPath, cwd)
	if err != nil && output.IsVerbose() {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: file server: %v\n", err)
	}
	if fileServer != nil {
		defer fileServer.Close()
	}

	// Build action args
	var actionArgs []string
	if diagnose {
		actionArgs = []string{"diagnose"}
	} else {
		actionArgs = args[1:]
	}

	// Build vsock render request.
	// renderWidgetFlag is set by runRenderPipeline (auto-detected or from --widget flag).
	req := &vm.VsockRequest{
		Code:          string(scriptContent),
		Render:        true,
		Widget:        renderWidgetFlag,
		Actions:       actionArgs,
		RenderTimeout: renderTimeoutFlag,
		MaxRows:       renderRowsFlag,
		RenderJSON:    renderJSONFlag || output.IsJSON(),
		Verbose:       output.IsVerbose(),
	}

	resp, err := vm.ExecuteViaVsock(info.VsockPath, vm.VsockPort, req)
	if err != nil {
		return fmt.Errorf("executing render via VM: %w", err)
	}

	// Print render output to stdout
	if resp.RenderOutput != "" {
		fmt.Fprint(cmd.OutOrStdout(), resp.RenderOutput)
	}

	// Detect illustrated_message errors in render output (React error boundary UI).
	// These render successfully but indicate a widget error — should fail the command.
	hasIllustratedError := resp.RenderOutput != "" &&
		strings.Contains(resp.RenderOutput, "[illustrated_message] icon=\"warning\"")

	// Compress error if present
	var errorText string
	if resp.Error != nil && *resp.Error != "" {
		filename := filepath.Base(scriptPath)
		errorText = dhexec.CompressError(*resp.Error, filename, output.IsVerbose())
	}

	// Print stderr, deduplicating with the error field
	stderr := resp.Stderr
	if stderr != "" && errorText != "" {
		if strings.TrimSpace(stderr) == strings.TrimSpace(*resp.Error) ||
			strings.TrimSpace(stderr) == strings.TrimSpace(errorText) {
			stderr = ""
		}
	}
	if stderr != "" {
		fmt.Fprint(cmd.ErrOrStderr(), stderr)
	}

	if resp.ExitCode != 0 {
		if errorText != "" {
			fmt.Fprintln(cmd.ErrOrStderr(), errorText)
		}
		os.Exit(resp.ExitCode)
	}

	if hasIllustratedError {
		os.Exit(1)
	}

	return nil
}

// tryRenderPoolCheckout attempts to check out a warm VM from the pool daemon.
// Like exec, pool is tried by default unless DH_VM_POOL=0 is set.
// If the pool daemon isn't running, it auto-starts one for next time.
func tryRenderPoolCheckout(cmd *cobra.Command, version, dhHome string) *vm.CheckoutInfo {
	if os.Getenv("DH_VM_POOL") == "0" {
		return nil
	}

	checkout, err := vm.PoolCheckout()
	if err != nil {
		if errors.Is(err, vm.ErrPoolNotRunning) {
			autoStartRenderPool(dhHome, version, cmd.ErrOrStderr())
		} else if output.IsVerbose() {
			fmt.Fprintf(cmd.ErrOrStderr(), "Pool checkout: %v, falling back to cold restore\n", err)
		}
		return nil
	}

	if version != "" && checkout.Version != version {
		if output.IsVerbose() {
			fmt.Fprintf(cmd.ErrOrStderr(), "Pool version mismatch (pool=%s, requested=%s), falling back to cold restore\n",
				checkout.Version, version)
		}
		vm.DestroyCheckedOutVM(checkout)
		return nil
	}

	return checkout
}

// autoStartRenderPool forks a pool daemon in the background so the next
// invocation gets a fast pool checkout instead of a cold restore.
func autoStartRenderPool(dhHome, version string, stderr io.Writer) {
	snapVersion := version
	if snapVersion == "" {
		vmPaths := vm.NewVMPaths(dhHome)
		snapVersions, err := listSnapshotVersions(vmPaths)
		if err != nil || len(snapVersions) == 0 {
			return
		}
		snapVersion = snapVersions[0]
	}

	vmPaths := vm.NewVMPaths(dhHome)
	if vm.CheckSnapshot(vmPaths, snapVersion) != nil {
		return
	}

	if output.IsVerbose() {
		fmt.Fprintf(stderr, "Auto-starting pool daemon (first render uses cold restore)...\n")
	}

	exePath, err := os.Executable()
	if err != nil {
		return
	}

	args := []string{"vm", "pool", "start", "--background",
		"-n", "1",
		"--idle-timeout", "5m",
		"--version", snapVersion,
	}
	if output.IsVerbose() {
		args = append(args, "-v")
	}

	poolCmd := exec.Command(exePath, args...)
	poolCmd.Env = os.Environ()
	if dhHome != "" {
		poolCmd.Env = append(poolCmd.Env, "DH_HOME="+dhHome)
	}
	poolCmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	os.MkdirAll(vmPaths.Base, 0o755)
	logFile, err := os.OpenFile(fmt.Sprintf("%s/pool.log", vmPaths.Base), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	poolCmd.Stdout = logFile
	poolCmd.Stderr = logFile
	poolCmd.Start()
	logFile.Close()
}
