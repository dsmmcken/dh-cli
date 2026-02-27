package exec

import (
	"bytes"
	"context"
	"encoding/json"
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/dsmmcken/dh-cli/src/internal/config"
	"github.com/dsmmcken/dh-cli/src/internal/java"
	"github.com/dsmmcken/dh-cli/src/internal/output"
)

//go:embed runner.py
var runnerScript string

// RunnerScript returns the embedded runner.py content (for testing).
func RunnerScript() string {
	return runnerScript
}

// ExecConfig holds all configuration for an exec invocation.
type ExecConfig struct {
	// Code source (exactly one must be set)
	Code       string // from -c flag
	ScriptPath string // positional arg (file path or "-" for stdin)

	// Server options
	Port    int
	JVMArgs string
	Version string // explicit --version flag

	// Display options
	ShowTables    bool
	ShowTableMeta bool
	JSONMode      bool
	Verbose       bool
	Quiet         bool
	Timeout       int // seconds, 0 = no timeout

	// Remote options
	Host          string
	AuthType      string
	AuthToken     string
	TLS           bool
	TLSCACert     string
	TLSClientCert string
	TLSClientKey  string

	// VM mode (experimental)
	VMMode bool

	// Resolved state (populated by Run)
	ConfigDir    string
	Stderr       io.Writer
	Stdout       io.Writer
	ProcessStart time.Time // when Go process started (for startup diagnostics)
}

// ExecCommand wraps exec.Command for testability.
var ExecCommand = exec.Command

