package java

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// versionRe matches lines like: openjdk version "21.0.5" or java version "17.0.2"
var versionRe = regexp.MustCompile(`(?:openjdk|java) version "([^"]+)"`)

// ParseVersion extracts the version string from `java -version` stderr output.
func ParseVersion(output string) (string, error) {
	m := versionRe.FindStringSubmatch(output)
	if m == nil {
		return "", fmt.Errorf("could not parse java version from output: %s", output)
	}
	return m[1], nil
}

// MeetsMinimum returns true if version's major component is >= min.
func MeetsMinimum(version string, min int) bool {
	major, err := parseMajor(version)
	if err != nil {
		return false
	}
	return major >= min
}

// parseMajor extracts the major version number.
// Handles both old-style (1.8.0_xxx → 8) and new-style (17.0.2 → 17).
func parseMajor(version string) (int, error) {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) == 0 {
		return 0, fmt.Errorf("empty version string")
	}
	first, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid major version: %s", parts[0])
	}
	// Old-style versioning: 1.8.0 → major is 8
	if first == 1 && len(parts) > 1 {
		second, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, fmt.Errorf("invalid minor version: %s", parts[1])
		}
		return second, nil
	}
	return first, nil
}
