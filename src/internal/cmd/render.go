package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	dhexec "github.com/dsmmcken/dh-cli/src/internal/exec"
	"github.com/dsmmcken/dh-cli/src/internal/config"
	"github.com/dsmmcken/dh-cli/src/internal/java"
	"github.com/dsmmcken/dh-cli/src/internal/output"
	"github.com/dsmmcken/dh-cli/src/internal/render"
	"github.com/spf13/cobra"
)

var (
	renderURLFlag     string
	renderWidgetFlag  string
	renderPortFlag    int
	renderTimeoutFlag int
	renderRowsFlag    int
	renderVersionFlag string
	renderJVMArgsFlag string
	renderJSONFlag    bool
	renderVMFlag      bool
	renderPoolFlag    bool
)

func addRenderCommands(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "render SCRIPT [actions...]",
		Short: "Render a Deephaven UI widget and interact with it",
		Long: `Render a Deephaven UI widget in a headless environment.

Actions (left to right after the script path):
  snapshot                    Print accessibility tree (default)
  click <target>              Click by text or @ref
  fill <target> <value>       Fill text field
  select <target> <value>     Select picker option
  tables                      List exported tables
  table <id>                  Fetch table data
  html                        Dump rendered HTML
  wait <ms>                   Pause for async effects

Examples:
  dh render test_button.py
  dh render test_button.py click "Primary"
  dh render test_form.py fill "Name" "Alice" click "Submit"
  dh render test_button.py --url http://localhost:10000 snapshot
  dh render test_table.py tables --rows 20`,
		Args:               cobra.MinimumNArgs(1),
		RunE:               runRender,
		DisableFlagParsing: false,
	}

	flags := cmd.Flags()
	flags.StringVar(&renderURLFlag, "url", "", "Connect to existing server (skip server start)")
	flags.StringVar(&renderWidgetFlag, "widget", "", "Widget name (default: auto-detect from script)")
	flags.IntVar(&renderPortFlag, "port", 0, "Server port (0 = auto-assign)")
	flags.IntVar(&renderTimeoutFlag, "timeout", 15000, "Render timeout in ms")
	flags.IntVar(&renderRowsFlag, "rows", 10, "Max table rows")
	flags.StringVar(&renderVersionFlag, "version", "", "Deephaven version to use")
	flags.StringVar(&renderJVMArgsFlag, "jvm-args", "-Xmx4g -DAuthHandlers=io.deephaven.auth.AnonymousAuthenticationHandler", "JVM arguments (quoted string)")
	flags.BoolVar(&renderJSONFlag, "json", false, "Output JSON instead of text")
	flags.BoolVar(&renderVMFlag, "vm", false, "Run in a Firecracker microVM (experimental, Linux only)")
	flags.BoolVar(&renderPoolFlag, "pool", false, "Use a warm VM from the pool daemon (requires --vm)")

	// Diagnose subcommand
	diagnoseCmd := &cobra.Command{
		Use:   "diagnose SCRIPT",
		Short: "Run diagnostic report on a Deephaven UI widget",
		Long: `Connect to a Deephaven server, render a widget, and produce a diagnostic JSON report.

Examples:
  dh render diagnose test_button.py
  dh render diagnose test_button.py --url http://localhost:10000`,
		Args: cobra.ExactArgs(1),
		RunE: runRenderDiagnose,
	}

	diagnoseFlags := diagnoseCmd.Flags()
	diagnoseFlags.StringVar(&renderURLFlag, "url", "", "Connect to existing server (skip server start)")
	diagnoseFlags.StringVar(&renderWidgetFlag, "widget", "", "Widget name (default: auto-detect from script)")
	diagnoseFlags.IntVar(&renderPortFlag, "port", 0, "Server port (0 = auto-assign)")
	diagnoseFlags.IntVar(&renderTimeoutFlag, "timeout", 15000, "Render timeout in ms")
	diagnoseFlags.StringVar(&renderVersionFlag, "version", "", "Deephaven version to use")
	diagnoseFlags.StringVar(&renderJVMArgsFlag, "jvm-args", "-Xmx4g -DAuthHandlers=io.deephaven.auth.AnonymousAuthenticationHandler", "JVM arguments (quoted string)")
	diagnoseFlags.BoolVar(&renderVMFlag, "vm", false, "Run in a Firecracker microVM (experimental, Linux only)")
	diagnoseFlags.BoolVar(&renderPoolFlag, "pool", false, "Use a warm VM from the pool daemon (requires --vm)")

	cmd.AddCommand(diagnoseCmd)
	parent.AddCommand(cmd)
}

func runRender(cmd *cobra.Command, args []string) error {
	return runRenderPipeline(cmd, args, false)
}

