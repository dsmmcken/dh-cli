package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/dsmmcken/dh-cli/src/internal/discovery"
	"github.com/dsmmcken/dh-cli/src/internal/output"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List running Deephaven servers",
		Long:  "Discover and list all running Deephaven servers (processes and Docker containers).",
		Args:  cobra.NoArgs,
		RunE:  runList,
	}
}

func runList(cmd *cobra.Command, args []string) error {
	servers, err := discovery.Discover()
	if err != nil {
		if output.IsJSON() {
			output.PrintError(os.Stderr, "discovery_error", err.Error())
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		os.Exit(output.ExitError)
	}

	if output.IsJSON() {
		return output.PrintJSON(cmd.OutOrStdout(), map[string]any{
			"servers": servers,
		})
	}

	if len(servers) == 0 {
		if !output.IsQuiet() {
			fmt.Fprintln(cmd.OutOrStdout(), "No running Deephaven servers found.")
		}
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "PORT\tPID\tSOURCE\tSCRIPT")
	for _, s := range servers {
		pid := ""
		if s.PID > 0 {
			pid = fmt.Sprintf("%d", s.PID)
		}
		script := s.Script
		if s.ContainerID != "" {
			script = fmt.Sprintf("%s (%s)", s.Script, s.ContainerID[:minLen(12, len(s.ContainerID))])
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", s.Port, pid, s.Source, script)
	}
	return w.Flush()
}

func minLen(a, b int) int {
	if a < b {
		return a
	}
	return b
}
