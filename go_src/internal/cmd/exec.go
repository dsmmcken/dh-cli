package cmd

import (
	"fmt"
	"os"

	dhexec "github.com/dsmmcken/dh-cli/go_src/internal/exec"
	"github.com/dsmmcken/dh-cli/go_src/internal/output"
	"github.com/spf13/cobra"
)

var (
	execCodeFlag          string
	execPortFlag          int
	execJVMArgsFlag       string
	execTimeoutFlag       int
	execNoShowTablesFlag  bool
	execNoTableMetaFlag   bool
	execVersionFlag       string
	execHostFlag          string
	execAuthTypeFlag      string
	execAuthTokenFlag     string
	execTLSFlag           bool
	execTLSCACertFlag     string
	execTLSClientCertFlag string
	execTLSClientKeyFlag  string
	execVMFlag            bool
)

func addExecCommand(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "exec [SCRIPT]",
		Short: "Execute Python code on a Deephaven server",
		Long: `Execute Python code on a Deephaven server in batch mode.

Code can be provided via -c flag, a script file, or stdin (use - for stdin).

Examples:
  dhg exec -c "print('hello')"
  dhg exec script.py
  echo "print('hi')" | dhg exec -
  dhg exec -c "from deephaven import empty_table; t = empty_table(5)"
  dhg exec -c "print('remote')" --host remote.example.com`,
		Args:              cobra.MaximumNArgs(1),
		DisableFlagParsing: false,
		RunE:              runExec,
	}

	flags := cmd.Flags()
	flags.StringVarP(&execCodeFlag, "code", "c", "", "Python code to execute")
	flags.IntVar(&execPortFlag, "port", 10000, "Server port")
	flags.StringVar(&execJVMArgsFlag, "jvm-args", "-Xmx4g", "JVM arguments (quoted string)")
	flags.IntVar(&execTimeoutFlag, "timeout", 0, "Execution timeout in seconds (0 = no timeout)")
	flags.BoolVar(&execNoShowTablesFlag, "no-show-tables", false, "Do not show table previews")
	flags.BoolVar(&execNoTableMetaFlag, "no-table-meta", false, "Do not show column types and row counts")
	flags.StringVar(&execVersionFlag, "version", "", "Deephaven version to use")
	flags.StringVar(&execHostFlag, "host", "", "Remote server host (enables remote mode)")
	flags.StringVar(&execAuthTypeFlag, "auth-type", "", "Authentication type for remote connection")
	flags.StringVar(&execAuthTokenFlag, "auth-token", "", "Authentication token for remote connection")
	flags.BoolVar(&execTLSFlag, "tls", false, "Use TLS for remote connection")
	flags.StringVar(&execTLSCACertFlag, "tls-ca-cert", "", "Path to CA certificate for TLS")
	flags.StringVar(&execTLSClientCertFlag, "tls-client-cert", "", "Path to client certificate for TLS")
	flags.StringVar(&execTLSClientKeyFlag, "tls-client-key", "", "Path to client private key for TLS")
	flags.BoolVar(&execVMFlag, "vm", false, "Execute in a Firecracker microVM (experimental, Linux only)")

	parent.AddCommand(cmd)
}

func runExec(cmd *cobra.Command, args []string) error {
	cfg := &dhexec.ExecConfig{
		Code:          execCodeFlag,
		Port:          execPortFlag,
		JVMArgs:       execJVMArgsFlag,
		Timeout:       execTimeoutFlag,
		ShowTables:    !execNoShowTablesFlag,
		ShowTableMeta: !execNoTableMetaFlag,
		JSONMode:      output.IsJSON(),
		Verbose:       output.IsVerbose(),
		Quiet:         output.IsQuiet(),
		Version:       execVersionFlag,
		Host:          execHostFlag,
		AuthType:      execAuthTypeFlag,
		AuthToken:     execAuthTokenFlag,
		TLS:           execTLSFlag,
		TLSCACert:     execTLSCACertFlag,
		TLSClientCert: execTLSClientCertFlag,
		TLSClientKey:  execTLSClientKeyFlag,
		VMMode:        execVMFlag,
		ConfigDir:     ConfigDir,
		Stderr:        cmd.ErrOrStderr(),
		Stdout:        cmd.OutOrStdout(),
	}

	// Positional arg is a script path
	if len(args) > 0 {
		cfg.ScriptPath = args[0]
	}

	exitCode, jsonResult, err := dhexec.Run(cfg)
	if err != nil {
		if output.IsJSON() {
			_ = output.PrintError(cmd.ErrOrStderr(), "exec_error", err.Error())
		} else {
			fmt.Fprintln(cmd.ErrOrStderr(), "Error:", err)
		}
		os.Exit(exitCode)
	}

	if jsonResult != nil {
		if err := output.PrintJSON(cmd.OutOrStdout(), jsonResult); err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), "Error writing JSON:", err)
			os.Exit(1)
		}
	}

	// Exit with the child's exit code
	if exitCode != 0 {
		os.Exit(exitCode)
	}

	return nil
}
