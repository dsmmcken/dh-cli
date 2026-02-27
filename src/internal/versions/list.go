package versions

import (
	"os"
	"path/filepath"
	"time"
)

// InstalledVersion represents a locally installed Deephaven version.
type InstalledVersion struct {
	Version     string    `json:"version"`
	IsDefault   bool      `json:"is_default"`
	InstalledAt time.Time `json:"installed_at"`
}

// ListInstalled scans <dhHome>/versions/ and returns installed versions sorted descending.
func ListInstalled(dhHome string) ([]InstalledVersion, error) {
	versionsDir := filepath.Join(dhHome, "versions")
	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var versions []string
	for _, e := range entries {
		if e.IsDir() {
			versions = append(versions, e.Name())
		}
	}

	SortVersionsDesc(versions)

	var result []InstalledVersion
	for _, v := range versions {
		vDir := filepath.Join(versionsDir, v)
		meta, err := ReadMeta(vDir)
		iv := InstalledVersion{
			Version: v,
		}
		if err == nil {
			iv.InstalledAt = meta.InstalledAt
		}
		result = append(result, iv)
	}

	return result, nil
}
