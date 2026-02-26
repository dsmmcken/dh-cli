//go:build linux

package vm

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

//go:embed vm_runner.py
var vmRunnerScript string

// dockerfileTemplate creates a minimal Linux image with JVM + Deephaven.
const dockerfileTemplate = `FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 python3-pip python3-venv python3-dev \
    openjdk-17-jre-headless \
    iproute2 \
    && rm -rf /var/lib/apt/lists/*

RUN python3 -m pip install --no-cache-dir --upgrade setuptools wheel
RUN python3 -m pip install --no-cache-dir \
    deephaven-server==%s \
    pydeephaven==%s

COPY init.sh /sbin/init.sh
RUN chmod +x /sbin/init.sh
COPY vm_runner.py /opt/vm_runner.py
`

// initScriptTemplate is the VM init process that starts Deephaven.
// Communication with the host is via vsock (no TAP networking needed).
const initScriptTemplate = `#!/bin/bash
# Mount essential filesystems
mount -t proc proc /proc
mount -t sysfs sysfs /sys
mount -t devtmpfs devtmpfs /dev

# Ensure loopback interface is up (required for localhost TCP after snapshot restore)
ip link set lo up

# Ensure pip-installed packages are on Python's path.
# Firecracker's minimal boot can cause sys.prefix detection issues.
export PYTHONPATH=/usr/local/lib/python3.10/dist-packages

# Start Deephaven server
python3 -c "
import os, sys, time, pathlib
os.environ.setdefault('JAVA_HOME', '/usr/lib/jvm/java-17-openjdk-amd64')
from deephaven_server import Server
s = Server(port=10000, jvm_args=[
    '-Xms32m', '-Xmx4g',
    '-XX:-AlwaysPreTouch',
    '-XX:+UseG1GC',
    '-XX:MaxMetaspaceSize=256m',
    '-XX:+TieredCompilation',
    '-XX:CompileThreshold=100',
    '-DAuthHandlers=io.deephaven.auth.AnonymousAuthenticationHandler',
])
s.start()
pathlib.Path('/tmp/dh_ready').touch()
while True:
    time.sleep(3600)
" &

# Wait for Deephaven readiness
for i in $(seq 1 600); do
    [ -f /tmp/dh_ready ] && break
    sleep 0.1
done

echo "DH_READY" > /dev/ttyS0 2>/dev/null || true

# Start the runner daemon. It connects a pydeephaven Session to localhost:10000,
# then listens on vsock port 10000 for JSON requests from the host.
# Both the daemon and its warm Session are captured in the VM snapshot.
python3 /opt/vm_runner.py &

# Wait for runner readiness
for i in $(seq 1 300); do
    [ -f /tmp/runner_ready ] && break
    sleep 0.1
done

echo "RUNNER_READY" > /dev/ttyS0 2>/dev/null || true

# Keep init alive
exec sleep infinity
`

// buildRootfsDocker builds an ext4 rootfs image using Docker.
func buildRootfsDocker(paths *VMPaths, version string, stderr io.Writer) error {
	rootfsPath := paths.RootfsForVersion(version)

	// Create temp build context
	tmpDir, err := os.MkdirTemp("", "dhg-vm-build-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write Dockerfile
	dockerfile := fmt.Sprintf(dockerfileTemplate, version, version)
	if err := os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		return fmt.Errorf("writing Dockerfile: %w", err)
	}

	// Write init script
	if err := os.WriteFile(filepath.Join(tmpDir, "init.sh"), []byte(initScriptTemplate), 0o755); err != nil {
		return fmt.Errorf("writing init.sh: %w", err)
	}

	// Write vm_runner.py (the in-VM execution daemon)
	if err := os.WriteFile(filepath.Join(tmpDir, "vm_runner.py"), []byte(vmRunnerScript), 0o644); err != nil {
		return fmt.Errorf("writing vm_runner.py: %w", err)
	}

	imageName := fmt.Sprintf("dhg-vm-%s", version)

	// Docker build
	fmt.Fprintf(stderr, "Building Docker image %s...\n", imageName)
	buildCmd := exec.Command("docker", "build", "-t", imageName, tmpDir)
	buildCmd.Stdout = stderr
	buildCmd.Stderr = stderr
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("docker build failed: %w", err)
	}

	// Create container (remove any stale one from a previous interrupted run first)
	containerName := "dhg-vm-export-tmp"
	exec.Command("docker", "rm", "-f", containerName).Run()
	createCmd := exec.Command("docker", "create", "--name", containerName, imageName)
	createOut, err := createCmd.Output()
	if err != nil {
		return fmt.Errorf("docker create failed: %w", err)
	}
	containerID := string(createOut[:12])
	defer exec.Command("docker", "rm", "-f", containerName).Run()

	// Export container filesystem to tarball
	tarPath := filepath.Join(tmpDir, "rootfs.tar")
	fmt.Fprintf(stderr, "Exporting container %s filesystem...\n", containerID)
	exportCmd := exec.Command("docker", "export", "-o", tarPath, containerName)
	exportCmd.Stderr = stderr
	if err := exportCmd.Run(); err != nil {
		return fmt.Errorf("docker export failed: %w", err)
	}

	// Create ext4 image from tarball
	fmt.Fprintf(stderr, "Creating ext4 rootfs image...\n")
	if err := createExt4FromTar(tarPath, rootfsPath, stderr); err != nil {
		return fmt.Errorf("creating ext4 image: %w", err)
	}

	// Cleanup Docker image
	exec.Command("docker", "rmi", imageName).Run()

	fmt.Fprintf(stderr, "Rootfs created at %s\n", rootfsPath)
	return nil
}

