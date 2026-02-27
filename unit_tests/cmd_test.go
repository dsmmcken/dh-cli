package tests

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dsmmcken/dh-cli/src/internal/cmd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func execRoot(t *testing.T, args ...string) (stdout string, err error) {
	t.Helper()
	c := cmd.NewRootCmd()
	buf := new(bytes.Buffer)
	c.SetOut(buf)
	c.SetErr(buf)
	c.SetArgs(args)
	err = c.Execute()
	return buf.String(), err
}

func TestVersion(t *testing.T) {
	out, err := execRoot(t, "--version")
	require.NoError(t, err)
	assert.Contains(t, out, "dh v")
}

func TestHelp(t *testing.T) {
	out, err := execRoot(t, "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "Usage:")
	assert.Contains(t, out, "dh [")
}

func TestVerboseQuietMutualExclusion(t *testing.T) {
	_, err := execRoot(t, "--verbose", "--quiet")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestJSONFlag(t *testing.T) {
	_, err := execRoot(t, "--json", "--help")
	assert.NoError(t, err)
}

func TestUnknownArgs(t *testing.T) {
	_, err := execRoot(t, "nonexistent")
	require.Error(t, err)
}

func TestHelpListsCommands(t *testing.T) {
	out, err := execRoot(t, "--help")
	require.NoError(t, err)
	assert.True(t, strings.Contains(out, "Usage:") || strings.Contains(out, "Available Commands:"))
}
