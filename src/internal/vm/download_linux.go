//go:build linux

package vm

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// EnsureFirecracker downloads the Firecracker binary if not present.
func EnsureFirecracker(paths *VMPaths, stderr io.Writer) error {
	if _, err := os.Stat(paths.Firecracker); err == nil {
		return nil // already exists
	}

	if err := os.MkdirAll(paths.Base, 0o755); err != nil {
		return fmt.Errorf("creating vm dir: %w", err)
	}

	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x86_64"
	} else if arch == "arm64" {
		arch = "aarch64"
	}

	url := fmt.Sprintf(
		"https://github.com/firecracker-microvm/firecracker/releases/download/%s/firecracker-%s-%s.tgz",
		FirecrackerVersion, FirecrackerVersion, arch,
	)

	fmt.Fprintf(stderr, "Downloading Firecracker %s for %s...\n", FirecrackerVersion, arch)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("downloading firecracker: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading firecracker: HTTP %d from %s", resp.StatusCode, url)
	}

	// Extract the firecracker binary from the tarball
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("decompressing firecracker archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	binaryName := fmt.Sprintf("release-%s-%s/firecracker-%s-%s", FirecrackerVersion, arch, FirecrackerVersion, arch)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("firecracker binary not found in archive (expected %s)", binaryName)
		}
		if err != nil {
			return fmt.Errorf("reading firecracker archive: %w", err)
		}

		if strings.HasSuffix(hdr.Name, "firecracker-"+FirecrackerVersion+"-"+arch) {
			f, err := os.OpenFile(paths.Firecracker, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
			if err != nil {
				return fmt.Errorf("creating firecracker binary: %w", err)
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				os.Remove(paths.Firecracker)
				return fmt.Errorf("writing firecracker binary: %w", err)
			}
			f.Close()
			fmt.Fprintf(stderr, "Firecracker installed at %s\n", paths.Firecracker)
			return nil
		}
	}
}

// EnsureKernel downloads a pre-built vmlinux kernel from Firecracker's CI
// artifacts bucket if not present. It queries the S3 bucket to find the latest
// available kernel build.
func EnsureKernel(paths *VMPaths, stderr io.Writer) error {
	if _, err := os.Stat(paths.Kernel); err == nil {
		return nil // already exists
	}

	if err := os.MkdirAll(paths.Base, 0o755); err != nil {
		return fmt.Errorf("creating vm dir: %w", err)
	}

	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x86_64"
	} else if arch == "arm64" {
		arch = "aarch64"
	}

	// Find the latest CI build that has a kernel for this arch
	kernelKey, err := findLatestKernel(arch)
	if err != nil {
		return fmt.Errorf("finding kernel: %w", err)
	}

	url := fmt.Sprintf("https://s3.amazonaws.com/spec.ccfc.min/%s", kernelKey)
	kernelName := kernelKey[strings.LastIndex(kernelKey, "/")+1:]

	fmt.Fprintf(stderr, "Downloading kernel %s...\n", kernelName)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("downloading kernel: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading kernel: HTTP %d from %s", resp.StatusCode, url)
	}

	tmpPath := paths.Kernel + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("creating kernel file: %w", err)
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing kernel: %w", err)
	}
	f.Close()

	if err := os.Rename(tmpPath, paths.Kernel); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("moving kernel into place: %w", err)
	}

	fmt.Fprintf(stderr, "Kernel installed at %s\n", paths.Kernel)
	return nil
}

// findLatestKernel queries the Firecracker CI S3 bucket to find the latest
// vmlinux-6.1.x kernel for the given architecture.
func findLatestKernel(arch string) (string, error) {
	// List CI builds (sorted lexically, newest last)
	listURL := "https://s3.amazonaws.com/spec.ccfc.min?prefix=firecracker-ci/&delimiter=/"
	resp, err := http.Get(listURL)
	if err != nil {
		return "", fmt.Errorf("listing CI builds: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading CI build list: %w", err)
	}

	// Parse prefixes from XML (simple extraction, no xml package needed)
	content := string(body)
	var prefixes []string
	for {
		idx := strings.Index(content, "<Prefix>firecracker-ci/2")
		if idx == -1 {
			break
		}
		content = content[idx+len("<Prefix>"):]
		end := strings.Index(content, "</Prefix>")
		if end == -1 {
			break
		}
		prefixes = append(prefixes, content[:end])
		content = content[end:]
	}

	if len(prefixes) == 0 {
		return "", fmt.Errorf("no CI builds found in S3 bucket")
	}

	// Try from newest to oldest to find a build with a 6.1.x kernel
	for i := len(prefixes) - 1; i >= 0; i-- {
		prefix := prefixes[i]
		kernelKey, err := findKernelInBuild(prefix, arch)
		if err == nil {
			return kernelKey, nil
		}
	}

	return "", fmt.Errorf("no vmlinux-6.1.x kernel found for %s in any CI build", arch)
}

// findKernelInBuild looks for a vmlinux-6.1.x kernel in a specific CI build.
func findKernelInBuild(buildPrefix, arch string) (string, error) {
	listURL := fmt.Sprintf(
		"https://s3.amazonaws.com/spec.ccfc.min?prefix=%s%s/vmlinux-6.1&delimiter=/",
		buildPrefix, arch,
	)
	resp, err := http.Get(listURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	content := string(body)
	// Find <Key>...</Key> entries that match vmlinux-6.1.NNN (no .config suffix)
	var bestKey string
	for {
		idx := strings.Index(content, "<Key>")
		if idx == -1 {
			break
		}
		content = content[idx+len("<Key>"):]
		end := strings.Index(content, "</Key>")
		if end == -1 {
			break
		}
		key := content[:end]
		content = content[end:]

		// Skip .config files and -no-acpi variants
		if strings.HasSuffix(key, ".config") || strings.Contains(key, "-no-acpi") {
			continue
		}
		if strings.Contains(key, "vmlinux-6.1.") {
			bestKey = key
		}
	}

	if bestKey == "" {
		return "", fmt.Errorf("no kernel found")
	}
	return bestKey, nil
}

// EnsureRootfs downloads or retrieves the rootfs quickstart image if no rootfs exists.
func EnsureRootfs(paths *VMPaths, version string, stderr io.Writer) error {
	rootfsPath := paths.RootfsForVersion(version)
	if _, err := os.Stat(rootfsPath); err == nil {
		return nil
	}

	if err := os.MkdirAll(paths.RootfsDir, 0o755); err != nil {
		return fmt.Errorf("creating rootfs dir: %w", err)
	}

	// Try Docker-based build first
	if _, err := findDocker(); err == nil {
		return buildRootfsDocker(paths, version, stderr)
	}

	return fmt.Errorf("Docker is required to build the VM rootfs. Install Docker and retry")
}

func findDocker() (string, error) {
	return exec.LookPath("docker")
}
