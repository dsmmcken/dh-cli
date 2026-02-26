# Plan: Store VM artifacts in workspace for persistence across sandbox sessions

## Context

When running `dhg` inside a Docker sandbox that mounts `/workspace`, all VM artifacts (firecracker binary, kernel, rootfs, snapshots) are built into `~/.dhg/vm/` which lives on the container's ephemeral filesystem. This means every new sandbox session requires a full `vm prepare` (2-5 minutes).

The workspace directory `/workspace` is mounted from the host and persists across sessions. If VM artifacts are stored there, they can be reused without rebuilding.

## Approach

**No code changes needed.** The existing `--config-dir` flag and `DHG_HOME` environment variable already control where all artifacts are stored, including VM paths.

### How it works today

`config.DHGHome()` resolves in this order:
1. `--config-dir` flag → `config.SetConfigDir()`
2. `DHG_HOME` env var
3. `~/.dhg` (default)

All VM paths derive from `DHGHome() + "/vm"` via `NewVMPaths()`.

### Solution

Set `DHG_HOME` to a directory inside the workspace. Two options:

**Option A: Environment variable (recommended)**
```bash
export DHG_HOME=/workspace/.dhg
dhg vm prepare
dhg exec --vm script.py
```

**Option B: Flag per-command**
```bash
dhg --config-dir /workspace/.dhg vm prepare
dhg --config-dir /workspace/.dhg exec --vm script.py
```

This stores everything under `/workspace/.dhg/vm/` which persists across sandbox sessions since `/workspace` is a host mount.

### For CLAUDE.md / sandbox setup

Add to the sandbox persistent config or CLAUDE.md:
```bash
export DHG_HOME=/workspace/.dhg
```

## Files involved (no modifications needed)

- `/workspace/go_src/internal/config/config.go` — `DHGHome()` already supports `DHG_HOME` env and `--config-dir`
- `/workspace/go_src/internal/vm/vm.go` — `NewVMPaths()` derives all paths from `dhgHome`
- `/workspace/go_src/internal/cmd/vm.go` — All VM commands call `config.SetConfigDir(ConfigDir)` first

## Verification

1. `export DHG_HOME=/workspace/.dhg`
2. `dhg vm prepare` — artifacts land in `/workspace/.dhg/vm/`
3. Kill and restart sandbox (keeping workspace mount)
4. `dhg vm status` — should show existing snapshot as "ready"
5. `dhg exec --vm 'print("hello")'` — should restore from existing snapshot
