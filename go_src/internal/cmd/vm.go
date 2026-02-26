package cmd

import (
	"fmt"
	"os"

	"github.com/dsmmcken/dh-cli/go_src/internal/config"
	"github.com/dsmmcken/dh-cli/go_src/internal/output"
	"github.com/dsmmcken/dh-cli/go_src/internal/versions"
	"github.com/dsmmcken/dh-cli/go_src/internal/vm"
	"github.com/spf13/cobra"
)

var vmVersionFlag string

func addVMCommands(parent *cobra.Command) {
	vmCmd := &cobra.Command{
		Use:   "vm",
		Short: "Manage Firecracker microVMs for fast execution (experimental, Linux only)",
		Long: `Manage Firecracker microVMs with snapshotted Deephaven servers.

This is an experimental feature that uses Firecracker microVMs to achieve
near-instant Deephaven server startup via snapshot restore.

Subcommands:
  prepare  Build rootfs and create snapshot for a Deephaven version
  status   Show snapshot and prerequisite status
  clean    Remove VM artifacts (rootfs, snapshots, run state)`,
	}

	// dhg vm prepare
	prepareCmd := &cobra.Command{
		Use:   "prepare",
		Short: "Build rootfs and create VM snapshot",
		Long: `Prepare a Firecracker VM snapshot for fast Deephaven execution.

This command:
  1. Downloads Firecracker binary and kernel (if needed)
  2. Builds an ext4 rootfs image with JVM + Deephaven (via Docker)
  3. Boots a fresh Firecracker VM from the rootfs
  4. Waits for the Deephaven server to fully initialize
  5. Pauses the VM and creates a memory+state snapshot

First run takes 2-5 minutes. Subsequent runs for the same version
skip the rootfs build.

Requirements: Linux, /dev/kvm access, Docker.`,
		RunE: runVMPrepare,
	}
	prepareCmd.Flags().StringVar(&vmVersionFlag, "version", "", "Deephaven version (default: resolved version)")

	// dhg vm status
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show VM prerequisites and snapshot status",
		RunE:  runVMStatus,
	}

	// dhg vm clean
	cleanCmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove VM artifacts",
		Long:  "Remove rootfs images, snapshots, and runtime state from ~/.dhg/vm/.",
		RunE:  runVMClean,
	}
	cleanCmd.Flags().StringVar(&vmVersionFlag, "version", "", "Clean only this version (default: all)")

	vmCmd.AddCommand(prepareCmd, statusCmd, cleanCmd)
	parent.AddCommand(vmCmd)
}

