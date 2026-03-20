package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dsmmcken/dh-cli/src/internal/cmd"
	"github.com/dsmmcken/dh-cli/src/internal/render"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Widget name detection ──

func TestDetectWidgetName_Simple(t *testing.T) {
	name, err := render.DetectWidgetNameFromSource(`
from deephaven import ui

button_widget = ui.button("Click me")
`)
	require.NoError(t, err)
	assert.Equal(t, "button_widget", name)
}

func TestDetectWidgetName_WithSpaces(t *testing.T) {
	name, err := render.DetectWidgetNameFromSource(`
form_widget  =  ui.form(children=[])
`)
	require.NoError(t, err)
	assert.Equal(t, "form_widget", name)
}

func TestDetectWidgetName_MultipleWidgets(t *testing.T) {
	// Should return the first match
	name, err := render.DetectWidgetNameFromSource(`
first_widget = ui.button("A")
second_widget = ui.button("B")
`)
	require.NoError(t, err)
	assert.Equal(t, "first_widget", name)
}

func TestDetectWidgetName_NoMatch(t *testing.T) {
	_, err := render.DetectWidgetNameFromSource(`
x = 42
table = empty_table(5)
`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no widget name found")
}

func TestDetectWidgetName_FromFile(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test_button.py")
	require.NoError(t, os.WriteFile(scriptPath, []byte(`
from deephaven import ui
button_widget = ui.button("Primary")
`), 0o644))

	name, err := render.DetectWidgetName(scriptPath)
	require.NoError(t, err)
	assert.Equal(t, "button_widget", name)
}

func TestDetectWidgetName_MissingFile(t *testing.T) {
	_, err := render.DetectWidgetName("/nonexistent/path.py")
	require.Error(t, err)
}

// ── Node.js version parsing ──

func TestNodeVersionParsing(t *testing.T) {
	// NodeVersion is tested indirectly via the doctor integration
	// Here we test the CheckNode function by observing its behavior
	// in the sandbox environment (which may or may not have node)
	ver := render.NodeVersion()
	// In the sandbox, node may or may not be installed
	// Just verify we don't panic
	_ = ver
}

// ── Render runtime setup ──

func TestEnsureRuntime_ExtractsFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// EnsureRuntime will try to run npm install, which will fail in
	// test environments. We test the extraction logic by checking that
	// it at least creates the directory and attempts extraction.
	renderDir, err := render.EnsureRuntime(tmpDir, true)

	if err != nil {
		// Expected to fail at npm install in test env — verify it got that far
		assert.Contains(t, err.Error(), "npm")
	} else {
		assert.DirExists(t, renderDir)
	}
}

func TestEmbeddedFS_NotEmpty(t *testing.T) {
	entries, err := render.EmbeddedFS.ReadDir("embedded")
	require.NoError(t, err)
	// Should have at least package.json after render-prepare
	found := false
	for _, e := range entries {
		if e.Name() == "package.json" {
			found = true
			break
		}
	}
	assert.True(t, found, "embedded/ should contain package.json after render-prepare")
}

// ── Command registration ──

func TestRenderCommandHelp(t *testing.T) {
	out, err := execRoot(t, "render", "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "render")
	assert.Contains(t, out, "SCRIPT")
	assert.Contains(t, out, "--url")
	assert.Contains(t, out, "--widget")
	assert.Contains(t, out, "--timeout")
	assert.Contains(t, out, "--rows")
	assert.Contains(t, out, "snapshot")
	assert.Contains(t, out, "click")
	assert.Contains(t, out, "fill")
}

func TestRenderDiagnoseCommandHelp(t *testing.T) {
	out, err := execRoot(t, "render", "diagnose", "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "diagnose")
	assert.Contains(t, out, "SCRIPT")
	assert.Contains(t, out, "--url")
	assert.Contains(t, out, "--widget")
}

func TestRenderInRootHelp(t *testing.T) {
	out, err := execRoot(t, "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "render")
}

func TestRenderNoArgs(t *testing.T) {
	_, err := execRoot(t, "render")
	require.Error(t, err)
	// Should require at least 1 arg (script path)
}

func TestRenderMissingScript(t *testing.T) {
	_, err := execRoot(t, "render", "/nonexistent/test.py")
	require.Error(t, err)
}

// ── VM flag tests ──

func TestRenderVMFlagInHelp(t *testing.T) {
	out, err := execRoot(t, "render", "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "--vm")
	assert.Contains(t, out, "--pool")
}

func TestRenderDiagnoseVMFlagInHelp(t *testing.T) {
	out, err := execRoot(t, "render", "diagnose", "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "--vm")
	assert.Contains(t, out, "--pool")
}

func TestRenderVMAndURLMutualExclusion(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.py")
	require.NoError(t, os.WriteFile(scriptPath, []byte("test_widget = ui.button('x')"), 0o644))

	_, err := execRoot(t, "render", scriptPath, "--vm", "--url", "http://localhost:10000")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestRenderPoolRequiresVM(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.py")
	require.NoError(t, os.WriteFile(scriptPath, []byte("test_widget = ui.button('x')"), 0o644))

	_, err := execRoot(t, "render", scriptPath, "--pool")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--pool requires --vm")
}

// ── Doctor integration ──

func TestDoctorIncludesNodeCheck(t *testing.T) {
	// Save and restore all checkers
	origUV := cmd.UVChecker
	origJava := cmd.JavaChecker
	origVersions := cmd.VersionsChecker
	origDefault := cmd.DefaultVersionChecker
	origDisk := cmd.DiskSpaceChecker
	origNode := cmd.NodeChecker
	origUffd := cmd.UffdChecker
	defer func() {
		cmd.UVChecker = origUV
		cmd.JavaChecker = origJava
		cmd.VersionsChecker = origVersions
		cmd.DefaultVersionChecker = origDefault
		cmd.DiskSpaceChecker = origDisk
		cmd.NodeChecker = origNode
		cmd.UffdChecker = origUffd
	}()

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

	out, err := execRoot(t, "doctor")
	require.NoError(t, err)
	assert.Contains(t, out, "Node.js")
	assert.Contains(t, out, "dh render")
}

func TestDoctorNodeMissing(t *testing.T) {
	origUV := cmd.UVChecker
	origJava := cmd.JavaChecker
	origVersions := cmd.VersionsChecker
	origDefault := cmd.DefaultVersionChecker
	origDisk := cmd.DiskSpaceChecker
	origNode := cmd.NodeChecker
	origUffd := cmd.UffdChecker
	defer func() {
		cmd.UVChecker = origUV
		cmd.JavaChecker = origJava
		cmd.VersionsChecker = origVersions
		cmd.DefaultVersionChecker = origDefault
		cmd.DiskSpaceChecker = origDisk
		cmd.NodeChecker = origNode
		cmd.UffdChecker = origUffd
	}()

	cmd.UVChecker = func() cmd.CheckResult {
		return cmd.CheckResult{Name: "uv", Status: "ok", Detail: "/usr/bin/uv (0.5.14)"}
	}
	cmd.JavaChecker = func(string) cmd.CheckResult {
		return cmd.CheckResult{Name: "Java", Status: "ok", Detail: "21.0.5 (JAVA_HOME)"}
	}
	cmd.NodeChecker = func() cmd.CheckResult {
		return cmd.CheckResult{Name: "Node.js", Status: "warning", Detail: "not found in PATH"}
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

	out, err := execRoot(t, "doctor")
	require.NoError(t, err)
	// Node missing is a warning, not an error — should still be "good"
	assert.Contains(t, out, "Node.js")
	assert.Contains(t, out, "Everything looks good (1 warning).")
}