// Run executes the dh exec workflow. Returns exit code, optional JSON result, and error.
func Run(cfg *ExecConfig) (int, map[string]any, error) {
	runStart := time.Now()

	if cfg.Stderr == nil {
		cfg.Stderr = os.Stderr
	}
	if cfg.Stdout == nil {
		cfg.Stdout = os.Stdout
	}

	// Validate inputs
	if cfg.Code != "" && cfg.ScriptPath != "" {
		return output.ExitError, nil, fmt.Errorf("cannot use both -c and a script file")
	}
	if cfg.Code == "" && cfg.ScriptPath == "" {
		return output.ExitError, nil, fmt.Errorf("must provide either -c CODE or a script file (use - for stdin)")
	}

	// Read code from source
	userCode, err := readCode(cfg)
	if err != nil {
		return output.ExitError, nil, err
	}
	if strings.TrimSpace(userCode) == "" {
		// Empty script is a no-op success
		if cfg.JSONMode {
			return output.ExitSuccess, map[string]any{
				"exit_code":   0,
				"stdout":      "",
				"stderr":      "",
				"result_repr": nil,
				"error":       nil,
				"tables":      []any{},
			}, nil
		}
		return output.ExitSuccess, nil, nil
	}

	// Resolve version
	config.SetConfigDir(cfg.ConfigDir)
	dhHome := config.DHHome()
	envVersion := os.Getenv("DH_VERSION")

	version, err := config.ResolveVersion(cfg.Version, envVersion)
	if err != nil && cfg.VMMode {
		// In VM mode, fall back to the latest available snapshot
		if v, snapErr := latestSnapshotVersion(dhHome); snapErr == nil {
			version = v
			err = nil
		}
	}
	if err != nil {
		return output.ExitError, nil, fmt.Errorf("resolving version: %w", err)
	}

	if cfg.Verbose {
		fmt.Fprintf(cfg.Stderr, "Resolved version: %s (resolve=%dms)\n", version, time.Since(runStart).Milliseconds())
	}

	// VM mode: delegate to Firecracker-based execution
	isRemote := cfg.Host != ""
	if cfg.VMMode {
		if isRemote {
			return output.ExitError, nil, fmt.Errorf("cannot use both --vm and --host flags")
		}
		return runVM(cfg, userCode, version, dhHome)
	}

	// Find venv python
	pythonBin, err := FindVenvPython(dhHome, version)
	if err != nil {
		return output.ExitError, nil, fmt.Errorf("finding venv python: %w", err)
	}

	if cfg.Verbose {
		fmt.Fprintf(cfg.Stderr, "Venv python: %s\n", pythonBin)
	}

	// Ensure pydeephaven is installed
	if err := EnsurePydeephaven(pythonBin, version, cfg.Quiet, cfg.Stderr); err != nil {
		return output.ExitError, nil, fmt.Errorf("ensuring pydeephaven: %w", err)
	}

	// Detect Java for embedded mode
	var javaHome string
	if !isRemote {
		javaInfo, err := java.Detect(dhHome)
		if err != nil {
			return output.ExitError, nil, fmt.Errorf("detecting Java: %w", err)
		}
		if !javaInfo.Found {
			return output.ExitError, nil, fmt.Errorf("Java not found; install Java 17+ or set JAVA_HOME")
		}
		javaHome = javaInfo.Home
		if cfg.Verbose {
			fmt.Fprintf(cfg.Stderr, "Using Java: %s (version %s)\n", javaInfo.Path, javaInfo.Version)
		}
	}

	// Build runner args
	runnerArgs := buildRunnerArgs(cfg, isRemote)

	// Resolve script path and CWD for the runner
	callerCwd, _ := os.Getwd()
	if cfg.ScriptPath != "" && cfg.ScriptPath != "-" {
		absPath, err := filepath.Abs(cfg.ScriptPath)
		if err == nil {
			runnerArgs = append(runnerArgs, "--script-path", absPath)
		}
	}
	runnerArgs = append(runnerArgs, "--cwd", callerCwd)

	// Set up context with optional timeout
	ctx := context.Background()
	var cancel context.CancelFunc
	if cfg.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(cfg.Timeout)*time.Second)
		defer cancel()
	} else {
		ctx, cancel = context.WithCancel(ctx)
		defer cancel()
	}

	// Build command: python -c "<runner script>" <args...>
	cmdArgs := append([]string{"-c", runnerScript}, runnerArgs...)
	cmd := exec.CommandContext(ctx, pythonBin, cmdArgs...)

	// Set JAVA_HOME for embedded mode
	cmd.Env = os.Environ()
	if !isRemote && javaHome != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("JAVA_HOME=%s", javaHome))
	}

	// Process group for clean cleanup
	cmd.SysProcAttr = processGroupAttr()

	// Pipe user code to stdin
	cmd.Stdin = strings.NewReader(userCode)

	start := time.Now()

	if cfg.JSONMode {
		// JSON mode: capture stdout, forward stderr
		var stdoutBuf bytes.Buffer
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = cfg.Stderr

		err := cmd.Start()
		if err != nil {
			return output.ExitError, nil, fmt.Errorf("starting runner: %w", err)
		}

		// Forward SIGINT to child process group
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT)
		go func() {
			for range sigCh {
				if cmd.Process != nil {
					killProcessGroup(cmd.Process.Pid)
				}
			}
		}()
		defer func() { signal.Stop(sigCh); close(sigCh) }()

		waitErr := cmd.Wait()
		elapsed := time.Since(start).Seconds()

		// Check for timeout
		if ctx.Err() == context.DeadlineExceeded {
			if cmd.Process != nil {
				killProcessGroup(cmd.Process.Pid)
			}
			jsonResult := map[string]any{
				"exit_code":       output.ExitTimeout,
				"stdout":          "",
				"stderr":          "",
				"result_repr":     nil,
				"error":           fmt.Sprintf("Execution timed out after %d seconds", cfg.Timeout),
				"tables":          []any{},
				"version":         version,
				"java_home":       javaHome,
				"elapsed_seconds": elapsed,
			}
			return output.ExitTimeout, jsonResult, nil
		}

		// Parse runner's JSON output
		exitCode := exitCodeFromErr(waitErr)
		runnerJSON := stdoutBuf.String()

		var runnerResult map[string]any
		if err := json.Unmarshal([]byte(runnerJSON), &runnerResult); err != nil {
			// Runner didn't produce valid JSON; wrap what we have
			runnerResult = map[string]any{
				"exit_code":   exitCode,
				"stdout":      runnerJSON,
				"stderr":      "",
				"result_repr": nil,
				"error":       nil,
				"tables":      []any{},
			}
		}

		// Augment with Go-side info
		runnerResult["version"] = version
		runnerResult["java_home"] = javaHome
		runnerResult["elapsed_seconds"] = elapsed

		return exitCode, runnerResult, nil
	}

	// Normal mode: forward stdout/stderr directly
	cmd.Stdout = cfg.Stdout
	cmd.Stderr = cfg.Stderr

	err = cmd.Start()
	if err != nil {
		return output.ExitError, nil, fmt.Errorf("starting runner: %w", err)
	}

	// Forward SIGINT to child process group
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	go func() {
		for range sigCh {
			if cmd.Process != nil {
				killProcessGroup(cmd.Process.Pid)
			}
		}
	}()
	defer func() { signal.Stop(sigCh); close(sigCh) }()

	waitErr := cmd.Wait()

	// Check for timeout
	if ctx.Err() == context.DeadlineExceeded {
		if cmd.Process != nil {
			killProcessGroup(cmd.Process.Pid)
		}
		fmt.Fprintf(cfg.Stderr, "Error: Execution timed out after %d seconds\n", cfg.Timeout)
		return output.ExitTimeout, nil, nil
	}

	return exitCodeFromErr(waitErr), nil, nil
}

