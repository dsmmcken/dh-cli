package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Config represents the ~/.dh/config.toml file.
type Config struct {
	DefaultVersion string  `toml:"default_version,omitempty" json:"default_version"`
	Install        Install `toml:"install,omitempty" json:"install"`
}

// Install holds installation preferences.
type Install struct {
	Plugins       []string `toml:"plugins,omitempty" json:"plugins"`
	PythonVersion string   `toml:"python_version,omitempty" json:"python_version"`
}

// configDirOverride is set by the --config-dir flag or DH_HOME env var.
var configDirOverride string

// SetConfigDir allows the CLI to pass in the --config-dir / DH_HOME value.
func SetConfigDir(dir string) {
	configDirOverride = dir
}

// DHHome returns the config directory path.
// Precedence: --config-dir flag / SetConfigDir > DH_HOME env > ~/.dh
func DHHome() string {
	if configDirOverride != "" {
		return configDirOverride
	}
	if v := os.Getenv("DH_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".dh")
	}
	return filepath.Join(home, ".dh")
}

// ConfigPath returns the full path to config.toml.
func ConfigPath() string {
	return filepath.Join(DHHome(), "config.toml")
}

// EnsureDir creates the DHG home directory if it does not exist.
func EnsureDir() error {
	return os.MkdirAll(DHHome(), 0o755)
}

// Load reads config.toml and returns a Config struct.
// If the file does not exist, it returns a zero-value Config (defaults).
func Load() (*Config, error) {
	cfg := &Config{}
	data, err := os.ReadFile(ConfigPath())
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config.toml: %w", err)
	}
	return cfg, nil
}

// Save writes the Config struct back to config.toml.
func Save(cfg *Config) error {
	if err := EnsureDir(); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	return os.WriteFile(ConfigPath(), data, 0o644)
}

// validKeys lists the dot-separated keys that can be used with Get/Set.
var validKeys = map[string]bool{
	"default_version":        true,
	"install.plugins":        true,
	"install.python_version": true,
}

// Get retrieves a single config value by dot-separated key.
func Get(key string) (string, error) {
	if !validKeys[key] {
		return "", fmt.Errorf("unknown config key: %s", key)
	}
	cfg, err := Load()
	if err != nil {
		return "", err
	}
	return getField(cfg, key)
}

// Set sets a single config value by dot-separated key.
func Set(key, value string) error {
	if !validKeys[key] {
		return fmt.Errorf("unknown config key: %s", key)
	}
	cfg, err := Load()
	if err != nil {
		return err
	}
	if err := setField(cfg, key, value); err != nil {
		return err
	}
	return Save(cfg)
}

func getField(cfg *Config, key string) (string, error) {
	switch key {
	case "default_version":
		return cfg.DefaultVersion, nil
	case "install.plugins":
		return strings.Join(cfg.Install.Plugins, ","), nil
	case "install.python_version":
		return cfg.Install.PythonVersion, nil
	default:
		return "", fmt.Errorf("unknown config key: %s", key)
	}
}

func setField(cfg *Config, key, value string) error {
	switch key {
	case "default_version":
		cfg.DefaultVersion = value
	case "install.plugins":
		if value == "" {
			cfg.Install.Plugins = nil
		} else {
			cfg.Install.Plugins = strings.Split(value, ",")
		}
	case "install.python_version":
		cfg.Install.PythonVersion = value
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}
	return nil
}
