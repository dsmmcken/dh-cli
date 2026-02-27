# Phase 2: Version Management

**Depends on:** Phase 0 (scaffold), Phase 1a (config)
**Parallel with:** Phase 1b (Java), Phase 1c (discovery) — can start as soon as Phase 1a is done

## Goal

Implement Deephaven version install/uninstall/use/list, PyPI version lookup, and the corresponding `dh` commands. This is the core workflow: installing a Deephaven version into `~/.dh/versions/<VERSION>/.venv`.

## Files to create/modify

```
go_src/
  internal/
    versions/
      install.go           # Install version via uv
      uninstall.go         # Remove version directory
      list.go              # List installed versions, read meta.toml
      pypi.go              # PyPI API client (fetch available versions)
      meta.go              # Read/write meta.toml per version
  cmd/dhg/
    install.go             # dh install [VERSION]
    uninstall.go           # dh uninstall <VERSION>
    use.go                 # dh use <VERSION>
    versions.go            # dh versions
```

## Internal package: `internal/versions`

### install.go
- `Install(dhgHome, version, pythonVer string, plugins []string) error`
- Steps:
  1. Create `<dhgHome>/versions/<version>/`
  2. `uv venv <dir>/.venv --python <pythonVer>`
  3. `uv pip install --python <dir>/.venv/bin/python deephaven-server==<ver> pydeephaven==<ver> <plugins...>`
  4. Write `meta.toml`
  5. If first installed version, set as default via config
- Use `var execCommand = exec.Command` pattern for testability
- Stream progress to stderr (or JSON progress lines if `--json`)

### uninstall.go
- `Uninstall(dhgHome, version string) error`
- Remove `<dhgHome>/versions/<version>/` directory
- If uninstalled version was the default, update config to latest remaining (or clear)

### list.go
- `ListInstalled(dhgHome string) ([]InstalledVersion, error)`
- Scan `<dhgHome>/versions/*/`, read each `meta.toml`
- `InstalledVersion` struct: `Version string`, `IsDefault bool`, `InstalledAt time.Time`, `Packages map[string]string`
- Sort by semver descending

### pypi.go
- `FetchRemoteVersions(limit int) ([]string, error)`
- Hit PyPI JSON API: `https://pypi.org/pypi/deephaven-server/json`
- Parse `releases` keys, filter valid semver, sort descending
- Cache response in `<dhgHome>/cache/` with TTL

### meta.go
- `ReadMeta(versionDir string) (*Meta, error)`
- `WriteMeta(versionDir string, meta *Meta) error`
- Meta struct: `Installed time.Time`, `Packages map[string]string`

## Commands

### `dh install [VERSION]`
- Default VERSION: `latest` (resolved from PyPI)
- Flags: `--no-plugins`, `--python`
- Human output: progress spinner/bar
- JSON output: `{"version": "42.0", "status": "installed", ...}`

### `dh uninstall <VERSION>`
- Flag: `--force` (skip confirmation)
- Confirmation prompt unless `--force` or `--json`

### `dh use <VERSION>`
- Flag: `--local` (write `.dhrc` in cwd)
- Validates version is installed
- Uses config package to write

### `dh versions`
- Flags: `--remote`, `--limit`, `--all`
- Human output: table with version, default marker, install date
- JSON output: `{"installed": [...], "default_version": "...", "remote": [...]}`

## Tests

### Unit tests (`go_unit_tests/versions_test.go`)
- `meta.toml` roundtrip read/write
- Version sorting (semver)
- PyPI response parsing (mock HTTP)
- Install flow with mocked `exec.Command` — verify uv commands called correctly
- Uninstall removes directory, updates default
- List scans directory, reads metadata

### Behaviour tests (`go_behaviour_tests/testdata/scripts/`)
- `install.txtar`: `dh install` creates version dir, `dh install --json` returns JSON
- `versions.txtar`: `dh versions --json` with nothing installed → empty array; after install → populated
- `use.txtar`: `dh use <ver>` updates config, `dh use <ver> --local` creates `.dhrc`
- `uninstall.txtar`: `dh uninstall <ver> --force` removes dir

## Verification

```bash
./dh versions                    # empty
./dh install --json              # installs latest
./dh versions --json             # shows installed
./dh use 42.0
./dh config get default_version  # → 42.0
./dh uninstall 42.0 --force
./dh versions                    # empty again
```
