package versions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// ExecCommand is a wrapper around exec.Command for testability.
var ExecCommand = exec.Command

// Install installs a Deephaven version into <dhgHome>/versions/<version>/.
func Install(dhgHome, version, pythonVer string, plugins []string, onProgress func(string)) error {
	versionDir := filepath.Join(dhgHome, "versions", version)

	// Check if already installed
	if _, err := os.Stat(versionDir); err == nil {
		return fmt.Errorf("version %s is already installed at %s", version, versionDir)
	}

	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		return fmt.Errorf("creating version directory: %w", err)
	}

	// On failure, clean up the directory
	success := false
	defer func() {
		if !success {
			os.RemoveAll(versionDir)
		}
	}()

	// Step 1: Create venv
	if onProgress != nil {
		onProgress(fmt.Sprintf("Creating Python %s virtual environment...", pythonVer))
	}
	venvDir := filepath.Join(versionDir, ".venv")
	cmd := ExecCommand("uv", "venv", venvDir, "--python", pythonVer)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("creating venv: %w", err)
	}

	// Step 2: Install packages
	if onProgress != nil {
		onProgress(fmt.Sprintf("Installing deephaven-server==%s...", version))
	}

	pythonBin := filepath.Join(venvDir, "bin", "python")
	if runtime.GOOS == "windows" {
		pythonBin = filepath.Join(venvDir, "Scripts", "python.exe")
	}

	pipArgs := []string{"pip", "install", "--python", pythonBin,
		fmt.Sprintf("deephaven-server==%s", version),
		fmt.Sprintf("pydeephaven==%s", version),
	}
	for _, p := range plugins {
		pipArgs = append(pipArgs, p)
	}
	cmd = ExecCommand("uv", pipArgs...)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("installing packages: %w", err)
	}

	// Step 3: Write meta.toml
	if onProgress != nil {
		onProgress("Writing metadata...")
	}
	meta := &Meta{
		InstalledAt: time.Now().UTC(),
	}
	if err := WriteMeta(versionDir, meta); err != nil {
		return fmt.Errorf("writing meta.toml: %w", err)
	}

	success = true

	if onProgress != nil {
		onProgress(fmt.Sprintf("Version %s installed successfully.", version))
	}
	return nil
}
