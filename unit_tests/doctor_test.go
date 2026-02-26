package tests

import (
	"encoding/json"
	"testing"

	"github.com/dsmmcken/dh-cli/src/internal/cmd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoctorHelpShowsFixFlag(t *testing.T) {
	out, err := execRoot(t, "doctor", "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "--fix")
}

func TestDoctorJSONOutput(t *testing.T) {
	// Save and restore checkers
	origUV := cmd.UVChecker
	origJava := cmd.JavaChecker
	origVersions := cmd.VersionsChecker
	origDefault := cmd.DefaultVersionChecker
	origDisk := cmd.DiskSpaceChecker
	defer func() {
		cmd.UVChecker = origUV
		cmd.JavaChecker = origJava
		cmd.VersionsChecker = origVersions
		cmd.DefaultVersionChecker = origDefault
		cmd.DiskSpaceChecker = origDisk
	}()

	cmd.UVChecker = func() cmd.CheckResult {
		return cmd.CheckResult{Name: "uv", Status: "ok", Detail: "/usr/bin/uv (0.5.14)"}
	}
	cmd.JavaChecker = func(string) cmd.CheckResult {
		return cmd.CheckResult{Name: "Java", Status: "ok", Detail: "21.0.5 (JAVA_HOME)"}
	}
	cmd.VersionsChecker = func(string) cmd.CheckResult {
		return cmd.CheckResult{Name: "Versions", Status: "ok", Detail: "2 installed"}
	}
	cmd.DefaultVersionChecker = func(string) cmd.CheckResult {
		return cmd.CheckResult{Name: "Default", Status: "ok", Detail: "42.0"}
	}
	cmd.DiskSpaceChecker = func(string) cmd.CheckResult {
		return cmd.CheckResult{Name: "Disk", Status: "ok", Detail: "50.0 GB free in ~/.dhg"}
	}

	out, err := execRoot(t, "doctor", "--json")
	require.NoError(t, err)

	var report cmd.DoctorReport
	require.NoError(t, json.Unmarshal([]byte(out), &report))

	assert.True(t, report.Healthy)
	assert.Len(t, report.Checks, 5)

	for _, c := range report.Checks {
		assert.NotEmpty(t, c.Name)
		assert.NotEmpty(t, c.Status)
		assert.NotEmpty(t, c.Detail)
	}
}

func TestDoctorHealthyFalseOnError(t *testing.T) {
	origUV := cmd.UVChecker
	origJava := cmd.JavaChecker
	origVersions := cmd.VersionsChecker
	origDefault := cmd.DefaultVersionChecker
	origDisk := cmd.DiskSpaceChecker
	defer func() {
		cmd.UVChecker = origUV
		cmd.JavaChecker = origJava
		cmd.VersionsChecker = origVersions
		cmd.DefaultVersionChecker = origDefault
		cmd.DiskSpaceChecker = origDisk
	}()

	cmd.UVChecker = func() cmd.CheckResult {
		return cmd.CheckResult{Name: "uv", Status: "error", Detail: "not found"}
	}
	cmd.JavaChecker = func(string) cmd.CheckResult {
		return cmd.CheckResult{Name: "Java", Status: "ok", Detail: "21.0.5 (JAVA_HOME)"}
	}
	cmd.VersionsChecker = func(string) cmd.CheckResult {
		return cmd.CheckResult{Name: "Versions", Status: "ok", Detail: "2 installed"}
	}
	cmd.DefaultVersionChecker = func(string) cmd.CheckResult {
		return cmd.CheckResult{Name: "Default", Status: "ok", Detail: "42.0"}
	}
	cmd.DiskSpaceChecker = func(string) cmd.CheckResult {
		return cmd.CheckResult{Name: "Disk", Status: "ok", Detail: "50.0 GB free"}
	}

	out, err := execRoot(t, "doctor", "--json")
	require.NoError(t, err)

	var report cmd.DoctorReport
	require.NoError(t, json.Unmarshal([]byte(out), &report))

	assert.False(t, report.Healthy)
}

func TestDoctorHealthyTrueWithWarnings(t *testing.T) {
	origUV := cmd.UVChecker
	origJava := cmd.JavaChecker
	origVersions := cmd.VersionsChecker
	origDefault := cmd.DefaultVersionChecker
	origDisk := cmd.DiskSpaceChecker
	defer func() {
		cmd.UVChecker = origUV
		cmd.JavaChecker = origJava
		cmd.VersionsChecker = origVersions
		cmd.DefaultVersionChecker = origDefault
		cmd.DiskSpaceChecker = origDisk
	}()

	cmd.UVChecker = func() cmd.CheckResult {
		return cmd.CheckResult{Name: "uv", Status: "ok", Detail: "/usr/bin/uv (0.5.14)"}
	}
	cmd.JavaChecker = func(string) cmd.CheckResult {
		return cmd.CheckResult{Name: "Java", Status: "ok", Detail: "21.0.5 (JAVA_HOME)"}
	}
	cmd.VersionsChecker = func(string) cmd.CheckResult {
		return cmd.CheckResult{Name: "Versions", Status: "warning", Detail: "0 installed"}
	}
	cmd.DefaultVersionChecker = func(string) cmd.CheckResult {
		return cmd.CheckResult{Name: "Default", Status: "ok", Detail: "42.0"}
	}
	cmd.DiskSpaceChecker = func(string) cmd.CheckResult {
		return cmd.CheckResult{Name: "Disk", Status: "warning", Detail: "2.1 GB free"}
	}

	out, err := execRoot(t, "doctor", "--json")
	require.NoError(t, err)

	var report cmd.DoctorReport
	require.NoError(t, json.Unmarshal([]byte(out), &report))

	assert.True(t, report.Healthy)
}

func TestDoctorHumanOutput(t *testing.T) {
	origUV := cmd.UVChecker
	origJava := cmd.JavaChecker
	origVersions := cmd.VersionsChecker
	origDefault := cmd.DefaultVersionChecker
	origDisk := cmd.DiskSpaceChecker
	defer func() {
		cmd.UVChecker = origUV
		cmd.JavaChecker = origJava
		cmd.VersionsChecker = origVersions
		cmd.DefaultVersionChecker = origDefault
		cmd.DiskSpaceChecker = origDisk
	}()

	cmd.UVChecker = func() cmd.CheckResult {
		return cmd.CheckResult{Name: "uv", Status: "ok", Detail: "/usr/bin/uv (0.5.14)"}
	}
	cmd.JavaChecker = func(string) cmd.CheckResult {
		return cmd.CheckResult{Name: "Java", Status: "ok", Detail: "21.0.5 (JAVA_HOME)"}
	}
	cmd.VersionsChecker = func(string) cmd.CheckResult {
		return cmd.CheckResult{Name: "Versions", Status: "ok", Detail: "2 installed"}
	}
	cmd.DefaultVersionChecker = func(string) cmd.CheckResult {
		return cmd.CheckResult{Name: "Default", Status: "ok", Detail: "42.0"}
	}
	cmd.DiskSpaceChecker = func(string) cmd.CheckResult {
		return cmd.CheckResult{Name: "Disk", Status: "ok", Detail: "50.0 GB free"}
	}

	out, err := execRoot(t, "doctor")
	require.NoError(t, err)

	assert.Contains(t, out, "Deephaven CLI Doctor")
	assert.Contains(t, out, "uv")
	assert.Contains(t, out, "Java")
	assert.Contains(t, out, "Versions")
	assert.Contains(t, out, "Default")
	assert.Contains(t, out, "Disk")
	assert.Contains(t, out, "Everything looks good.")
}

func TestDoctorHumanOutputWithWarnings(t *testing.T) {
	origUV := cmd.UVChecker
	origJava := cmd.JavaChecker
	origVersions := cmd.VersionsChecker
	origDefault := cmd.DefaultVersionChecker
	origDisk := cmd.DiskSpaceChecker
	defer func() {
		cmd.UVChecker = origUV
		cmd.JavaChecker = origJava
		cmd.VersionsChecker = origVersions
		cmd.DefaultVersionChecker = origDefault
		cmd.DiskSpaceChecker = origDisk
	}()

	cmd.UVChecker = func() cmd.CheckResult {
		return cmd.CheckResult{Name: "uv", Status: "ok", Detail: "/usr/bin/uv (0.5.14)"}
	}
	cmd.JavaChecker = func(string) cmd.CheckResult {
		return cmd.CheckResult{Name: "Java", Status: "ok", Detail: "21.0.5 (JAVA_HOME)"}
	}
	cmd.VersionsChecker = func(string) cmd.CheckResult {
		return cmd.CheckResult{Name: "Versions", Status: "warning", Detail: "0 installed"}
	}
	cmd.DefaultVersionChecker = func(string) cmd.CheckResult {
		return cmd.CheckResult{Name: "Default", Status: "ok", Detail: "42.0"}
	}
	cmd.DiskSpaceChecker = func(string) cmd.CheckResult {
		return cmd.CheckResult{Name: "Disk", Status: "ok", Detail: "50.0 GB free"}
	}

	out, err := execRoot(t, "doctor")
	require.NoError(t, err)

	assert.Contains(t, out, "Everything looks good (1 warning).")
}
