//go:build !linux

package cmd

import "github.com/spf13/cobra"

func addVMCommands(parent *cobra.Command) {
	// VM commands are Linux-only; no-op on other platforms
}
