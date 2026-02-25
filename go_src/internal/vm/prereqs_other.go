//go:build !linux

package vm

import (
	"fmt"
	"io"
)

// PrereqError describes a failed prerequisite check.
type PrereqError struct {
	Check   string
	Message string
	Hint    string
	AutoFix bool
}

func (e *PrereqError) Error() string {
	return fmt.Sprintf("%s: %s", e.Check, e.Message)
}

func KVMAccessible() bool { return false }

func CheckPrerequisites(_ *VMPaths) []*PrereqError {
	return []*PrereqError{{
		Check:   "platform",
		Message: "VM mode requires Linux with KVM support",
	}}
}

func CheckSnapshot(_ *VMPaths, _ string) error {
	return fmt.Errorf("VM mode requires Linux with KVM support")
}

func FixKVMAccess(_ io.Writer) error {
	return fmt.Errorf("VM mode requires Linux with KVM support")
}

func HasNonAutoFixErrors(errs []*PrereqError) bool {
	return len(errs) > 0
}

func FormatPrereqErrors(errs []*PrereqError) string {
	s := ""
	for _, e := range errs {
		s += fmt.Sprintf("  [FAIL] %s\n", e.Error())
	}
	return s
}
