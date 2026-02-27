# Plan: Sub-500ms VM Execution — Phase 2

## Context

After phase 1 (lazy UFFD + file-based results), `dh exec --vm` is at ~745ms (930ms wall).
Timing breakdown from `--verbose`:

```
resolve=0ms prereqs=0ms restore=62ms vsock=612ms
build_wrapper_ms=31 run_script_ms=156 read_result_ms=0
total=675ms (since entry=675ms)
real 0m0.930s
```

Three bottlenecks remain:
1. **425ms of page faults** — vsock=612ms but only 187ms of measured in-VM work
2. **255ms Go process startup** — 930ms wall minus 675ms internal
3. **187ms in-VM work** — build_wrapper(31ms) + run_script(156ms) — minor but worth trimming

## Change 1: Hybrid UFFD — pre-copy hot pages (~200-300ms saving)

**File**: `go_src/internal/vm/uffd_linux.go`

### Problem

Fully lazy UFFD means every page the runner daemon touches after resume triggers a
fault + VM exit + UFFDIO_COPY + VM re-entry (~0.5-1ms per fault). The Python
interpreter, stdlib, socket/json modules, JVM code cache, and DH classes all
fault in cold — hundreds of faults before `handle_request` timing even starts.

### Approach

Pre-copy the first N MB of data extents before `ResumeVM`, then lazy for the rest.
The "hot" pages (kernel, Python interpreter, JVM code cache) tend to be at low
file offsets since they're loaded early during boot.

In `doPopulate()`, after receiving the UFFD fd + regions:
1. Compute the set of "eager" chunks: first 256MB of data extents
2. UFFDIO_COPY those chunks (parallel, same as old eager path but smaller)
3. Mark them in `populatedChunks` so the lazy handler skips them
4. Start lazy handler for remaining faults
5. Signal `done`

```go
// Pre-copy the first 256MB of data extents to avoid cold-start page faults
// on the critical path (Python interpreter, JVM, kernel). Lazy for the rest.
const eagerPreloadBytes = 256 * 1024 * 1024

var eagerJobs []copyJob
var eagerBytes uint64
for _, ri := range regionInfos {
    base := ri.region.BaseHostVirtAddr
    regionStart := ri.region.Offset
    for _, ext := range ri.extents {
        if eagerBytes >= eagerPreloadBytes {
            break
        }
        // ... build copyJob for this extent (or portion of it) ...
        // ... mark chunks in populatedChunks ...
    }
}
parallelCopy(eagerJobs, copyWorkers)
```

The 256MB threshold is tunable via `DH_VM_EAGER_MB` env var for experimentation.

### Expected impact

- Adds ~60-120ms to restore (256MB at ~2-4GB/s effective with parallel copy)
- Saves ~300ms+ in page faults (most critical-path pages pre-populated)
- Net: ~200-300ms saving

## Change 2: Parallel fault handler (~50-100ms saving)

**File**: `go_src/internal/vm/uffd_linux.go`

### Problem

`lazyFaultHandlerV2` is single-threaded. With `DefaultVCPUCount=2`, two vCPUs
can fault simultaneously, but only one is served at a time. While one 2MB
UFFDIO_COPY blocks (~0.5ms), the other vCPU's fault waits in the kernel.

### Approach

Dispatch `handleLazyFault` to a goroutine pool instead of calling inline:

```go
faultCh := make(chan uint64, 64)
for w := 0; w < 4; w++ {
    go func() {
        for faultAddr := range faultCh {
            h.handleLazyFault(uffdFd, faultAddr, regions)
        }
    }()
}

// In event loop:
case _UFFD_EVENT_PAGEFAULT:
    faultAddr := *(*uint64)(unsafe.Pointer(&msg[16]))
    faultCh <- faultAddr
```

The `populatedChunks` mutex already provides thread safety. 4 workers matches
the copy worker count and gives headroom beyond the 2 vCPUs.

### Expected impact

- Overlaps UFFDIO_COPY with fault delivery — second vCPU unblocked faster
- ~50-100ms saving on remaining lazy faults (the ones after the 256MB eager region)

