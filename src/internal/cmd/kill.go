package cmd

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/dsmmcken/dh-cli/src/internal/discovery"
	"github.com/dsmmcken/dh-cli/src/internal/output"
	"github.com/spf13/cobra"
)

func newKillCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "kill <PORT>",
		Short: "Stop a running Deephaven server",
		Long:  "Stop a Deephaven server running on the specified port.",
		Args:  cobra.ExactArgs(1),
		RunE:  runKill,
	}
}

func runKill(cmd *cobra.Command, args []string) error {
	port, err := strconv.Atoi(args[0])
	if err != nil {
		msg := fmt.Sprintf("invalid port: %s", args[0])
		if output.IsJSON() {
			output.PrintError(os.Stderr, "invalid_port", msg)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %s\n", msg)
		}
		os.Exit(output.ExitError)
	}

	err = discovery.Kill(port)
	if err != nil {
		var notFound *discovery.NotFoundError
		if errors.As(err, &notFound) {
			if output.IsJSON() {
				output.PrintError(os.Stderr, "server_not_found", notFound.Error())
			} else {
				fmt.Fprintf(os.Stderr, "Error: %s\n", notFound.Error())
			}
			os.Exit(output.ExitNotFound)
		}
		if output.IsJSON() {
			output.PrintError(os.Stderr, "kill_error", err.Error())
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		os.Exit(output.ExitError)
	}

	if output.IsJSON() {
		return output.PrintJSON(cmd.OutOrStdout(), map[string]any{
			"status": "stopped",
			"port":   port,
		})
	}

	if !output.IsQuiet() {
		fmt.Fprintf(cmd.OutOrStdout(), "Server on port %d stopped.\n", port)
	}
	return nil
}
