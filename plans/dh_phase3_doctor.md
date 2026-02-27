# Phase 3: Doctor Command

**Depends on:** Phase 1a (config), Phase 1b (Java), Phase 1c (discovery), Phase 2 (versions)
**This is a leaf phase** — nothing depends on it.

## Goal

Implement `dh doctor` which runs diagnostic checks across all subsystems and reports environment health.

## Files to create/modify

```
go_src/
  cmd/dhg/
    doctor.go              # dh doctor command
```

No new internal package — doctor calls into existing packages: `config`, `java`, `versions`, `discovery`.

## Command: `dh doctor`

Runs checks in order and reports results. Flags: `--fix` (attempt auto-fix), `--json`.

### Checks

1. **uv** — `uv --version` succeeds, report path and version
2. **Java** — `java.Detect()` finds compatible Java (>= 17)
3. **Versions** — `versions.ListInstalled()` returns count
4. **Default version** — `config.Load()` has a default set, and it exists on disk
5. **Disk space** — check free space in `~/.dh/`, warn if < 5 GB

### Human output
```
Deephaven CLI Doctor

  ✓ uv         /home/user/.local/bin/uv (0.5.14)
  ✓ Java       21.0.5 (JAVA_HOME)
  ✓ Versions   2 installed
  ✓ Default    42.0
  ⚠ Disk       2.1 GB free in ~/.dh

Everything looks good (1 warning).
```

### JSON output
```json
{
  "healthy": true,
  "checks": [
    {"name": "uv", "status": "ok", "detail": "/home/user/.local/bin/uv (0.5.14)"},
    {"name": "java", "status": "ok", "detail": "21.0.5 (JAVA_HOME)"},
    {"name": "versions", "status": "ok", "detail": "2 installed"},
    {"name": "default_version", "status": "ok", "detail": "42.0"},
    {"name": "disk_space", "status": "warning", "detail": "2.1 GB free"}
  ]
}
```

### `--fix` behavior
- uv missing → suggest install command
- Java missing → run `java.Install()`
- No versions → run install flow
- No default → set to latest installed

## Tests

### Unit tests (`go_unit_tests/doctor_test.go`)
- Each check returns correct status with mocked dependencies
- `--fix` triggers appropriate remediation
- `healthy` is false when any check is `error` status, true when all `ok` or `warning`

### Behaviour tests (`go_behaviour_tests/testdata/scripts/doctor.txtar`)
- `dh doctor` → stdout contains `uv` and `Java`
- `dh doctor --json` → valid JSON, has `"healthy"`, `"checks"` array
- Each check has `"name"`, `"status"`, `"detail"`
- `dh doctor --no-color` → no ANSI escapes

## Verification

```bash
./dh doctor
./dh doctor --json
./dh doctor --json | jq '.checks | length'  # → 5
```
