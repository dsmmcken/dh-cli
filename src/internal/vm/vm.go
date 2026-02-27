// Package vm provides Firecracker microVM management for fast Deephaven execution.
// This is an experimental feature that requires Linux with KVM support.
package vm

import (
	"path/filepath"
	"time"
)

const (
	// DefaultDHPort is the Deephaven gRPC port inside the VM.
	DefaultDHPort = 10000

	// DefaultMemSizeMiB is the default VM memory size.
	// The snapshot is made sparse via balloon inflation + hole punching,
	// so this can be larger than the actual data without increasing
	// snapshot/restore cost.
	DefaultMemSizeMiB = 4608 // 4.5 GiB — room for 4g JVM heap + OS + Python

	// DefaultVCPUCount is the default number of vCPUs.
	DefaultVCPUCount = 2

	// FileServerPort is the vsock port for the host file server.
	// The guest LD_PRELOAD library connects to CID=2:FileServerPort to fetch
	// workspace files on demand.
	FileServerPort = 10001

	// FirecrackerVersion is the version of Firecracker to download.
	FirecrackerVersion = "v1.12.0"
)

// VMConfig holds configuration for VM operations.
type VMConfig struct {
	DHGHome string // ~/.dhg
	Version string // Deephaven version
	Verbose bool
	UseUffd bool // use UFFD eager page population for snapshot restore
}

// VMPaths returns canonical paths for VM artifacts.
type VMPaths struct {
	Base        string // ~/.dhg/vm
	Firecracker string // ~/.dhg/vm/firecracker
	Kernel      string // ~/.dhg/vm/vmlinux
	RootfsDir   string // ~/.dhg/vm/rootfs
	SnapshotDir string // ~/.dhg/vm/snapshots
	RunDir      string // ~/.dhg/vm/run
}

// NewVMPaths creates VMPaths for a given DHG home directory.
func NewVMPaths(dhgHome string) *VMPaths {
	base := filepath.Join(dhgHome, "vm")
	return &VMPaths{
		Base:        base,
		Firecracker: filepath.Join(base, "firecracker"),
		Kernel:      filepath.Join(base, "vmlinux"),
		RootfsDir:   filepath.Join(base, "rootfs"),
		SnapshotDir: filepath.Join(base, "snapshots"),
		RunDir:      filepath.Join(base, "run"),
	}
}

// RootfsForVersion returns the path to the ext4 rootfs for a version.
func (p *VMPaths) RootfsForVersion(version string) string {
	return filepath.Join(p.RootfsDir, "deephaven-"+version+".ext4")
}

// SnapshotDirForVersion returns the snapshot directory for a version.
func (p *VMPaths) SnapshotDirForVersion(version string) string {
	return filepath.Join(p.SnapshotDir, version)
}

// InstanceDir returns the run directory for a specific instance.
func (p *VMPaths) InstanceDir(instanceID string) string {
	return filepath.Join(p.RunDir, instanceID)
}

// SnapshotMetadata is persisted alongside each snapshot.
type SnapshotMetadata struct {
	Version    string    `json:"version"`
	CreatedAt  time.Time `json:"created_at"`
	DHPort     int       `json:"dh_port"`
	MemSizeMiB int       `json:"mem_size_mib,omitempty"` // VM memory at snapshot time
	BalloonMiB int       `json:"balloon_mib,omitempty"`  // balloon inflation at snapshot time
}

// InstanceInfo tracks a running VM instance.
type InstanceInfo struct {
	ID        string `json:"id"`
	PID       int    `json:"pid"`
	Version   string `json:"version"`
	VsockPath string `json:"vsock_path"` // Path to the vsock UDS
}
