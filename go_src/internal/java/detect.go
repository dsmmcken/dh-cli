package java

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// MinimumVersion is the minimum Java version required by Deephaven.
const MinimumVersion = 17

// JavaInfo describes a Java installation.
type JavaInfo struct {
	Found   bool   `json:"found"`
	Version string `json:"version"`
	Path    string `json:"path"`
	Home    string `json:"home"`
	Source  string `json:"source"`
}

// Detect locates Java on the system by checking, in order:
//  1. $JAVA_HOME/bin/java
//  2. java on $PATH
//  3. <dhgHome>/java/*/bin/java (managed install)
func Detect(dhgHome string) (*JavaInfo, error) {
	// 1. Check JAVA_HOME
	if javaHome := os.Getenv("JAVA_HOME"); javaHome != "" {
		javaPath := filepath.Join(javaHome, "bin", "java")
		if info, err := probeJava(javaPath, javaHome, "JAVA_HOME"); err == nil {
			return info, nil
		}
	}

	// 2. Check PATH
	if javaPath, err := exec.LookPath("java"); err == nil {
		// Resolve symlinks to get the real path
		resolved, _ := filepath.EvalSymlinks(javaPath)
		if resolved == "" {
			resolved = javaPath
		}
		home := javaHomeFromBin(resolved)
		if info, err := probeJava(resolved, home, "PATH"); err == nil {
			return info, nil
		}
	}

	// 3. Check managed installs under <dhgHome>/java/*/bin/java
	if dhgHome != "" {
		pattern := filepath.Join(dhgHome, "java", "*", "bin", "java")
		matches, _ := filepath.Glob(pattern)
		if len(matches) == 0 {
			// Also check one level deeper for macOS layout: java/*/Contents/Home/bin/java
			pattern = filepath.Join(dhgHome, "java", "*", "Contents", "Home", "bin", "java")
			matches, _ = filepath.Glob(pattern)
		}
		for _, m := range matches {
			home := javaHomeFromBin(m)
			if info, err := probeJava(m, home, "managed"); err == nil {
				return info, nil
			}
		}
	}

	return &JavaInfo{Found: false}, nil
}

// probeJava runs java -version and returns a JavaInfo if the binary exists and responds.
func probeJava(javaPath, home, source string) (*JavaInfo, error) {
	version, err := runJavaVersion(javaPath)
	if err != nil {
		return nil, err
	}
	return &JavaInfo{
		Found:   true,
		Version: version,
		Path:    javaPath,
		Home:    home,
		Source:  source,
	}, nil
}

// runJavaVersion executes `java -version` and parses the version string.
func runJavaVersion(javaPath string) (string, error) {
	cmd := exec.Command(javaPath, "-version")
	// java -version writes to stderr
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return ParseVersion(string(out))
}

// javaHomeFromBin derives JAVA_HOME from a path like /usr/lib/jvm/java-21/bin/java.
func javaHomeFromBin(binJava string) string {
	// Strip /bin/java
	dir := filepath.Dir(binJava) // .../bin
	home := filepath.Dir(dir)    // .../java-21
	// Verify it looks right
	if strings.HasSuffix(dir, "bin") {
		return home
	}
	return filepath.Dir(binJava)
}
