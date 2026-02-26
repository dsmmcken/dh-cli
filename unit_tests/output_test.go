package tests

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/dsmmcken/dh-cli/src/internal/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrintJSON(t *testing.T) {
	buf := new(bytes.Buffer)
	err := output.PrintJSON(buf, map[string]string{"key": "value"})
	require.NoError(t, err)

	var result map[string]string
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "value", result["key"])
}

func TestPrintError(t *testing.T) {
	buf := new(bytes.Buffer)
	err := output.PrintError(buf, "test_error", "something went wrong")
	require.NoError(t, err)

	var result map[string]string
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "test_error", result["error"])
	assert.Equal(t, "something went wrong", result["message"])
}

func TestExitCodes(t *testing.T) {
	assert.Equal(t, 0, output.ExitSuccess)
	assert.Equal(t, 1, output.ExitError)
	assert.Equal(t, 2, output.ExitNetwork)
	assert.Equal(t, 3, output.ExitTimeout)
	assert.Equal(t, 4, output.ExitNotFound)
	assert.Equal(t, 130, output.ExitInterrupted)
}

func TestSetAndGetFlags(t *testing.T) {
	output.SetFlags(true, true, false)
	assert.True(t, output.IsJSON())
	assert.True(t, output.IsQuiet())
	assert.False(t, output.IsVerbose())

	output.SetFlags(false, false, true)
	assert.False(t, output.IsJSON())
	assert.False(t, output.IsQuiet())
	assert.True(t, output.IsVerbose())

	// Reset
	output.SetFlags(false, false, false)
}
