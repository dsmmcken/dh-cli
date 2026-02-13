//go:build !windows

package discovery

import "syscall"

func killProcess(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}
