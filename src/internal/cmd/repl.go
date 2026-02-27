package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dsmmcken/dh-cli/src/internal/config"
	dhexec "github.com/dsmmcken/dh-cli/src/internal/exec"
	"github.com/dsmmcken/dh-cli/src/internal/java"
	"github.com/dsmmcken/dh-cli/src/internal/output"
	"github.com/dsmmcken/dh-cli/src/internal/repl"
	"github.com/spf13/cobra"
)

var (
	replPortFlag          int
	replJVMArgsFlag       string
	replVersionFlag       string
	replHostFlag          string
	replAuthTypeFlag      string
	replAuthTokenFlag     string
	replTLSFlag           bool
	replTLSCACertFlag     string
	replTLSClientCertFlag string
	replTLSClientKeyFlag  string
)

func addReplCommand(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "repl",
		Short: "Interactive Python REPL on a Deephaven server",
		Long: `Start an interactive REPL connected to a Deephaven server.

Provides a multi-line input area with a scrollable log view showing
stdout, stderr, errors, and result values.

Examples:
  dh repl                                    # Embedded mode
  dh repl --host localhost:10000             # Remote mode
  dh repl --port 8080                        # Custom port`,
		Args: cobra.NoArgs,
		RunE: runRepl,
	}

	flags := cmd.Flags()
	flags.IntVar(&replPortFlag, "port", 10000, "Server port")
	flags.StringVar(&replJVMArgsFlag, "jvm-args", "-Xmx4g -DAuthHandlers=io.deephaven.auth.AnonymousAuthenticationHandler", "JVM arguments (quoted string)")
	flags.StringVar(&replVersionFlag, "version", "", "Deephaven version to use")
	flags.StringVar(&replHostFlag, "host", "", "Remote server host (enables remote mode)")
	flags.StringVar(&replAuthTypeFlag, "auth-type", "", "Authentication type for remote connection")
	flags.StringVar(&replAuthTokenFlag, "auth-token", "", "Authentication token for remote connection")
	flags.BoolVar(&replTLSFlag, "tls", false, "Use TLS for remote connection")
	flags.StringVar(&replTLSCACertFlag, "tls-ca-cert", "", "Path to CA certificate for TLS")
	flags.StringVar(&replTLSClientCertFlag, "tls-client-cert", "", "Path to client certificate for TLS")
	flags.StringVar(&replTLSClientKeyFlag, "tls-client-key", "", "Path to client private key for TLS")

	parent.AddCommand(cmd)
}

func runRepl(cmd *cobra.Command, args []string) error {
	// Resolve version
	config.SetConfigDir(ConfigDir)
	dhHome := config.DHHome()
	envVersion := os.Getenv("DH_VERSION")

	version, err := config.ResolveVersion(replVersionFlag, envVersion)
	if err != nil {
		return fmt.Errorf("resolving version: %w", err)
	}

	// Find venv python
	pythonBin, err := dhexec.FindVenvPython(dhHome, version)
	if err != nil {
		return fmt.Errorf("finding venv python: %w", err)
	}

	// Ensure pydeephaven
	if err := dhexec.EnsurePydeephaven(pythonBin, version, output.IsQuiet(), cmd.ErrOrStderr()); err != nil {
		return fmt.Errorf("ensuring pydeephaven: %w", err)
	}

	// Detect Java for embedded mode
	var javaHome string
	isRemote := replHostFlag != ""
	if !isRemote {
		javaInfo, err := java.Detect(dhHome)
		if err != nil {
			return fmt.Errorf("detecting Java: %w", err)
		}
		if !javaInfo.Found {
			return fmt.Errorf("Java not found; install Java 17+ or set JAVA_HOME")
		}
		javaHome = javaInfo.Home
	}

	// Build session config
	cfg := repl.SessionConfig{
		Port:          replPortFlag,
		JVMArgs:       replJVMArgsFlag,
		Version:       version,
		Host:          replHostFlag,
		AuthType:      replAuthTypeFlag,
		AuthToken:     replAuthTokenFlag,
		TLS:           replTLSFlag,
		TLSCACert:     replTLSCACertFlag,
		TLSClientCert: replTLSClientCertFlag,
		TLSClientKey:  replTLSClientKeyFlag,
		PythonBin:     pythonBin,
		JavaHome:      javaHome,
		DHHome:       dhHome,
	}

	// Create and run the TUI
	model := repl.NewREPLModel(cfg)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err = p.Run()
	return err
}
