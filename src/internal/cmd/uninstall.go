package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/dsmmcken/dh-cli/src/internal/config"
	"github.com/dsmmcken/dh-cli/src/internal/output"
	"github.com/dsmmcken/dh-cli/src/internal/versions"
	"github.com/spf13/cobra"
)

var uninstallForceFlag bool

func newUninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall <VERSION>",
		Short: "Uninstall a Deephaven version",
		Long:  "Remove an installed Deephaven version and its virtual environment.",
		Args:  cobra.ExactArgs(1),
		RunE:  runUninstall,
	}

	cmd.Flags().BoolVar(&uninstallForceFlag, "force", false, "Skip confirmation prompt")

	return cmd
}

func runUninstall(cmd *cobra.Command, args []string) error {
	config.SetConfigDir(ConfigDir)
	dhHome := config.DHHome()
	version := args[0]

	// Confirm unless --force or --json
	if !uninstallForceFlag && !output.IsJSON() {
		fmt.Fprintf(cmd.ErrOrStderr(), "Uninstall Deephaven %s? [y/N] ", version)
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(cmd.ErrOrStderr(), "Cancelled.")
			return nil
		}
	}

	if err := versions.Uninstall(dhHome, version); err != nil {
		if output.IsJSON() {
			return output.PrintError(cmd.ErrOrStderr(), "uninstall_error", err.Error())
		}
		return err
	}

	// If uninstalled version was the default, update config
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.DefaultVersion == version {
		// Find the latest remaining version
		installed, err := versions.ListInstalled(dhHome)
		if err != nil {
			return err
		}
		if len(installed) > 0 {
			cfg.DefaultVersion = installed[0].Version
		} else {
			cfg.DefaultVersion = ""
		}
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
	}

	if output.IsJSON() {
		return output.PrintJSON(cmd.OutOrStdout(), map[string]any{
			"version": version,
			"status":  "uninstalled",
		})
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Uninstalled Deephaven %s\n", version)
	return nil
}
