//go:build windows

package exec

import (
	"fmt"
	"os/exec"
	"syscall"
)

// processGroupAttr returns SysProcAttr to create a new process group on Windows.
func processGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: 0x00000200} // CREATE_NEW_PROCESS_GROUP
}

// killProcessGroup kills the process tree on Windows using taskkill.
func killProcessGroup(pid int) error {
	cmd := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid))
	return cmd.Run()
}
