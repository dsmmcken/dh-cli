package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/dsmmcken/dh-cli/src/internal/config"
	"github.com/dsmmcken/dh-cli/src/internal/output"
	"github.com/dsmmcken/dh-cli/src/internal/versions"
	"github.com/dsmmcken/dh-cli/src/internal/vm"
	"github.com/spf13/cobra"
)

var (
	poolSizeFlag        int
	poolIdleTimeoutFlag string
	poolBackgroundFlag  bool
	poolJSONFlag        bool
)

func addPoolCommands(vmCmd *cobra.Command) {
	poolCmd := &cobra.Command{
		Use:   "pool",
		Short: "Manage the VM pool daemon for fast execution",
		Long: `Manage a pool of pre-warmed Firecracker VMs.

The pool daemon keeps VMs restored from snapshot and ready to execute code
immediately, reducing latency from ~700ms (cold restore) to ~20ms (warm pool).

Subcommands:
  start   Start the pool daemon
  stop    Stop the pool daemon
  status  Show pool status
  scale   Adjust pool size`,
	}

	// dh vm pool start
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the pool daemon",
		Long: `Start the VM pool daemon in the foreground (or background with --background).

The daemon pre-warms VMs from a snapshot and serves exec requests over a Unix
socket. It auto-shuts down after the idle timeout.`,
		RunE: runPoolStart,
	}
	startCmd.Flags().IntVarP(&poolSizeFlag, "size", "n", 1, "Number of warm VMs to maintain")
	startCmd.Flags().StringVar(&poolIdleTimeoutFlag, "idle-timeout", "5m", "Shut down after this duration of inactivity")
	startCmd.Flags().StringVar(&vmVersionFlag, "version", "", "Deephaven version (default: resolved version)")
	startCmd.Flags().BoolVar(&poolBackgroundFlag, "background", false, "Daemonize the pool daemon (internal)")
	startCmd.Flags().MarkHidden("background")

	// dh vm pool stop
	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the pool daemon",
		RunE:  runPoolStop,
	}

	// dh vm pool status
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show pool status",
		RunE:  runPoolStatus,
	}
	statusCmd.Flags().BoolVar(&poolJSONFlag, "json", false, "Output as JSON")

	// dh vm pool scale
	scaleCmd := &cobra.Command{
		Use:   "scale N",
		Short: "Adjust pool size",
		Args:  cobra.ExactArgs(1),
		RunE:  runPoolScale,
	}

	poolCmd.AddCommand(startCmd, stopCmd, statusCmd, scaleCmd)
	vmCmd.AddCommand(poolCmd)
}

func runPoolStart(cmd *cobra.Command, args []string) error {
	config.SetConfigDir(ConfigDir)
	dhHome := config.DHHome()

	version, err := config.ResolveVersion(vmVersionFlag, os.Getenv("DH_VERSION"))
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "No version specified, fetching latest from PyPI...\n")
		latest, pypiErr := versions.FetchLatestVersion()
		if pypiErr != nil {
			return fmt.Errorf("resolving version: %w (PyPI fallback also failed: %v)", err, pypiErr)
		}
		version = latest
	}

	idleTimeout, err := time.ParseDuration(poolIdleTimeoutFlag)
	if err != nil {
		return fmt.Errorf("invalid idle-timeout: %w", err)
	}

	// If --background, daemonize by re-execing ourselves
	if poolBackgroundFlag {
		return runPoolDaemonBackground(cmd, version, dhHome, idleTimeout)
	}

	useUffd := os.Getenv("DH_VM_NO_UFFD") != "1"

	pool := vm.NewPool(vm.PoolConfig{
		DHHome:      dhHome,
		Version:     version,
		TargetSize:  poolSizeFlag,
		IdleTimeout: idleTimeout,
		Verbose:     output.IsVerbose(),
		UseUffd:     useUffd,
	})

	// Handle signals
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	return pool.Start(ctx, cmd.ErrOrStderr())
}

