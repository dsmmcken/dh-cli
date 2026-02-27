//go:build linux

package vm

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"strings"
)

// PrereqError describes a failed prerequisite check.
type PrereqError struct {
	Check   string
	Message string
	Hint    string
	AutoFix bool // true if FixPrerequisites can resolve this
}

func (e *PrereqError) Error() string {
	if e.Hint != "" {
		return fmt.Sprintf("%s: %s\n  Hint: %s", e.Check, e.Message, e.Hint)
	}
	return fmt.Sprintf("%s: %s", e.Check, e.Message)
}

// KVMAccessible returns true if /dev/kvm exists and is read-write accessible.
func KVMAccessible() bool {
	f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// CheckPrerequisites verifies all requirements for VM mode.
func CheckPrerequisites(paths *VMPaths) []*PrereqError {
	var errs []*PrereqError

	// /dev/kvm exists and is accessible
	if _, err := os.Stat("/dev/kvm"); err != nil {
		errs = append(errs, &PrereqError{
			Check:   "/dev/kvm",
			Message: "KVM not available — is this a virtual machine without nested virtualization?",
			Hint:    "Enable KVM: sudo modprobe kvm_intel (or kvm_amd)",
		})
	} else if !KVMAccessible() {
		errs = append(errs, &PrereqError{
			Check:   "/dev/kvm",
			Message: "permission denied",
			Hint:    "Will be fixed automatically by 'dh vm prepare', or run: sudo setfacl -m u:${USER}:rw /dev/kvm",
			AutoFix: true,
		})
	}

	// Firecracker binary
	if _, err := os.Stat(paths.Firecracker); err != nil {
		errs = append(errs, &PrereqError{
			Check:   "firecracker",
			Message: "firecracker binary not found",
			Hint:    fmt.Sprintf("Run 'dh vm prepare' to auto-download, or place binary at %s", paths.Firecracker),
			AutoFix: true,
		})
	}

	// Kernel binary
	if _, err := os.Stat(paths.Kernel); err != nil {
		errs = append(errs, &PrereqError{
			Check:   "kernel",
			Message: "vmlinux kernel not found",
			Hint:    fmt.Sprintf("Run 'dh vm prepare' to auto-download, or place vmlinux at %s", paths.Kernel),
			AutoFix: true,
		})
	}

	return errs
}

// FixKVMAccess attempts to grant the current user read-write access to /dev/kvm.
// Uses setfacl (scoped to just the current user, instant, no re-login needed).
// Falls back to adding the user to the kvm group (requires re-login).
func FixKVMAccess(stderr io.Writer) error {
	if KVMAccessible() {
		return nil
	}

	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("getting current user: %w", err)
	}

	// Install acl package if setfacl is missing (Ubuntu doesn't ship it by default)
	if _, err := exec.LookPath("setfacl"); err != nil {
		fmt.Fprintf(stderr, "Installing acl package for setfacl...\n")
		installCmd := exec.Command("sudo", "apt-get", "install", "-y", "acl")
		installCmd.Stdin = os.Stdin
		installCmd.Stdout = stderr
		installCmd.Stderr = stderr
		if err := installCmd.Run(); err != nil {
			return fmt.Errorf("failed to install acl package: %w\n  Manually run: sudo apt install acl && sudo setfacl -m u:%s:rw /dev/kvm", err, currentUser.Username)
		}
	}

	// setfacl — grants access only to this user, takes effect immediately
	fmt.Fprintf(stderr, "Granting KVM access via: sudo setfacl -m u:%s:rw /dev/kvm\n", currentUser.Username)
	cmd := exec.Command("sudo", "setfacl", "-m", fmt.Sprintf("u:%s:rw", currentUser.Username), "/dev/kvm")
	cmd.Stdin = os.Stdin
	cmd.Stdout = stderr
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("setfacl failed: %w\n  Manually run: sudo setfacl -m u:%s:rw /dev/kvm", err, currentUser.Username)
	}

	if !KVMAccessible() {
		return fmt.Errorf("setfacl succeeded but /dev/kvm still not accessible")
	}

	fmt.Fprintf(stderr, "KVM access granted.\n")
	return nil
}

// HasNonAutoFixErrors returns true if there are prerequisite errors that cannot
// be automatically resolved by prepare.
func HasNonAutoFixErrors(errs []*PrereqError) bool {
	for _, e := range errs {
		if !e.AutoFix {
			return true
		}
	}
	return false
}

// FormatPrereqErrors formats prerequisite errors with FAIL/FIXABLE labels.
func FormatPrereqErrors(errs []*PrereqError) string {
	var b strings.Builder
	for _, e := range errs {
		label := "FAIL"
		if e.AutoFix {
			label = "FIXABLE"
		}
		fmt.Fprintf(&b, "  [%s] %s: %s\n", label, e.Check, e.Message)
		if e.Hint != "" {
			fmt.Fprintf(&b, "         %s\n", e.Hint)
		}
	}
	return b.String()
}

// CheckSnapshot verifies a snapshot exists and is valid for the given version.
func CheckSnapshot(paths *VMPaths, version string) error {
	snapDir := paths.SnapshotDirForVersion(version)

	for _, name := range []string{"metadata.json", "snapshot_mem", "snapshot_vmstate", "disk.ext4"} {
		path := fmt.Sprintf("%s/%s", snapDir, name)
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("no valid snapshot for version %s (missing %s). Run: dh vm prepare --version %s", version, name, version)
		}
	}
	return nil
}
