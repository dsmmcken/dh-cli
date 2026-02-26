# Plan: Sub-500ms VM Execution

## Context

`dhg exec --vm` currently takes ~1 second. The two dominant costs are:
1. **UFFD eager page copy**: ~600ms — copies ALL ~900MB of data pages before resuming VM
2. **Read result via Arrow/gRPC**: ~50-100ms — 2 gRPC round-trips + Arrow serialization + pickle/base64

Most prior optimizations (warmup, handler removal, parallel prereqs, balloon, pre-load) are already implemented. These two changes target the remaining bottlenecks.

## Change 1: Fully Lazy UFFD (~600ms saving)

**File**: `go_src/internal/vm/uffd_linux.go`

Skip the eager `parallelCopy` entirely. Resume the VM immediately and serve page faults lazily from the pre-cached mmap using 2MB-aligned UFFDIO_COPY chunks.

### Modify `doPopulate()` (line 277)

Remove the "wait for preWarm, build copy jobs, parallelCopy" block. After receiving the UFFD fd + regions:
1. Initialize `populatedChunks` map (tracks which 2MB chunks are already populated)
2. Start `lazyFaultHandlerV2` goroutine for ALL faults
3. Return nil immediately (signals `done` to `Wait()` in machine_linux.go)

Gate with `DHG_VM_EAGER_UFFD=1` env var for fallback to old eager behavior.

### New `lazyFaultHandlerV2` + `handleLazyFault` methods

Replace the existing 4KB-only hole handler. On each `_UFFD_EVENT_PAGEFAULT`:
1. Find which memory region contains the fault address
2. Compute 2MB-aligned chunk (clipped to region bounds)
3. Check `populatedChunks` map (mutex-protected) — skip if already done
4. `UFFDIO_COPY` the 2MB chunk from mmap → VM address space
5. Handle `EEXIST` gracefully (benign race)

Why 2MB chunks work:
- The mmap reads zeros for hole regions, so UFFDIO_COPY handles both data and holes
- 200MB touched / 2MB = ~100 faults × ~20μs = ~2ms fault overhead
- Plus ~20ms memcpy at memory bandwidth — total ~25ms vs 600ms eager

### Add to `uffdHandler` struct

```go
lazyMu          sync.Mutex
populatedChunks map[uint64]struct{}
```

### Keep old code

Keep `parallelCopy` and old `lazyFaultHandler` — gated behind `DHG_VM_EAGER_UFFD=1`.

## Change 2: File-Based Result Channel (~50-100ms saving)

**File**: `go_src/internal/vm/vm_runner.py`

### Modify `build_wrapper()` (line 56)

Replace the tail that creates `__dh_result_table` via `empty_table + update + pickle + base64` with:
```python
with open('/tmp/__dh_result.json', 'w') as __dh_f:
    __dh_json.dump(__dh_results_dict, __dh_f)
```

Remove `pickle`, `base64`, `empty_table` imports from wrapper. Add `json`.

### New `read_result_file()` replacing `read_result_table()`

```python
def read_result_file():
    with open('/tmp/__dh_result.json', 'r') as f:
        return json.load(f)
```

### Update `handle_request()` (line 215)

Call `read_result_file()` instead of `read_result_table(session)`.

### Remove dead code

`read_result_table()`, `cleanup_result_table()`, top-level `import base64`, `import pickle`.

## Files to modify

| File | Change |
|------|--------|
| `go_src/internal/vm/uffd_linux.go` | Lazy UFFD: skip eager copy, 2MB chunk fault handler |
| `go_src/internal/vm/vm_runner.py` | File-based results: JSON file instead of DH table |

## Expected timing

| Phase | Before | After |
|-------|--------|-------|
| FC launch + UFFD handshake | 150ms | 150ms |
| Eager UFFDIO_COPY | 600ms | 0ms |
| ResumeVM + vsock | 15ms | 15ms |
| Lazy faults (overlapped) | 0ms | ~25ms |
| run_script (JVM) | 150-200ms | ~130-170ms |
| read_result | 50-100ms | ~2ms |
| Other | 30ms | 30ms |
| **Total** | **~1000ms** | **~350-400ms** |

## Verification

```bash
# Build
cd go_src && go vet ./... && CGO_ENABLED=0 go build -ldflags="-X github.com/dsmmcken/dh-cli/go_src/internal/cmd.Version=0.1.0" -o dhg ./cmd/dhg && cp dhg ~/.local/bin/dhg

# Rebuild snapshot (required — vm_runner.py changed)
dhg vm clean --version <ver>
dhg vm prepare --version <ver>

# Test
time dhg exec --vm --verbose -c "print('hello')"

# Fallback to eager (for comparison)
time DHG_VM_EAGER_UFFD=1 dhg exec --vm --verbose -c "print('hello')"

# Test complex workload
dhg exec --vm -c "from deephaven import empty_table; t = empty_table(1000000).update(['x = i'])"
```
