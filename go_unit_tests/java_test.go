package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dsmmcken/dh-cli/go_src/internal/java"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseVersion_OpenJDK21(t *testing.T) {
	output := `openjdk version "21.0.5" 2024-10-15
OpenJDK Runtime Environment Temurin-21.0.5+11 (build 21.0.5+11)
OpenJDK 64-Bit Server VM Temurin-21.0.5+11 (build 21.0.5+11, mixed mode, sharing)`
	v, err := java.ParseVersion(output)
	require.NoError(t, err)
	assert.Equal(t, "21.0.5", v)
}

func TestParseVersion_OracleJDK17(t *testing.T) {
	output := `java version "17.0.2" 2022-01-18 LTS
Java(TM) SE Runtime Environment (build 17.0.2+8-LTS-86)
Java HotSpot(TM) 64-Bit Server VM (build 17.0.2+8-LTS-86, mixed mode, sharing)`
	v, err := java.ParseVersion(output)
	require.NoError(t, err)
	assert.Equal(t, "17.0.2", v)
}

func TestParseVersion_OpenJDK11(t *testing.T) {
	output := `openjdk version "11.0.21" 2023-10-17
OpenJDK Runtime Environment (build 11.0.21+9-post-Ubuntu-0ubuntu120.04)
OpenJDK 64-Bit Server VM (build 11.0.21+9-post-Ubuntu-0ubuntu120.04, mixed mode, sharing)`
	v, err := java.ParseVersion(output)
	require.NoError(t, err)
	assert.Equal(t, "11.0.21", v)
}

func TestParseVersion_OldStyle18(t *testing.T) {
	output := `java version "1.8.0_381"
Java(TM) SE Runtime Environment (build 1.8.0_381-b09)
Java HotSpot(TM) 64-Bit Server VM (build 25.381-b09, mixed mode)`
	v, err := java.ParseVersion(output)
	require.NoError(t, err)
	assert.Equal(t, "1.8.0_381", v)
}

func TestParseVersion_Invalid(t *testing.T) {
	_, err := java.ParseVersion("not a valid output")
	require.Error(t, err)
}

func TestParseVersion_Empty(t *testing.T) {
	_, err := java.ParseVersion("")
	require.Error(t, err)
}

func TestMeetsMinimum_21_GTE_17(t *testing.T) {
	assert.True(t, java.MeetsMinimum("21.0.5", 17))
}

func TestMeetsMinimum_17_GTE_17(t *testing.T) {
	assert.True(t, java.MeetsMinimum("17.0.2", 17))
}

func TestMeetsMinimum_11_LT_17(t *testing.T) {
	assert.False(t, java.MeetsMinimum("11.0.21", 17))
}

func TestMeetsMinimum_OldStyle_8_LT_17(t *testing.T) {
	assert.False(t, java.MeetsMinimum("1.8.0_381", 17))
}

func TestMeetsMinimum_Invalid(t *testing.T) {
	assert.False(t, java.MeetsMinimum("invalid", 17))
}

func TestDetect_NoJava(t *testing.T) {
	// Use a temp dir with no Java to ensure nothing is found via managed path.
	// Clear JAVA_HOME and use an empty PATH to prevent system Java detection.
	t.Setenv("JAVA_HOME", "")
	t.Setenv("PATH", t.TempDir())

	info, err := java.Detect(t.TempDir())
	require.NoError(t, err)
	assert.False(t, info.Found)
}

func TestDetect_ManagedInstall(t *testing.T) {
	// Create a fake managed Java installation
	dhgHome := t.TempDir()
	jdkDir := filepath.Join(dhgHome, "java", "jdk-21.0.5+11")
	binDir := filepath.Join(jdkDir, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))

	// Create a fake java binary that outputs version info
	fakeJava := filepath.Join(binDir, "java")
	script := `#!/bin/sh
echo 'openjdk version "21.0.5" 2024-10-15' >&2
echo 'OpenJDK Runtime Environment Temurin-21.0.5+11 (build 21.0.5+11)' >&2
`
	require.NoError(t, os.WriteFile(fakeJava, []byte(script), 0o755))

	// Clear JAVA_HOME and set PATH to empty so only managed path is found
	t.Setenv("JAVA_HOME", "")
	t.Setenv("PATH", t.TempDir())

	info, err := java.Detect(dhgHome)
	require.NoError(t, err)
	assert.True(t, info.Found)
	assert.Equal(t, "21.0.5", info.Version)
	assert.Equal(t, "managed", info.Source)
	assert.Equal(t, fakeJava, info.Path)
}

func TestDetect_JAVA_HOME(t *testing.T) {
	// Create a fake JAVA_HOME
	javaHome := t.TempDir()
	binDir := filepath.Join(javaHome, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))

	fakeJava := filepath.Join(binDir, "java")
	script := `#!/bin/sh
echo 'openjdk version "17.0.2" 2022-01-18' >&2
`
	require.NoError(t, os.WriteFile(fakeJava, []byte(script), 0o755))

	t.Setenv("JAVA_HOME", javaHome)
	t.Setenv("PATH", t.TempDir())

	info, err := java.Detect(t.TempDir())
	require.NoError(t, err)
	assert.True(t, info.Found)
	assert.Equal(t, "17.0.2", info.Version)
	assert.Equal(t, "JAVA_HOME", info.Source)
}