// runPoolDaemonBackground forks the pool daemon as a background process.
func runPoolDaemonBackground(cmd *cobra.Command, version, dhHome string, idleTimeout time.Duration) error {
	// Build the command to run in background (without --background to avoid recursion)
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable path: %w", err)
	}

	poolArgs := []string{"vm", "pool", "start",
		"-n", fmt.Sprintf("%d", poolSizeFlag),
		"--idle-timeout", idleTimeout.String(),
		"--version", version,
	}
	if output.IsVerbose() {
		poolArgs = append(poolArgs, "-v")
	}

	paths := vm.NewVMPaths(dhHome)
	logPath := fmt.Sprintf("%s/pool.log", paths.Base)
	pidPath := fmt.Sprintf("%s/pool.pid", paths.Base)
	os.MkdirAll(paths.Base, 0o755)

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}

	daemonCmd := exec.Command(exePath, poolArgs...)
	daemonCmd.Stdout = logFile
	daemonCmd.Stderr = logFile
	daemonCmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	// Propagate environment
	daemonCmd.Env = os.Environ()

	if err := daemonCmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("starting daemon: %w", err)
	}

	// Write PID file
	os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", daemonCmd.Process.Pid)), 0o644)
	logFile.Close()

	// Wait for socket to appear (up to 10s)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if vm.PoolProbe() {
			fmt.Fprintf(cmd.ErrOrStderr(), "Pool daemon started (pid=%d, version=%s, size=%d, log=%s)\n",
				daemonCmd.Process.Pid, version, poolSizeFlag, logPath)
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "Pool daemon started (pid=%d) but socket not ready yet. Check %s\n",
		daemonCmd.Process.Pid, logPath)
	return nil
}

func runPoolStop(cmd *cobra.Command, args []string) error {
	if !vm.PoolProbe() {
		fmt.Fprintln(cmd.ErrOrStderr(), "Pool daemon is not running.")
		return nil
	}

	resp, err := vm.PoolCommand(&vm.PoolRequest{Type: "stop"})
	if err != nil {
		return fmt.Errorf("sending stop: %w", err)
	}
	if resp.Type == "error" {
		return fmt.Errorf("pool error: %s", resp.Error)
	}

	fmt.Fprintln(cmd.ErrOrStderr(), "Pool daemon stopped.")
	return nil
}

func runPoolStatus(cmd *cobra.Command, args []string) error {
	if !vm.PoolProbe() {
		if poolJSONFlag {
			return output.PrintJSON(cmd.OutOrStdout(), map[string]any{
				"running": false,
			})
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Pool daemon is not running.")
		return nil
	}

	resp, err := vm.PoolCommand(&vm.PoolRequest{Type: "status"})
	if err != nil {
		return fmt.Errorf("getting status: %w", err)
	}
	if resp.Type == "error" {
		return fmt.Errorf("pool error: %s", resp.Error)
	}

	if poolJSONFlag || output.IsJSON() {
		data, _ := json.MarshalIndent(resp.Status, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	}

	s := resp.Status
	fmt.Fprintf(cmd.OutOrStdout(), "Pool daemon (pid=%d)\n", s.PID)
	fmt.Fprintf(cmd.OutOrStdout(), "  Version:      %s\n", s.Version)
	fmt.Fprintf(cmd.OutOrStdout(), "  Ready VMs:    %d / %d\n", s.Ready, s.TargetSize)
	fmt.Fprintf(cmd.OutOrStdout(), "  Idle:         %ds (timeout: %ds)\n", s.IdleSeconds, s.IdleTimeout)
	return nil
}

func runPoolScale(cmd *cobra.Command, args []string) error {
	if !vm.PoolProbe() {
		return fmt.Errorf("pool daemon is not running")
	}

	var n int
	if _, err := fmt.Sscanf(args[0], "%d", &n); err != nil {
		return fmt.Errorf("invalid size: %s", args[0])
	}

	resp, err := vm.PoolCommand(&vm.PoolRequest{Type: "scale", TargetSize: n})
	if err != nil {
		return fmt.Errorf("sending scale: %w", err)
	}
	if resp.Type == "error" {
		return fmt.Errorf("pool error: %s", resp.Error)
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "Pool size set to %d.\n", n)
	return nil
}
