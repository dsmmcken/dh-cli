# Fix Pool Render Hang

## Problem

`dh render --vm` hangs when using a warm VM from the pool daemon. The first
render (cold restore) works, but the second render from a pre-warmed pool VM
hangs indefinitely.

## Root Cause

**Two bugs:**

### Bug 1: Premature UFFD handler close (primary hang)

In `pool_linux.go:handleCheckout()`, the UFFD handler was closed immediately
when a VM was checked out:

```go
// Close UFFD handler — page population is already complete.
if pvm.uffdCloser != nil {
    pvm.uffdCloser.Close()
}
```

This was based on the incorrect assumption that all pages are populated by the
time the VM enters the ready queue. In reality, hybrid UFFD mode (the default)
only eagerly copies the first ~256MB of snapshot data. The remaining pages are
served lazily by `lazyFaultHandlerV2` — a goroutine that runs for the lifetime
of the VM and handles page faults on demand via `UFFDIO_COPY`.

When `Close()` is called:
1. The UFFD fd is closed
2. The lazy fault handler goroutine is cancelled
3. The mmap of the snapshot memory file is unmapped

After this, any page fault on an unpopulated page in the VM's guest memory
falls through to the kernel's anonymous-page handler, which returns a **zero
page** instead of the correct snapshot data. This silently corrupts the guest's
memory, causing the JVM, Python runtime, or kernel to crash. With the guest
dead, the vsock `CONNECT` from the host times out — appearing as a hang.

### Bug 2: Read-only disk blocks JVM compilation

Pool VMs use `ReadOnlyDisk: true` so multiple VMs can share the same ext4 disk
image safely. However, the JVM's query compiler writes compiled class files to
`/root/.cache/deephaven/script-session-classes/` on the ext4 root filesystem.
With a read-only disk, these writes fail with `EBADMSG` or
`Couldn't create package directories`.

## Fix

### UFFD handler lifetime (pool_linux.go)

Instead of closing the UFFD handler at checkout, spawn a background goroutine
(`deferUffdClose`) that monitors the VM process. The handler stays alive,
serving lazy page faults, until the Firecracker process exits (killed by the
client's `DestroyCheckedOutVM`). The goroutine also respects the pool's
shutdown signal.

### JVM cache writability (pool_linux.go + rootfs_linux.go + vm_runner.py)

Three-layer fix:

1. **Runtime workaround** (`fillOne` in pool_linux.go): After restoring each
   pool VM, send an init request that mounts tmpfs over `/root/.cache/deephaven`.
   This works with existing snapshots that don't have the init.sh change.

2. **Init script fix** (rootfs_linux.go): Add `mount -t tmpfs tmpfs
   /root/.cache/deephaven` before the JVM starts. New snapshots built with this
   change have writable JVM cache from boot time.

3. **Runner daemon fallback** (vm_runner.py): `_ensure_writable_jvm_cache()`
   checks at daemon startup and mounts tmpfs if the root fs is read-only.

## Benchmarks

All benchmarks run on the same sandbox with the iris dashboard test script.

| Scenario | Wall Time | Notes |
|----------|-----------|-------|
| Cold render (before fix) | 10404ms | Baseline, no pool |
| Pool render (before fix) | HANG | UFFD handler closed, VM crashes |
| Pool render #1 (after fix) | 4619ms | Fresh pool VM, UFFD enabled |
| Pool render #2 (after fix) | 4431ms | Backfilled VM |
| Pool render #3 (after fix) | 4760ms | Backfilled VM |

Pool renders are ~55% faster than cold renders, saving the VM restore time
(~300ms) and benefiting from warm V8 compile cache and JSAPI file cache.

## Files Changed

- `src/internal/vm/pool_linux.go` — UFFD handler lifetime fix + JVM cache mount
- `src/internal/vm/rootfs_linux.go` — init script tmpfs mount for JVM cache
- `src/internal/vm/vm_runner.py` — runtime JVM cache writability check
