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

	dhexec "github.com/dsmmcken/dh-cli/src/internal/exec"
	"github.com/dsmmcken/dh-cli/src/internal/config"
	"github.com/dsmmcken/dh-cli/src/internal/java"
	"github.com/dsmmcken/dh-cli/src/internal/output"
	"github.com/dsmmcken/dh-cli/src/internal/tui/screens"
	"github.com/spf13/cobra"
)

var (
	servePortFlag      int
	serveJVMArgsFlag   string
	serveNoBrowserFlag bool
	serveIframeFlag    string
	serveVersionFlag   string
)

func addServeCommand(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "serve SCRIPT",
		Short: "Run script and keep server alive (dashboards/services)",
		Long: `Run a script and keep the Deephaven server running.

Use for:
  - Dashboards and visualizations
  - Long-running data pipelines
  - Services that need persistent server

Opens browser automatically. Server runs until Ctrl+C.

Examples:
  dhg serve dashboard.py
  dhg serve dashboard.py --port 8080
  dhg serve dashboard.py --iframe my_widget
  dhg serve dashboard.py --no-browser`,
		Args: cobra.ExactArgs(1),
		RunE: runServe,
	}

	flags := cmd.Flags()
	flags.IntVar(&servePortFlag, "port", 10000, "Server port")
	flags.StringVar(&serveJVMArgsFlag, "jvm-args", "-Xmx4g", "JVM arguments (quoted string)")
	flags.BoolVar(&serveNoBrowserFlag, "no-browser", false, "Don't open browser automatically")
	flags.StringVar(&serveIframeFlag, "iframe", "", "Open browser to iframe URL for the given widget name")
	flags.StringVar(&serveVersionFlag, "version", "", "Deephaven version to use")

	parent.AddCommand(cmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	scriptPath := args[0]

	// Read script file
	scriptContent, err := os.ReadFile(scriptPath)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: reading script file %s: %v\n", scriptPath, err)
		os.Exit(output.ExitError)
	}

	// Resolve version
	config.SetConfigDir(ConfigDir)
	dhgHome := config.DHGHome()
	envVersion := os.Getenv("DHG_VERSION")

	version, err := config.ResolveVersion(serveVersionFlag, envVersion)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: resolving version: %v\n", err)
		os.Exit(output.ExitError)
	}

	if output.IsVerbose() {
		fmt.Fprintf(cmd.ErrOrStderr(), "Resolved version: %s\n", version)
	}

	// Find venv python
	pythonBin, err := dhexec.FindVenvPython(dhgHome, version)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: finding venv python: %v\n", err)
		os.Exit(output.ExitError)
	}

	if output.IsVerbose() {
		fmt.Fprintf(cmd.ErrOrStderr(), "Venv python: %s\n", pythonBin)
	}

	// Ensure pydeephaven is installed
	if err := dhexec.EnsurePydeephaven(pythonBin, version, output.IsQuiet(), cmd.ErrOrStderr()); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: ensuring pydeephaven: %v\n", err)
		os.Exit(output.ExitError)
	}

	// Detect Java (serve is always embedded mode)
	javaInfo, err := java.Detect(dhgHome)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: detecting Java: %v\n", err)
		os.Exit(output.ExitError)
	}
	if !javaInfo.Found {
		fmt.Fprintln(cmd.ErrOrStderr(), "Error: Java not found; install Java 17+ or set JAVA_HOME")
		os.Exit(output.ExitError)
	}

	if output.IsVerbose() {
		fmt.Fprintf(cmd.ErrOrStderr(), "Using Java: %s (version %s)\n", javaInfo.Path, javaInfo.Version)
	}

	// Build runner args
	runnerArgs := []string{"--mode", "serve"}
	runnerArgs = append(runnerArgs, "--port", fmt.Sprintf("%d", servePortFlag))
	if serveJVMArgsFlag != "" {
		runnerArgs = append(runnerArgs, fmt.Sprintf("--jvm-args=%s", serveJVMArgsFlag))
	}
	if serveIframeFlag != "" {
		runnerArgs = append(runnerArgs, "--iframe", serveIframeFlag)
	}

	// Resolve script path
	callerCwd, _ := os.Getwd()
	absPath, err := filepath.Abs(scriptPath)
	if err == nil {
		runnerArgs = append(runnerArgs, "--script-path", absPath)
	}
	runnerArgs = append(runnerArgs, "--cwd", callerCwd)

	if !output.IsQuiet() {
		fmt.Fprintln(cmd.ErrOrStderr(), "Starting Deephaven...")
	}

	// Build command: python -c "<runner script>" <args...>
	cmdArgs := append([]string{"-c", dhexec.RunnerScript()}, runnerArgs...)
	process := exec.CommandContext(cmd.Context(), pythonBin, cmdArgs...)

	// Set JAVA_HOME
	process.Env = append(os.Environ(), fmt.Sprintf("JAVA_HOME=%s", javaInfo.Home))

	// Process group for clean cleanup
	process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Pipe user script to stdin
	process.Stdin = strings.NewReader(string(scriptContent))
	process.Stderr = cmd.ErrOrStderr()

	// Pipe stdout so we can detect the ready sentinel
	stdoutPipe, err := process.StdoutPipe()
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: creating stdout pipe: %v\n", err)
		os.Exit(output.ExitError)
	}

	if err := process.Start(); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: starting runner: %v\n", err)
		os.Exit(output.ExitError)
	}

	// Signal handling: first SIGINT → graceful, second → force kill
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	var sigCount int32
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
	defer func() { signal.Stop(sigCh); close(sigCh) }()

	// Read stdout line by line, detect ready sentinel
	scanner := bufio.NewScanner(stdoutPipe)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "__DHG_READY__:") {
			url := strings.TrimPrefix(line, "__DHG_READY__:")
			if !serveNoBrowserFlag {
				screens.OpenBrowser(url)
			}
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), line)
		}
	}

	waitErr := process.Wait()
	exitCode := 0
	if exitErr, ok := waitErr.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else if waitErr != nil {
		exitCode = 1
	}

	if exitCode != 0 {
		os.Exit(exitCode)
	}

	return nil
}
