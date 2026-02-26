package cmd

import (
	"fmt"
	"time"

	"github.com/dsmmcken/dh-cli/src/internal/config"
	"github.com/dsmmcken/dh-cli/src/internal/output"
	"github.com/dsmmcken/dh-cli/src/internal/versions"
	"github.com/spf13/cobra"
)

var (
	installNoPluginsFlag bool
	installPythonFlag    string
)

func newInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install [VERSION]",
		Short: "Install a Deephaven version",
		Long:  "Install a Deephaven version. If VERSION is omitted, installs the latest version from PyPI.",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runInstall,
	}

	cmd.Flags().BoolVar(&installNoPluginsFlag, "no-plugins", false, "Skip installing default plugins")
	cmd.Flags().StringVar(&installPythonFlag, "python", "3.13", "Python version for the virtual environment")

	return cmd
}

func runInstall(cmd *cobra.Command, args []string) error {
	config.SetConfigDir(ConfigDir)
	dhgHome := config.DHGHome()

	version := ""
	if len(args) > 0 {
		version = args[0]
	}

	// Resolve version from PyPI if not specified
	if version == "" {
		if !output.IsQuiet() {
			fmt.Fprintln(cmd.ErrOrStderr(), "Fetching latest version from PyPI...")
		}
		latest, err := versions.FetchLatestVersion()
		if err != nil {
			if output.IsJSON() {
				return output.PrintError(cmd.ErrOrStderr(), "pypi_error", err.Error())
			}
			return fmt.Errorf("failed to fetch latest version: %w", err)
		}
		version = latest
	}

	// Load config for plugins
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	var plugins []string
	if !installNoPluginsFlag && len(cfg.Install.Plugins) > 0 {
		plugins = cfg.Install.Plugins
	}

	pythonVer := installPythonFlag
	if pythonVer == "3.13" && cfg.Install.PythonVersion != "" {
		pythonVer = cfg.Install.PythonVersion
	}

	start := time.Now()

	onProgress := func(msg string) {
		if !output.IsQuiet() {
			fmt.Fprintln(cmd.ErrOrStderr(), msg)
		}
	}

	if err := versions.Install(dhgHome, version, pythonVer, plugins, onProgress); err != nil {
		if output.IsJSON() {
			return output.PrintError(cmd.ErrOrStderr(), "install_error", err.Error())
		}
		return err
	}

	// If first installed version or no default, set as default
	setAsDefault := false
	if cfg.DefaultVersion == "" {
		cfg.DefaultVersion = version
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
		setAsDefault = true
	}

	elapsed := time.Since(start).Seconds()

	if output.IsJSON() {
		return output.PrintJSON(cmd.OutOrStdout(), map[string]any{
			"version":         version,
			"status":          "installed",
			"path":            fmt.Sprintf("%s/versions/%s", dhgHome, version),
			"set_as_default":  setAsDefault,
			"elapsed_seconds": elapsed,
		})
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Installed Deephaven %s\n", version)
	if setAsDefault {
		fmt.Fprintf(cmd.OutOrStdout(), "Set as default version.\n")
	}

	return nil
}
