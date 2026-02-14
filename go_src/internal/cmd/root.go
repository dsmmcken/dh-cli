package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dsmmcken/dh-cli/go_src/internal/config"
	"github.com/dsmmcken/dh-cli/go_src/internal/output"
	"github.com/dsmmcken/dh-cli/go_src/internal/tui"
	"github.com/dsmmcken/dh-cli/go_src/internal/versions"
	"github.com/spf13/cobra"
)

var Version = "dev"

var (
	jsonFlag    bool
	verboseFlag bool
	quietFlag   bool
	noColorFlag bool
	ConfigDir   string
)

func NewRootCmd() *cobra.Command {
	cmd := newRootCmd()
	addConfigCommands(cmd)
	addJavaCommands(cmd)
	addDiscoveryCommands(cmd)
	addVersionCommands(cmd)
	addDoctorCommand(cmd)
	addSetupCommand(cmd)
	addExecCommand(cmd)
	addServeCommand(cmd)
	return cmd
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "dhg",
		Short: "Deephaven CLI tool",
		Long:  "dhg — CLI tool for managing Deephaven installations, servers, and configuration.",
		Version: fmt.Sprintf("dhg v%s", Version),
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if verboseFlag && quietFlag {
				return fmt.Errorf("--verbose and --quiet are mutually exclusive")
			}
			if jsonFlag {
				quietFlag = true
			}
			output.SetFlags(jsonFlag, quietFlag, verboseFlag)
			return nil
		},
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fi, _ := os.Stdin.Stat()
			isTTY := (fi.Mode() & os.ModeCharDevice) != 0
			if !isTTY {
				return cmd.Help()
			}

			config.SetConfigDir(ConfigDir)
			dhgHome := config.DHGHome()

			// Determine mode based on whether versions are installed
			installed, _ := versions.ListInstalled(dhgHome)
			mode := tui.WizardMode
			if len(installed) > 0 {
				mode = tui.MenuMode
			}

			p := tea.NewProgram(tui.NewApp(mode, dhgHome), tea.WithAltScreen())
			_, err := p.Run()
			return err
		},
	}

	rootCmd.SetVersionTemplate("{{.Version}}\n")

	pflags := rootCmd.PersistentFlags()
	pflags.BoolVarP(&jsonFlag, "json", "j", false, "Output as JSON")
	pflags.BoolVarP(&verboseFlag, "verbose", "v", false, "Extra detail to stderr")
	pflags.BoolVarP(&quietFlag, "quiet", "q", false, "Suppress non-essential output")
	pflags.BoolVar(&noColorFlag, "no-color", false, "Disable ANSI colors")
	pflags.StringVar(&ConfigDir, "config-dir", "", "Override config directory (default: ~/.dhg)")

	// Environment variable bindings
	if v := os.Getenv("DHG_HOME"); v != "" && ConfigDir == "" {
		ConfigDir = v
	}
	if os.Getenv("NO_COLOR") != "" {
		noColorFlag = true
	}
	if os.Getenv("DHG_JSON") == "1" {
		jsonFlag = true
	}

	return rootCmd
}

func Execute() error {
	cmd := NewRootCmd()
	return cmd.Execute()
}
