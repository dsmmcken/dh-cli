//go:build !linux

package vm

import (
	"context"
	"fmt"
	"io"
)

// MachineHandle is a placeholder for the firecracker.Machine type on non-Linux.
type MachineHandle struct{}

func (m *MachineHandle) StopVMM() {}
func (m *MachineHandle) Pid() int { return 0 }

func BootAndSnapshot(_ context.Context, _ *VMConfig, _ *VMPaths, _ io.Writer) error {
	return fmt.Errorf("VM mode requires Linux with KVM support")
}

func RestoreFromSnapshot(_ context.Context, _ *VMConfig, _ *VMPaths, _ io.Writer) (*InstanceInfo, *MachineHandle, io.Closer, error) {
	return nil, nil, nil, fmt.Errorf("VM mode requires Linux with KVM support")
}

func DestroyInstance(_ *MachineHandle, _ *InstanceInfo, _ *VMPaths) {}
