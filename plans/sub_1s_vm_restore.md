# Plan: Sub-1s VM Restore for `dhg exec --vm`

## Context

After the balloon + sparse UFFD work, `dhg exec --vm -c "print('hello')"` takes
~1.7s. The breakdown is approximately:

| Phase | Time | Where |
|-------|------|-------|
| Go CLI overhead (version, prereqs, cleanup) | ~20ms | `exec_vm_linux.go` |
| Firecracker launch + SDK handlers | ~150ms | `machine_linux.go:378` machine.Start |
| UFFD eager data page copy (923 MiB) | ~700ms | `uffd_linux.go` populateRegionDataOnly |
| VM resume (ResumeVM API call) | ~5ms | `machine_linux.go:396` |
| waitForVsock polling | ~10-30ms | `machine_linux.go:407` |
| run_script (JVM-side execution) | ~360ms | `vm_runner.py` handle_request |
| vsock I/O + JSON marshal | ~10ms | Go + Python |

**Goal**: Reduce total time to under 1 second. No daemon/background processes —
all optimizations must be within the single `dhg exec --vm` invocation.

## Changes (ordered by impact)

### 1. Fully lazy UFFD — skip eager copy entirely (~600ms saving)

The biggest single win. Currently we eagerly `UFFDIO_COPY` all 923 MiB of data
pages before resuming the VM. But a simple `print('hello')` only touches a
fraction of those pages.

**Approach**: Pre-load the snapshot file into the kernel page cache, then serve
ALL page faults (data and holes) lazily on demand.

**`go_src/internal/vm/uffd_linux.go`**

Add a `preloadFile` method to `uffdHandler`:
```go
// Called immediately after startUffdHandler, before FC connects.
// Opens the file, mmaps it, and triggers background readahead via
// madvise(MADV_WILLNEED). Returns immediately — the kernel reads
// pages into page cache asynchronously.
func (h *uffdHandler) preloadFile() error
```

Modify `doPopulate()`:
- Remove the `populateRegionDataOnly` call (no eager UFFDIO_COPY)
- Remove `populateRegionFull` fallback
- Still scan data extents (to distinguish data from holes in the lazy handler)
- Signal `done` immediately after receiving UFFD fd + regions — the VM can resume
- Start a single `lazyFaultHandler` that handles ALL faults:
  - For faults in data extents → `UFFDIO_COPY` from pre-cached mmap (fast, ~2-5μs per 4KB page)
  - For faults in hole regions → `UFFDIO_ZEROPAGE` (instant, no memcpy)

The lazy handler needs to know which extents are data vs holes. Store a sorted
slice of `dataExtent` per region and binary-search on fault address.

**Performance math**: If `print('hello')` touches ~100MB of the 923MB data
region, that's ~25,600 page faults × ~3μs each = ~77ms. Compare to 700ms for
eagerly copying all 923MB.

### 2. Pre-load snapshot file during FC launch (~overlap, saves ~200ms)

Currently the snapshot file is opened and mmap'd inside `doPopulate()` (after FC
connects). This blocks on disk I/O if the file isn't cached.

**`go_src/internal/vm/uffd_linux.go`**

Move file open + mmap to `startUffdHandler`, before `machine.Start()` begins.
Use `madvise(MADV_WILLNEED)` to trigger non-blocking readahead. By the time the
VM starts faulting, the file should be fully cached.

```go
func startUffdHandler(ctx, socketPath, memFilePath, stderr) (*uffdHandler, error) {
    // ... existing listener setup ...

    // Pre-load: open + mmap + MADV_WILLNEED (non-blocking readahead)
    h.preloadFile()  // returns immediately

    go h.run(ctx, stderr)
    return h, nil
}
```

This overlaps the ~200ms of file I/O with Firecracker's startup time.

### 3. Better JVM warmup during prepare (~100-200ms saving)

Currently: 5 iterations of `x = 1`. This barely triggers JIT compilation.

**`go_src/internal/vm/machine_linux.go`** — `BootAndSnapshot()`

Replace the warmup loop with a realistic workload that mirrors actual `dhg exec`
usage. The wrapper script calls `run_script`, `empty_table`, `update`, `to_arrow`,
pickle, and base64 — all need JIT warmup.

```go
warmupScripts := []string{
    // Iteration 1-5: basic execution path
    "x = 1",
    // Iteration 6-10: table creation (triggers deephaven.update JIT)
    "from deephaven import empty_table\nt = empty_table(1).update(['x = i'])",
    // Iteration 11-15: full wrapper pattern with pickle+base64
    `import pickle, base64
t = empty_table(1).update(['x = i'])
d = {"stdout": "test", "result_repr": "1"}
base64.b64encode(pickle.dumps(d))`,
}

for i := 0; i < 20; i++ {
    script := warmupScripts[min(i/5, len(warmupScripts)-1)]
    // Use the actual build_wrapper + run_script + read_result pattern
    // by sending through ExecuteViaVsock (same path as real exec)
    warmupReq := &VsockRequest{Code: script, ShowTables: true}
    warmupResp, err := ExecuteViaVsock(vsockPath, VsockPort, warmupReq)
    // ... error handling, print timing ...
}
```

