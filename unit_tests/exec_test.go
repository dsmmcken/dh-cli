package tests

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	dhexec "github.com/dsmmcken/dh-cli/src/internal/exec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecConfigValidation_BothCodeAndScript(t *testing.T) {
	cfg := &dhexec.ExecConfig{
		Code:       "print('hello')",
		ScriptPath: "script.py",
	}
	exitCode, _, err := dhexec.Run(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot use both -c and a script file")
	assert.Equal(t, 1, exitCode)
}

func TestExecConfigValidation_NeitherCodeNorScript(t *testing.T) {
	cfg := &dhexec.ExecConfig{}
	exitCode, _, err := dhexec.Run(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must provide either -c CODE or a script file")
	assert.Equal(t, 1, exitCode)
}

func TestExecConfigValidation_CodeAlone(t *testing.T) {
	// This will fail at version resolution (no installed versions),
	// but should pass input validation
	cfg := &dhexec.ExecConfig{
		Code:      "print('hello')",
		ConfigDir: t.TempDir(),
	}
	_, _, err := dhexec.Run(cfg)
	require.Error(t, err)
	// Should fail at version resolution, not input validation
	assert.Contains(t, err.Error(), "resolving version")
}

func TestExecConfigValidation_ScriptAlone(t *testing.T) {
	// Create a temp script file
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.py")
	require.NoError(t, os.WriteFile(scriptPath, []byte("print('hello')"), 0o644))

	cfg := &dhexec.ExecConfig{
		ScriptPath: scriptPath,
		ConfigDir:  t.TempDir(),
	}
	_, _, err := dhexec.Run(cfg)
	require.Error(t, err)
	// Should fail at version resolution, not input validation
	assert.Contains(t, err.Error(), "resolving version")
}

func TestExecEmptyCode(t *testing.T) {
	cfg := &dhexec.ExecConfig{
		Code: "   ",
	}
	exitCode, _, err := dhexec.Run(cfg)
	assert.NoError(t, err)
	assert.Equal(t, 0, exitCode)
}

func TestExecEmptyCodeJSON(t *testing.T) {
	cfg := &dhexec.ExecConfig{
		Code:     "   ",
		JSONMode: true,
	}
	exitCode, jsonResult, err := dhexec.Run(cfg)
	assert.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	require.NotNil(t, jsonResult)
	assert.Equal(t, 0, jsonResult["exit_code"])
	assert.Equal(t, "", jsonResult["stdout"])
}

func TestFindVenvPython(t *testing.T) {
	// Create a fake venv directory structure
	tmpDir := t.TempDir()
	version := "0.35.1"

	var pythonPath string
	if runtime.GOOS == "windows" {
		pythonPath = filepath.Join(tmpDir, "versions", version, ".venv", "Scripts", "python.exe")
	} else {
		pythonPath = filepath.Join(tmpDir, "versions", version, ".venv", "bin", "python")
	}

	require.NoError(t, os.MkdirAll(filepath.Dir(pythonPath), 0o755))
	require.NoError(t, os.WriteFile(pythonPath, []byte("fake python"), 0o755))

	result, err := dhexec.FindVenvPython(tmpDir, version)
	require.NoError(t, err)
	assert.Equal(t, pythonPath, result)
}

func TestFindVenvPython_NotInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := dhexec.FindVenvPython(tmpDir, "0.99.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "venv python not found")
	assert.Contains(t, err.Error(), "0.99.0")
}

func TestModeDetection_NoHost(t *testing.T) {
	// No --host means embedded mode
	cfg := &dhexec.ExecConfig{
		Code:      "print('hello')",
		ConfigDir: t.TempDir(),
	}
	_, _, err := dhexec.Run(cfg)
	require.Error(t, err)
	// Embedded mode fails at version resolution
	assert.Contains(t, err.Error(), "resolving version")
}

func TestModeDetection_WithHost(t *testing.T) {
	// With --host means remote mode
	cfg := &dhexec.ExecConfig{
		Code:      "print('hello')",
		Host:      "remote.example.com",
		ConfigDir: t.TempDir(),
	}
	_, _, err := dhexec.Run(cfg)
	require.Error(t, err)
	// Remote mode still fails at version resolution
	assert.Contains(t, err.Error(), "resolving version")
}

func TestRunnerScriptEmbedded(t *testing.T) {
	script := dhexec.RunnerScript()
	require.NotEmpty(t, script)
	assert.Contains(t, script, "def main():")
	assert.Contains(t, script, "argparse")
	assert.Contains(t, script, "build_wrapper")
	assert.Contains(t, script, "get_assigned_names")
	assert.Contains(t, script, "read_result_table")
	assert.Contains(t, script, "get_table_preview")
	assert.Contains(t, script, "--mode")
	assert.Contains(t, script, "--output-json")
}

func TestExecCommandHelp(t *testing.T) {
	out, err := execRoot(t, "exec", "--help")
	require.NoError(t, err)
	assert.True(t, strings.Contains(out, "-c") || strings.Contains(out, "--code"),
		"help should mention -c flag")
	assert.Contains(t, out, "--timeout")
	assert.Contains(t, out, "--host")
	assert.Contains(t, out, "--port")
	assert.Contains(t, out, "exec")
}

func TestExecCommandInRootHelp(t *testing.T) {
	out, err := execRoot(t, "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "exec")
}
