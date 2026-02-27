//go:build !linux

package exec

import (
	"fmt"

	"github.com/dsmmcken/dh-cli/src/internal/output"
)

func runVM(cfg *ExecConfig, userCode, version, dhHome string) (int, map[string]any, error) {
	return output.ExitError, nil, fmt.Errorf("--vm flag requires Linux with KVM support")
}