// readCode reads user code from -c flag, file, or stdin.
func readCode(cfg *ExecConfig) (string, error) {
	if cfg.Code != "" {
		return cfg.Code, nil
	}
	if cfg.ScriptPath == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("reading stdin: %w", err)
		}
		return string(data), nil
	}
	data, err := os.ReadFile(cfg.ScriptPath)
	if err != nil {
		return "", fmt.Errorf("reading script file %s: %w", cfg.ScriptPath, err)
	}
	return string(data), nil
}

// buildRunnerArgs constructs the CLI arguments for the runner script.
func buildRunnerArgs(cfg *ExecConfig, isRemote bool) []string {
	var args []string
	if isRemote {
		args = append(args, "--mode", "remote")
		args = append(args, "--host", cfg.Host)
	} else {
		args = append(args, "--mode", "embedded")
	}

	args = append(args, "--port", fmt.Sprintf("%d", cfg.Port))

	if cfg.JVMArgs != "" {
		args = append(args, fmt.Sprintf("--jvm-args=%s", cfg.JVMArgs))
	}

	if cfg.ShowTables {
		args = append(args, "--show-tables")
	}

	if cfg.ShowTableMeta {
		args = append(args, "--show-table-meta")
	}

	if cfg.JSONMode {
		args = append(args, "--output-json")
	}

	// Remote auth options
	if isRemote {
		if cfg.AuthType != "" {
			args = append(args, "--auth-type", cfg.AuthType)
		}
		if cfg.AuthToken != "" {
			args = append(args, "--auth-token", cfg.AuthToken)
		}
		if cfg.TLS {
			args = append(args, "--tls")
		}
		if cfg.TLSCACert != "" {
			args = append(args, "--tls-ca-cert", cfg.TLSCACert)
		}
		if cfg.TLSClientCert != "" {
			args = append(args, "--tls-client-cert", cfg.TLSClientCert)
		}
		if cfg.TLSClientKey != "" {
			args = append(args, "--tls-client-key", cfg.TLSClientKey)
		}
	}

	return args
}

// FindVenvPython locates the venv python binary for a given version.
func FindVenvPython(dhHome, version string) (string, error) {
	var pythonBin string
	if runtime.GOOS == "windows" {
		pythonBin = filepath.Join(dhHome, "versions", version, ".venv", "Scripts", "python.exe")
	} else {
		pythonBin = filepath.Join(dhHome, "versions", version, ".venv", "bin", "python")
	}
	if _, err := os.Stat(pythonBin); err != nil {
		return "", fmt.Errorf("venv python not found at %s (is version %s installed?)", pythonBin, version)
	}
	return pythonBin, nil
}

// EnsurePydeephaven checks if pydeephaven is installed in the venv, and installs it if missing.
func EnsurePydeephaven(pythonBin, version string, quiet bool, stderr io.Writer) error {
	// Check if pydeephaven is importable
	checkCmd := ExecCommand(pythonBin, "-c", "import pydeephaven")
	checkCmd.Stdout = nil
	checkCmd.Stderr = nil
	if err := checkCmd.Run(); err == nil {
		return nil // already installed
	}

	// Install it
	if !quiet && stderr != nil {
		fmt.Fprintln(stderr, "Installing pydeephaven...")
	}
	installCmd := ExecCommand("uv", "pip", "install", "--python", pythonBin, fmt.Sprintf("pydeephaven==%s", version))
	installCmd.Stderr = stderr
	if err := installCmd.Run(); err != nil {
		return fmt.Errorf("installing pydeephaven: %w", err)
	}
	return nil
}

// latestSnapshotVersion scans the VM snapshots directory and returns the
// latest version that has a complete snapshot. This allows --vm mode to
// work without an explicit version when a snapshot has been prepared.
func latestSnapshotVersion(dhHome string) (string, error) {
	snapshotDir := filepath.Join(dhHome, "vm", "snapshots")
	entries, err := os.ReadDir(snapshotDir)
	if err != nil {
		return "", err
	}
	var versions []string
	for _, e := range entries {
		if e.IsDir() {
			// Check that the snapshot is complete (has metadata.json)
			meta := filepath.Join(snapshotDir, e.Name(), "metadata.json")
			if _, err := os.Stat(meta); err == nil {
				versions = append(versions, e.Name())
			}
		}
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("no VM snapshots found in %s", snapshotDir)
	}
	sort.Strings(versions)
	return versions[len(versions)-1], nil
}

// exitCodeFromErr extracts the exit code from a process wait error.
func exitCodeFromErr(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return 1
}
