# Plan: Balloon + Sparse-aware UFFD for Faster VM Restore

## Context

`dh exec --vm` restores a Firecracker VM from snapshot. With UFFD eager page
population, the total exec time is ~2.4s (1.5s restore + 0.5s run_script). The
1.5s is spent UFFDIO_COPY'ing all 2GB of VM memory, even though Deephaven only
uses ~300MB at snapshot time.

Meanwhile, the VM's JVM heap is limited to 1GB (`-Xms1g -Xmx1g`) while non-VM
modes default to `-Xmx4g`. This limits what users can do in VM mode.

**Goal**: Increase JVM heap to 4GB, increase VM memory to 6GB, but *reduce*
effective restore time by snapshotting only used pages (~300-500MB).

## Approach

1. Add a virtio-balloon device to the VM
2. Inflate the balloon before snapshotting to reclaim unused guest pages
3. Snapshot produces a **sparse** memory file (6GB logical, ~500MB physical)
4. UFFD handler detects sparse holes via `SEEK_HOLE`/`SEEK_DATA`
5. Uses `UFFDIO_ZEROPAGE` for holes (instant, no memcpy) and `UFFDIO_COPY` only for data regions

**Expected result**: ~300-500ms UFFD population instead of 1500ms, with 6GB VM
memory and 4GB Deephaven heap.

## Changes

### 1. Increase VM memory and JVM heap

**`go_src/internal/vm/vm.go`**
- Change `DefaultMemSizeMiB` from `2048` to `6144`

**`go_src/internal/vm/rootfs_linux.go`** (initScriptTemplate, line 58-59)
- Change JVM args from `-Xms1g -Xmx1g` to `-Xms512m -Xmx4g`
- Keep `-XX:+AlwaysPreTouch` — this is fine, it pre-touches committed pages
  at JVM start which are then included in the snapshot
- Actually: **remove** `-XX:+AlwaysPreTouch` — with a 4GB max heap, this would
  pre-touch 4GB at JVM start, defeating the balloon. The JVM should only commit
  pages as needed. `-Xms512m` sets the initial heap; the JVM grows to 4GB on
  demand.

### 2. Add balloon device to BootAndSnapshot

**`go_src/internal/vm/machine_linux.go`** — `BootAndSnapshot()` function

After `machine.Start(ctx)` (line 113), before warmup:
```go
// Create balloon device (start with 0 — don't reclaim anything yet)
if err := machine.CreateBalloon(ctx, 0, true, 0); err != nil {
    return fmt.Errorf("creating balloon device: %w", err)
}
```

After JVM warmup (line 153), before `PauseVM`:
```go
// Inflate balloon to reclaim unused guest memory.
// This makes the snapshot memory file sparse — only used pages are stored.
// deflateOnOom=true ensures the guest can reclaim this memory later.
balloonTarget := int64(DefaultMemSizeMiB - 512) // leave ~512 MiB uninflated
if err := machine.UpdateBalloon(ctx, balloonTarget); err != nil {
    return fmt.Errorf("inflating balloon: %w", err)
}
// Give the balloon driver time to reclaim pages
time.Sleep(2 * time.Second)
```

The balloon target should be aggressive — reclaim everything except ~512 MiB
(enough for JVM heap committed pages + OS + runner daemon). The guest balloon
driver frees pages back to the host, making those regions holes in the snapshot.

### 3. Sparse-aware UFFD handler

**`go_src/internal/vm/uffd_linux.go`**

Add new constant:
```go
// UFFDIO_ZEROPAGE ioctl: _IOWR(0xAA, 0x04, struct uffdio_zeropage)
// sizeof(uffdio_zeropage) = 32 bytes (uffdio_range{16} + mode{8} + zeropage{8})
const _UFFDIO_ZEROPAGE = 0xc020aa04
```

Add new struct:
```go
type uffdioZeropage struct {
    start    uint64 // start of range
    len      uint64 // length of range
    mode     uint64 // flags (0)
    zeropage int64  // output: bytes zeroed
}
```

Add a `dataExtent` type and scanner function:
```go
type dataExtent struct {
    offset uint64 // offset in file
    length uint64 // length of data (non-hole) region
}

// scanDataExtents uses SEEK_HOLE/SEEK_DATA to find non-hole regions
// in the snapshot file within the given offset+size range.
func scanDataExtents(f *os.File, offset, size uint64) ([]dataExtent, error) {
    // Walk through the range using lseek(SEEK_DATA) and lseek(SEEK_HOLE)
    // to identify contiguous data (non-hole) extents.
    // Returns only the data extents — everything else is a hole.
}
```

