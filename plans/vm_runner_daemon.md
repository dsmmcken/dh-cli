# Plan: Move Code Execution Into the VM (In-VM Runner Daemon)

## Context

`dh exec --vm` restores a Firecracker VM in ~73ms but then spends ~12s on the host side: spawning Python, importing pydeephaven (336ms), establishing a gRPC session (2.6s), and executing code with JVM warmup (7.5s). All of this can be done once during `dh vm prepare` and frozen into the snapshot.

## Approach

Replace the host-side Python pipeline with an **in-VM runner daemon** that pre-connects a pydeephaven Session to Deephaven. This daemon + warm session are captured in the snapshot. On `dh exec --vm`, the Go code sends user code as JSON over vsock and reads back JSON results. No Python on the host at all.

**Before:** Host Go → TCP proxy → vsock → VM bridge → DH gRPC (12s)
**After:** Host Go → vsock → VM runner daemon (already connected) → DH (est. <200ms)

## Protocol

Newline-delimited JSON over vsock port 10000.

**Request:** `{"code": "...", "show_tables": false, "show_table_meta": false}\n`
**Response:** `{"exit_code": 0, "stdout": "...", "stderr": "", "result_repr": null, "error": null, "tables": []}\n`

## Implementation Steps

### Step 1: Create `internal/vm/vm_runner.py`

New file — the Python daemon that runs inside the VM. Port these functions from `runner.py`:
- `get_assigned_names()` + `_extract_names()` — AST parsing
- `build_wrapper()` — wrapper generation (no cwd/script_path params, not applicable in VM)
- `read_result_table()` — unpickle results from `__dh_result_table`
- `cleanup_result_table()` — delete temp table
- `get_table_preview()` — table metadata + first 10 rows

Main loop:
1. Wait for `/tmp/dh_ready`
2. Connect pydeephaven Session to localhost:10000
3. Listen on vsock port 10000
4. Per connection: read JSON line → execute via warm session → write JSON response

Handle probe connections (empty data from `waitForVsock`) gracefully — just close.

### Step 2: Update `internal/vm/rootfs_linux.go`

- Add `//go:embed vm_runner.py` to embed the daemon script
- Update `dockerfileTemplate`: add `COPY vm_runner.py /opt/vm_runner.py`
- Update `initScriptTemplate`: replace vsock-to-TCP bridge with `python3 /opt/vm_runner.py &`
- Update `buildRootfsDocker()`: write vm_runner.py to Docker build context

### Step 3: Rebuild rootfs and snapshot, verify daemon starts

```bash
make install-local
dh vm clean --version 41.1
dh vm prepare --version 41.1
```

### Step 4: Update `internal/vm/vm.go`

Simplify `InstanceInfo` — remove `LocalPort` and `proxyCleanup` fields (no TCP proxy).

### Step 5: Update `internal/vm/machine_linux.go`

- Add `VsockRequest`/`VsockResponse` types and `ExecuteViaVsock()` function
- Remove `startVsockProxy()` and `bridgeVsock()` (no longer needed)
- Simplify `RestoreFromSnapshot()` — remove TCP proxy setup, return simpler InstanceInfo
- Simplify `DestroyInstance()` — no proxyCleanup
- Reduce post-vsock sleep in `BootAndSnapshot` from 2s to 500ms (runner daemon is already warm when vsock is reachable)

### Step 6: Rewrite `internal/exec/exec_vm_linux.go`

Replace entire Python subprocess pipeline with:
1. `vm.RestoreFromSnapshot()`
2. `vm.ExecuteViaVsock(info.VsockPath, vm.VsockPort, request)`
3. Format and display results in Go

Remove: `FindVenvPython`, `EnsurePydeephaven`, `buildRunnerArgs`, runner.py subprocess, `bytes`/`exec`/`encoding/json` imports related to runner process management.

Also remove the temporary timing instrumentation added during debugging.

### Step 7: Build and test end-to-end

```bash
make install-local
dh exec --vm --version 41.1 -c "print('Hello from Firecracker VM')"
dh exec --vm --version 41.1 -c "1 + 1"
dh exec --vm --version 41.1 --json -c "print('hello')"
dh exec --vm --version 41.1 -c "raise ValueError('test')"
time dh exec --vm --version 41.1 -c "print('hello')"  # should be <1s
```

## Files Changed

| File | Action |
|------|--------|
| `internal/vm/vm_runner.py` | **NEW** — in-VM daemon |
| `internal/vm/rootfs_linux.go` | Embed vm_runner.py, update Dockerfile + init script |
| `internal/vm/vm.go` | Simplify InstanceInfo |
| `internal/vm/machine_linux.go` | Add ExecuteViaVsock, remove proxy code, simplify restore/destroy |
| `internal/exec/exec_vm_linux.go` | Rewrite: vsock JSON instead of Python subprocess |

**Unchanged:** `runner.py` (still used for non-VM exec modes), `exec.go`, `cmd/exec.go`, `cmd/vm.go`
