package versions

import (
	"fmt"
	"os"
	"path/filepath"
)

// Uninstall removes a Deephaven version directory.
func Uninstall(dhHome, version string) error {
	versionDir := filepath.Join(dhHome, "versions", version)

	if _, err := os.Stat(versionDir); os.IsNotExist(err) {
		return fmt.Errorf("version %s is not installed", version)
	}

	if err := os.RemoveAll(versionDir); err != nil {
		return fmt.Errorf("removing version directory: %w", err)
	}

	return nil
}
