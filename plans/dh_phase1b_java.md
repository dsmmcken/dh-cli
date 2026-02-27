# Phase 1b: Java Management

**Depends on:** Phase 0 (scaffold)
**Parallel with:** Phase 1a (config), Phase 1c (discovery)

## Goal

Implement Java detection, Java installation (Eclipse Temurin JDK 21), and the `dh java` / `dh java install` commands.

## Files to create/modify

```
go_src/
  internal/
    java/
      detect.go            # Find Java on the system
      install.go           # Download and extract Temurin JDK
      version.go           # Parse java -version output
  cmd/dhg/
    java.go                # dh java (show status)
    java_install.go        # dh java install
```

## Internal package: `internal/java`

### detect.go
- `Detect(dhgHome string) (*JavaInfo, error)` — check in order:
  1. `$JAVA_HOME/bin/java`
  2. `java` on `$PATH`
  3. `<dhgHome>/java/*/bin/java` (managed install)
- `JavaInfo` struct: `Found bool`, `Version string`, `Path string`, `Home string`, `Source string` (JAVA_HOME/PATH/managed)
- Minimum version: 17

### version.go
- `ParseVersion(output string) (string, error)` — parse `java -version` stderr output
- Handle formats: `openjdk version "21.0.5"`, `java version "17.0.2"`, etc.
- `MeetsMinimum(version string, min int) bool`

### install.go
- `Install(dhgHome string, jdkVersion int, force bool) (*JavaInfo, error)`
- Download from Adoptium API: `https://api.adoptium.net/v3/binary/latest/<ver>/ga/<os>/<arch>/jdk/hotspot/normal/eclipse`
- Detect OS (`runtime.GOOS`) and arch (`runtime.GOARCH`) → map to Adoptium naming
- Extract `.tar.gz` (Linux/macOS) or `.zip` (Windows) to `<dhgHome>/java/`
- Make `bin/java` executable
- Return `JavaInfo` for the installed JDK

## Commands

### `dh java`
Show Java status. Human-readable or JSON.

JSON: `{"found": true, "version": "21.0.5", "path": "...", "home": "...", "source": "JAVA_HOME"}`

### `dh java install`
Download Temurin JDK. Flags: `--jdk-version` (default 21), `--force`.