package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestVMSubcommandRegistered(t *testing.T) {
	root := NewRootCmd()

	// Find the "vm" subcommand
	var vmCmd *cobra.Command
	for _, c := range root.Commands() {
		if c.Use == "vm" {
			vmCmd = c
			break
		}
	}

	if vmCmd == nil {
		t.Fatal("'vm' subcommand not registered on root command")
	}

	// Check subcommands
	subNames := map[string]bool{}
	for _, c := range vmCmd.Commands() {
		subNames[c.Name()] = true
	}

	for _, name := range []string{"prepare", "status", "clean"} {
		if !subNames[name] {
			t.Errorf("'vm %s' subcommand not found", name)
		}
	}
}

func TestExecVMFlagRegistered(t *testing.T) {
	root := NewRootCmd()

	// Find the "exec" subcommand
	var execCmd *cobra.Command
	for _, c := range root.Commands() {
		if c.Name() == "exec" {
			execCmd = c
			break
		}
	}

	if execCmd == nil {
		t.Fatal("'exec' subcommand not registered")
	}

	// Check --vm flag exists
	vmFlag := execCmd.Flags().Lookup("vm")
	if vmFlag == nil {
		t.Fatal("--vm flag not registered on exec command")
	}

	if vmFlag.DefValue != "false" {
		t.Errorf("--vm default = %q, want %q", vmFlag.DefValue, "false")
	}
}

func TestVMStatusCommand(t *testing.T) {
	root := NewRootCmd()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"vm", "status"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("vm status failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Prerequisites:") {
		t.Errorf("vm status output missing 'Prerequisites:', got:\n%s", output)
	}
	if !strings.Contains(output, "Snapshots:") {
		t.Errorf("vm status output missing 'Snapshots:', got:\n%s", output)
	}
}
