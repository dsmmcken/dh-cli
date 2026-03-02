//go:build linux

package cmd

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dsmmcken/dh-cli/src/internal/config"
	"github.com/dsmmcken/dh-cli/src/internal/output"
	"github.com/dsmmcken/dh-cli/src/internal/tui/screens"
	"github.com/dsmmcken/dh-cli/src/internal/vm"
	"github.com/spf13/cobra"
)

func runServeVM(cmd *cobra.Command, args []string) error {
	scriptPath := args[0]

	// Read script file
	scriptContent, err := os.ReadFile(scriptPath)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: reading script file %s: %v\n", scriptPath, err)
		os.Exit(output.ExitError)
	}

	// Resolve version: flag → env → config (.dhrc, config.toml) → latest snapshot
	config.SetConfigDir(ConfigDir)
	dhHome := config.DHHome()
	vmPaths := vm.NewVMPaths(dhHome)

	version, err := config.ResolveVersion(serveVersionFlag, os.Getenv("DH_VERSION"))
	if err != nil {
		// Try latest snapshot as last resort
		snapVersions, snapErr := listSnapshotVersions(vmPaths)
		if snapErr != nil || len(snapVersions) == 0 {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: resolving version: %v\n", err)
			os.Exit(output.ExitError)
		}
		version = snapVersions[0]
	}

	if output.IsVerbose() {
		fmt.Fprintf(cmd.ErrOrStderr(), "Resolved version: %s\n", version)
	}

	// Check prerequisites and snapshot in parallel
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
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: VM prerequisites not met:\n  %s\n", strings.Join(msgs, "\n  "))
		os.Exit(output.ExitError)
	}
	if snapErr != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", snapErr)
		os.Exit(output.ExitError)
	}

	// Restore VM from snapshot
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
	info, machine, uffdCloser, err := vm.RestoreFromSnapshot(cmd.Context(), vmCfg, vmPaths, cmd.ErrOrStderr())
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: restoring VM: %v\n", err)
		os.Exit(output.ExitError)
	}
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

	// Start host file server for workspace file access.
	// Must use SnapVsockPath (the original snapshot path) because Firecracker
	// constructs guest→host listener paths from the path embedded in the snapshot
	// state, not the renamed per-instance path.
	cwd, _ := os.Getwd()
	fileServer, err := vm.StartFileServer(info.SnapVsockPath, cwd)
	if err != nil && output.IsVerbose() {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: file server: %v\n", err)
	}
	if fileServer != nil {
		defer fileServer.Close()
	}

	// Start TCP→vsock HTTP proxy (host→guest direction uses the per-instance path)
	listenAddr := fmt.Sprintf("127.0.0.1:%d", servePortFlag)
	var proxyStderr io.Writer
	if output.IsVerbose() {
		proxyStderr = cmd.ErrOrStderr()
	}
	proxy, err := vm.StartHTTPProxy(listenAddr, info.VsockPath, proxyStderr)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: starting HTTP proxy on %s: %v\n", listenAddr, err)
		os.Exit(output.ExitError)
	}
	defer proxy.Close()

	// Execute user script via vsock
	req := &vm.VsockRequest{
		Code: string(scriptContent),
	}
	resp, err := vm.ExecuteViaVsock(info.VsockPath, vm.VsockPort, req)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: executing script: %v\n", err)
		os.Exit(output.ExitError)
	}

	// Print any script output
	if resp.Stdout != "" {
		fmt.Fprint(cmd.OutOrStdout(), resp.Stdout)
		if !strings.HasSuffix(resp.Stdout, "\n") {
			fmt.Fprintln(cmd.OutOrStdout())
		}
	}
	if resp.Stderr != "" {
		fmt.Fprint(cmd.ErrOrStderr(), resp.Stderr)
		if !strings.HasSuffix(resp.Stderr, "\n") {
			fmt.Fprintln(cmd.ErrOrStderr())
		}
	}

	if resp.ExitCode != 0 {
		errMsg := ""
		if resp.Error != nil {
			errMsg = *resp.Error
		}
		if errMsg != "" {
			fmt.Fprintln(cmd.ErrOrStderr(), errMsg)
		}
		os.Exit(resp.ExitCode)
	}

	// Construct URL and open browser
	url := fmt.Sprintf("http://localhost:%d", servePortFlag)
	if serveIframeFlag != "" {
		url = fmt.Sprintf("%s/iframe/widget/?name=%s", url, serveIframeFlag)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Server running at %s\n", url)
	fmt.Fprintln(cmd.OutOrStdout(), "Press Ctrl+C to stop.")

	if !serveNoBrowserFlag {
		screens.OpenBrowser(url)
	}

	// Wait for signal — VM stays alive until user exits
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	signal.Stop(sigCh)

	fmt.Fprintln(cmd.ErrOrStderr(), "\nShutting down...")
	// Cleanup happens via defers (proxy, file server, VM destroy)
	return nil
}

// listSnapshotVersions returns available complete snapshot versions, newest first.
// Only snapshots with metadata.json are considered complete.
func listSnapshotVersions(paths *vm.VMPaths) ([]string, error) {
	entries, err := os.ReadDir(paths.SnapshotDir)
	if err != nil {
		return nil, err
	}
	var versions []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Only include complete snapshots (have metadata.json)
		if err := vm.CheckSnapshot(paths, e.Name()); err == nil {
			versions = append(versions, e.Name())
		}
	}
	// Reverse so newest is first (directory listing is alphabetical,
	// and semver sorts correctly for our purposes)
	for i, j := 0, len(versions)-1; i < j; i, j = i+1, j-1 {
		versions[i], versions[j] = versions[j], versions[i]
	}
	return versions, nil
}
