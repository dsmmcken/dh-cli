package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// ResolveVersion determines which Deephaven version to use.
// Precedence:
//  1. flagVersion (from --version flag)
//  2. envVersion (from DH_VERSION env var)
//  3. .dhrc walk-up from cwd
//  4. config.toml default_version
//  5. Latest installed version (scan ~/.dh/versions/)
func ResolveVersion(flagVersion, envVersion string) (string, error) {
	// 1. Explicit flag
	if flagVersion != "" {
		return flagVersion, nil
	}

	// 2. Environment variable
	if envVersion != "" {
		return envVersion, nil
	}

	// 3. .dhrc walk-up
	cwd, err := os.Getwd()
	if err == nil {
		if rcPath, err := FindDHRC(cwd); err == nil && rcPath != "" {
			if ver, err := ReadDHRC(rcPath); err == nil {
				return ver, nil
			}
		}
	}

	// 4. config.toml default_version
	cfg, err := Load()
	if err == nil && cfg.DefaultVersion != "" {
		return cfg.DefaultVersion, nil
	}

	// 5. Latest installed version
	ver, err := latestInstalledVersion()
	if err == nil {
		return ver, nil
	}

	return "", fmt.Errorf("no Deephaven version configured; use --version, set DH_VERSION, create .dhrc, or run dh install")
}

// latestInstalledVersion scans ~/.dh/versions/ and returns the latest
// directory name (sorted lexicographically, last = latest).
func latestInstalledVersion() (string, error) {
	versionsDir := filepath.Join(DHHome(), "versions")
	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		return "", err
	}

	var versions []string
	for _, e := range entries {
		if e.IsDir() {
			versions = append(versions, e.Name())
		}
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("no versions installed in %s", versionsDir)
	}

	sort.Strings(versions)
	return versions[len(versions)-1], nil
}
