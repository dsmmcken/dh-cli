package cmd

import (
	"fmt"
	"text/tabwriter"

	"github.com/dsmmcken/dh-cli/src/internal/config"
	"github.com/dsmmcken/dh-cli/src/internal/output"
	"github.com/dsmmcken/dh-cli/src/internal/versions"
	"github.com/spf13/cobra"
)

var (
	versionsRemoteFlag bool
	versionsLimitFlag  int
	versionsAllFlag    bool
)

func addVersionCommands(parent *cobra.Command) {
	versionsCmd := &cobra.Command{
		Use:   "versions",
		Short: "List installed Deephaven versions",
		Long:  "Show installed Deephaven versions and optionally query PyPI for available versions.",
		Args:  cobra.NoArgs,
		RunE:  runVersions,
	}

	versionsCmd.Flags().BoolVar(&versionsRemoteFlag, "remote", false, "Also show available remote versions from PyPI")
	versionsCmd.Flags().IntVar(&versionsLimitFlag, "limit", 20, "Limit number of remote versions shown")
	versionsCmd.Flags().BoolVar(&versionsAllFlag, "all", false, "Show all remote versions (no limit)")

	parent.AddCommand(versionsCmd)
	parent.AddCommand(newInstallCmd())
	parent.AddCommand(newUninstallCmd())
	parent.AddCommand(newUseCmd())
}

func runVersions(cmd *cobra.Command, args []string) error {
	config.SetConfigDir(ConfigDir)
	dhHome := config.DHHome()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	installed, err := versions.ListInstalled(dhHome)
	if err != nil {
		return err
	}
	if installed == nil {
		installed = []versions.InstalledVersion{}
	}

	// Mark the default version
	for i := range installed {
		if installed[i].Version == cfg.DefaultVersion {
			installed[i].IsDefault = true
		}
	}

	if output.IsJSON() {
		result := map[string]any{
			"installed":       installed,
			"default_version": cfg.DefaultVersion,
		}

		if versionsRemoteFlag || versionsAllFlag {
			limit := versionsLimitFlag
			if versionsAllFlag {
				limit = 0
			}
			remote, err := versions.FetchRemoteVersions(limit)
			if err != nil {
				return err
			}
			result["remote"] = remote
		}

		return output.PrintJSON(cmd.OutOrStdout(), result)
	}

	// Human output
	if len(installed) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No Deephaven versions installed.")
		fmt.Fprintln(cmd.OutOrStdout(), "Run 'dh install' to install the latest version.")
	} else {
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "VERSION\tDEFAULT\tINSTALLED")
		for _, v := range installed {
			def := ""
			if v.IsDefault {
				def = "*"
			}
			date := ""
			if !v.InstalledAt.IsZero() {
				date = v.InstalledAt.Format("2006-01-02")
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", v.Version, def, date)
		}
		w.Flush()
	}

	if versionsRemoteFlag || versionsAllFlag {
		limit := versionsLimitFlag
		if versionsAllFlag {
			limit = 0
		}
		remote, err := versions.FetchRemoteVersions(limit)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "\nAvailable on PyPI (%d versions):\n", len(remote))
		for _, v := range remote {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", v)
		}
	}

	return nil
}
