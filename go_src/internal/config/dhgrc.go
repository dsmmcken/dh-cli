package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const dhgrcFile = ".dhgrc"

// FindDHGRC walks up from startDir looking for a .dhgrc file.
// Returns the path to the file if found, or empty string and nil if not found.
func FindDHGRC(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}

	for {
		candidate := filepath.Join(dir, dhgrcFile)
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

// ReadDHGRC reads the version string from a .dhgrc file.
// The file is expected to contain just the version string (optionally with whitespace).
func ReadDHGRC(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading .dhgrc: %w", err)
	}
	version := strings.TrimSpace(string(data))
	if version == "" {
		return "", fmt.Errorf(".dhgrc is empty: %s", path)
	}
	return version, nil
}

// WriteDHGRC writes a version string to a .dhgrc file in the given directory.
func WriteDHGRC(dir, version string) error {
	path := filepath.Join(dir, dhgrcFile)
	return os.WriteFile(path, []byte(version+"\n"), 0o644)
}
