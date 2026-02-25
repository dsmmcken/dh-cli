# Phase 1a: Config System

**Depends on:** Phase 0 (scaffold)
**Parallel with:** Phase 1b (Java), Phase 1c (discovery)

## Goal

Implement the `~/.dhg/` config directory, `config.toml` read/write, `.dhgrc` local override, version resolution precedence, and the `dhg config` command group.

## Files to create/modify

```
go_src/
  internal/
    config/
      config.go            # Load/save ~/.dhg/config.toml, ensure dir exists
      dhgrc.go             # .dhgrc read/write, walk-up-from-cwd logic
      resolve.go           # Version resolution precedence
  cmd/dhg/
    config.go              # dhg config, config set, config get, config path
```

## Internal package: `internal/config`

### config.go
- `DHGHome() string` — returns `--config-dir` or `DHG_HOME` or `~/.dhg`
- `EnsureDir()` — create `~/.dhg/` if missing
- `Load() (*Config, error)` — read `config.toml`, return struct
- `Save(*Config) error` — write back to `config.toml`
- `Get(key string) (string, error)` — get a single value
- `Set(key, value string) error` — set a single value
- Config struct: `DefaultVersion`, `Install.Plugins`, `Install.PythonVersion`

### dhgrc.go
- `FindDHGRC(startDir string) (string, error)` — walk up from dir looking for `.dhgrc`
- `ReadDHGRC(path string) (string, error)` — read version from `.dhgrc`
- `WriteDHGRC(dir, version string) error` — write `.dhgrc` in dir

### resolve.go
- `ResolveVersion(flagVersion, envVersion string) (string, error)` — precedence:
  1. `--version` flag
  2. `DHG_VERSION` env var
  3. `.dhgrc` walk-up
  4. `config.toml` default_version
  5. Latest installed version

## Commands

### `dhg config`
Show all config as human-readable or JSON.

### `dhg config set <KEY> <VALUE>`
Set a value in config.toml. Validate key exists.

### `dhg config get <KEY>`
Print raw value to stdout (no newline decoration in `--quiet`/script mode).

### `dhg config path`
Print the config file path.

## Tests

### Unit tests (`go_unit_tests/config_test.go`)
- TOML parsing: valid config, missing file (returns defaults), malformed TOML
- `Set` then `Get` roundtrip
- `.dhgrc` walk-up: finds in cwd, finds in parent, stops at root, handles missing
- Version resolution: flag > env > dhgrc > config > latest, each level tested
- `EnsureDir` creates directory if missing

### Behaviour tests (`go_behaviour_tests/testdata/scripts/config.txtar`)
- `dhg config path` → stdout contains `.dhg/config.toml`
- `dhg config set default_version 42.0` then `dhg config get default_version` → `42.0`
- `dhg config --json` → valid JSON with key/values
- `dhg config get nonexistent` → exit 1, error message

## Verification

```bash
./dhg config path
./dhg config set default_version 42.0
./dhg config get default_version   # → 42.0
./dhg config --json
```
