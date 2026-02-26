//go:build windows

package repl

import (
	"fmt"
	"os/exec"
	"syscall"
)

func processGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: 0x00000200} // CREATE_NEW_PROCESS_GROUP
}

func killProcessGroup(pid int) error {
	cmd := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid))
	return cmd.Run()
}
