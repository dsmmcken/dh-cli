# Optimize warm VM exec path

## Context

`dh exec --vm` warm execution is ~144ms. Two sources of unnecessary overhead on the
hot path (pool is running, VM pre-warmed):

1. **Double pool socket connect**: `PoolProbe()` connects/closes just to check liveness,
   then `PoolExec()` connects again to actually execute. One round-trip wasted.
2. **Filesystem-heavy version resolution**: `ResolveVersion` walks directories for
   `.dhrc`, reads `config.toml`, scans `~/.dh/versions/` — all before we even try the
   pool, which already knows its version.

Estimated savings: 10-30ms combined.

## Changes

### 1. Eliminate PoolProbe double-connect

**`src/internal/vm/pool_client.go`**
- Add `var ErrPoolNotRunning` sentinel error
- Return it (wrapping the dial error) when `net.DialTimeout` fails in `poolRPC`

**`src/internal/exec/exec_vm_linux.go`**
- Remove `vm.PoolProbe()` call from `tryPoolExec`
- Call `vm.PoolExec()` directly
- On `errors.Is(err, vm.ErrPoolNotRunning)` → auto-start pool daemon, fall through to cold
- On other errors → just fall through (pool is running but had a problem)

### 2. Skip version resolution for VM pool path

**`src/internal/exec/exec.go` — `Run()`**
- For VM mode: do quick version resolve (flag + env var only, no filesystem I/O)
- Pass the result (possibly empty string) to `runVM()`
- Non-VM path unchanged

**`src/internal/exec/exec_vm_linux.go` — `runVM()`**
- Pool path: pass version (possibly empty) to `tryPoolExec`
- Cold path fallback: if version is still empty, do full `config.ResolveVersion` +
  `latestSnapshotVersion` at that point (existing behavior, just deferred)

**`src/internal/exec/exec_vm_linux.go` — `tryPoolExec()`**
- Change return signature to include effective version:
  `(int, map[string]any, *vm.VsockResponse, string, error)`
- If input version is empty → accept pool's version (`poolResp.Version`)
- If input version is non-empty → verify match (existing mismatch check)
- Use effective version in JSON result and pass it back to caller

**`src/internal/exec/exec_vm_linux.go` — `runVM()` caller site**
- Use the version returned by `tryPoolExec` for `formatVsockResponse`

### Trade-off note

When version is unset (no `--version`, no `DH_VERSION`), the pool path will now accept
whatever version the pool is running, skipping `.dhrc` / `config.toml` lookup. This is
acceptable because:
- VM pool users typically have one pool version running
- `.dhrc` is primarily for the non-VM installed-versions path
- Strict version control uses `--version` or `DH_VERSION`, which are still checked
- The cold fallback still does full resolution

## Files modified

1. `src/internal/vm/pool_client.go` — add `ErrPoolNotRunning`
2. `src/internal/exec/exec.go` — skip full resolve for VM mode
3. `src/internal/exec/exec_vm_linux.go` — remove PoolProbe, handle empty version, return effective version

## Verification

```bash
make test
make vet
```

End-to-end (on a machine with VM support):
```bash
# Start pool, then time warm exec
dh vm pool start -n 1
time dh exec --vm -c "print('hello world')"

# Verify version flows correctly
dh exec --vm -c "print('hello')" --json  # check "version" field in output

# Verify cold path still works
DH_VM_POOL=0 dh exec --vm -c "print('hello')"

# Verify auto-start still works (stop pool first)
dh vm pool stop
time dh exec --vm -c "print('hello')"  # should cold-start and auto-start pool
time dh exec --vm -c "print('hello')"  # should use pool
```
