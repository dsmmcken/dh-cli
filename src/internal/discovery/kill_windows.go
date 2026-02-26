//go:build windows

package discovery

import "fmt"

func killProcess(pid int) error {
	return fmt.Errorf("process kill is not supported on Windows")
}
