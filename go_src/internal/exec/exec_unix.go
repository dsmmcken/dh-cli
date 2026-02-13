//go:build !windows

package exec

import "syscall"

// processGroupAttr returns SysProcAttr to create a new process group on unix.
func processGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup sends SIGKILL to the process group.
func killProcessGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGKILL)
}
