package cmd

import "github.com/spf13/cobra"

func addDiscoveryCommands(parent *cobra.Command) {
	parent.AddCommand(newListCmd())
	parent.AddCommand(newKillCmd())
}