20 iterations with progressively complex scripts ensures the JVM's C2 compiler
kicks in for all hot methods (Deephaven's run_script, Arrow serialization, etc.).

**`go_src/internal/vm/rootfs_linux.go`** — JVM args

Add JIT-tuning flags:
```
-XX:+TieredCompilation
-XX:CompileThreshold=100
```

The lower CompileThreshold (default 10000) triggers C2 compilation much sooner,
so 20 warmup iterations is enough to compile the hot paths.

### 4. Direct vsock connect — skip polling loop (~20ms saving)

**`go_src/internal/vm/machine_linux.go`** — `RestoreFromSnapshot()`

The daemon was in `accept()` at snapshot time. After `ResumeVM`, it responds
within microseconds. Replace the 10ms-interval poll loop with a single connect
attempt with a short timeout:

```go
// After ResumeVM — daemon resumes from accept() immediately
conn, err := connectVsock(vsockPath, VsockPort)
if err != nil {
    // Fallback: poll briefly in case of timing edge case
    if pollErr := waitForVsock(restoreCtx, vsockPath, VsockPort, 2*time.Second); pollErr != nil {
        return ...
    }
}
if conn != nil {
    conn.Close()
}
```

Also reduce `waitForVsock` poll interval from 10ms to 1ms for the fallback path.

### 5. Remove unnecessary SDK handlers (~20-40ms saving)

**`go_src/internal/vm/machine_linux.go`** — `RestoreFromSnapshot()`

After `WithSnapshot` sets up the handler chain, it contains:
`SetupNetwork → StartVMM → CreateLogFiles → BootstrapLogging → LoadSnapshot`

For snapshot restore, we don't need network setup (no TAP), log file creation,
or bootstrap logging. Remove them:

```go
machine.Handlers.FcInit = machine.Handlers.FcInit.Remove(firecracker.AddVsocksHandlerName)
machine.Handlers.FcInit = machine.Handlers.FcInit.Remove(firecracker.SetupNetworkHandlerName)
machine.Handlers.FcInit = machine.Handlers.FcInit.Remove(firecracker.CreateLogFilesHandlerName)
machine.Handlers.FcInit = machine.Handlers.FcInit.Remove(firecracker.BootstrapLoggingHandlerName)
```

This leaves only `StartVMM` (launches Firecracker process) and `LoadSnapshot`.

### 6. Parallel prereq + snapshot checks (~5ms saving)

**`go_src/internal/exec/exec_vm_linux.go`** — `runVM()`

Run CheckPrerequisites, CheckSnapshot, and CleanupStaleInstances concurrently
using a simple errgroup pattern:

```go
var prereqErrs []*vm.PrereqError
var snapErr error

var wg sync.WaitGroup
wg.Add(2)
go func() { defer wg.Done(); prereqErrs = vm.CheckPrerequisites(vmPaths) }()
go func() { defer wg.Done(); snapErr = vm.CheckSnapshot(vmPaths, version) }()
go vm.CleanupStaleInstances(vmPaths) // fire-and-forget
wg.Wait()
```

## Files to modify

| File | Change |
|------|--------|
| `go_src/internal/vm/uffd_linux.go` | Fully lazy UFFD, pre-load file, region-aware lazy handler |
| `go_src/internal/vm/machine_linux.go` | Better warmup (20 iters, varied scripts), direct vsock, remove SDK handlers |
| `go_src/internal/vm/rootfs_linux.go` | Add `-XX:CompileThreshold=100` JVM flag |
| `go_src/internal/exec/exec_vm_linux.go` | Parallel prereqs/snapshot checks |

## Expected result

| Phase | Before | After | Savings |
|-------|--------|-------|---------|
| UFFD eager copy | 700ms | 0ms (lazy) | 700ms |
| Lazy faults during exec | 0ms | ~80ms | -80ms |
| File I/O (overlapped with FC) | 200ms | 0ms (overlapped) | 200ms |
| run_script (JVM) | 360ms | ~160ms | 200ms |
| waitForVsock | 20ms | ~1ms | 19ms |
| SDK handlers | 150ms | ~110ms | 40ms |
| Prereqs (parallel) | 10ms | 0ms (overlapped) | 10ms |

**Estimated total**: ~110ms (FC launch) + ~80ms (lazy faults) + ~160ms (run_script)
+ ~10ms (vsock + JSON) = **~360-500ms**.

Conservative estimate accounting for uncertainties: **~500-800ms**.

## Verification

### 1. Build
```bash
cd go_src && go vet ./... && CGO_ENABLED=0 go build -o dhg ./cmd/dhg && cp dhg ~/.local/bin/dhg
```

### 2. Rebuild snapshot (required for warmup changes)
```bash
dhg vm clean --version 41.1
dhg vm prepare --version 41.1
```

Watch warmup output — should see ~20 iterations with decreasing run_script times.

### 3. Test fully lazy UFFD
```bash
time dhg exec --vm --verbose -c "print('hello')"
```

Should show restore time dramatically reduced (sub-200ms vs previous ~1200ms).

### 4. Compare with File backend
```bash
time DHG_VM_NO_UFFD=1 dhg exec --vm --verbose -c "print('hello')"
```

### 5. Test complex workload (verify lazy faults don't degrade)
```bash
dhg exec --vm -c "
from deephaven import empty_table
t = empty_table(10_000_000).update(['x = i', 'y = x * x'])
print(t.to_string(num_rows=5))
"
```

### 6. Unit tests
```bash
cd go_unit_tests && go test ./...
```
