//go:build !linux

package vm

import (
	"fmt"
	"io"
)

func EnsureFirecracker(_ *VMPaths, _ io.Writer) error {
	return fmt.Errorf("VM mode requires Linux")
}

func EnsureKernel(_ *VMPaths, _ io.Writer) error {
	return fmt.Errorf("VM mode requires Linux")
}

func EnsureRootfs(_ *VMPaths, _ string, _ io.Writer) error {
	return fmt.Errorf("VM mode requires Linux")
}
