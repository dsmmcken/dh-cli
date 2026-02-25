package vm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewVMPaths(t *testing.T) {
	paths := NewVMPaths("/home/user/.dhg")

	if paths.Base != "/home/user/.dhg/vm" {
		t.Errorf("Base = %q, want %q", paths.Base, "/home/user/.dhg/vm")
	}
	if paths.Firecracker != "/home/user/.dhg/vm/firecracker" {
		t.Errorf("Firecracker = %q, want %q", paths.Firecracker, "/home/user/.dhg/vm/firecracker")
	}
	if paths.Kernel != "/home/user/.dhg/vm/vmlinux" {
		t.Errorf("Kernel = %q, want %q", paths.Kernel, "/home/user/.dhg/vm/vmlinux")
	}
	if paths.RootfsDir != "/home/user/.dhg/vm/rootfs" {
		t.Errorf("RootfsDir = %q, want %q", paths.RootfsDir, "/home/user/.dhg/vm/rootfs")
	}
	if paths.SnapshotDir != "/home/user/.dhg/vm/snapshots" {
		t.Errorf("SnapshotDir = %q, want %q", paths.SnapshotDir, "/home/user/.dhg/vm/snapshots")
	}
	if paths.RunDir != "/home/user/.dhg/vm/run" {
		t.Errorf("RunDir = %q, want %q", paths.RunDir, "/home/user/.dhg/vm/run")
	}
}

func TestRootfsForVersion(t *testing.T) {
	paths := NewVMPaths("/home/user/.dhg")
	got := paths.RootfsForVersion("0.36.0")
	want := "/home/user/.dhg/vm/rootfs/deephaven-0.36.0.ext4"
	if got != want {
		t.Errorf("RootfsForVersion = %q, want %q", got, want)
	}
}

func TestSnapshotDirForVersion(t *testing.T) {
	paths := NewVMPaths("/home/user/.dhg")
	got := paths.SnapshotDirForVersion("0.36.0")
	want := "/home/user/.dhg/vm/snapshots/0.36.0"
	if got != want {
		t.Errorf("SnapshotDirForVersion = %q, want %q", got, want)
	}
}

func TestInstanceDir(t *testing.T) {
	paths := NewVMPaths("/home/user/.dhg")
	got := paths.InstanceDir("exec-12345")
	want := "/home/user/.dhg/vm/run/exec-12345"
	if got != want {
		t.Errorf("InstanceDir = %q, want %q", got, want)
	}
}

func TestCheckSnapshot_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	paths := NewVMPaths(tmpDir)

	err := CheckSnapshot(paths, "0.36.0")
	if err == nil {
		t.Error("expected error for missing snapshot, got nil")
	}
}

func TestCheckSnapshot_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	paths := NewVMPaths(tmpDir)

	// Create all required snapshot files
	snapDir := paths.SnapshotDirForVersion("0.36.0")
	if err := os.MkdirAll(snapDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"metadata.json", "snapshot_mem", "snapshot_vmstate", "disk.ext4"} {
		if err := os.WriteFile(filepath.Join(snapDir, name), []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	err := CheckSnapshot(paths, "0.36.0")
	if err != nil {
		t.Errorf("expected no error for valid snapshot, got: %v", err)
	}
}

func TestCheckSnapshot_Incomplete(t *testing.T) {
	tmpDir := t.TempDir()
	paths := NewVMPaths(tmpDir)

	// Create only some files
	snapDir := paths.SnapshotDirForVersion("0.36.0")
	if err := os.MkdirAll(snapDir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(snapDir, "metadata.json"), []byte("test"), 0o644)
	// Missing: snapshot_mem, snapshot_vmstate, disk.ext4

	err := CheckSnapshot(paths, "0.36.0")
	if err == nil {
		t.Error("expected error for incomplete snapshot, got nil")
	}
}

func TestCheckPrerequisites(t *testing.T) {
	tmpDir := t.TempDir()
	paths := NewVMPaths(tmpDir)

	errs := CheckPrerequisites(paths)
	// On any platform, should return at least one error (missing binaries)
	if len(errs) == 0 {
		t.Error("expected at least one prerequisite error, got none")
	}

	// Verify error has Check and Message fields
	for _, e := range errs {
		if e.Check == "" {
			t.Error("prerequisite error has empty Check field")
		}
		if e.Message == "" {
			t.Error("prerequisite error has empty Message field")
		}
	}
}

