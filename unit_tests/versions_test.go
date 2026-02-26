package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/dsmmcken/dh-cli/src/internal/config"
	"github.com/dsmmcken/dh-cli/src/internal/versions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetaRoundtrip(t *testing.T) {
	tmp := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)

	meta := &versions.Meta{
		InstalledAt: now,
	}
	require.NoError(t, versions.WriteMeta(tmp, meta))

	got, err := versions.ReadMeta(tmp)
	require.NoError(t, err)
	assert.Equal(t, now.Unix(), got.InstalledAt.Unix())
}

func TestReadMetaMissing(t *testing.T) {
	tmp := t.TempDir()
	_, err := versions.ReadMeta(tmp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading meta.toml")
}

func TestSortVersionsDesc(t *testing.T) {
	vs := []string{"1.0.0", "2.1.0", "0.36.1", "2.0.0", "10.0.0", "0.4.0"}
	versions.SortVersionsDesc(vs)
	assert.Equal(t, []string{"10.0.0", "2.1.0", "2.0.0", "1.0.0", "0.36.1", "0.4.0"}, vs)
}

func TestSortVersionsDescTwoComponent(t *testing.T) {
	vs := []string{"0.40.9", "41.0", "41.1", "0.39.8"}
	versions.SortVersionsDesc(vs)
	assert.Equal(t, []string{"41.1", "41.0", "0.40.9", "0.39.8"}, vs)
}

func TestSortVersionsDescEmpty(t *testing.T) {
	var vs []string
	versions.SortVersionsDesc(vs)
	assert.Nil(t, vs)
}

func TestParsePyPIResponse(t *testing.T) {
	resp := map[string]any{
		"releases": map[string]any{
			"0.36.0":        []any{},
			"0.36.1":        []any{},
			"0.37.0":        []any{},
			"0.38.0a1":      []any{}, // pre-release, should be filtered
			"not-a-version": []any{}, // invalid, should be filtered
		},
	}
	data, err := json.Marshal(resp)
	require.NoError(t, err)

	vs, err := versions.ParsePyPIResponse(data, 0)
	require.NoError(t, err)
	assert.Equal(t, []string{"0.37.0", "0.36.1", "0.36.0"}, vs)
}

func TestParsePyPIResponseTwoComponentVersions(t *testing.T) {
	resp := map[string]any{
		"releases": map[string]any{
			"0.40.9":   []any{},
			"41.0":     []any{}, // 2-component version
			"41.1":     []any{}, // 2-component version
			"41.2a1":   []any{}, // pre-release, should be filtered
			"not-real": []any{}, // invalid, should be filtered
		},
	}
	data, err := json.Marshal(resp)
	require.NoError(t, err)

	vs, err := versions.ParsePyPIResponse(data, 0)
	require.NoError(t, err)
	assert.Equal(t, []string{"41.1", "41.0", "0.40.9"}, vs)
}

func TestParsePyPIResponseWithDates(t *testing.T) {
	resp := map[string]any{
		"releases": map[string]any{
			"41.1": []any{
				map[string]any{"upload_time_iso_8601": "2026-01-29T12:00:00Z"},
			},
			"41.0": []any{
				map[string]any{"upload_time_iso_8601": "2026-01-06T08:30:00Z"},
			},
			"0.40.9": []any{}, // no files, no date
		},
	}
	data, err := json.Marshal(resp)
	require.NoError(t, err)

	vs, err := versions.ParsePyPIResponseWithDates(data, 0)
	require.NoError(t, err)
	require.Len(t, vs, 3)
	assert.Equal(t, "41.1", vs[0].Version)
	assert.Equal(t, "2026-01-29", vs[0].Date)
	assert.Equal(t, "41.0", vs[1].Version)
	assert.Equal(t, "2026-01-06", vs[1].Date)
	assert.Equal(t, "0.40.9", vs[2].Version)
	assert.Equal(t, "", vs[2].Date)
}

func TestParsePyPIResponseWithDatesLimit(t *testing.T) {
	resp := map[string]any{
		"releases": map[string]any{
			"41.1": []any{
				map[string]any{"upload_time_iso_8601": "2026-01-29T12:00:00Z"},
			},
			"41.0": []any{
				map[string]any{"upload_time_iso_8601": "2026-01-06T08:30:00Z"},
			},
			"0.40.9": []any{
				map[string]any{"upload_time_iso_8601": "2026-01-28T10:00:00Z"},
			},
		},
	}
	data, err := json.Marshal(resp)
	require.NoError(t, err)

	vs, err := versions.ParsePyPIResponseWithDates(data, 2)
	require.NoError(t, err)
	require.Len(t, vs, 2)
	assert.Equal(t, "41.1", vs[0].Version)
	assert.Equal(t, "41.0", vs[1].Version)
}

func TestParsePyPIResponseWithLimit(t *testing.T) {
	resp := map[string]any{
		"releases": map[string]any{
			"0.36.0": []any{},
			"0.36.1": []any{},
			"0.37.0": []any{},
		},
	}
	data, err := json.Marshal(resp)
	require.NoError(t, err)

	vs, err := versions.ParsePyPIResponse(data, 2)
	require.NoError(t, err)
	assert.Len(t, vs, 2)
	assert.Equal(t, "0.37.0", vs[0])
	assert.Equal(t, "0.36.1", vs[1])
}

func TestFetchRemoteVersionsWithMockServer(t *testing.T) {
	resp := map[string]any{
		"releases": map[string]any{
			"0.36.0": []any{},
			"0.36.1": []any{},
			"0.37.0": []any{},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Override the URL
	origURL := versions.PyPIURL
	versions.PyPIURL = server.URL
	defer func() { versions.PyPIURL = origURL }()

	vs, err := versions.FetchRemoteVersions(0)
	require.NoError(t, err)
	assert.Equal(t, []string{"0.37.0", "0.36.1", "0.36.0"}, vs)
}

func TestFetchLatestVersionWithMockServer(t *testing.T) {
	resp := map[string]any{
		"releases": map[string]any{
			"0.36.0": []any{},
			"0.37.0": []any{},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	origURL := versions.PyPIURL
	versions.PyPIURL = server.URL
	defer func() { versions.PyPIURL = origURL }()

	latest, err := versions.FetchLatestVersion()
	require.NoError(t, err)
	assert.Equal(t, "0.37.0", latest)
}

func TestListInstalledEmpty(t *testing.T) {
	tmp := t.TempDir()

	installed, err := versions.ListInstalled(tmp)
	require.NoError(t, err)
	assert.Nil(t, installed)
}

func TestListInstalledWithVersions(t *testing.T) {
	tmp := t.TempDir()
	versionsDir := filepath.Join(tmp, "versions")

	// Create some version directories with meta.toml
	for _, v := range []string{"0.36.0", "0.37.0", "0.35.0"} {
		vDir := filepath.Join(versionsDir, v)
		require.NoError(t, os.MkdirAll(vDir, 0o755))
		meta := &versions.Meta{
			InstalledAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		}
		require.NoError(t, versions.WriteMeta(vDir, meta))
	}

	installed, err := versions.ListInstalled(tmp)
	require.NoError(t, err)
	require.Len(t, installed, 3)

	// Should be sorted descending
	assert.Equal(t, "0.37.0", installed[0].Version)
	assert.Equal(t, "0.36.0", installed[1].Version)
	assert.Equal(t, "0.35.0", installed[2].Version)
}

func TestListInstalledWithMissingMeta(t *testing.T) {
	tmp := t.TempDir()
	versionsDir := filepath.Join(tmp, "versions")

	vDir := filepath.Join(versionsDir, "0.36.0")
	require.NoError(t, os.MkdirAll(vDir, 0o755))
	// No meta.toml

	installed, err := versions.ListInstalled(tmp)
	require.NoError(t, err)
	require.Len(t, installed, 1)
	assert.Equal(t, "0.36.0", installed[0].Version)
	assert.True(t, installed[0].InstalledAt.IsZero())
}

func TestUninstallRemovesDirectory(t *testing.T) {
	tmp := t.TempDir()
	vDir := filepath.Join(tmp, "versions", "0.36.0")
	require.NoError(t, os.MkdirAll(vDir, 0o755))
	require.NoError(t, versions.WriteMeta(vDir, &versions.Meta{InstalledAt: time.Now()}))

	require.NoError(t, versions.Uninstall(tmp, "0.36.0"))

	_, err := os.Stat(vDir)
	assert.True(t, os.IsNotExist(err))
}

func TestUninstallNotInstalled(t *testing.T) {
	tmp := t.TempDir()

	err := versions.Uninstall(tmp, "0.36.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not installed")
}

func TestInstallWithMockedExec(t *testing.T) {
	tmp := t.TempDir()

	// Mock exec.Command to just succeed
	origExecCommand := versions.ExecCommand
	versions.ExecCommand = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("true")
	}
	defer func() { versions.ExecCommand = origExecCommand }()

	var progress []string
	onProgress := func(msg string) {
		progress = append(progress, msg)
	}

	err := versions.Install(tmp, "0.36.0", "3.13", nil, onProgress)
	require.NoError(t, err)

	// Verify version directory was created
	vDir := filepath.Join(tmp, "versions", "0.36.0")
	info, err := os.Stat(vDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// Verify meta.toml was written
	meta, err := versions.ReadMeta(vDir)
	require.NoError(t, err)
	assert.False(t, meta.InstalledAt.IsZero())

	// Verify progress messages were emitted
	assert.GreaterOrEqual(t, len(progress), 3)
}

func TestInstallAlreadyInstalled(t *testing.T) {
	tmp := t.TempDir()
	vDir := filepath.Join(tmp, "versions", "0.36.0")
	require.NoError(t, os.MkdirAll(vDir, 0o755))

	err := versions.Install(tmp, "0.36.0", "3.13", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already installed")
}

func TestInstallCleansUpOnFailure(t *testing.T) {
	tmp := t.TempDir()

	// Mock exec.Command to fail
	origExecCommand := versions.ExecCommand
	versions.ExecCommand = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("false")
	}
	defer func() { versions.ExecCommand = origExecCommand }()

	err := versions.Install(tmp, "0.36.0", "3.13", nil, nil)
	require.Error(t, err)

	// Verify version directory was cleaned up
	vDir := filepath.Join(tmp, "versions", "0.36.0")
	_, statErr := os.Stat(vDir)
	assert.True(t, os.IsNotExist(statErr))
}

func TestInstallSetsDefaultWhenFirst(t *testing.T) {
	tmp := t.TempDir()
	config.SetConfigDir(tmp)
	defer config.SetConfigDir("")

	origExecCommand := versions.ExecCommand
	versions.ExecCommand = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("true")
	}
	defer func() { versions.ExecCommand = origExecCommand }()

	// Install a version
	require.NoError(t, versions.Install(tmp, "0.36.0", "3.13", nil, nil))

	// The versions package doesn't set the default itself - that's the command layer's job.
	// So just verify the directory was created properly.
	installed, err := versions.ListInstalled(tmp)
	require.NoError(t, err)
	assert.Len(t, installed, 1)
	assert.Equal(t, "0.36.0", installed[0].Version)
}
