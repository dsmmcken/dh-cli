# Filesystem Copy-on-Write (CoW) and Overlay Strategies for Firecracker VMs

## Research Report

**Date:** 2026-02-11
**Purpose:** Evaluate strategies for efficiently sharing a base repository image (git working directory with pre-installed dependencies) across many parallel Firecracker microVMs, where each VM writes independently.

**Key Requirements:**
- Minimal disk overhead (shared base, independent writes)
- Fast "clone" times (target: sub-second to low seconds)
- Scale to 50+ concurrent VMs
- Ability to extract changes (diff) after work is done
- Compatible with Firecracker's block device model

---

## Table of Contents

1. [OverlayFS](#1-overlayfs)
2. [Device Mapper Thin Provisioning (dm-thin)](#2-device-mapper-thin-provisioning-dm-thin)
3. [Btrfs Snapshots and Subvolumes](#3-btrfs-snapshots-and-subvolumes)
4. [ZFS Clones and Snapshots](#4-zfs-clones-and-snapshots)
5. [QCOW2 Backing Files](#5-qcow2-backing-files)
6. [virtiofs](#6-virtiofs)
7. [9pfs (Plan 9 Filesystem)](#7-9pfs-plan-9-filesystem)
8. [Squashfs / EROFS as Read-Only Base](#8-squashfs--erofs-as-read-only-base)
9. [NBD (Network Block Device)](#9-nbd-network-block-device)
10. [Performance Comparison and Recommendations](#10-performance-comparison-and-recommendations)
11. [Production Examples](#11-production-examples)

---

## 1. OverlayFS

### How It Works

OverlayFS is a Linux kernel union filesystem that combines two directory trees -- a read-only **lower** layer and a writable **upper** layer -- into a single **merged** view. A fourth directory, **workdir**, is required for internal bookkeeping (must be on the same filesystem as upperdir).

```
         ┌─────────────┐
         │   merged/    │  ← Unified view (mount point)
         ├─────────────┤
         │   upper/     │  ← Writable layer (new/modified files)
         ├─────────────┤
         │   lower/     │  ← Read-only layer (base image)
         └─────────────┘
```

**Read path:** If a file exists in upper, it is served from there; otherwise it falls through to lower.

**Write path:** Writes always go to upper. If modifying a file that only exists in lower, the entire file is first **copied up** to upper, then modified there.

**Deletion:** Creates a "whiteout" file in upper to mask the lower file.

### Mount Command

```bash
# Basic mount
mount -t overlay overlay \
  -o lowerdir=/path/to/lower,upperdir=/path/to/upper,workdir=/path/to/work \
  /path/to/merged

# Multiple read-only lower layers (rightmost = bottom)
mount -t overlay overlay \
  -o lowerdir=/lower1:/lower2:/lower3,upperdir=/upper,workdir=/work \
  /merged

# Read-only overlay (no upper/work needed)
mount -t overlay overlay \
  -o lowerdir=/lower1:/lower2 \
  /merged
```

### Block Device Support

OverlayFS operates on **directories only**, not block devices. It works at the file level, not the block level. This means:
- It cannot be used directly as a Firecracker block device
- It must be used **inside** the guest VM (on top of mounted block devices) or on the **host** to prepare filesystem images

### Nesting Limits

- OverlayFS does **not nest** -- you cannot use an OverlayFS mount as the upper layer of another OverlayFS mount
- overlay2 (Docker's driver) supports up to **128 lower layers**
- In practice, Docker build fails around 122 layers with "max depth exceeded"
- The lower layer itself can be another OverlayFS mount (read-only)

### Performance Characteristics

- **Copy-up overhead:** Modifying any file from lower copies the **entire file** to upper, even if only 1 byte changed. This is costly for large files.
- **Metadata overhead:** Low -- whiteout files and opaque directories are lightweight
- **Read performance:** Near-native for files in upper; small lookup overhead for files in lower
- **Write performance:** First write to a lower file has copy-up penalty; subsequent writes are native speed

### Extracting the Diff (Upper Layer)

After work is done, the upper directory contains exactly the diff:
- New files created by the VM
- Modified copies of lower files
- Whiteout files for deletions

Simply reading the upper directory gives you the complete delta. Tools like `rsync` or `tar` on the upper directory capture all changes.

### Use with Firecracker

OverlayFS cannot be used as a Firecracker block device directly. There are two approaches:

**Approach A: Inside the guest (E2B approach)**
1. Pass a read-only rootfs as the first block device (`/dev/vda`)
2. Pass a sparse writable ext4 image as the second block device (`/dev/vdb`)
3. Use a custom init script that mounts overlayfs inside the guest:
   ```bash
   mount /dev/vda /mnt/lower -o ro
   mount /dev/vdb /mnt/rw
   mkdir -p /mnt/rw/upper /mnt/rw/work /mnt/merged
   mount -t overlay overlay \
     -o lowerdir=/mnt/lower,upperdir=/mnt/rw/upper,workdir=/mnt/rw/work \
     /mnt/merged
   pivot_root /mnt/merged /mnt/merged/mnt
   ```

**Approach B: On the host**
1. Create OverlayFS on host with shared lower and per-VM upper
2. Create a loopback block device from a file inside the merged view
3. Pass that as a Firecracker block device
4. **Caveat:** This is complex and fragile

### Extracting Changes After VM Execution

With Approach A, after the VM shuts down:
1. Mount the writable ext4 image (`/dev/vdb` equivalent) on the host
2. The `upper/` directory within it contains all changes
3. `tar -cf changes.tar -C /mnt/upper .` captures the diff

---

## 2. Device Mapper Thin Provisioning (dm-thin)

### How It Works

Device mapper thin provisioning operates at the **block level** in the Linux kernel. It manages a shared **thin pool** (consisting of a data device and a metadata device) from which **thin volumes** are allocated. Blocks are allocated on-demand (thin provisioning) and can be shared between volumes via snapshots (copy-on-write).

```
        ┌──────────────────────────────────┐
        │         Thin Pool                │
        │  ┌──────────┐  ┌──────────────┐ │
        │  │ Metadata  │  │  Data Device │ │
        │  │  Device   │  │ (shared pool)│ │
        │  └──────────┘  └──────────────┘ │
        ├──────────────────────────────────┤
        │  thin_vol_1  thin_vol_2  ...     │  ← Thin volumes
        │  (snapshot)  (snapshot)           │    share data blocks
        └──────────────────────────────────┘
```

### Key Advantages

- **Block-level CoW:** Only modified blocks are duplicated, not entire files
- **Recursive snapshots:** Snapshots of snapshots with O(1) performance (flat metadata structure, unlike older dm-snapshot which was O(depth))
- **Space efficiency:** Blocks allocated on demand; shared blocks counted once
- **Native Firecracker integration:** Produces `/dev/mapper/*` block devices that Firecracker can use directly

### Setup Commands (Step-by-Step)

```bash
# 1. Create backing files for the thin pool
dd if=/dev/zero of=/opt/pool-data bs=1 count=0 seek=100G    # sparse 100G data
dd if=/dev/zero of=/opt/pool-meta bs=1 count=0 seek=2G       # sparse 2G metadata

# 2. Create loop devices
DATA_LOOP=$(losetup --find --show /opt/pool-data)
META_LOOP=$(losetup --find --show /opt/pool-meta)

# 3. Calculate sizes in 512-byte sectors
DATA_SZ=$(blockdev --getsz $DATA_LOOP)
META_SZ=$(blockdev --getsz $META_LOOP)

# 4. Create thin pool
#    Format: <metadata_dev> <data_dev> <data_block_size> <low_water_mark>
#    data_block_size=128 sectors = 64KB (good for snapshot-heavy workloads)
dmsetup create mypool --table \
  "0 $DATA_SZ thin-pool $META_LOOP $DATA_LOOP 128 32768"

# 5. Create a thin volume (ID=0) for the base image
dmsetup message /dev/mapper/mypool 0 "create_thin 0"

# 6. Activate it (10GB virtual size = 20971520 sectors)
dmsetup create base-vol --table \
  "0 20971520 thin /dev/mapper/mypool 0"

# 7. Format and populate with base image
mkfs.ext4 /dev/mapper/base-vol
mount /dev/mapper/base-vol /mnt/base
# ... copy git repo and dependencies ...
umount /mnt/base

# 8. Create a snapshot for each VM
#    Must suspend the origin first to ensure consistency
dmsetup suspend /dev/mapper/base-vol
dmsetup message /dev/mapper/mypool 0 "create_snap 1 0"   # snap ID=1 from origin ID=0
dmsetup message /dev/mapper/mypool 0 "create_snap 2 0"   # snap ID=2
dmsetup message /dev/mapper/mypool 0 "create_snap 3 0"   # snap ID=3
dmsetup resume /dev/mapper/base-vol

# 9. Activate snapshots as block devices
dmsetup create vm-1-vol --table "0 20971520 thin /dev/mapper/mypool 1"
dmsetup create vm-2-vol --table "0 20971520 thin /dev/mapper/mypool 2"
dmsetup create vm-3-vol --table "0 20971520 thin /dev/mapper/mypool 3"

# 10. Pass /dev/mapper/vm-1-vol to Firecracker as a block device
```

### Alternative: dm-snapshot (Simpler, Non-Pooled)

This is the approach used by Parandrus and Julia Evans for Firecracker:

```bash
# Create read-only base loop device
BASE_LOOP=$(losetup --find --show --read-only /opt/rootfs/base.ext4)
BASE_SZ=$(blockdev --getsz $BASE_LOOP)

# Create sparse overlay file per VM
OVERLAY_FILE=/opt/overlays/vm-${ID}.cow
truncate --size=5G $OVERLAY_FILE
OVERLAY_LOOP=$(losetup --find --show $OVERLAY_FILE)
OVERLAY_SZ=$((5 * 1024 * 1024 * 1024 / 512))  # sectors

# Layer 1: Base image + zero padding
dmsetup create base-$ID <<EOF
0 $BASE_SZ linear $BASE_LOOP 0
$BASE_SZ $((OVERLAY_SZ - BASE_SZ)) zero
EOF

# Layer 2: Copy-on-write snapshot
dmsetup create overlay-$ID <<EOF
0 $OVERLAY_SZ snapshot /dev/mapper/base-$ID $OVERLAY_LOOP P 8
EOF

# /dev/mapper/overlay-$ID is the block device for Firecracker
```

### Performance Characteristics

- **Snapshot creation:** Sub-second (metadata-only operation)
- **Read performance:** Unmodified blocks served from shared base (potential cache hits)
- **Write performance:** First write to a shared block triggers block-level copy (typically 64KB-128KB), then native speed
- **Scaling:** dm-thin handles many snapshots well due to flat metadata; dm-snapshot scales less well (write to origin touches all snapshot COW devices)
- **Fragmentation:** Can become an issue with many random writes over time

### Extracting Changes

**dm-thin:** Use `thin_delta` tool to identify changed blocks between a snapshot and its origin:
```bash
thin_dump /dev/mapper/mypool_tmeta | thin_delta --snap1 0 --snap2 1
```

**dm-snapshot:** The COW overlay file contains all changes. Mount it alongside the base to reconstruct the modified filesystem, or use block-level comparison tools.

### Limitations

- Written blocks remain allocated even after deletion (no TRIM/discard passthrough in all configurations)
- dm-snapshot: write performance degrades with many concurrent snapshots of the same origin (O(N) per origin write)
- dm-thin: requires careful metadata device sizing for many snapshots with lots of change

---

## 3. Btrfs Snapshots and Subvolumes

### How It Works

Btrfs is a copy-on-write filesystem with native support for subvolumes and instant snapshots. A subvolume is an independently mountable filesystem tree within a Btrfs volume. A snapshot creates a new subvolume that shares all data blocks with its source.

```bash
# Create a subvolume
btrfs subvolume create /mnt/btrfs/base-repo

# Populate it
cp -a /path/to/repo/* /mnt/btrfs/base-repo/

# Create a writable snapshot (instant, metadata-only)
btrfs subvolume snapshot /mnt/btrfs/base-repo /mnt/btrfs/vm-1
btrfs subvolume snapshot /mnt/btrfs/base-repo /mnt/btrfs/vm-2
btrfs subvolume snapshot /mnt/btrfs/base-repo /mnt/btrfs/vm-3
```

### CoW Semantics

- Snapshot creation is **O(1)** -- it only creates a new tree root in metadata
- Data blocks are shared until modified; modified blocks are written to new locations
- There is a caveat: snapshot creation may need to flush dirty data first, which can take time under heavy write load

### Performance Under Concurrent Writers

- Each snapshot writer operates independently; no cross-snapshot locking for data
- Metadata updates (especially reference counting) can become a bottleneck with many concurrent writers
- Compression (lzo, zstd) adds CPU overhead; disable for write-heavy workloads

### Send/Receive for Extracting Diffs

```bash
# Create read-only snapshot before and after work
btrfs subvolume snapshot -r /mnt/btrfs/base-repo /mnt/btrfs/base-snap
# ... VM does its work on /mnt/btrfs/vm-1 ...
btrfs subvolume snapshot -r /mnt/btrfs/vm-1 /mnt/btrfs/vm-1-after

# Extract the diff as a send stream
btrfs send -p /mnt/btrfs/base-snap /mnt/btrfs/vm-1-after > diff.btrfs

# Or for metadata-only diff (faster, no file data)
btrfs send --no-data -p /mnt/btrfs/base-snap /mnt/btrfs/vm-1-after > diff-meta.btrfs

# Apply on another system
btrfs receive /mnt/target < diff.btrfs
```

### Subvolume as Firecracker Rootfs

Btrfs subvolumes are directories, not block devices. To use with Firecracker:

1. Create a subvolume snapshot per VM
2. Create a loopback image from the subvolume contents:
   ```bash
   # Option A: Create ext4 image from snapshot
   mkfs.ext4 /opt/vm-images/vm-1.ext4
   mount /opt/vm-images/vm-1.ext4 /mnt/img
   cp -a /mnt/btrfs/vm-1/* /mnt/img/
   umount /mnt/img
   # Pass /opt/vm-images/vm-1.ext4 to Firecracker
   ```
3. **Problem:** This defeats the purpose of CoW since you're copying data into a new image.
4. **Alternative:** Use Btrfs on the host purely for its `cp --reflink=always` capability:
   ```bash
   # Instant CoW copy of an ext4 image file
   cp --reflink=always base.ext4 vm-1.ext4
   # Pass vm-1.ext4 to Firecracker -- writes are CoW at block level
   ```

### `cp --reflink=always` Approach

When the host filesystem is Btrfs (or XFS with reflinks), `cp --reflink=always` creates an instant CoW copy of a file. This is particularly useful for Firecracker disk images:

```bash
# Prepare base image once
dd if=/dev/zero of=base.ext4 bs=1M count=0 seek=10240  # 10GB sparse
mkfs.ext4 base.ext4
mount base.ext4 /mnt/base
# ... install repo and dependencies ...
umount /mnt/base

# Clone for each VM (instant on Btrfs/XFS)
cp --reflink=always base.ext4 vm-1.ext4
cp --reflink=always base.ext4 vm-2.ext4

# Each VM gets its own block device that shares unchanged blocks
```

### Known Issues

- Btrfs metadata overhead grows with number of snapshots
- Random write performance can degrade due to CoW fragmentation
- `btrfs send` can be slow for large diffs (reports of being "extremely slow" in some configurations)
- RAID5/6 on Btrfs is still considered unstable

---

## 4. ZFS Clones and Snapshots

### How It Works

ZFS provides datasets, snapshots, and clones with full CoW semantics:

```bash
# Create a dataset
zfs create pool/base-repo

# Populate it
cp -a /path/to/repo/* /pool/base-repo/

# Create a snapshot (instant, read-only)
zfs snapshot pool/base-repo@pristine

# Create writable clones from the snapshot
zfs clone pool/base-repo@pristine pool/vm-1
zfs clone pool/base-repo@pristine pool/vm-2
zfs clone pool/base-repo@pristine pool/vm-3
```

### ARC Cache Sharing

A major advantage of ZFS clones:
- All clones share the same blocks in the **Adaptive Replacement Cache (ARC)**
- Parent blocks are very likely already cached, making clones "extra-snappy" performance-wise
- Multiple clones reading the same data benefit from a single cached copy
- ARC parameters tunable via `/sys/module/zfs/parameters/` on Linux

### Performance Characteristics

- **Snapshot creation:** Instant (metadata-only)
- **Clone creation:** Instant (creates a new dataset pointing at snapshot's blocks)
- **Read performance:** Excellent due to ARC cache sharing across clones
- **Write performance:** CoW at block level (typically 128KB record size)
- **ARC memory:** ZFS 2.3+ improved fast cache eviction to prevent OOM issues

### ZFS zvol for Firecracker Block Devices

ZFS volumes (zvols) present as block devices and can be used with Firecracker:

```bash
# Create a zvol
zfs create -V 10G pool/base-zvol
mkfs.ext4 /dev/zvol/pool/base-zvol
# ... populate ...

# Snapshot
zfs snapshot pool/base-zvol@pristine

# Clone (produces new block device)
zfs clone pool/base-zvol@pristine pool/vm-1-zvol
# /dev/zvol/pool/vm-1-zvol is a block device for Firecracker
```

### Extracting Diffs

```bash
# Send incremental stream (diff between snapshots)
zfs snapshot pool/vm-1@after-work
zfs send -i pool/base-repo@pristine pool/vm-1@after-work > diff.zfs

# Or use zfs diff for file-level changes
zfs diff pool/base-repo@pristine pool/vm-1
```

### Linux Licensing Considerations

- ZFS is licensed under **CDDL**, which is incompatible with Linux's **GPLv2**
- Cannot be merged into the mainline Linux kernel
- Distributed as a **DKMS kernel module** (built out-of-tree)
- Ubuntu includes ZFS support officially (Canonical's legal position: binary module is acceptable)
- No legal challenge has been brought as of 2024, but the legal situation remains ambiguous
- OpenZFS 2.0+ merged Linux and FreeBSD codebases, providing good feature parity
- Dual-licensing proposal (CDDL + GPLv2) exists but CDDL's terms prevent license changes to existing code

### Practical Concerns

- Requires DKMS module installation and maintenance
- Kernel upgrades may break ZFS module compatibility
- ARC memory consumption must be tuned carefully (`zfs_arc_max`)
- Higher operational complexity than ext4/Btrfs

---

## 5. QCOW2 Backing Files

### How QCOW2 CoW Works

QCOW2 (QEMU Copy-on-Write v2) is an image format with native CoW support:

```
  ┌────────────┐
  │  Overlay    │  ← Writable; stores only changed clusters
  │  (vm-1.qcow2) │
  ├────────────┤
  │  Backing    │  ← Read-only shared base
  │  (base.qcow2) │
  └────────────┘
```

- Files are divided into **clusters** (default 64KB)
- If a cluster is not present in the overlay, it is read from the backing file
- Writes allocate new clusters in the overlay
- Supports backing chains (overlay -> backing -> backing -> ...)

```bash
# Create base image
qemu-img create -f qcow2 base.qcow2 10G

# Create overlay with backing file
qemu-img create -f qcow2 -b base.qcow2 -F qcow2 vm-1.qcow2
qemu-img create -f qcow2 -b base.qcow2 -F qcow2 vm-2.qcow2

# Each overlay starts at ~196KB (metadata only)
```

### Firecracker Does NOT Support QCOW2

**Firecracker only supports raw disk images.** It does not have QCOW2 parsing code. This is by design -- Firecracker's minimal device model excludes complex storage formats for security and simplicity.

The only way to use Firecracker with QCOW2-like CoW is:
1. Convert QCOW2 to raw: `qemu-img convert base.qcow2 -O raw base.raw` (loses CoW)
2. Use device mapper for block-level CoW (recommended alternative)
3. Use NBD with qemu-nbd to serve a QCOW2 image as a block device (adds latency)

### Alternatives for Block-Level CoW with Firecracker

Since Firecracker needs raw block devices, the viable alternatives are:
- **dm-thin provisioning** (best fit -- see Section 2)
- **dm-snapshot** (simpler but less scalable)
- **`cp --reflink=always`** on Btrfs/XFS host filesystem
- **ZFS zvols** (if ZFS is acceptable)
- **LVM thin snapshots** (LVM wrapper around dm-thin)

---

## 6. virtiofs

### How It Works

virtiofs provides host-to-guest filesystem sharing through:
1. A **virtiofsd** daemon on the host (FUSE-based)
2. **VIRTIO transport** carrying FUSE messages to the guest
3. Guest mounts the shared filesystem via the `virtiofs` kernel driver

### DAX (Direct Access) Mode

DAX maps host page cache directly into guest memory:
- **Without DAX:** Sequential reads ~35 MiB/s
- **With DAX:** Sequential reads ~643 MiB/s (18x improvement)
- Bypasses guest page cache, reducing memory footprint
- File contents accessed via shared memory window without hypervisor communication

### Cache Modes

- `cache=none`: No caching, all operations go to host
- `cache=auto`: Limited caching with invalidation
- `cache=always`: Aggressive caching (suitable for read-only or single-writer)

### Firecracker Does NOT Support virtiofs

**virtiofs is NOT available in Firecracker.** This has been discussed extensively:
- GitHub Issue #1180: "Host Filesystem Sharing" -- tracked since early Firecracker development
- A WIP Pull Request (#1351) for VirtioFS was created but not merged
- The Firecracker team found the attack surface implications too large
- Even read-only mode has significant code complexity concerns
- Firecracker's minimal device model intentionally excludes virtiofs

### Workarounds

- Use MMDS (MicroVM Metadata Service) for small read-only data
- Use vsock to stream files from host to guest
- Use additional block devices for sharing data

### If virtiofs Were Available (Reference)

For comparison, in QEMU/KVM with virtiofs:
```bash
# Host: Start virtiofsd
virtiofsd --socket-path=/tmp/vhostqemu \
  --shared-dir=/path/to/repo \
  --cache=always

# QEMU: Add virtio-fs device
qemu-system-x86_64 ... \
  -chardev socket,id=char0,path=/tmp/vhostqemu \
  -device vhost-user-fs-pci,chardev=char0,tag=myfs \
  -object memory-backend-memfd,id=mem,size=4G,share=on

# Guest: Mount
mount -t virtiofs myfs /mnt/repo
```

---

## 7. 9pfs (Plan 9 Filesystem)

### How It Works

9P is a network protocol from Plan 9 used for host-guest filesystem sharing:
- QEMU implements VirtFS (9P over virtio) for this purpose
- The host exposes a directory; the guest mounts it via the 9p kernel module

```bash
# QEMU example
qemu-system-x86_64 ... \
  -fsdev local,id=fsdev0,path=/path/to/share,security_model=mapped \
  -device virtio-9p-pci,fsdev=fsdev0,mount_tag=hostshare

# Guest mount
mount -t 9p -o trans=virtio hostshare /mnt/share
```

### Performance Limitations

9P is **known to be slow**:
- Prioritizes simplicity and compatibility over performance
- High per-operation overhead (each file operation is a network round-trip)
- No DAX or shared memory optimizations
- Not suitable for high-throughput or low-latency workloads

### Firecracker Does NOT Support 9P

**Firecracker does not support 9p/VirtFS.** The project explicitly rejected 9P-based implementations due to:
- Security concerns (large attack surface from filesystem protocol parsing)
- Performance limitations
- Preference for block device isolation

---

## 8. Squashfs / EROFS as Read-Only Base

### Squashfs

A compressed read-only filesystem commonly used for base layers:

```bash
# Create squashfs image from directory
mksquashfs /path/to/repo repo-base.squashfs \
  -comp zstd -Xcompression-level 3

# Mount (read-only)
mount -t squashfs repo-base.squashfs /mnt/lower -o ro
```

**Characteristics:**
- **Compression:** Supports gzip, lzo, xz, zstd, lz4
- **Block size:** Fixed input block size (default 128KB); variable-sized compressed output
- **Read performance:** Good sequential; compressed blocks may not align with disk I/O boundaries
- **Compression ratios:** Excellent (typically 2-3x for source code repositories)
- **Random read overhead:** Needs to decompress entire block even for small reads

### EROFS (Enhanced Read-Only File System)

A newer read-only filesystem optimized for read performance:

```bash
# Create EROFS image
mkfs.erofs -zlz4hc repo-base.erofs /path/to/repo

# Mount
mount -t erofs repo-base.erofs /mnt/lower -o ro
```

**Characteristics:**
- **Fixed output compression:** Compressed chunks are fixed size (default 4KB), aligned with disk I/O
- **Random read performance:** Significantly better than Squashfs (less I/O amplification)
- **Sequential read:** Slightly worse compression ratio than Squashfs due to smaller blocks
- **Interleaved storage:** Data and metadata stored together for better locality
- **Kernel support:** Mainline since Linux 5.4; actively developed

### EROFS vs Squashfs Performance Summary

| Metric | Squashfs | EROFS |
|--------|----------|-------|
| Sequential read throughput | Better (higher compression) | Slightly lower |
| Random read latency | Higher (128KB blocks) | Lower (4KB blocks) |
| I/O amplification | Higher | Lower |
| Compression ratio | Better | Good |
| Metadata overhead | Higher | Lower |

### Combining with Writable Overlay

The key pattern for our use case:

```bash
# 1. Create compressed read-only base
mksquashfs /path/to/repo base.squashfs -comp zstd

# 2. For each VM, combine with writable overlay
mount -t squashfs base.squashfs /mnt/lower -o ro
mkdir -p /mnt/upper /mnt/work /mnt/merged
mount -t overlay overlay \
  -o lowerdir=/mnt/lower,upperdir=/mnt/upper,workdir=/mnt/work \
  /mnt/merged
```

For Firecracker, this combination must happen **inside the guest VM**:
1. Pass squashfs/erofs image as read-only block device
2. Pass sparse ext4 image as writable block device
3. Guest init script mounts overlay combining both

### Space Savings Example

For a typical Node.js project with node_modules:
- Uncompressed: ~500MB
- Squashfs (zstd): ~150MB (3.3x compression)
- EROFS (lz4hc): ~180MB (2.8x compression)
- Shared across 50 VMs: 150MB vs 25GB (uncompressed per-VM copies)

---

## 9. NBD (Network Block Device)

### How It Works

NBD allows a remote server to expose a block device over the network:

```bash
# Server: Serve a read-only base image
qemu-nbd --read-only --shared=50 --port=10809 base.raw

# Client: Connect
nbd-client server-host 10809 /dev/nbd0

# Or using nbdkit with CoW filter
nbdkit --filter=cow file base.raw
```

### CoW Filter (nbdkit-cow-filter)

nbdkit provides a copy-on-write filter that adds a writable overlay on top of a read-only plugin:

```bash
# Serve read-only base with per-connection CoW overlay
nbdkit --filter=cow file base.raw

# All connections share the same CoW overlay by default
# (Changes from one client visible to others)
```

**Key characteristics:**
- Overlay stored in a temporary file in `/var/tmp`
- Default block size: 64KB
- All connections see the same view (shared overlay, not per-client)
- Overlay discarded when nbdkit exits

### Performance

- **Stream read (10 Gbps network):** ~210 MB/s async I/O, ~160 MB/s block I/O
- **Stream write:** ~70 MB/s async I/O, ~60 MB/s block I/O
- **Latency:** Higher than local block devices due to network round-trips
- **Supports compression:** zlib, fastlz, skipz for bandwidth reduction

### Use with Firecracker

Firecracker does not natively speak NBD protocol. To use NBD:
1. Connect NBD on the **host** to create `/dev/nbd0`
2. Use that as a Firecracker block device
3. **Problem:** All VMs would share the same block device (no isolation)
4. **Solution:** Layer dm-snapshot on top of NBD for per-VM CoW:
   ```bash
   # Connect read-only NBD
   nbd-client server 10809 /dev/nbd0
   # Create per-VM CoW overlay using dm-snapshot
   dmsetup create vm-1 --table \
     "0 $SIZE snapshot /dev/nbd0 /dev/loop1 P 8"
   ```

### Limitations for Our Use Case

- Adds network latency to every I/O operation
- Complexity of running NBD server alongside Firecracker
- Not significantly better than local device mapper for single-host scenarios
- More relevant for multi-host setups where base image is on shared storage

---

## 10. Performance Comparison and Recommendations

### Comparison Matrix

| Strategy | Clone Time | Disk Overhead | Scales to 50+ VMs | Diff Extraction | Firecracker Compatible |
|----------|-----------|---------------|-------------------|-----------------|----------------------|
| **dm-thin** | <100ms | Very low (block-level) | Excellent | `thin_delta` tool | Direct (block device) |
| **dm-snapshot** | <100ms | Low (block-level) | Good (write amplification) | COW file analysis | Direct (block device) |
| **OverlayFS (in-guest)** | <1s (sparse file creation) | Low (file-level CoW) | Good | Read upper dir | Via guest init |
| **Btrfs reflink** | <100ms (`cp --reflink`) | Low (block-level) | Good | `btrfs send -p` | Via raw image copy |
| **ZFS zvol clone** | <100ms | Very low (block-level) | Excellent (ARC sharing) | `zfs send -i` | Direct (zvol) |
| **Squashfs + overlay** | <1s | Excellent (compressed base) | Good | Read upper dir | Via guest init |
| **NBD + CoW** | ~1s (connection setup) | Low | Moderate | COW analysis | Indirect (via host NBD) |
| **QCOW2** | <100ms | Very low | Excellent | `qemu-img compare` | **NOT SUPPORTED** |
| **virtiofs** | N/A | N/A | N/A | N/A | **NOT SUPPORTED** |
| **9pfs** | N/A | N/A | N/A | N/A | **NOT SUPPORTED** |

### Fastest Clone Time

1. **dm-thin snapshot creation:** Sub-millisecond (metadata-only `dmsetup message`)
2. **ZFS clone:** Sub-millisecond (metadata-only)
3. **Btrfs snapshot / reflink copy:** Sub-millisecond to ~100ms
4. **dm-snapshot:** ~100ms (create loop device + dmsetup)

### Lowest Memory Overhead

1. **dm-thin / dm-snapshot:** Zero additional memory (kernel page cache shared naturally)
2. **ZFS zvol:** ARC explicitly shares cache across clones (best cache efficiency but ZFS uses more baseline memory)
3. **Squashfs base:** Compressed base reduces page cache pressure
4. **OverlayFS:** Normal page cache behavior

### Best Scaling to 50+ VMs

1. **dm-thin:** Designed for many thin volumes; flat metadata structure; no O(N) write amplification
2. **ZFS zvol clones:** ARC sharing makes reads extremely efficient; well-tested at scale
3. **Btrfs reflink:** Each image is independent; no shared metadata bottleneck
4. **dm-snapshot:** Origin writes touch all snapshot COW devices (O(N)); acceptable if origin is read-only
5. **OverlayFS (in-guest):** Each VM has independent overlay; host just manages sparse files

### Easiest Diff Extraction

1. **OverlayFS:** Just read the upper directory -- trivially simple
2. **Btrfs send/receive:** Native diff streaming between snapshots
3. **ZFS send -i:** Incremental stream between snapshots
4. **dm-thin thin_delta:** Block-level diff identification
5. **dm-snapshot:** Raw COW file analysis (requires custom tooling)

### Recommended Strategy for Our Use Case

**Primary Recommendation: dm-thin provisioning**

- Direct Firecracker block device compatibility (no guest init tricks needed)
- Sub-millisecond snapshot creation
- Block-level CoW (efficient for large files like compiled binaries)
- Proven in production (firecracker-containerd uses this exact approach)
- Good diff extraction via thin_delta

**Runner-Up: OverlayFS in-guest with Squashfs base**

- Simplest conceptual model
- Excellent compression for source code repositories
- Trivial diff extraction (read upper directory)
- Requires custom guest init script
- E2B uses this approach in production at scale

**Alternative: Btrfs/XFS `cp --reflink=always`**

- Simplest host-side setup (just copy a file)
- No device mapper complexity
- Requires Btrfs or XFS on host
- Good for moderate scale (50 VMs manageable)
- CodeSandbox uses this approach

---

## 11. Production Examples

### E2B (OverlayFS in-guest)

[E2B Blog: Scaling Firecracker Using OverlayFS](https://e2b.dev/blog/scaling-firecracker-using-overlayfs-to-save-disk-space)

- Read-only Squashfs rootfs as first block device (`/dev/vda`)
- Sparse ext4 overlay as second block device (`/dev/vdb`)
- Custom `overlay-init` script runs before systemd
- Kernel args: `init=/sbin/overlay-init overlay_root=/vdb`
- Supports `overlay_root=ram` for tmpfs-based ephemeral instances

### CodeSandbox (Memory + Disk CoW)

[CodeSandbox Blog: How We Clone a Running VM in 2 Seconds](https://codesandbox.io/blog/how-we-clone-a-running-vm-in-2-seconds)

- **Disk:** CoW snapshots using device mapper or `cp --reflink=always`
- **Memory:** userfaultfd-based memory sharing with write protection
- **Performance:** Pause (16ms) + Snapshot (100ms) + Copy (800ms) + Start (400ms) = ~1.3s total
- **Advanced:** userfaultfd with page source tables for O(1) ancestry resolution
- **Result:** Reliable 1.5s fork regardless of VM memory size

### Parandrus (dm-snapshot)

[Parandrus: Space Efficient Filesystems for Firecracker](https://parandrus.dev/devicemapper/)

- Two-layer device mapper: linear+zero base, snapshot overlay
- Shared read-only base across all VMs
- Sparse overlay files (allocate on write only)
- Caveat: deleted blocks remain allocated in COW layer

### firecracker-containerd (dm-thin)

[firecracker-containerd Snapshotter Docs](https://github.com/firecracker-microvm/firecracker-containerd/blob/main/docs/snapshotter.md)

- Uses containerd's devmapper snapshotter
- Thin pool with configurable data block size
- Container rootfs as thin snapshot, hot-plugged as virtio-block
- Production-grade integration with containerd image management

### AWS Lambda (Firecracker Snapshots)

[Firecracker Snapshot Documentation](https://github.com/firecracker-microvm/firecracker/blob/main/docs/snapshotting/snapshot-support.md)

- Full microVM snapshots (memory + device state)
- Restore in **~4-10ms** using MAP_PRIVATE with on-demand page loading
- Used for Lambda SnapStart to reduce cold start times
- Memory file remains needed for entire VM lifetime (MAP_PRIVATE CoW)

---

## Summary Decision Matrix

For our specific use case (git repo with dependencies, 50+ VMs, need to extract diffs):

| Criterion | Best Option |
|-----------|------------|
| Simplicity | OverlayFS in-guest + Squashfs base |
| Raw performance | dm-thin provisioning |
| Diff extraction | OverlayFS (read upper dir) |
| Disk space efficiency | Squashfs base + OverlayFS |
| Firecracker integration | dm-thin (native block device) |
| Proven at scale | dm-thin (firecracker-containerd) |
| Operational simplicity | `cp --reflink=always` on Btrfs host |

**Recommended architecture: Hybrid approach**
1. **Base image:** Squashfs-compressed read-only root with repo and dependencies
2. **Block-level sharing:** dm-thin pool for efficient snapshot management
3. **In-guest overlay:** OverlayFS combining read-only base with writable thin volume
4. **Diff extraction:** Read OverlayFS upper directory after VM shutdown

This combines the compression benefits of Squashfs, the block-level efficiency of dm-thin, and the simple diff extraction of OverlayFS.
