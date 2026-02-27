# Research: Sharing Host Filesystem Files with Firecracker MicroVM

**Date:** 2026-02-27
**Context:** The `dh exec --vm` command restores a Firecracker microVM from snapshot, executes user Python code via vsock, and tears down the VM. The VM has a single ext4 rootfs drive, communicates via vsock (port 10000), and runs on Linux with KVM. Current architecture sends code as a string over the vsock JSON protocol. The goal is to enable user scripts to reference files from the host workspace (e.g., `import mymodule`, `open("data.csv")`) with zero or near-zero impact on the ~50ms snapshot restore time.

---

## Current Architecture Summary

From `src/internal/vm/machine_linux.go`:
- Single drive: `rootfs` (ext4, read-write, at `snapDir/disk.ext4`)
- Vsock: CID=3, port 10000, UDS path embedded in snapshot state
- Snapshot restore: File backend (MAP_PRIVATE demand paging) or UFFD (eager population)
- The disk path must match what was used at snapshot time (Firecracker re-binds block devices at their original paths)
- The vsock UDS path is embedded in the snapshot binary state and must exist at the same absolute path on restore

---

## Approach 1: Additional Block Device (Second Drive)

### How It Works
Configure Firecracker with two drives at snapshot creation time: the rootfs drive and a second "workspace" drive. At restore time, swap the backing file of the second drive with a freshly-prepared ext4 image containing the user's workspace files.

### Firecracker Compatibility

**Critical constraint:** Firecracker requires that block devices at restore time match the configuration embedded in the snapshot. You cannot *add* new drives at snapshot restore time -- all drives must have been present when the snapshot was created. However, you **can** point a drive_id at a different backing file at the same path.

The approach is:
1. During `dh vm prepare`, configure two drives: `rootfs` (the Deephaven rootfs) and `workspace` (an empty ext4 image at a known path)
2. The guest kernel sees `/dev/vdb` and can mount it
3. At snapshot time, both drives are part of the VM state
4. At restore time, replace the file at the workspace drive's path with a new ext4 image containing the user's files

**Key API detail:** The `PATCH /drives/{drive_id}` API can update the `path_on_host` for a block device on a running VM (triggering a virtio rescan), but this is NOT available for snapshot-restored VMs. Instead, you simply place a different file at the same filesystem path before calling LoadSnapshot.

### Impact on Startup Time
- **Preparing the ext4 image:** Creating a small ext4 image and copying files into it takes ~10-50ms for a handful of files (dominated by `mkfs.ext4` or `debugfs` write overhead)
- **Restore overhead:** Zero additional restore overhead -- Firecracker's demand paging loads disk blocks lazily on first access
- **Total added latency:** ~10-50ms for image preparation (can be parallelized with other restore setup)

### Complexity
- **Medium.** Requires modifying `BootAndSnapshot` to add a second drive, modifying `RestoreFromSnapshot` to prepare and swap the workspace image, and adding guest-side mount logic (either in the init script or triggered post-restore)
- Guest must mount `/dev/vdb` at a known path (e.g., `/workspace`). This mount needs to happen either:
  - At boot time via init script (but the mount point is empty at snapshot time)
  - Post-restore via a vsock command before executing user code

### Read/Write Capability
- **Read-write** if the drive is configured as `is_read_only: false`
- **Read-only** is also possible

### Lazy Loading
- **Yes, inherently.** Firecracker's virtio-block serves pages on demand from the backing file. Only accessed blocks are read.

### Verdict
**Strong candidate.** Well-supported by Firecracker, no new kernel/device dependencies, inherent lazy loading via demand paging. Main challenge is preparing the ext4 image fast enough and mounting it in the guest post-restore.

---

## Approach 2: virtiofs / vhost-user-fs

