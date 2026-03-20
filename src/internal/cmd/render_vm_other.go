//go:build !linux

package cmd

import (
	"fmt"
	"os"

	"github.com/dsmmcken/dh-cli/src/internal/output"
	"github.com/spf13/cobra"
)

func runRenderVM(cmd *cobra.Command, args []string, diagnose bool) error {
	fmt.Fprintln(cmd.ErrOrStderr(), "Error: --vm flag requires Linux with KVM support")
	os.Exit(output.ExitError)
	return nil
}