Modify `doPopulate()`:
- After opening the snapshot file (but before mmap), scan for data extents
  per region using `scanDataExtents(f, region.Offset, region.Size)`
- Pass the extent info to `populateRegion()`

Modify `populateRegion()` (or replace with `populateRegionSparse()`):
- For each data extent within the region: `UFFDIO_COPY` (existing logic, chunked)
- For each hole between/around data extents: `UFFDIO_ZEROPAGE`
- The hole regions can use large chunks (the whole hole) since UFFDIO_ZEROPAGE
  just maps zero pages without copying anything

### 4. Handle UFFD events after resume (safety)

**`go_src/internal/vm/uffd_linux.go`**

After eager population completes, the UFFD fd stays open (required for VM
lifetime). If the balloon deflates during execution (deflateOnOom), Firecracker
sends `UFFD_EVENT_REMOVE`. If nobody reads these events, they accumulate but
shouldn't block the VM (the uffd fd is just a regular fd with a buffer).

For safety, add an event drain goroutine after `doPopulate()` returns success:
```go
// After population, drain any UFFD events (balloon deflation, etc.)
// This goroutine runs until Close() cancels the context.
go h.drainEvents()
```

The drain goroutine should `poll()` on the UFFD fd and read/discard events.
For `UFFD_EVENT_REMOVE`, no action is needed since all pages are already
populated. This is defensive — for short-lived exec it likely never fires.

**If this proves unnecessary in testing** (i.e., the VM works fine without
draining), skip this step. The VM is short-lived and destroyed after exec.

### 5. Snapshot metadata update

**`go_src/internal/vm/vm.go`** — `SnapshotMetadata`

Add fields to track balloon state:
```go
type SnapshotMetadata struct {
    Version      string    `json:"version"`
    CreatedAt    time.Time `json:"created_at"`
    DHPort       int       `json:"dh_port"`
    MemSizeMiB   int       `json:"mem_size_mib"`   // VM memory at snapshot time
    BalloonMiB   int       `json:"balloon_mib"`     // balloon inflation at snapshot time
}
```

### 6. Rootfs rebuild required

Changing the JVM args requires rebuilding the rootfs and re-preparing the
snapshot:
```bash
dh vm clean --version 41.1
dh vm prepare --version 41.1
```

Users will need to do this after upgrading `dh` with this change.

## Files to modify

| File | Change |
|------|--------|
| `go_src/internal/vm/vm.go` | `DefaultMemSizeMiB` 2048→6144, metadata fields |
| `go_src/internal/vm/rootfs_linux.go` | JVM args: `-Xms512m -Xmx4g`, remove `-XX:+AlwaysPreTouch` |
| `go_src/internal/vm/machine_linux.go` | Balloon create+inflate in `BootAndSnapshot` |
| `go_src/internal/vm/uffd_linux.go` | UFFDIO_ZEROPAGE, sparse scanning, optional event drain |

## Verification

### 1. Build
```bash
cd go_src && go vet ./... && CGO_ENABLED=0 go build -o dhg ./cmd/dhg && cp dhg ~/.local/bin/dhg
```

### 2. Rebuild rootfs + snapshot
```bash
dh vm clean --version 41.1
dh vm prepare --version 41.1
```

During prepare, verbose output should show balloon inflation and the snapshot
should produce a sparse file. Check:
```bash
ls -lh ~/.dh/vm/snapshots/41.1/snapshot_mem    # logical size (6GB)
du -h ~/.dh/vm/snapshots/41.1/snapshot_mem      # physical size (should be ~500MB)
```

### 3. Test UFFD mode
```bash
dh exec --vm --verbose -c "print('Hello from sparse UFFD!')"
```

Expected verbose output should show populated MiB much less than 6144
(e.g., "UFFD: populated 1 regions, data=500 MiB, zeroed=5644 MiB").

### 4. Performance comparison
```bash
time dh exec --vm --verbose -c "print('hello')"
time DH_VM_NO_UFFD=1 dh exec --vm --verbose -c "print('hello')"
```

Target: UFFD mode under 1.5s total (down from 2.4s).

### 5. Memory-intensive workload (verify 4GB heap works)
```bash
dh exec --vm -c "
from deephaven import empty_table
t = empty_table(10_000_000).update(['x = i', 'y = x * x', 'z = (double)x / 3.14'])
print(t.to_string(num_rows=5))
"
```

### 6. Unit tests
```bash
cd go_unit_tests && go test ./...
```
