# Phase 1a: Config System

**Depends on:** Phase 0 (scaffold)
**Parallel with:** Phase 1b (Java), Phase 1c (discovery)

## Goal

Implement the `~/.dh/` config directory, `config.toml` read/write, `.dhrc` local override, version resolution precedence, and the `dh config` command group.

## Files to create/modify

```
go_src/
  internal/
    config/
      config.go            # Load/save ~/.dh/config.toml, ensure dir exists
      dhgrc.go             # .dhrc read/write, walk-up-from-cwd logic
      resolve.go           # Version resolution precedence
  cmd/dhg/
    config.go              # dh config, config set, config get, config path
```

## Internal package: `internal/config`

### config.go
- `DHGHome() string` — returns `--config-dir` or `DH_HOME` or `~/.dh`
- `EnsureDir()` — create `~/.dh/` if missing
- `Load() (*Config, error)` — read `config.toml`, return struct
- `Save(*Config) error` — write back to `config.toml`
- `Get(key string) (string, error)` — get a single value
- `Set(key, value string) error` — set a single value
- Config struct: `DefaultVersion`, `Install.Plugins`, `Install.PythonVersion`

### dhgrc.go
- `FindDHGRC(startDir string) (string, error)` — walk up from dir looking for `.dhrc`
- `ReadDHGRC(path string) (string, error)` — read version from `.dhrc`
- `WriteDHGRC(dir, version string) error` — write `.dhrc` in dir

### resolve.go
- `ResolveVersion(flagVersion, envVersion string) (string, error)` — precedence:
  1. `--version` flag
  2. `DH_VERSION` env var
  3. `.dhrc` walk-up
  4. `config.toml` default_version
  5. Latest installed version

## Commands

### `dh config`
Show all config as human-readable or JSON.

### `dh config set <KEY> <VALUE>`
Set a value in config.toml. Validate key exists.

### `dh config get <KEY>`
Print raw value to stdout (no newline decoration in `--quiet`/script mode).

### `dh config path`
Print the config file path.

## Tests

### Unit tests (`go_unit_tests/config_test.go`)
- TOML parsing: valid config, missing file (returns defaults), malformed TOML
- `Set` then `Get` roundtrip
- `.dhrc` walk-up: finds in cwd, finds in parent, stops at root, handles missing
- Version resolution: flag > env > dhgrc > config > latest, each level tested
- `EnsureDir` creates directory if missing

### Behaviour tests (`go_behaviour_tests/testdata/scripts/config.txtar`)
- `dh config path` → stdout contains `.dhg/config.toml`
- `dh config set default_version 42.0` then `dh config get default_version` → `42.0`
- `dh config --json` → valid JSON with key/values
- `dh config get nonexistent` → exit 1, error message

## Verification

```bash
./dh config path
./dh config set default_version 42.0
./dh config get default_version   # → 42.0
./dh config --json
```
