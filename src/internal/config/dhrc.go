package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const dhrcFile = ".dhrc"

// FindDHRC walks up from startDir looking for a .dhrc file.
// Returns the path to the file if found, or empty string and nil if not found.
func FindDHRC(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}

	for {
		candidate := filepath.Join(dir, dhrcFile)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			return "", nil
		}
		dir = parent
	}
}

// ReadDHRC reads the version string from a .dhrc file.
// The file is expected to contain just the version string (optionally with whitespace).
func ReadDHRC(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading .dhrc: %w", err)
	}
	version := strings.TrimSpace(string(data))
	if version == "" {
		return "", fmt.Errorf(".dhrc is empty: %s", path)
	}
	return version, nil
}

// WriteDHRC writes a version string to a .dhrc file in the given directory.
func WriteDHRC(dir, version string) error {
	path := filepath.Join(dir, dhrcFile)
	return os.WriteFile(path, []byte(version+"\n"), 0o644)
}
