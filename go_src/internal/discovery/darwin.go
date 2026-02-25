//go:build darwin

package discovery

import (
	"os/exec"
)

// discoverProcesses finds listening TCP sockets on macOS using lsof.
func discoverProcesses() ([]Server, error) {
	out, err := exec.Command("lsof", "-iTCP", "-sTCP:LISTEN", "-P", "-n", "-F", "pcn").Output()
	if err != nil {
		// lsof may fail if no TCP sockets are open; return empty
		return nil, nil
	}
	return ParseLsofOutput(string(out)), nil
}
