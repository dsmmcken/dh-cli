//go:build linux

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/dsmmcken/dh-cli/src/internal/vm"
)

func checkUffd() CheckResult {
	if vm.ProbeUffd() {
		return CheckResult{
			Name:   "VM/UFFD",
			Status: "ok",
			Detail: "userfaultfd available (fast snapshot restore)",
		}
	}

	return CheckResult{
		Name:   "VM/UFFD",
		Status: "warning",
		Detail: "disabled — VM snapshots use slow file-backed restore. Fix: sudo sysctl -w vm.unprivileged_userfaultfd=1",
	}
}

func fixUffd() error {
	cmd := exec.Command("sudo", "-n", "sysctl", "-w", "vm.unprivileged_userfaultfd=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run: sudo sysctl -w vm.unprivileged_userfaultfd=1")
	}

	// Persist across reboots
	persistCmd := exec.Command("sudo", "-n", "tee", "/etc/sysctl.d/99-userfaultfd.conf")
	persistCmd.Stdin = strings.NewReader("vm.unprivileged_userfaultfd=1\n")
	persistCmd.Run() // best-effort

	return nil
}
