package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dsmmcken/dh-cli/src/internal/java"
	"github.com/dsmmcken/dh-cli/src/internal/output"
	"github.com/spf13/cobra"
)

func addJavaCommands(parent *cobra.Command) {
	javaCmd := &cobra.Command{
		Use:   "java",
		Short: "Show Java status",
		Long:  "Detect and display information about the Java installation used by Deephaven.",
		Args:  cobra.NoArgs,
		RunE:  runJavaStatus,
	}

	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Install Eclipse Temurin JDK",
		Long:  "Download and install Eclipse Temurin JDK for use with Deephaven.",
		Args:  cobra.NoArgs,
		RunE:  runJavaInstall,
	}

	installCmd.Flags().IntVar(&jdkVersionFlag, "jdk-version", 21, "JDK major version to install")
	installCmd.Flags().BoolVar(&forceFlag, "force", false, "Force reinstall even if already present")

	javaCmd.AddCommand(installCmd)
	parent.AddCommand(javaCmd)
}

var (
	jdkVersionFlag int
	forceFlag      bool
)

func getDhgHome() string {
	if ConfigDir != "" {
		return ConfigDir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".dhg")
	}
	return filepath.Join(home, ".dhg")
}

func runJavaStatus(cmd *cobra.Command, args []string) error {
	dhgHome := getDhgHome()
	info, err := java.Detect(dhgHome)
	if err != nil {
		if output.IsJSON() {
			return output.PrintError(cmd.ErrOrStderr(), "java_detect_error", err.Error())
		}
		return fmt.Errorf("failed to detect Java: %w", err)
	}

	if output.IsJSON() {
		return output.PrintJSON(cmd.OutOrStdout(), info)
	}

	if !info.Found {
		fmt.Fprintln(cmd.OutOrStdout(), "Java: not found")
		fmt.Fprintln(cmd.OutOrStdout(), "Run 'dhg java install' to install Eclipse Temurin JDK.")
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Java: %s\n", info.Version)
	fmt.Fprintf(cmd.OutOrStdout(), "Path: %s\n", info.Path)
	fmt.Fprintf(cmd.OutOrStdout(), "Home: %s\n", info.Home)
	fmt.Fprintf(cmd.OutOrStdout(), "Source: %s\n", info.Source)

	if !java.MeetsMinimum(info.Version, java.MinimumVersion) {
		fmt.Fprintf(cmd.OutOrStdout(), "\nWarning: Java %s does not meet minimum version %d.\n", info.Version, java.MinimumVersion)
		fmt.Fprintln(cmd.OutOrStdout(), "Run 'dhg java install' to install a compatible version.")
	}

	return nil
}

func runJavaInstall(cmd *cobra.Command, args []string) error {
	dhgHome := getDhgHome()
	info, err := java.Install(dhgHome, jdkVersionFlag, forceFlag)
	if err != nil {
		if output.IsJSON() {
			return output.PrintError(cmd.ErrOrStderr(), "java_install_error", err.Error())
		}
		return err
	}

	if output.IsJSON() {
		return output.PrintJSON(cmd.OutOrStdout(), info)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Installed Java %s at %s\n", info.Version, info.Home)
	return nil
}