func runRenderDiagnose(cmd *cobra.Command, args []string) error {
	return runRenderPipeline(cmd, args, true)
}

func runRenderPipeline(cmd *cobra.Command, args []string, diagnose bool) error {
	// Validate flag combinations
	if renderVMFlag && renderURLFlag != "" {
		return fmt.Errorf("--vm and --url are mutually exclusive")
	}
	if renderPoolFlag && !renderVMFlag {
		return fmt.Errorf("--pool requires --vm")
	}
	if renderVMFlag && cmd.Flags().Changed("jvm-args") {
		return fmt.Errorf("--vm and --jvm-args are mutually exclusive (JVM runs inside the VM)")
	}

	scriptPath := args[0]

	// Verify script exists
	if _, err := os.Stat(scriptPath); err != nil {
		return fmt.Errorf("script not found: %s", scriptPath)
	}

	// Resolve absolute path
	absScript, err := filepath.Abs(scriptPath)
	if err != nil {
		return fmt.Errorf("resolving script path: %w", err)
	}

	// Detect widget name
	widget := renderWidgetFlag
	if widget == "" {
		detected, err := render.DetectWidgetName(absScript)
		if err != nil {
			// VM mode: defer widget detection to the VM — run the script first
			// so execution errors (more actionable) surface before "no widget found".
			if renderVMFlag {
				return runRenderVM(cmd, args, diagnose)
			}
			return fmt.Errorf("auto-detecting widget name: %w\nUse --widget to specify it manually.", err)
		}
		widget = detected
		renderWidgetFlag = detected // make available to VM path
		if output.IsVerbose() {
			fmt.Fprintf(cmd.ErrOrStderr(), "Detected widget: %s\n", widget)
		}
	}

	// VM mode — skip Node.js/npm/server checks, delegate to VM
	if renderVMFlag {
		return runRenderVM(cmd, args, diagnose)
	}

	// Verify Node.js is available
	nodePath, err := render.CheckNode()
	if err != nil {
		return err
	}

	// Set up render runtime (extract embedded JS, npm install)
	config.SetConfigDir(ConfigDir)
	dhHome := config.DHHome()
	renderDir, err := render.EnsureRuntime(dhHome, output.IsQuiet())
	if err != nil {
		return fmt.Errorf("setting up render runtime: %w", err)
	}

	// Determine server URL — either provided or we start one
	serverURL := renderURLFlag
	var serverProcess *exec.Cmd

	if serverURL == "" {
		url, proc, err := startServerForRender(cmd, absScript, dhHome)
		if err != nil {
			return err
		}
		serverURL = url
		serverProcess = proc
		defer func() {
			if serverProcess != nil && serverProcess.Process != nil {
				killServerProcess(serverProcess)
			}
		}()
	}

	// Build oneshot.mjs command
	oneshotPath := filepath.Join(renderDir, "bin", "oneshot.mjs")
	cssLoaderPath := filepath.Join(renderDir, "src", "css-loader.mjs")

	nodeArgs := []string{
		"--no-warnings",
		"--import", cssLoaderPath,
		oneshotPath,
		"--url", serverURL,
		"--widget", widget,
		"--timeout", fmt.Sprintf("%d", renderTimeoutFlag),
		"--rows", fmt.Sprintf("%d", renderRowsFlag),
	}

	if renderJSONFlag || output.IsJSON() {
		nodeArgs = append(nodeArgs, "--json")
	}

	if diagnose {
		nodeArgs = append(nodeArgs, "diagnose")
	} else {
		// Append action args (everything after script path)
		nodeArgs = append(nodeArgs, args[1:]...)
	}

	nodeCmd := exec.Command(nodePath, nodeArgs...)
	nodeCmd.Stdout = cmd.OutOrStdout()
	nodeCmd.Stderr = cmd.ErrOrStderr()

	// Forward signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for range sigCh {
			if nodeCmd.Process != nil {
				nodeCmd.Process.Signal(syscall.SIGINT)
			}
		}
	}()
	defer func() { signal.Stop(sigCh); close(sigCh) }()

	nodeErr := nodeCmd.Run()

	// Clean up server BEFORE potentially exiting.
	// Send SIGINT first for graceful shutdown, escalate to SIGKILL after 3s.
	if serverProcess != nil && serverProcess.Process != nil {
		killServerProcess(serverProcess)
		serverProcess = nil // prevent defer from double-killing
	}

	if nodeErr != nil {
		if exitErr, ok := nodeErr.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("render failed: %w", nodeErr)
	}

	return nil
}

