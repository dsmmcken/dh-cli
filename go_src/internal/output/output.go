package output

import (
	"encoding/json"
	"fmt"
	"io"
)

// Exit codes
const (
	ExitSuccess     = 0
	ExitError       = 1
	ExitNetwork     = 2
	ExitTimeout     = 3
	ExitNotFound    = 4
	ExitInterrupted = 130
)

var (
	flagJSON    bool
	flagQuiet   bool
	flagVerbose bool
)

// SetFlags is called by the root command's PersistentPreRun to propagate flag values.
func SetFlags(jsonMode, quiet, verbose bool) {
	flagJSON = jsonMode
	flagQuiet = quiet
	flagVerbose = verbose
}

// IsJSON returns true when --json mode is active.
func IsJSON() bool { return flagJSON }

// IsQuiet returns true when --quiet mode is active.
func IsQuiet() bool { return flagQuiet }

// IsVerbose returns true when --verbose mode is active.
func IsVerbose() bool { return flagVerbose }

// PrintJSON marshals v as JSON and writes it to w.
func PrintJSON(w io.Writer, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

// PrintError writes a JSON error envelope to w.
func PrintError(w io.Writer, code string, message string) error {
	return PrintJSON(w, map[string]string{
		"error":   code,
		"message": message,
	})
}
