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

// stubAllDoctorCheckers replaces all doctor checkers with stubs and returns a restore function.
func stubAllDoctorCheckers(t *testing.T) func() {
	t.Helper()
	origUV := cmd.UVChecker
	origJava := cmd.JavaChecker
	origNode := cmd.NodeChecker
	origVersions := cmd.VersionsChecker
	origDefault := cmd.DefaultVersionChecker
	origDisk := cmd.DiskSpaceChecker
	origUffd := cmd.UffdChecker

	cmd.UVChecker = func() cmd.CheckResult {
		return cmd.CheckResult{Name: "uv", Status: "ok", Detail: "/usr/bin/uv (0.5.14)"}
	}
	cmd.JavaChecker = func(string) cmd.CheckResult {
		return cmd.CheckResult{Name: "Java", Status: "ok", Detail: "21.0.5 (JAVA_HOME)"}
	}
	cmd.NodeChecker = func() cmd.CheckResult {
		return cmd.CheckResult{Name: "Node.js", Status: "ok", Detail: "v22.1.0 (required for dh render)"}
	}
	cmd.VersionsChecker = func(string) cmd.CheckResult {
		return cmd.CheckResult{Name: "Versions", Status: "ok", Detail: "2 installed"}
	}
	cmd.DefaultVersionChecker = func(string) cmd.CheckResult {
		return cmd.CheckResult{Name: "Default", Status: "ok", Detail: "42.0"}
	}
	cmd.DiskSpaceChecker = func(string) cmd.CheckResult {
		return cmd.CheckResult{Name: "Disk", Status: "ok", Detail: "50.0 GB free in ~/.dh"}
	}
	cmd.UffdChecker = func() cmd.CheckResult {
		return cmd.CheckResult{Name: "VM/UFFD", Status: "ok", Detail: "userfaultfd available"}
	}

	return func() {
		cmd.UVChecker = origUV
		cmd.JavaChecker = origJava
		cmd.NodeChecker = origNode
		cmd.VersionsChecker = origVersions
		cmd.DefaultVersionChecker = origDefault
		cmd.DiskSpaceChecker = origDisk
		cmd.UffdChecker = origUffd
	}
}

func TestDoctorJSONOutput(t *testing.T) {
	defer stubAllDoctorCheckers(t)()

	out, err := execRoot(t, "doctor", "--json")
	require.NoError(t, err)

	var report cmd.DoctorReport
	require.NoError(t, json.Unmarshal([]byte(out), &report))

	assert.True(t, report.Healthy)
	assert.Len(t, report.Checks, 7)

	for _, c := range report.Checks {
		assert.NotEmpty(t, c.Name)
		assert.NotEmpty(t, c.Status)
		assert.NotEmpty(t, c.Detail)
	}
}

func TestDoctorHealthyFalseOnError(t *testing.T) {
	defer stubAllDoctorCheckers(t)()

	cmd.UVChecker = func() cmd.CheckResult {
		return cmd.CheckResult{Name: "uv", Status: "error", Detail: "not found"}
	}

	out, err := execRoot(t, "doctor", "--json")
	require.NoError(t, err)

	var report cmd.DoctorReport
	require.NoError(t, json.Unmarshal([]byte(out), &report))

	assert.False(t, report.Healthy)
}

func TestDoctorHealthyTrueWithWarnings(t *testing.T) {
	defer stubAllDoctorCheckers(t)()

	cmd.VersionsChecker = func(string) cmd.CheckResult {
		return cmd.CheckResult{Name: "Versions", Status: "warning", Detail: "0 installed"}
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
	defer stubAllDoctorCheckers(t)()

	out, err := execRoot(t, "doctor")
	require.NoError(t, err)

	assert.Contains(t, out, "Deephaven CLI Doctor")
	assert.Contains(t, out, "uv")
	assert.Contains(t, out, "Java")
	assert.Contains(t, out, "Node.js")
	assert.Contains(t, out, "Versions")
	assert.Contains(t, out, "Default")
	assert.Contains(t, out, "Disk")
	assert.Contains(t, out, "Everything looks good.")
}

func TestDoctorHumanOutputWithWarnings(t *testing.T) {
	defer stubAllDoctorCheckers(t)()

	cmd.VersionsChecker = func(string) cmd.CheckResult {
		return cmd.CheckResult{Name: "Versions", Status: "warning", Detail: "0 installed"}
	}

	out, err := execRoot(t, "doctor")
	require.NoError(t, err)

	assert.Contains(t, out, "Everything looks good (1 warning).")
}