// startServerForRender starts a Deephaven server for the given script,
// waits for the __DH_READY__ sentinel, and returns the server URL and process.
func startServerForRender(cmd *cobra.Command, scriptPath, dhHome string) (string, *exec.Cmd, error) {
	// Read script
	scriptContent, err := os.ReadFile(scriptPath)
	if err != nil {
		return "", nil, fmt.Errorf("reading script: %w", err)
	}

	// Resolve version
	envVersion := os.Getenv("DH_VERSION")
	version, err := config.ResolveVersion(renderVersionFlag, envVersion)
	if err != nil {
		return "", nil, fmt.Errorf("resolving version: %w", err)
	}

	if output.IsVerbose() {
		fmt.Fprintf(cmd.ErrOrStderr(), "Resolved version: %s\n", version)
	}

	// Find venv python
	pythonBin, err := dhexec.FindVenvPython(dhHome, version)
	if err != nil {
		return "", nil, fmt.Errorf("finding venv python: %w", err)
	}

	// Ensure pydeephaven
	if err := dhexec.EnsurePydeephaven(pythonBin, version, output.IsQuiet(), cmd.ErrOrStderr()); err != nil {
		return "", nil, fmt.Errorf("ensuring pydeephaven: %w", err)
	}

	// Detect Java
	javaInfo, err := java.Detect(dhHome)
	if err != nil {
		return "", nil, fmt.Errorf("detecting Java: %w", err)
	}
	if !javaInfo.Found {
		return "", nil, fmt.Errorf("Java not found; install Java 17+ or set JAVA_HOME")
	}

	// Build runner args for serve mode with auto-port
	port := renderPortFlag
	if port == 0 {
		port = 0 // auto-assign
	}

	runnerArgs := []string{"--mode", "serve"}
	runnerArgs = append(runnerArgs, "--port", fmt.Sprintf("%d", port))
	if renderJVMArgsFlag != "" {
		runnerArgs = append(runnerArgs, fmt.Sprintf("--jvm-args=%s", renderJVMArgsFlag))
	}

	callerCwd, _ := os.Getwd()
	runnerArgs = append(runnerArgs, "--script-path", scriptPath)
	runnerArgs = append(runnerArgs, "--cwd", callerCwd)

	if !output.IsQuiet() {
		fmt.Fprintln(cmd.ErrOrStderr(), "Starting server...")
	}

	cmdArgs := append([]string{"-c", dhexec.RunnerScript()}, runnerArgs...)
	process := exec.CommandContext(cmd.Context(), pythonBin, cmdArgs...)
	process.Env = append(os.Environ(), fmt.Sprintf("JAVA_HOME=%s", javaInfo.Home))
	process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	process.Stdin = strings.NewReader(string(scriptContent))
	process.Stderr = cmd.ErrOrStderr()

	stdoutPipe, err := process.StdoutPipe()
	if err != nil {
		return "", nil, fmt.Errorf("creating stdout pipe: %w", err)
	}

	if err := process.Start(); err != nil {
		return "", nil, fmt.Errorf("starting server: %w", err)
	}

	// Signal handling for the server process
	var sigCount int32
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for range sigCh {
			count := atomic.AddInt32(&sigCount, 1)
			if process.Process != nil {
				if count == 1 {
					syscall.Kill(-process.Process.Pid, syscall.SIGINT)
				} else {
					syscall.Kill(-process.Process.Pid, syscall.SIGKILL)
				}
			}
		}
	}()

	// Wait for __DH_READY__
	scanner := bufio.NewScanner(stdoutPipe)
	var serverURL string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "__DH_READY__:") {
			serverURL = strings.TrimPrefix(line, "__DH_READY__:")
			break
		}
		// Forward non-sentinel output
		if output.IsVerbose() {
			fmt.Fprintln(cmd.ErrOrStderr(), line)
		}
	}

	// Stop signal handler for the server (the render pipeline manages cleanup)
	signal.Stop(sigCh)
	close(sigCh)

	if serverURL == "" {
		// Server died before becoming ready
		process.Wait()
		return "", nil, fmt.Errorf("server exited without becoming ready")
	}

	if !output.IsQuiet() {
		fmt.Fprintf(cmd.ErrOrStderr(), "Server ready on %s\n", serverURL)
	}

	return serverURL, process, nil
}

// killServerProcess sends SIGKILL to the server process group.
// The JVM inside runner.py doesn't always exit promptly on SIGINT,
// so we skip the grace period and go straight to SIGKILL.
func killServerProcess(proc *exec.Cmd) {
	pgid := -proc.Process.Pid
	syscall.Kill(pgid, syscall.SIGKILL)

	// Wait with a timeout so we don't hang forever
	done := make(chan struct{})
	go func() {
		proc.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		// Already sent SIGKILL — nothing more we can do
	}
}