// fixMergedUsr restores the merged-usr symlinks that Docker export breaks.
// On Ubuntu 22.04+, /lib is a symlink to /usr/lib (and similarly for /bin,
// /sbin, /lib64). Docker export stores files under both /lib/... and /usr/lib/...,
// so tar extraction creates /lib as a real directory. This function merges the
// contents back and restores the symlinks.
func fixMergedUsr(rootDir string, stderr io.Writer) {
	mergedDirs := []string{"lib", "lib64", "bin", "sbin"}
	for _, name := range mergedDirs {
		topDir := filepath.Join(rootDir, name)
		usrDir := filepath.Join(rootDir, "usr", name)

		// Check if topDir is a real directory (not a symlink)
		fi, err := os.Lstat(topDir)
		if err != nil || fi.Mode()&os.ModeSymlink != 0 {
			continue // doesn't exist or already a symlink
		}
		if !fi.IsDir() {
			continue
		}

		// Ensure /usr/<name> exists
		os.MkdirAll(usrDir, 0o755)

		// Merge contents from /<name>/ into /usr/<name>/
		// Use cp -a to preserve all attributes and handle nested structures
		cpCmd := exec.Command("cp", "-a", "--no-clobber", topDir+"/.", usrDir+"/")
		cpCmd.Stderr = stderr
		cpCmd.Run() // best-effort

		// Remove the real directory and replace with symlink
		os.RemoveAll(topDir)
		os.Symlink(filepath.Join("usr", name), topDir)

		fmt.Fprintf(stderr, "Fixed merged-usr: /%s -> /usr/%s\n", name, name)
	}
}

// canSudo returns true if passwordless sudo is available.
func canSudo() bool {
	err := exec.Command("sudo", "-n", "true").Run()
	return err == nil
}

// createExt4FromTar creates an ext4 filesystem image from a tar archive.
// Uses sudo if available (needed in Docker where fakeroot hangs), otherwise
// falls back to fakeroot for environments without root access.
func createExt4FromTar(tarPath, outputPath string, stderr io.Writer) error {
	extractDir, err := os.MkdirTemp("", "dhg-rootfs-extract-*")
	if err != nil {
		return fmt.Errorf("creating extract dir: %w", err)
	}
	defer func() {
		// Need sudo to remove root-owned files if we used sudo to extract
		if canSudo() {
			exec.Command("sudo", "rm", "-rf", extractDir).Run()
		} else {
			os.RemoveAll(extractDir)
		}
	}()

	if canSudo() {
		return createExt4WithSudo(tarPath, outputPath, extractDir, stderr)
	}
	return createExt4WithFakeroot(tarPath, outputPath, extractDir, stderr)
}

// createExt4WithSudo extracts and builds the ext4 image using sudo.
// This is the preferred path inside Docker containers where fakeroot hangs.
func createExt4WithSudo(tarPath, outputPath, extractDir string, stderr io.Writer) error {
	fmt.Fprintf(stderr, "Extracting container filesystem (via sudo)...\n")
	tarCmd := exec.Command("sudo", "tar", "xf", tarPath, "-C", extractDir)
	tarCmd.Stderr = stderr
	if err := tarCmd.Run(); err != nil {
		return fmt.Errorf("extracting tar: %w", err)
	}

	fixMergedUsr(extractDir, stderr)

	initPath := filepath.Join(extractDir, "sbin", "init")
	os.Remove(initPath)
	os.Symlink("/sbin/init.sh", initPath)

	fmt.Fprintf(stderr, "Creating ext4 image from filesystem...\n")
	mkfsCmd := exec.Command("sudo", "mke2fs",
		"-t", "ext4",
		"-d", extractDir,
		"-F",
		"-b", "4096",
		outputPath,
		"2G",
	)
	mkfsCmd.Stderr = stderr
	if err := mkfsCmd.Run(); err != nil {
		return fmt.Errorf("mke2fs failed: %w", err)
	}

	// Make the output file owned by the calling user
	if u := os.Getenv("USER"); u != "" {
		exec.Command("sudo", "chown", u+":"+u, outputPath).Run()
	}

	return nil
}

// createExt4WithFakeroot extracts and builds the ext4 image using fakeroot.
// Fakeroot fakes root ownership without actual privileges, but can hang
// inside Docker due to SysV IPC issues.
func createExt4WithFakeroot(tarPath, outputPath, extractDir string, stderr io.Writer) error {
	fakerootState := filepath.Join(extractDir, ".fakeroot.state")

	// Prefer fakeroot-tcp (avoids SysV IPC semaphore hangs in some containers)
	fakeroot := "fakeroot"
	if p, err := exec.LookPath("fakeroot-tcp"); err == nil {
		fakeroot = p
	}

	fmt.Fprintf(stderr, "Extracting container filesystem (via fakeroot)...\n")
	tarCmd := exec.Command(fakeroot, "-s", fakerootState, "--",
		"tar", "xf", tarPath, "-C", extractDir)
	tarCmd.Stderr = stderr
	if err := tarCmd.Run(); err != nil {
		return fmt.Errorf("extracting tar: %w", err)
	}

	fixMergedUsr(extractDir, stderr)

	initPath := filepath.Join(extractDir, "sbin", "init")
	os.Remove(initPath)
	os.Symlink("/sbin/init.sh", initPath)

	os.Remove(fakerootState)

	fmt.Fprintf(stderr, "Creating ext4 image from filesystem...\n")
	mkfsCmd := exec.Command(fakeroot, "-i", fakerootState, "--",
		"mke2fs",
		"-t", "ext4",
		"-d", extractDir,
		"-F",
		"-b", "4096",
		outputPath,
		"2G",
	)
	mkfsCmd.Stderr = stderr
	if err := mkfsCmd.Run(); err != nil {
		return fmt.Errorf("mke2fs failed: %w", err)
	}

	return nil
}