func runVMPrepare(cmd *cobra.Command, args []string) error {
	config.SetConfigDir(ConfigDir)
	dhgHome := config.DHGHome()

	version, err := config.ResolveVersion(vmVersionFlag, os.Getenv("DHG_VERSION"))
	if err != nil {
		// No version configured anywhere — fetch latest from PyPI
		fmt.Fprintf(cmd.ErrOrStderr(), "No version specified, fetching latest from PyPI...\n")
		latest, pypiErr := versions.FetchLatestVersion()
		if pypiErr != nil {
			return fmt.Errorf("resolving version: %w (PyPI fallback also failed: %v)", err, pypiErr)
		}
		version = latest
	}

	paths := vm.NewVMPaths(dhgHome)

	// Step 1: Download firecracker binary
	fmt.Fprintf(cmd.ErrOrStderr(), "Ensuring Firecracker binary...\n")
	if err := vm.EnsureFirecracker(paths, cmd.ErrOrStderr()); err != nil {
		return fmt.Errorf("ensuring firecracker: %w", err)
	}

	// Step 2: Download kernel
	fmt.Fprintf(cmd.ErrOrStderr(), "Ensuring kernel...\n")
	if err := vm.EnsureKernel(paths, cmd.ErrOrStderr()); err != nil {
		return fmt.Errorf("ensuring kernel: %w", err)
	}

	// Step 3: Check prerequisites and auto-fix what we can
	prereqErrs := vm.CheckPrerequisites(paths)
	if len(prereqErrs) > 0 {
		// If the only issues are auto-fixable, try to fix them
		if vm.HasNonAutoFixErrors(prereqErrs) {
			fmt.Fprintf(cmd.ErrOrStderr(), "\n%s", vm.FormatPrereqErrors(prereqErrs))
			return fmt.Errorf("prerequisites not met (cannot auto-fix)")
		}

		// Try to fix KVM access automatically
		if !vm.KVMAccessible() {
			fmt.Fprintf(cmd.ErrOrStderr(), "/dev/kvm is not accessible. Fixing...\n")
			if err := vm.FixKVMAccess(cmd.ErrOrStderr()); err != nil {
				return fmt.Errorf("fixing KVM access: %w", err)
			}
		}

		// Re-check after fixes
		prereqErrs = vm.CheckPrerequisites(paths)
		if len(prereqErrs) > 0 {
			fmt.Fprintf(cmd.ErrOrStderr(), "\n%s", vm.FormatPrereqErrors(prereqErrs))
			return fmt.Errorf("prerequisites not met")
		}
	}

	// Step 4: Build rootfs (if not exists)
	rootfsPath := paths.RootfsForVersion(version)
	if _, err := os.Stat(rootfsPath); os.IsNotExist(err) {
		fmt.Fprintf(cmd.ErrOrStderr(), "Building rootfs for version %s (this may take a few minutes)...\n", version)
		if err := vm.EnsureRootfs(paths, version, cmd.ErrOrStderr()); err != nil {
			return fmt.Errorf("building rootfs: %w", err)
		}
	} else {
		fmt.Fprintf(cmd.ErrOrStderr(), "Rootfs exists: %s\n", rootfsPath)
	}

	// Step 5: Boot VM and create snapshot
	fmt.Fprintf(cmd.ErrOrStderr(), "Booting VM and creating snapshot for version %s...\n", version)
	vmCfg := &vm.VMConfig{
		DHGHome: dhgHome,
		Version: version,
		Verbose: output.IsVerbose(),
	}
	if err := vm.BootAndSnapshot(cmd.Context(), vmCfg, paths, cmd.ErrOrStderr()); err != nil {
		return fmt.Errorf("creating snapshot: %w", err)
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "Snapshot ready for version %s. Use 'dhg exec --vm' for fast execution.\n", version)

	if output.IsJSON() {
		return output.PrintJSON(cmd.OutOrStdout(), map[string]any{
			"version":      version,
			"snapshot_dir": paths.SnapshotDirForVersion(version),
			"status":       "ready",
		})
	}

	return nil
}

func runVMStatus(cmd *cobra.Command, args []string) error {
	config.SetConfigDir(ConfigDir)
	dhgHome := config.DHGHome()
	paths := vm.NewVMPaths(dhgHome)

	// Check prerequisites
	fmt.Fprintln(cmd.OutOrStdout(), "Prerequisites:")
	prereqErrs := vm.CheckPrerequisites(paths)
	if len(prereqErrs) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "  All prerequisites met.")
	} else {
		fmt.Fprint(cmd.OutOrStdout(), vm.FormatPrereqErrors(prereqErrs))
	}

	// List snapshots
	fmt.Fprintln(cmd.OutOrStdout(), "\nSnapshots:")
	entries, err := os.ReadDir(paths.SnapshotDir)
	if err != nil || len(entries) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "  No snapshots found.")
	} else {
		for _, e := range entries {
			if e.IsDir() {
				ver := e.Name()
				if err := vm.CheckSnapshot(paths, ver); err == nil {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s: ready\n", ver)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s: incomplete\n", ver)
				}
			}
		}
	}

	if output.IsJSON() {
		snapshots := []map[string]any{}
		if entries, err := os.ReadDir(paths.SnapshotDir); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					status := "ready"
					if err := vm.CheckSnapshot(paths, e.Name()); err != nil {
						status = "incomplete"
					}
					snapshots = append(snapshots, map[string]any{
						"version": e.Name(),
						"status":  status,
					})
				}
			}
		}
		return output.PrintJSON(cmd.OutOrStdout(), map[string]any{
			"prerequisites_ok": len(prereqErrs) == 0,
			"snapshots":        snapshots,
		})
	}

	return nil
}

func runVMClean(cmd *cobra.Command, args []string) error {
	config.SetConfigDir(ConfigDir)
	dhgHome := config.DHGHome()
	paths := vm.NewVMPaths(dhgHome)

	if vmVersionFlag != "" {
		// Clean specific version
		snapDir := paths.SnapshotDirForVersion(vmVersionFlag)
		rootfs := paths.RootfsForVersion(vmVersionFlag)
		os.RemoveAll(snapDir)
		os.Remove(rootfs)
		fmt.Fprintf(cmd.ErrOrStderr(), "Cleaned VM artifacts for version %s\n", vmVersionFlag)
	} else {
		// Clean everything
		os.RemoveAll(paths.Base)
		fmt.Fprintf(cmd.ErrOrStderr(), "Cleaned all VM artifacts from %s\n", paths.Base)
	}
	return nil
}