### Firecracker Support
**Not supported.** Firecracker intentionally provides a minimal device model with only 5 emulated devices: virtio-net, virtio-block, virtio-vsock, serial console, and a minimal keyboard controller. [Virtio-fs was explicitly rejected](https://github.com/firecracker-microvm/firecracker/issues/1180) in August 2020 with: "Closing this for now as it doesn't look like a path we need to take Firecracker on."

A [WIP pull request (#1351)](https://github.com/firecracker-microvm/firecracker/pull/1351) attempted to implement a VirtioFS device backend but was never merged.

### Rationale for Rejection
Block storage (virtio-block) is simpler than filesystem passthrough, both in the virtio implementation and in the host filesystem implementation. Simplicity benefits security (smaller attack surface) and performance isolation reasoning.

### Verdict
**Not viable.** Would require forking Firecracker to add a new virtio device type. The upstream project has no plans to add this.

---

## Approach 3: virtio-9p (Plan 9 filesystem)

### Firecracker Support
**Not supported.** Firecracker [rejected 9p-based filesystem sharing for security concerns](https://github.com/firecracker-microvm/firecracker/issues/1180). The 9p protocol has known performance limitations -- it does not offer local filesystem semantics, and performance "cannot be alleviated without major protocol changes."

### Verdict
**Not viable.** Same as virtiofs -- not in Firecracker's device model and explicitly rejected upstream.

---

## Approach 4: OverlayFS (Guest-Side Overlay)

### How It Works
This is a well-established pattern used by [E2B](https://e2b.dev/blog/scaling-firecracker-using-overlayfs-to-save-disk-space) and [firecracker-containerd](https://github.com/firecracker-microvm/firecracker-containerd/blob/main/tools/image-builder/README.md):

1. The rootfs is a read-only squashfs/ext4 base image (drive 1, `vda`)
2. A second drive (drive 2, `vdb`) provides a writable ext4 overlay layer
3. An `overlay-init` script runs as `init=` before the real init:
   - Mounts the overlay device
   - Creates `upperdir` and `workdir` directories on it
   - Calls `mount -t overlay overlay -o lowerdir=/,upperdir=...,workdir=... /merged`
   - Does `pivot_root` to `/merged`
   - Execs the real `/sbin/init`

### Kernel Boot Parameters
```
init=/sbin/overlay-init overlay_root=vdb
```

### Compatibility with Snapshots
The overlay is set up during boot, before the snapshot is taken. At snapshot time, the overlay is already mounted. On restore:
- The **rootfs** (lowerdir) can be the same read-only base image shared across all VMs
- The **overlay** (upperdir on vdb) can be swapped to a fresh ext4 image containing workspace files

This is a variant of Approach 1 but with the added benefit that the workspace files appear as part of the root filesystem (no separate mount point needed). Files placed in the overlay's upperdir take precedence over the rootfs.

### Impact on Startup Time
- Same as Approach 1: ~10-50ms for preparing the overlay ext4 image
- No additional restore-time overhead

### Complexity
- **High initial, then medium ongoing.** Requires:
  - Restructuring the rootfs build to produce a read-only base + overlay-init script
  - Changing `BootAndSnapshot` to configure two drives with overlay boot params
  - Preparing workspace overlay images at exec time
- The E2B blog and `njapke/overlayfs-in-firecracker` GitHub repo provide working reference implementations

### Read/Write Capability
- **Read-write.** The overlay's upperdir captures all writes.

### Lazy Loading
- **Yes.** Files in the overlay ext4 image are loaded on demand by virtio-block. Files in the base rootfs are loaded from the separate read-only image.

### Verdict
**Strong candidate for a future iteration**, but higher complexity than Approach 1. Best suited if we also want to share a single read-only rootfs across concurrent VMs (space savings). For the immediate file-sharing need, Approach 1 is simpler.

---

## Approach 5: Injecting Files into ext4 Image (debugfs / loop mount)

### How It Works
Before restore, modify the existing `disk.ext4` snapshot disk to inject workspace files. Two sub-approaches:

#### 5a: debugfs (no mount required)
```bash
debugfs -w -R 'mkdir /workspace' disk.ext4
debugfs -w -R 'write /host/path/to/file.py /workspace/file.py' disk.ext4
```
- Operates directly on the ext4 image file without mounting
- No root/CAP_SYS_ADMIN required
- Can be scripted with `-f commandfile`

#### 5b: Loop mount + copy
```bash
mount -o loop disk.ext4 /mnt/tmp
cp -r /host/workspace/* /mnt/tmp/workspace/
umount /mnt/tmp
```
- Requires root or CAP_SYS_ADMIN for the mount
- More intuitive file operations
- Performance: loop device mount itself is fast (~1ms), file copy depends on size, umount flushes to backing file

### Performance Characteristics

**debugfs:** Each `write` command opens the image, writes directly to free blocks, and updates the inode. For small files (< 100 KB), this is ~1-5ms per file. For 10 files, ~10-50ms total. However, debugfs spawns a process per invocation (or batches with `-f`).

**Loop mount:** Mount is ~1-3ms. Copy of 10 small files is ~5-10ms. Unmount with sync is ~5-20ms. Total: ~15-30ms.

### Compatibility with Snapshots
**Major problem:** The snapshot's disk.ext4 captures the exact disk state at snapshot time. If you modify the disk image before restore, the guest kernel's ext4 driver may have cached metadata (superblock, inode tables, block bitmaps) in the VM's memory snapshot that now conflicts with the on-disk state. This leads to filesystem corruption.

**Workaround:** Use a **copy** of the disk image, modify the copy, and point the drive at the copy. But this reintroduces the cost of copying a potentially multi-GB disk image on every invocation (the exact thing the current code avoids: "Use snapshot disk directly -- the VM is ephemeral and destroyed after exec. This avoids copying a multi-GB disk image on every invocation.").

### Complexity
- **Low** for the injection itself
- **High** for correctness -- the ext4 cache coherency problem is a showstopper unless you copy the disk

### Read/Write Capability
- **Read-write** (with the copy approach)

### Lazy Loading
- **No.** Files must be written to the image upfront before restore.

### Verdict
**Not recommended** as the primary approach due to the disk copy requirement and cache coherency risks. However, `debugfs` is a useful building block for preparing workspace ext4 images for Approach 1 (where the second drive is a fresh image, not the snapshot disk).

---

## Approach 6: vsock-Based File Serving (Custom Protocol)

### How It Works
Run a file server on the host that listens on the vsock UDS. The guest runs a FUSE daemon that intercepts filesystem calls (open, read, stat, readdir) and translates them into vsock requests to the host.

Architecture: `Guest app -> FUSE mount -> vsock client -> [vsock transport] -> Host file server`

### Implementation
1. **Host side:** A Go goroutine that accepts vsock connections on a dedicated port (e.g., 10001) and serves file operations (stat, readdir, read, open) from a host directory
2. **Guest side:** A Python or C FUSE daemon that mounts at `/workspace` and forwards all operations over vsock to the host

### Compatibility with Snapshots
**Excellent.** The FUSE daemon can be part of the snapshot -- it starts, connects to vsock port 10001, and serves files. On restore, the host starts the file server before resuming the VM. The FUSE mount persists across snapshot/restore (it's in the guest kernel's VFS state).

**Caveat:** The FUSE daemon's vsock connection will be stale after restore. It needs to reconnect. This can be handled by having the FUSE daemon retry connections or by establishing a new connection per request.

### Impact on Startup Time
- **Zero upfront cost.** Files are loaded lazily on first access.
- **Per-file latency:** Each file operation involves a vsock round-trip. vsock latency is ~0.1-0.5ms per operation. For `open()` + `read()` of a small file: ~1-2ms.
- **First access penalty:** The first file access after restore requires reconnecting the vsock, which adds ~1-5ms.

### Complexity
- **High.** Requires:
  - A host-side file server (Go, ~200-400 lines)
  - A guest-side FUSE daemon (Python with `fusepy`, ~300-500 lines, or C with libfuse)
  - A wire protocol for file operations (could use 9p-over-vsock, NFS-over-vsock, or a custom JSON/protobuf protocol)
  - Error handling for disconnections, timeouts, etc.
  - The FUSE daemon must be included in the VM image and started at boot

### Read/Write Capability
- **Read-only** is simplest and sufficient for the use case
- **Read-write** is possible but adds significant complexity (write-back, sync, conflict resolution)

### Lazy Loading
- **Yes, inherently.** This is the primary advantage -- files are fetched on demand.

### Verdict
**Best approach for true lazy loading and large workspaces**, but the implementation complexity is high. The per-file vsock latency is low enough for most use cases. A good long-term solution.

---

## Approach 7: FUSE over vsock (virtiofsd-style)

### How It Works
This is a specific implementation of Approach 6, using the FUSE protocol (not a custom protocol). [Docker Desktop uses this approach](https://www.docker.com/blog/deep-dive-into-new-docker-desktop-filesharing-implementation/) for host file sharing on macOS/Windows via Hyper-V.

Architecture:
- Host: A FUSE server that reads FUSE protocol messages from vsock and translates them to host filesystem operations
- Guest: A FUSE client kernel driver (or userspace daemon) that forwards FUSE requests over vsock

### Compatibility with Firecracker
Firecracker does not implement the vhost-user protocol needed by virtiofsd. However, a **userspace implementation** is possible:
- The guest runs a FUSE daemon that accepts FUSE kernel requests via `/dev/fuse` and forwards them over vsock
- The host runs a companion daemon that receives FUSE operations and executes them on the host filesystem

This is essentially Approach 6 but with the FUSE wire protocol instead of a custom one.

### Versus Custom Protocol (Approach 6)
- **Pro:** FUSE protocol is well-defined and feature-complete
- **Con:** FUSE protocol is complex (dozens of opcodes), most of which are unnecessary for read-only file serving
- **Con:** FUSE has inherent overhead from kernel transitions (app -> kernel -> FUSE daemon -> vsock -> host)

### Verdict
**Viable but over-engineered** for the use case. A custom protocol (Approach 6) with just stat/readdir/read operations would be simpler and faster. FUSE protocol adds unnecessary complexity for read-only file serving.

---

## Approach 8: NBD (Network Block Device) over vsock

### How It Works
Run an [NBD server on the host](https://rwmj.wordpress.com/2019/10/21/nbd-over-af_vsock/) (e.g., `nbdkit`) that exposes a directory as a block device. The guest uses `nbd-client` to connect over AF_VSOCK and mount the remote block device.

The host creates an ext4 image of the workspace and serves it via NBD over vsock. The guest connects with `nbd-client` using AF_VSOCK (CID 2 for host, dedicated port), then mounts the resulting `/dev/nbd0` as ext4.

### Compatibility with Firecracker/Snapshots
- AF_VSOCK is supported by Firecracker
- NBD client is a kernel module (`nbd.ko`) that must be present in the guest kernel
- The NBD connection would be broken by snapshot/restore -- the guest needs to reconnect after restore
- `nbdkit` supports vsock via `nbdkit --vsock` flag
- `libnbd` has [native AF_VSOCK support](https://libguestfs.org/nbd_connect_vsock.3.html)

### Impact on Startup Time
- **Host side:** Starting `nbdkit` is ~5-10ms. Creating the ext4 image is ~10-50ms.
- **Guest side:** `nbd-client` connect + mount is ~5-10ms after restore
- **Total:** ~20-60ms, most parallelizable with restore

### Complexity
- **Medium-High.** Requires:
  - `nbdkit` on the host (or a custom Go NBD server)
  - `nbd-client` and `nbd.ko` kernel module in the guest
  - Reconnection logic after snapshot restore
  - Creating ext4 images on the host for each invocation

### Read/Write Capability
- **Read-write** is possible but dangerous (write-back over network)
- **Read-only** is safer and sufficient

### Lazy Loading
- **Yes.** NBD serves blocks on demand. Only accessed blocks are transferred over vsock.

### Verdict
**Technically interesting but adds unnecessary dependencies** (nbdkit, nbd kernel module, reconnection logic). The block device approach (Approach 1) achieves similar results with less complexity since Firecracker already has virtio-block.

---

## Approach 9: Pre-populating Files via vsock Protocol

### How It Works
Extend the existing vsock JSON protocol to include file contents alongside code:

```json
{
  "code": "import mymodule\nprint(mymodule.hello())",
  "files": {
    "/workspace/mymodule.py": "def hello(): return 'world'",
    "/workspace/data.csv": "a,b,c\n1,2,3\n"
  }
}
```

The runner daemon (`vm_runner.py`) writes files to disk before executing the code.

### Implementation
Modify `VsockRequest` in Go:
```go
type VsockRequest struct {
    Code          string            `json:"code"`
    Files         map[string]string `json:"files,omitempty"`
    ShowTables    bool              `json:"show_tables"`
    ShowTableMeta bool             `json:"show_table_meta"`
}
```

Modify `handle_request` in `vm_runner.py`:
```python
files = request.get("files", {})
for path, content in files.items():
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w") as f:
        f.write(content)
```

### Compatibility with Snapshots
**Perfect.** Uses the existing vsock channel, which already works with snapshot restore.

### Impact on Startup Time
- **Zero restore overhead.** Files are sent after restore, as part of the existing vsock request.
- **File transfer time:** Proportional to total file size. For 100 KB of files over vsock: ~1-5ms. For 10 MB: ~50-100ms. For 100 MB+: becomes a bottleneck.
- **Serialization overhead:** JSON base64-encoding for binary files doubles the payload size.

### Complexity
- **Very low.** ~20 lines of Go code + ~10 lines of Python. No new dependencies, no kernel modules, no new daemons.

### Read/Write Capability
- **Read-write** (files are written to the guest filesystem).

### Lazy Loading
- **No.** All files must be sent upfront before code execution. This is the main limitation -- if the user has a large workspace but only needs one file, all files are still transferred.

### Optimizations
- Use binary encoding (e.g., tar stream) instead of JSON for large payloads
- Support `files_from_paths` that reads host files and sends only those referenced in the code (via simple import/open analysis)
- Compress the payload with zstd

### Verdict
**Best "bang for the buck" approach for small workspaces** (< 10 MB). Trivial to implement, zero new dependencies, works perfectly with snapshots. The main limitation is scalability to large workspaces. **This should be the first approach implemented**, with a more sophisticated approach (1 or 6) added later if needed.

---

## Approach 10: Device Mapper (dm-linear / dm-snapshot)

### How It Works
Use Linux device mapper to create a composite block device that combines the rootfs with workspace data. [Julia Evans documented this approach for Firecracker](https://jvns.ca/blog/2021/01/27/day-47--using-device-mapper-to-manage-firecracker-images/) and [Parandrus expanded on it](https://parandrus.dev/devicemapper/).

```bash
# Create loop device for base rootfs (read-only)
LOOP=$(losetup --find --show --read-only rootfs.ext4)
SZ=$(blockdev --getsz rootfs.ext4)

# Create loop device for overlay (per-VM writable layer)
OVERLAY_SZ=$(blockdev --getsz overlay.ext4)
LOOP2=$(losetup --find --show overlay.ext4)

# Create dm-linear base (rootfs + zero padding)
printf "0 $SZ linear $LOOP 0\n$SZ $OVERLAY_SZ zero" | dmsetup create base-$ID

# Create dm-snapshot (copy-on-write)
echo "0 $OVERLAY_SZ snapshot /dev/mapper/base-$ID $LOOP2 P 8" | dmsetup create overlay-$ID
```

The resulting `/dev/mapper/overlay-$ID` is a block device that reads from the rootfs but writes to the overlay. This can be passed as the Firecracker drive backing file.

### Compatibility with Snapshots
**Problematic for the same reasons as Approach 5.** The guest kernel's ext4 caches are captured in the snapshot. Changing the underlying block device content (even through device mapper) creates cache incoherency.

However, for a **second drive** (Approach 1 + device mapper), this works well: the workspace drive's backing file can be a device mapper device without cache coherency issues (since the guest hasn't cached any workspace data at snapshot time -- the drive was empty/fresh).

### Impact on Startup Time
- `losetup`: ~1ms each
- `dmsetup create`: ~2-5ms each
- Total for two-layer setup: ~5-15ms

### Complexity
- **Medium.** Requires root for dmsetup, cleanup of loop devices and dm targets after VM exits, and error handling for stale devices.
- Compared to a plain ext4 image (Approach 1), device mapper adds complexity for marginal benefit in the single-VM-at-a-time use case.

### Read/Write Capability
- **Read-write** (writes go to the overlay layer)

### Lazy Loading
- **Yes.** Block-level demand paging.

### Verdict
**Over-engineered for single-VM use.** Device mapper shines when sharing a base image across many concurrent VMs (the primary use case in E2B/Kata Containers). For our sequential, single-VM-at-a-time use case, a plain ext4 image for the workspace drive (Approach 1) is simpler and equally performant.

---

## Recommendation: Phased Implementation

### Phase 1: vsock File Pre-population (Approach 9) -- Immediate

**Effort:** ~2-4 hours
**Added latency:** 0ms (restore) + proportional to file size (transfer)
**Limitation:** Not suitable for large workspaces (> ~10 MB)

Implementation:
1. Add `Files map[string]string` to `VsockRequest`
2. On the host: read workspace files, populate the `Files` map
3. In `vm_runner.py`: write files to disk before executing code
4. Support a `--workspace` flag or automatic detection of imports/file references

### Phase 2: Second Block Device (Approach 1) -- Near-term

**Effort:** ~1-2 days
**Added latency:** ~10-50ms (ext4 image preparation, parallelizable)
**Benefit:** Scales to large workspaces, inherent lazy loading

Implementation:
1. Modify `BootAndSnapshot` to add a second drive (`workspace`, empty ext4, `/dev/vdb`)
2. Add guest-side mount logic: either in init script (mount at boot) or via vsock command post-restore
3. At exec time: create a fresh ext4 image with workspace files (using `debugfs -w` for speed, or `mkfs.ext4` + loop mount)
4. Place the image at the workspace drive's path before calling `RestoreFromSnapshot`
5. The guest mounts `/dev/vdb` at `/workspace` (or it's already mounted from snapshot)

### Phase 3: vsock FUSE File Server (Approach 6) -- Long-term

**Effort:** ~3-5 days
**Added latency:** 0ms upfront, ~1-2ms per file access
**Benefit:** True lazy loading, works with arbitrarily large workspaces

Implementation:
1. Host: Go file server goroutine on vsock port 10001
2. Guest: Python FUSE daemon (fusepy) mounting at `/workspace`
3. Wire protocol: simple JSON-RPC over vsock for stat/readdir/read/open
4. Include FUSE daemon in VM image, start at boot, capture in snapshot
5. FUSE daemon reconnects to host vsock on first access after restore

---

## Summary Table

| Approach | FC Compatible | Snapshot OK | Startup Impact | Lazy Load | Complexity | R/W |
|----------|:------------:|:-----------:|:--------------:|:---------:|:----------:|:---:|
| 1. Second block device | Yes | Yes | +10-50ms | Yes | Medium | R/W |
| 2. virtiofs | **No** | N/A | N/A | N/A | N/A | N/A |
| 3. virtio-9p | **No** | N/A | N/A | N/A | N/A | N/A |
| 4. OverlayFS | Yes | Yes | +10-50ms | Yes | High | R/W |
| 5. Inject into ext4 | Partial* | **No*** | +10-50ms | No | Low | R/W |
| 6. vsock file server | Yes | Yes** | +0ms | **Yes** | High | R/O |
| 7. FUSE over vsock | Yes | Yes** | +0ms | **Yes** | Very High | R/O |
| 8. NBD over vsock | Yes | Yes** | +20-60ms | Yes | High | R/O |
| 9. vsock pre-populate | **Yes** | **Yes** | **+0ms** | No | **Very Low** | R/W |
| 10. Device mapper | Yes | Partial* | +5-15ms | Yes | Medium | R/W |

\* Requires copying the disk image to avoid cache coherency issues with the snapshot disk
\** Requires reconnection logic after snapshot restore

**Recommended implementation order:** 9 (immediate) -> 1 (near-term) -> 6 (long-term if needed)

---

## References

- [Firecracker Snapshot Support](https://github.com/firecracker-microvm/firecracker/blob/main/docs/snapshotting/snapshot-support.md)
- [Firecracker Issue #1180: Host Filesystem Sharing](https://github.com/firecracker-microvm/firecracker/issues/1180)
- [Firecracker Issue #4014: Drive mounts for snapshot loading](https://github.com/firecracker-microvm/firecracker/issues/4014)
- [Firecracker PR #1351: WIP VirtioFS device backend](https://github.com/firecracker-microvm/firecracker/pull/1351)
- [Firecracker Discussion #3061: Multiple VMs sharing rootfs](https://github.com/firecracker-microvm/firecracker/discussions/3061)
- [E2B: Scaling Firecracker with OverlayFS](https://e2b.dev/blog/scaling-firecracker-using-overlayfs-to-save-disk-space)
- [Parandrus: Space Efficient Filesystems for Firecracker](https://parandrus.dev/devicemapper/)
- [Julia Evans: Using device mapper with Firecracker](https://jvns.ca/blog/2021/01/27/day-47--using-device-mapper-to-manage-firecracker-images/)
- [njapke/overlayfs-in-firecracker](https://github.com/njapke/overlayfs-in-firecracker)
- [firecracker-containerd overlay-init](https://github.com/firecracker-microvm/firecracker-containerd/blob/main/tools/image-builder/README.md)
- [NBD over AF_VSOCK (Richard WM Jones)](https://rwmj.wordpress.com/2019/10/21/nbd-over-af_vsock/)
- [Docker Desktop FUSE File Sharing](https://www.docker.com/blog/deep-dive-into-new-docker-desktop-filesharing-implementation/)
- [Firecracker PATCH /drives API](https://github.com/firecracker-microvm/firecracker/blob/main/docs/api_requests/patch-block.md)
- [debugfs man page](https://man7.org/linux/man-pages/man8/debugfs.8.html)
