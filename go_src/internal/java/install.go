package java

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Install downloads and installs a Temurin JDK to <dhgHome>/java/.
func Install(dhgHome string, jdkVersion int, force bool) (*JavaInfo, error) {
	javaDir := filepath.Join(dhgHome, "java")

	// Check for existing installation
	if !force {
		existing, _ := filepath.Glob(filepath.Join(javaDir, "*", "bin", "java"))
		if len(existing) == 0 {
			existing, _ = filepath.Glob(filepath.Join(javaDir, "*", "Contents", "Home", "bin", "java"))
		}
		if len(existing) > 0 {
			return nil, fmt.Errorf("Java already installed at %s (use --force to reinstall)", filepath.Dir(filepath.Dir(existing[0])))
		}
	}

	osName, archName, err := adoptiumPlatform()
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf(
		"https://api.adoptium.net/v3/binary/latest/%d/ga/%s/%s/jdk/hotspot/normal/eclipse",
		jdkVersion, osName, archName,
	)

	// Remove existing managed Java if force
	if force {
		os.RemoveAll(javaDir)
	}

	if err := os.MkdirAll(javaDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create java directory: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Downloading Temurin JDK %d for %s/%s...\n", jdkVersion, osName, archName)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	if err := extractTarGz(resp.Body, javaDir); err != nil {
		return nil, fmt.Errorf("extraction failed: %w", err)
	}

	// Find the installed java binary
	info := findInstalledJava(javaDir)
	if info == nil {
		return nil, fmt.Errorf("installation completed but could not find java binary in %s", javaDir)
	}

	fmt.Fprintf(os.Stderr, "Installed Temurin JDK %s at %s\n", info.Version, info.Home)
	return info, nil
}

// adoptiumPlatform maps Go runtime OS/arch to Adoptium naming.
func adoptiumPlatform() (string, string, error) {
	osMap := map[string]string{
		"linux":  "linux",
		"darwin": "mac",
		"windows": "windows",
	}
	archMap := map[string]string{
		"amd64": "x64",
		"arm64": "aarch64",
	}

	osName, ok := osMap[runtime.GOOS]
	if !ok {
		return "", "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	archName, ok := archMap[runtime.GOARCH]
	if !ok {
		return "", "", fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
	}
	return osName, archName, nil
}

// extractTarGz extracts a .tar.gz archive to destDir.
func extractTarGz(r io.Reader, destDir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("gzip error: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Clean the path to prevent directory traversal
		cleanName := filepath.Clean(hdr.Name)
		if strings.Contains(cleanName, "..") {
			continue
		}
		target := filepath.Join(destDir, cleanName)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			os.Remove(target) // remove if exists
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		}
	}
	return nil
}

// findInstalledJava looks for a java binary in the javaDir after extraction.
func findInstalledJava(javaDir string) *JavaInfo {
	// Look for bin/java under extracted directory
	patterns := []string{
		filepath.Join(javaDir, "*", "bin", "java"),
		filepath.Join(javaDir, "*", "Contents", "Home", "bin", "java"), // macOS layout
	}
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		for _, m := range matches {
			home := javaHomeFromBin(m)
			if info, err := probeJava(m, home, "managed"); err == nil {
				return info
			}
		}
	}
	return nil
}
