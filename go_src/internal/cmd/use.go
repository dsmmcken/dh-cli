package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dsmmcken/dh-cli/go_src/internal/config"
	"github.com/dsmmcken/dh-cli/go_src/internal/output"
	"github.com/dsmmcken/dh-cli/go_src/internal/versions"
	"github.com/spf13/cobra"
)

var useLocalFlag bool

func newUseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "use <VERSION>",
		Short: "Set the default Deephaven version",
		Long:  "Set a specific installed Deephaven version as the default. Use --local to set for the current directory only.",
		Args:  cobra.ExactArgs(1),
		RunE:  runUse,
	}

	cmd.Flags().BoolVar(&useLocalFlag, "local", false, "Write .dhgrc in the current directory instead of updating global config")

	return cmd
}

func runUse(cmd *cobra.Command, args []string) error {
	config.SetConfigDir(ConfigDir)
	dhgHome := config.DHGHome()
	version := args[0]

	// Validate version is installed
	installed, err := versions.ListInstalled(dhgHome)
	if err != nil {
		return err
	}

	found := false
	for _, v := range installed {
		if v.Version == version {
			found = true
			break
		}
	}
	if !found {
		if output.IsJSON() {
			return output.PrintError(cmd.ErrOrStderr(), "not_installed", fmt.Sprintf("version %s is not installed", version))
		}
		return fmt.Errorf("version %s is not installed; run 'dhg install %s' first", version, version)
	}

	scope := "global"
	configPath := config.ConfigPath()

	if useLocalFlag {
		scope = "local"
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting current directory: %w", err)
		}
		configPath = filepath.Join(cwd, ".dhgrc")
		if err := config.WriteDHGRC(cwd, version); err != nil {
			return err
		}
	} else {
		if err := config.Set("default_version", version); err != nil {
			return err
		}
	}

	if output.IsJSON() {
		return output.PrintJSON(cmd.OutOrStdout(), map[string]any{
			"version":     version,
			"scope":       scope,
			"config_path": configPath,
		})
	}

	if useLocalFlag {
		fmt.Fprintf(cmd.OutOrStdout(), "Set local version to %s (wrote .dhgrc)\n", version)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Set default version to %s\n", version)
	}

	return nil
}
