//go:build windows

package discovery

import "fmt"

// discoverProcesses is not implemented on Windows.
func discoverProcesses() ([]Server, error) {
	return nil, fmt.Errorf("process discovery is not supported on Windows")
}
