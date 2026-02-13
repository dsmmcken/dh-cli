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

// semverRegexp matches versions like 1.2.3, 0.36.1, 41.1, etc. (patch is optional).
var semverRegexp = regexp.MustCompile(`^\d+\.\d+(\.\d+)?$`)

// RemoteVersion holds a version string and its PyPI release date.
type RemoteVersion struct {
	Version string
	Date    string // "YYYY-MM-DD" or empty
}

// pypiResponse is the subset of the PyPI JSON response we care about.
type pypiResponse struct {
	Releases map[string]json.RawMessage `json:"releases"`
}

// pypiFile is one file entry inside a release; we only need upload_time_iso_8601.
type pypiFile struct {
	UploadTime string `json:"upload_time_iso_8601"`
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

// FetchRemoteVersionsWithDates fetches versions with their PyPI release dates.
func FetchRemoteVersionsWithDates(limit int) ([]RemoteVersion, error) {
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

	return ParsePyPIResponseWithDates(body, limit)
}

// ParsePyPIResponseWithDates parses a PyPI JSON response and returns sorted versions with dates.
func ParsePyPIResponseWithDates(data []byte, limit int) ([]RemoteVersion, error) {
	var pypi pypiResponse
	if err := json.Unmarshal(data, &pypi); err != nil {
		return nil, fmt.Errorf("parsing PyPI JSON: %w", err)
	}

	dateMap := make(map[string]string)
	var versionStrs []string
	for v, raw := range pypi.Releases {
		if !semverRegexp.MatchString(v) {
			continue
		}
		versionStrs = append(versionStrs, v)

		var files []pypiFile
		if err := json.Unmarshal(raw, &files); err == nil && len(files) > 0 {
			ts := files[0].UploadTime
			if len(ts) >= 10 {
				dateMap[v] = ts[:10]
			}
		}
	}

	SortVersionsDesc(versionStrs)

	if limit > 0 && len(versionStrs) > limit {
		versionStrs = versionStrs[:limit]
	}

	result := make([]RemoteVersion, len(versionStrs))
	for i, v := range versionStrs {
		result[i] = RemoteVersion{Version: v, Date: dateMap[v]}
	}
	return result, nil
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
