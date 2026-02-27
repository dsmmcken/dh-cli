# Plan: UFFD Eager Page Population — Handoff / Remaining Work

## What's Done

All code is written, compiles, passes tests and vet. The SDK has been upgraded
to the latest `main` which supports `WithMemoryBackend("Uffd", socketPath)`.

### Files created
- **`internal/vm/uffd_linux.go`** — Complete UFFD handler (~200 LOC):
  - `startUffdHandler()` — creates UDS listener, spawns goroutine
  - Goroutine: accept → recvmsg (SCM_RIGHTS for UFFD fd) → mmap snapshot_mem → UFFDIO_COPY in 256MB chunks → signal done
  - `Wait()` — blocks until population complete
  - `Close()` — cleans up fd, listener, socket
  - Compile-time size assertion for `ufffdioCopy` struct

### Files modified
- **`internal/vm/vm.go`** — Added `UseUffd bool` to `VMConfig`
- **`internal/vm/machine_linux.go`** — `RestoreFromSnapshot` now has UFFD and File branches:
  - Returns `io.Closer` as 4th value (UFFD handler or nil)
  - UFFD path: start handler → `WithSnapshot("", statePath, WithMemoryBackend("Uffd", ...))` with `ResumeVM=false` → `machine.Start()` → `uffd.Wait()` → `machine.ResumeVM()`
  - File path: unchanged (`WithSnapshot(memPath, statePath)` with `ResumeVM=true`)
- **`internal/vm/machine_other.go`** — Updated stub signature to 4 return values
- **`internal/exec/exec_vm_linux.go`** — Updated caller, `DH_VM_NO_UFFD=1` env var for fallback, cleanup in defer and signal handler

### Cleanup done (same session)
- Deleted `network_linux.go`, `network_other.go` (dead TAP networking code)
- Removed `TapPrefix`, `SnapshotMetadata.{VMIP,HostIP,TapSubnet}`, `VMConfig.JVMArgs`
- Removed `NetworkSudoConfigured`, `FixNetworkAccess` from both prereqs files
- Removed `TestAllocateIPPair`

### SDK upgrade
- `firecracker-go-sdk` upgraded from `v1.0.0` to `v1.0.1-0.20251224190957-6fb280e993d4` (main)
- This brought `WithMemoryBackend`, `MemoryBackend` model, `ResumeVM` control

## What Failed on First Test

```
Error: too many fields: either `mem_backend` or `mem_file_path` exclusively is required.
```

**Root cause**: `WithSnapshot(memPath, statePath, ...)` was passing `memPath` as the
first arg even in UFFD mode. Firecracker rejects requests containing both
`mem_file_path` AND `mem_backend`.

**Fix applied**: Added `snapshotMemArg` variable — empty string for UFFD mode,
`memPath` for File mode. This is already in the code at `machine_linux.go:299-304`.

## What Still Needs Testing

The fix was applied but **not yet rebuilt/tested**. The remaining steps are:

### 1. Rebuild and test
```bash
cd go_src && make install-local
```

### 2. Test UFFD mode
```bash
dh exec --vm --version 41.1 --verbose -c "print('Hello from UFFD!')"
```

Expected: Should show "UFFD: populated N regions, 2048 MiB" in verbose output, then execute.

### 3. Possible issues to debug

**SDK validation**: `ValidateLoadSnapshot()` calls `os.Stat(cfg.Snapshot.GetMemBackendPath())`.
When `MemBackend` is set, `GetMemBackendPath()` returns the UFFD socket path. The socket
file exists because `startUffdHandler` creates the listener before `NewMachine` is called.
This *should* work, but verify.

**UFFDIO_COPY ioctl number**: The constant `0xc028aa03` is for amd64. If it fails with
EINVAL or ENOTTY, verify against:
```c
// _IOWR(0xAA, 0x03, struct uffdio_copy) where sizeof(uffdio_copy) = 40
// = 0xc0000000 | (40 << 16) | (0xAA << 8) | 0x03 = 0xc028aa03
```

**Firecracker UFFD protocol quirks**:
- Firecracker calls UFFDIO_API and UFFDIO_REGISTER itself before sending the fd.
  Our handler does NOT need to do those ioctls — just UFFDIO_COPY.
- Sometimes the first recvmsg arrives without the fd (retry logic is in the code, 5 attempts).
- The `base_host_virt_addr` is a `uint64` (not hex string) in the JSON.
- Both `page_size` (new) and `page_size_kib` (deprecated, same unit despite name) may be present.

**UFFD fd lifetime**: The fd must stay open for the entire VM lifetime. The handler's
`Close()` is called after `DestroyInstance()` in `exec_vm_linux.go`. If closed too early,
the VM gets SIGBUS. Verify ordering in the defer.

### 4. Test File fallback
```bash
DH_VM_NO_UFFD=1 dh exec --vm --version 41.1 --verbose -c "print('hello')"
```

Should behave identically to before (same ~5-6s timing).

### 5. Performance comparison
```bash
# UFFD (should be significantly faster — target: ~1-2s vs ~5-6s)
time dh exec --vm --version 41.1 --verbose -c "print('hello')"

# File baseline
time DH_VM_NO_UFFD=1 dh exec --vm --version 41.1 --verbose -c "print('hello')"
```

### 6. Correctness tests
```bash
dh exec --vm --version 41.1 -c "from deephaven import empty_table; t = empty_table(5).update(['x = i']); print(t.to_string())"
dh exec --vm --version 41.1 --json -c "1+1"
```

### 7. If snapshot needs re-preparing
The existing snapshot (File backend) should work with UFFD — the `snapshot_mem` file
format is the same. But if issues arise:
```bash
dh vm clean --version 41.1
dh vm prepare --version 41.1
```

## Architecture Summary

```
exec_vm_linux.go
  └─ RestoreFromSnapshot(UseUffd=true)
       ├─ startUffdHandler() → goroutine listening on instanceDir/uffd.sock
       ├─ NewMachine(WithSnapshot("", statePath, WithMemoryBackend("Uffd", "uffd.sock"), ResumeVM=false))
       ├─ machine.Start() → Firecracker connects to UFFD socket
       │    └─ goroutine: accept → recvmsg(SCM_RIGHTS) → mmap snapshot_mem → UFFDIO_COPY (8 ioctls for 2GB)
       ├─ uffd.Wait() → blocks until all pages populated
       ├─ machine.ResumeVM() → VM starts with all pages resident, zero page faults
       └─ waitForVsock() → runner daemon ready
```