## Change 3: Reduce Go process startup (~100-200ms saving)

### Problem

255ms elapses between process start and `runVM` entry. `resolve=0ms` and
`prereqs=0ms` shows config/checks are instant — the overhead is Go binary
startup + Cobra initialization.

### Investigation needed

Add a timestamp at the very top of `main()` and print it in verbose mode to
separate Go runtime init from Cobra dispatch:

**File**: `go_src/cmd/dhg/main.go`

```go
var processStart = time.Now()
```

**File**: `go_src/internal/exec/exec.go` — in `Run()`:

```go
if cfg.Verbose {
    fmt.Fprintf(cfg.Stderr, "Go startup: %dms\n", time.Since(processStart).Milliseconds())
}
```

### Potential causes & fixes

1. **Cobra init overhead**: Cobra registers all commands/flags at init time even
   when only `exec` is used. Fix: use lazy command registration or a minimal
   command parser for the hot path.

2. **Large binary size**: If the binary embeds large assets (runner.py, etc.),
   the OS page-faults them in at startup. Fix: check binary size, strip debug
   symbols (`-ldflags="-s -w"`), minimize embeds.

3. **init() functions**: Go packages with `init()` run before `main()`. Any
   expensive init in imported packages (logrus, firecracker-go-sdk, cobra)
   adds to startup. Fix: lazy imports or conditional registration.

4. **`-ldflags="-s -w"`**: Strip symbol table and DWARF debug info. Can reduce
   binary size significantly, improving mmap/page-in time.

### Quick test

```bash
# Check current binary size
ls -lh ~/.local/bin/dhg

# Build with stripped symbols
CGO_ENABLED=0 go build -ldflags="-s -w -X ..." -o dhg ./cmd/dhg

# Compare
ls -lh dhg
time dh exec --vm --verbose -c "print('hello')"
```

## Change 4: Reduce build_wrapper overhead (minor, ~10-20ms)

**File**: `go_src/internal/vm/vm_runner.py`

`build_wrapper_ms=31ms` is surprisingly high for string concatenation. This
includes page faults for the `ast` module (used by `get_assigned_names`).
Since AST parsing is only needed for `show_tables` (to find assigned variable
names), skip it when `show_tables=False`:

```python
if show_tables:
    assigned_names = get_assigned_names(code)
else:
    assigned_names = set()
```

## Files to modify

| File | Change |
|------|--------|
| `go_src/internal/vm/uffd_linux.go` | Hybrid eager/lazy UFFD, parallel fault handler |
| `go_src/cmd/dhg/main.go` | Process start timestamp for diagnostics |
| `go_src/internal/exec/exec.go` | Report Go startup time in verbose mode |
| `go_src/internal/vm/vm_runner.py` | Skip AST parse when show_tables=False |

## Expected combined timing

| Phase | Current | After | Savings |
|-------|---------|-------|---------|
| Go startup | 255ms | ~100-150ms | 100-150ms |
| Restore (hybrid eager) | 62ms | ~130ms | -68ms |
| Page faults (reduced) | 425ms | ~100-150ms | 275-325ms |
| build_wrapper | 31ms | ~10ms | 21ms |
| run_script | 156ms | 156ms | 0ms |
| read_result | 0ms | 0ms | 0ms |
| **Total wall** | **~930ms** | **~500-600ms** | **330-430ms** |

## Verification

```bash
# Build with stripped symbols
cd go_src && CGO_ENABLED=0 go build -ldflags="-s -w -X github.com/dsmmcken/dh-cli/go_src/internal/cmd.Version=0.1.0" -o dhg ./cmd/dhg && cp dhg ~/.local/bin/dhg

# No snapshot rebuild needed (changes are Go-side + lazy handler only)

# Test
time dh exec --vm --verbose -c "print('hello')"

# Compare eager preload sizes
DH_VM_EAGER_MB=128 time dh exec --vm --verbose -c "print('hello')"
DH_VM_EAGER_MB=512 time dh exec --vm --verbose -c "print('hello')"

# Verify complex workload still works
dh exec --vm -c "from deephaven import empty_table; t = empty_table(1000).update(['x = i'])"
```
