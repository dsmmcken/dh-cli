package versions

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

// PyPIURL is the URL to fetch version information. Exported for test overrides.
var PyPIURL = "https://pypi.org/pypi/deephaven-server/json"

// HTTPClient is the HTTP client used for PyPI requests. Exported for test overrides.
var HTTPClient = http.DefaultClient

// semverRegexp matches versions like 1.2.3, 0.36.1, etc.
var semverRegexp = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// pypiResponse is the subset of the PyPI JSON response we care about.
type pypiResponse struct {
	Releases map[string]json.RawMessage `json:"releases"`
}

// FetchRemoteVersions fetches available versions from PyPI, sorted descending by semver.
func FetchRemoteVersions(limit int) ([]string, error) {
	resp, err := HTTPClient.Get(PyPIURL)
	if err != nil {
		return nil, fmt.Errorf("fetching PyPI: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("PyPI returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading PyPI response: %w", err)
	}

	return ParsePyPIResponse(body, limit)
}

// ParsePyPIResponse parses a PyPI JSON response body and returns sorted versions.
func ParsePyPIResponse(data []byte, limit int) ([]string, error) {
	var pypi pypiResponse
	if err := json.Unmarshal(data, &pypi); err != nil {
		return nil, fmt.Errorf("parsing PyPI JSON: %w", err)
	}

	var versions []string
	for v := range pypi.Releases {
		if semverRegexp.MatchString(v) {
			versions = append(versions, v)
		}
	}

	SortVersionsDesc(versions)

	if limit > 0 && len(versions) > limit {
		versions = versions[:limit]
	}
	return versions, nil
}

// FetchLatestVersion returns the latest version from PyPI.
func FetchLatestVersion() (string, error) {
	versions, err := FetchRemoteVersions(1)
	if err != nil {
		return "", err
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("no versions found on PyPI")
	}
	return versions[0], nil
}

// SortVersionsDesc sorts version strings in descending semver order.
func SortVersionsDesc(versions []string) {
	sort.Slice(versions, func(i, j int) bool {
		return compareSemver(versions[i], versions[j]) > 0
	})
}

// compareSemver compares two semver strings. Returns >0 if a>b, <0 if a<b, 0 if equal.
func compareSemver(a, b string) int {
	aParts := parseSemverParts(a)
	bParts := parseSemverParts(b)
	for k := 0; k < 3; k++ {
		if aParts[k] != bParts[k] {
			return aParts[k] - bParts[k]
		}
	}
	return 0
}

func parseSemverParts(v string) [3]int {
	parts := strings.SplitN(v, ".", 3)
	var result [3]int
	for i, p := range parts {
		if i >= 3 {
			break
		}
		fmt.Sscanf(p, "%d", &result[i])
	}
	return result
}
