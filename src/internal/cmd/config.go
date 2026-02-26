package cmd

import (
	"fmt"

	"github.com/dsmmcken/dh-cli/src/internal/config"
	"github.com/dsmmcken/dh-cli/src/internal/output"
	"github.com/spf13/cobra"
)

func addConfigCommands(rootCmd *cobra.Command) {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage dhg configuration",
		Long:  "Show, get, and set values in the dhg config file (~/.dhg/config.toml).",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			config.SetConfigDir(ConfigDir)
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if output.IsJSON() {
				return output.PrintJSON(cmd.OutOrStdout(), cfg)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Config file: %s\n", config.ConfigPath())
			fmt.Fprintf(cmd.OutOrStdout(), "default_version = %s\n", cfg.DefaultVersion)
			fmt.Fprintf(cmd.OutOrStdout(), "install.python_version = %s\n", cfg.Install.PythonVersion)
			fmt.Fprintf(cmd.OutOrStdout(), "install.plugins = %v\n", cfg.Install.Plugins)
			return nil
		},
	}

	configGetCmd := &cobra.Command{
		Use:   "get <KEY>",
		Short: "Get a config value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			config.SetConfigDir(ConfigDir)
			val, err := config.Get(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), val)
			return nil
		},
	}

	configSetCmd := &cobra.Command{
		Use:   "set <KEY> <VALUE>",
		Short: "Set a config value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			config.SetConfigDir(ConfigDir)
			if err := config.Set(args[0], args[1]); err != nil {
				return err
			}
			if !output.IsQuiet() {
				fmt.Fprintf(cmd.OutOrStdout(), "Set %s = %s\n", args[0], args[1])
			}
			return nil
		},
	}

	configPathCmd := &cobra.Command{
		Use:   "path",
		Short: "Print config file path",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			config.SetConfigDir(ConfigDir)
			fmt.Fprintln(cmd.OutOrStdout(), config.ConfigPath())
			return nil
		},
	}

	configCmd.AddCommand(configGetCmd, configSetCmd, configPathCmd)
	rootCmd.AddCommand(configCmd)
}
