# Plan: Store VM artifacts in workspace for persistence across sandbox sessions

## Context

When running `dh` inside a Docker sandbox that mounts `/workspace`, all VM artifacts (firecracker binary, kernel, rootfs, snapshots) are built into `~/.dh/vm/` which lives on the container's ephemeral filesystem. This means every new sandbox session requires a full `vm prepare` (2-5 minutes).

The workspace directory `/workspace` is mounted from the host and persists across sessions. If VM artifacts are stored there, they can be reused without rebuilding.

## Approach

**No code changes needed.** The existing `--config-dir` flag and `DH_HOME` environment variable already control where all artifacts are stored, including VM paths.

### How it works today

`config.DHGHome()` resolves in this order:
1. `--config-dir` flag → `config.SetConfigDir()`
2. `DH_HOME` env var
3. `~/.dh` (default)

All VM paths derive from `DHGHome() + "/vm"` via `NewVMPaths()`.

### Solution

Set `DH_HOME` to a directory inside the workspace. Two options:

**Option A: Environment variable (recommended)**
```bash
export DH_HOME=/workspace/.dhg
dh vm prepare
dh exec --vm script.py
```

**Option B: Flag per-command**
```bash
dh --config-dir /workspace/.dh vm prepare
dh --config-dir /workspace/.dh exec --vm script.py
```

This stores everything under `/workspace/.dh/vm/` which persists across sandbox sessions since `/workspace` is a host mount.

### For CLAUDE.md / sandbox setup

Add to the sandbox persistent config or CLAUDE.md:
```bash
export DH_HOME=/workspace/.dhg
```

## Files involved (no modifications needed)

- `/workspace/go_src/internal/config/config.go` — `DHGHome()` already supports `DH_HOME` env and `--config-dir`
- `/workspace/go_src/internal/vm/vm.go` — `NewVMPaths()` derives all paths from `dhgHome`
- `/workspace/go_src/internal/cmd/vm.go` — All VM commands call `config.SetConfigDir(ConfigDir)` first

## Verification

1. `export DH_HOME=/workspace/.dh`
2. `dh vm prepare` — artifacts land in `/workspace/.dh/vm/`
3. Kill and restart sandbox (keeping workspace mount)
4. `dh vm status` — should show existing snapshot as "ready"
5. `dh exec --vm 'print("hello")'` — should restore from existing snapshot
