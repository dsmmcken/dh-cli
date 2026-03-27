package render

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// MinNodeVersion is the minimum required Node.js major version.
const MinNodeVersion = 20

// CheckNode verifies node is available and meets the minimum version requirement.
// Returns the path to the node binary and any error.
var CheckNode = checkNode

func checkNode() (string, error) {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		return "", fmt.Errorf("node not found in PATH (install Node.js %d+ for dh render)", MinNodeVersion)
	}

	out, err := exec.Command(nodePath, "--version").Output()
	if err != nil {
		return nodePath, fmt.Errorf("node found at %s but failed to get version: %w", nodePath, err)
	}

	version := strings.TrimSpace(string(out))
	major, err := parseNodeMajor(version)
	if err != nil {
		return nodePath, fmt.Errorf("could not parse node version %q: %w", version, err)
	}

	if major < MinNodeVersion {
		return nodePath, fmt.Errorf("node %s found but %d+ required for dh render", version, MinNodeVersion)
	}

	return nodePath, nil
}

// NodeVersion returns the node version string, or empty if node is unavailable.
func NodeVersion() string {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		return ""
	}
	out, err := exec.Command(nodePath, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// parseNodeMajor extracts the major version from a node version string like "v22.1.0".
func parseNodeMajor(version string) (int, error) {
	re := regexp.MustCompile(`^v?(\d+)`)
	m := re.FindStringSubmatch(version)
	if m == nil {
		return 0, fmt.Errorf("no major version found in %q", version)
	}
	return strconv.Atoi(m[1])
}

// EnsureRuntime extracts the embedded JS runtime to dhHome/render/ if needed,
// runs npm install, and returns the path to the runtime directory.
func EnsureRuntime(dhHome string, quiet bool) (string, error) {
	renderDir := filepath.Join(dhHome, "render")
	versionFile := filepath.Join(renderDir, ".version")

	// Compute hash of embedded files
	currentHash, err := embeddedHash()
	if err != nil {
		return "", fmt.Errorf("computing embedded hash: %w", err)
	}

	// Check if extraction is needed
	if existing, err := os.ReadFile(versionFile); err == nil {
		if strings.TrimSpace(string(existing)) == currentHash {
			return renderDir, nil
		}
	}

	// Extract embedded files
	if err := os.MkdirAll(renderDir, 0o755); err != nil {
		return "", fmt.Errorf("creating render directory: %w", err)
	}

	if err := extractEmbedded(renderDir); err != nil {
		return "", fmt.Errorf("extracting render runtime: %w", err)
	}

	// Run npm install
	npmPath, err := exec.LookPath("npm")
	if err != nil {
		return "", fmt.Errorf("npm not found in PATH (required for dh render setup)")
	}

	cmd := exec.Command(npmPath, "install", "--omit=dev", "--legacy-peer-deps", "--loglevel", "error")
	cmd.Dir = renderDir
	if !quiet {
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("npm install failed in %s: %w", renderDir, err)
	}

	// Write version marker
	if err := os.WriteFile(versionFile, []byte(currentHash), 0o644); err != nil {
		return "", fmt.Errorf("writing version file: %w", err)
	}

	return renderDir, nil
}

// extractEmbedded walks the embedded FS and writes files to destDir.
func extractEmbedded(destDir string) error {
	return fs.WalkDir(EmbeddedFS, "embedded", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Strip "embedded/" prefix to get relative path
		rel, err := filepath.Rel("embedded", path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		// Skip .gitkeep placeholder
		if d.Name() == ".gitkeep" {
			return nil
		}

		dest := filepath.Join(destDir, rel)

		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}

		data, err := EmbeddedFS.ReadFile(path)
		if err != nil {
			return err
		}

		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}

		return os.WriteFile(dest, data, 0o644)
	})
}

// embeddedHash computes a SHA-256 hash of all embedded file contents.
func embeddedHash() (string, error) {
	h := sha256.New()
	err := fs.WalkDir(EmbeddedFS, "embedded", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() == ".gitkeep" {
			return nil
		}
		data, err := EmbeddedFS.ReadFile(path)
		if err != nil {
			return err
		}
		h.Write([]byte(path))
		h.Write(data)
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

