package versions

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// Meta represents the meta.toml file stored in each version directory.
type Meta struct {
	InstalledAt time.Time `toml:"installed_at" json:"installed_at"`
}

// ReadMeta reads the meta.toml file from a version directory.
func ReadMeta(versionDir string) (*Meta, error) {
	data, err := os.ReadFile(filepath.Join(versionDir, "meta.toml"))
	if err != nil {
		return nil, fmt.Errorf("reading meta.toml: %w", err)
	}
	var m Meta
	if err := toml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing meta.toml: %w", err)
	}
	return &m, nil
}

// WriteMeta writes the meta.toml file to a version directory.
func WriteMeta(versionDir string, meta *Meta) error {
	data, err := toml.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshaling meta.toml: %w", err)
	}
	return os.WriteFile(filepath.Join(versionDir, "meta.toml"), data, 0o644)
}
