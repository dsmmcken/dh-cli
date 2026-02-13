package main

import (
	"fmt"
	"os"
	"slices"

	"github.com/dsmmcken/dh-cli/go_src/internal/cmd"
)

func main() {
	// Handle shorthand: dhg -c "code" -> dhg exec -c "code"
	if len(os.Args) >= 2 && os.Args[1] == "-c" {
		os.Args = slices.Insert(os.Args, 1, "exec")
	}

	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
