package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dsmmcken/dh-cli/src/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withTempDHGHome(t *testing.T) (string, func()) {
	t.Helper()
	tmp := t.TempDir()
	config.SetConfigDir(tmp)
	return tmp, func() { config.SetConfigDir("") }
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	_, cleanup := withTempDHGHome(t)
	defer cleanup()

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "", cfg.DefaultVersion)
	assert.Nil(t, cfg.Install.Plugins)
	assert.Equal(t, "", cfg.Install.PythonVersion)
}

func TestLoadValidConfig(t *testing.T) {
	tmp, cleanup := withTempDHGHome(t)
	defer cleanup()

	content := `default_version = "42.0"

[install]
plugins = ["plugin-a", "plugin-b"]
python_version = "3.11"
`
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "config.toml"), []byte(content), 0o644))

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "42.0", cfg.DefaultVersion)
	assert.Equal(t, []string{"plugin-a", "plugin-b"}, cfg.Install.Plugins)
	assert.Equal(t, "3.11", cfg.Install.PythonVersion)
}

func TestLoadMalformedTOML(t *testing.T) {
	tmp, cleanup := withTempDHGHome(t)
	defer cleanup()

	require.NoError(t, os.WriteFile(filepath.Join(tmp, "config.toml"), []byte("not valid [[ toml"), 0o644))

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing config.toml")
}

func TestSetThenGetRoundtrip(t *testing.T) {
	_, cleanup := withTempDHGHome(t)
	defer cleanup()

	require.NoError(t, config.Set("default_version", "42.0"))

	val, err := config.Get("default_version")
	require.NoError(t, err)
	assert.Equal(t, "42.0", val)
}

func TestGetUnknownKey(t *testing.T) {
	_, cleanup := withTempDHGHome(t)
	defer cleanup()

	_, err := config.Get("nonexistent_key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown config key")
}

func TestSetUnknownKey(t *testing.T) {
	_, cleanup := withTempDHGHome(t)
	defer cleanup()

	err := config.Set("nonexistent_key", "value")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown config key")
}

func TestEnsureDirCreatesDirectory(t *testing.T) {
	tmp := t.TempDir()
	newDir := filepath.Join(tmp, "subdir", ".dhg")
	config.SetConfigDir(newDir)
	defer config.SetConfigDir("")

	require.NoError(t, config.EnsureDir())

	info, err := os.Stat(newDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestDHGRCFindInCwd(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".dhgrc"), []byte("1.0.0\n"), 0o644))

	path, err := config.FindDHGRC(tmp)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmp, ".dhgrc"), path)
}

func TestDHGRCFindInParent(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".dhgrc"), []byte("2.0.0\n"), 0o644))

	child := filepath.Join(tmp, "subdir")
	require.NoError(t, os.MkdirAll(child, 0o755))

	path, err := config.FindDHGRC(child)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmp, ".dhgrc"), path)
}

func TestDHGRCNotFound(t *testing.T) {
	tmp := t.TempDir()
	// No .dhgrc file anywhere in tmp

	path, err := config.FindDHGRC(tmp)
	require.NoError(t, err)
	assert.Equal(t, "", path)
}

func TestReadDHGRC(t *testing.T) {
	tmp := t.TempDir()
	rcPath := filepath.Join(tmp, ".dhgrc")
	require.NoError(t, os.WriteFile(rcPath, []byte("  3.5.0  \n"), 0o644))

	ver, err := config.ReadDHGRC(rcPath)
	require.NoError(t, err)
	assert.Equal(t, "3.5.0", ver)
}

func TestReadDHGRCEmpty(t *testing.T) {
	tmp := t.TempDir()
	rcPath := filepath.Join(tmp, ".dhgrc")
	require.NoError(t, os.WriteFile(rcPath, []byte("  \n"), 0o644))

	_, err := config.ReadDHGRC(rcPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestWriteDHGRC(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, config.WriteDHGRC(tmp, "4.0.0"))

	data, err := os.ReadFile(filepath.Join(tmp, ".dhgrc"))
	require.NoError(t, err)
	assert.Equal(t, "4.0.0\n", string(data))
}

func TestResolveVersionFlagWins(t *testing.T) {
	_, cleanup := withTempDHGHome(t)
	defer cleanup()

	ver, err := config.ResolveVersion("1.0.0", "2.0.0")
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", ver)
}

func TestResolveVersionEnvWins(t *testing.T) {
	_, cleanup := withTempDHGHome(t)
	defer cleanup()

	ver, err := config.ResolveVersion("", "2.0.0")
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", ver)
}

func TestResolveVersionConfigFallback(t *testing.T) {
	_, cleanup := withTempDHGHome(t)
	defer cleanup()

	require.NoError(t, config.Set("default_version", "5.0.0"))

	ver, err := config.ResolveVersion("", "")
	require.NoError(t, err)
	assert.Equal(t, "5.0.0", ver)
}

func TestResolveVersionLatestInstalled(t *testing.T) {
	tmp, cleanup := withTempDHGHome(t)
	defer cleanup()

	// Create some version directories
	for _, v := range []string{"1.0.0", "3.0.0", "2.0.0"} {
		require.NoError(t, os.MkdirAll(filepath.Join(tmp, "versions", v), 0o755))
	}

	ver, err := config.ResolveVersion("", "")
	require.NoError(t, err)
	assert.Equal(t, "3.0.0", ver)
}

func TestResolveVersionNothingConfigured(t *testing.T) {
	_, cleanup := withTempDHGHome(t)
	defer cleanup()

	_, err := config.ResolveVersion("", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no Deephaven version configured")
}

func TestConfigPath(t *testing.T) {
	tmp, cleanup := withTempDHGHome(t)
	defer cleanup()

	assert.Equal(t, filepath.Join(tmp, "config.toml"), config.ConfigPath())
}

func TestSetInstallPythonVersion(t *testing.T) {
	_, cleanup := withTempDHGHome(t)
	defer cleanup()

	require.NoError(t, config.Set("install.python_version", "3.12"))
	val, err := config.Get("install.python_version")
	require.NoError(t, err)
	assert.Equal(t, "3.12", val)
}

func TestSetInstallPlugins(t *testing.T) {
	_, cleanup := withTempDHGHome(t)
	defer cleanup()

	require.NoError(t, config.Set("install.plugins", "a,b,c"))
	val, err := config.Get("install.plugins")
	require.NoError(t, err)
	assert.Equal(t, "a,b,c", val)
}
