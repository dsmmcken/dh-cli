package cmd

import (
	"github.com/dsmmcken/dh-cli/src/internal/config"
	"github.com/dsmmcken/dh-cli/src/internal/java"
	"github.com/dsmmcken/dh-cli/src/internal/output"
	"github.com/dsmmcken/dh-cli/src/internal/tui"
	"github.com/dsmmcken/dh-cli/src/internal/versions"
	"github.com/spf13/cobra"

	tea "github.com/charmbracelet/bubbletea"
)

var nonInteractiveFlag bool

func addSetupCommand(parent *cobra.Command) {
	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: "Run setup wizard",
		Long:  "Walk through initial setup: Java detection, Deephaven version install, and configuration.",
		Args:  cobra.NoArgs,
		RunE:  runSetup,
	}

	setupCmd.Flags().BoolVar(&nonInteractiveFlag, "non-interactive", false, "Auto-detect Java, install latest Deephaven, output JSON result")

	parent.AddCommand(setupCmd)
}

func runSetup(cmd *cobra.Command, args []string) error {
	config.SetConfigDir(ConfigDir)
	dhgHome := config.DHGHome()

	if nonInteractiveFlag {
		return runNonInteractiveSetup(cmd, dhgHome)
	}

	// Interactive mode: launch wizard TUI
	p := tea.NewProgram(tui.NewApp(tui.WizardMode, dhgHome), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func runNonInteractiveSetup(cmd *cobra.Command, dhgHome string) error {
	// Detect Java
	javaInfo, err := java.Detect(dhgHome)
	if err != nil {
		javaInfo = &java.JavaInfo{Found: false}
	}

	// Install latest version
	latest, err := versions.FetchLatestVersion()
	if err != nil {
		if output.IsJSON() {
			return output.PrintJSON(cmd.OutOrStdout(), map[string]any{
				"java": javaInfo,
				"deephaven": map[string]any{
					"version":        "",
					"installed":      false,
					"set_as_default": false,
					"error":          err.Error(),
				},
			})
		}
		return err
	}

	cfg, _ := config.Load()
	plugins := cfg.Install.Plugins
	if len(plugins) == 0 {
		plugins = []string{
			"deephaven-plugin-ui",
			"deephaven-plugin-plotly-express",
		}
	}
	pythonVer := cfg.Install.PythonVersion
	if pythonVer == "" {
		pythonVer = "3.13"
	}

	installErr := versions.Install(dhgHome, latest, pythonVer, plugins, nil)
	installed := installErr == nil

	if installed {
		_ = config.Set("default_version", latest)
	}

	result := map[string]any{
		"java": javaInfo,
		"deephaven": map[string]any{
			"version":        latest,
			"installed":      installed,
			"set_as_default": installed,
		},
	}

	if output.IsJSON() {
		return output.PrintJSON(cmd.OutOrStdout(), result)
	}

	if installed {
		cmd.Printf("Setup complete. Deephaven %s installed and set as default.\n", latest)
	} else {
		cmd.Printf("Setup incomplete: %s\n", installErr)
	}
	return nil
}
