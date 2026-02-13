# Firecracker MicroVM Research Report

**Date:** 2026-02-11
**Purpose:** Deep technical research for use in a local AI agent sandbox system where a developer runs a single command in a folder containing a git checkout, a Firecracker VM spawns with a copy of that repo inside it, and the user gets a bash shell. Multiple VMs can run in parallel, each starting from the current state of the repo. When done, changes are committed back.

---

## Table of Contents

1. [Firecracker Fundamentals](#1-firecracker-fundamentals)
2. [Root Filesystem and Kernel Setup](#2-root-filesystem-and-kernel-setup)
3. [API for VM Lifecycle](#3-api-for-vm-lifecycle)
4. [Networking](#4-networking)
5. [Block Device Attachment](#5-block-device-attachment)
6. [Snapshotting](#6-snapshotting)
7. [Boot Time](#7-boot-time)
8. [Security Model](#8-security-model)
9. [File Sharing with Host](#9-file-sharing-with-host)
10. [Existing Tooling](#10-existing-tooling)
11. [Production Users](#11-production-users)
12. [Comparison](#12-comparison-with-alternatives)
13. [Resource Requirements](#13-resource-requirements)
14. [Limitations](#14-limitations)
15. [Relevance to Our Use Case](#15-relevance-to-our-use-case)

---

## 1. Firecracker Fundamentals

### What Firecracker Is

Firecracker is a **Virtual Machine Monitor (VMM)** written in Rust (~50,000 lines of code) that uses the Linux **Kernel-based Virtual Machine (KVM)** to create and manage lightweight virtual machines called **microVMs**. It was open-sourced by AWS in 2018 and is the technology powering AWS Lambda and AWS Fargate.

**Latest version:** v1.14 (released 2025-12-17). New releases typically every 2-3 months.

### How It Differs from QEMU

| Aspect | Firecracker | QEMU |
|--------|-------------|------|
| **Codebase** | ~50,000 lines of Rust | ~2,000,000 lines of C |
| **Language** | Rust (memory-safe) | C |
| **Device model** | Minimal: 5 virtio devices only | Comprehensive: hundreds of emulated devices |
| **Boot time** | <=125ms to guest /sbin/init | ~18% slower on average |
| **Memory overhead** | <=5 MiB per VMM process | Significantly higher |
| **System calls at boot** | ~5,000 | ~730,000 |
| **GPU support** | No | Yes (passthrough) |
| **Architecture emulation** | No (KVM only, same arch) | Yes (cross-arch emulation) |
| **PCI support** | Optional (added v1.14, --enable-pci flag) | Full PCI/PCIe |
| **vhost I/O** | No (does not use vhost in host kernel) | Yes |

### Architecture Overview

```
Host OS (Linux with KVM)
  |
  +-- Firecracker process (per VM)
  |     |-- REST API (Unix socket)
  |     |-- Minimal device model
  |     |   |-- virtio-block (storage)
  |     |   |-- virtio-net (networking)
  |     |   |-- virtio-vsock (host<->guest comms)
  |     |   |-- virtio-balloon (memory management)
  |     |   |-- virtio-rng (entropy)
  |     |   |-- virtio-pmem (added v1.14)
  |     |   |-- virtio-mem (memory hotplug, added v1.14)
  |     |   +-- serial console (i8250)
  |     |-- KVM vCPU threads
  |     +-- API server thread
  |
  +-- Jailer process (optional, production security)
        |-- cgroup isolation
        |-- namespace isolation
        |-- seccomp filters
        +-- chroot jail
```

Firecracker consists of a **single process** per VM. It exposes a REST API on a Unix domain socket for configuration and control. All device emulation happens in user space within the Firecracker process -- it does **not** use the kernel vhost implementation for I/O, meaning all I/O goes through the VMM process (which means VMEXITS for I/O, a tradeoff for simplicity and security).

---

## 2. Root Filesystem and Kernel Setup

### Kernel Requirements

- **Format:** Uncompressed ELF kernel image (`vmlinux`) on x86_64; PE format on aarch64
- **Supported kernel versions:** 5.10, 5.10-no-acpi, and 6.1
- **Source:** Firecracker provides microVM-tuned kernel configs in `resources/guest_configs/` (e.g., `microvm-kernel-x86_64-5.10.config`, `microvm-kernel-x86_64-6.1.config`)
- **No initramfs needed:** The kernel must be able to mount the root filesystem directly
- **Filesystem support compiled in:** Whichever filesystem the rootfs uses must be compiled into the kernel (not as a module)

### Root Filesystem Requirements

- **Format:** ext4 file system image (most common); squashfs also possible for read-only base
- **Minimum contents:** An init system (e.g., `/sbin/init`, systemd, or a custom init script)
- **Creation example:**

```bash
# Create a 1GB ext4 image
dd if=/dev/zero of=rootfs.ext4 bs=1M count=1024
mkfs.ext4 rootfs.ext4

# Mount and populate
mkdir /tmp/rootfs
mount rootfs.ext4 /tmp/rootfs

# Install a minimal distro (e.g., via debootstrap for Debian/Ubuntu)
debootstrap --include=openssh-server,git focal /tmp/rootfs http://archive.ubuntu.com/ubuntu

# Or use Alpine for minimal size
# alpine-make-rootfs rootfs.ext4
```

### Boot Arguments

Typical boot arguments passed to the kernel:

```
console=ttyS0 reboot=k panic=1 pci=off root=/dev/vda rw
```

- `console=ttyS0` - serial console output
- `reboot=k` - use keyboard reset
- `panic=1` - reboot after 1 second on panic
- `pci=off` - disable PCI (unless using --enable-pci)
- `root=/dev/vda` - root device (first virtio-block drive)
- `rw` - mount root read-write

### Pre-built Resources

Firecracker provides pre-built kernel images and rootfs images for testing at:
- Kernel: `s3://spec.ccfc.min/` (various versions)
- Test rootfs images available in the CI pipeline

---

## 3. API for VM Lifecycle

### API Architecture

Firecracker exposes a **REST API over a Unix domain socket**. The API is specified in **OpenAPI format**. All VM configuration and control is done through this API.

### Starting Firecracker

```bash
# Start Firecracker process (it creates the API socket)
firecracker --api-sock /tmp/firecracker.socket

# Or with a config file (all-in-one):
firecracker --api-sock /tmp/firecracker.socket --config-file vm_config.json
```

### Configuration via API (curl examples)

**Set kernel (boot source):**
```bash
curl --unix-socket /tmp/firecracker.socket -i \
  -X PUT 'http://localhost/boot-source' \
  -H 'Accept: application/json' \
  -H 'Content-Type: application/json' \
  -d '{
    "kernel_image_path": "./vmlinux-5.10",
    "boot_args": "console=ttyS0 reboot=k panic=1 pci=off root=/dev/vda rw"
  }'
```

**Attach root drive:**
```bash
curl --unix-socket /tmp/firecracker.socket -i \
  -X PUT 'http://localhost/drives/rootfs' \
  -H 'Accept: application/json' \
  -H 'Content-Type: application/json' \
  -d '{
    "drive_id": "rootfs",
    "path_on_host": "./rootfs.ext4",
    "is_root_device": true,
    "is_read_only": false
  }'
```

**Configure machine (vCPUs and memory):**
```bash
curl --unix-socket /tmp/firecracker.socket -i \
  -X PUT 'http://localhost/machine-config' \
  -H 'Accept: application/json' \
  -H 'Content-Type: application/json' \
  -d '{
    "vcpu_count": 2,
    "mem_size_mib": 1024
  }'
```

**Add network interface:**
```bash
curl --unix-socket /tmp/firecracker.socket -i \
  -X PUT 'http://localhost/network-interfaces/eth0' \
  -H 'Accept: application/json' \
  -H 'Content-Type: application/json' \
  -d '{
    "iface_id": "eth0",
    "guest_mac": "06:00:AC:10:00:02",
    "host_dev_name": "tap0"
  }'
```

**Start the VM:**
```bash
curl --unix-socket /tmp/firecracker.socket -i \
  -X PUT 'http://localhost/actions' \
  -H 'Accept: application/json' \
  -H 'Content-Type: application/json' \
  -d '{"action_type": "InstanceStart"}'
```

### JSON Config File (all-in-one alternative)

```json
{
  "boot-source": {
    "kernel_image_path": "vmlinux-5.10",
    "boot_args": "console=ttyS0 reboot=k panic=1 pci=off root=/dev/vda rw"
  },
  "drives": [
    {
      "drive_id": "rootfs",
      "path_on_host": "rootfs.ext4",
      "is_root_device": true,
      "is_read_only": false
    }
  ],
  "machine-config": {
    "vcpu_count": 2,
    "mem_size_mib": 1024
  },
  "network-interfaces": [
    {
      "iface_id": "eth0",
      "guest_mac": "06:00:AC:10:00:02",
      "host_dev_name": "tap0"
    }
  ]
}
```

### Post-Boot API Operations

After boot, the API socket remains active for:
- Creating/restoring snapshots
- Updating block device paths (drive patching)
- Adjusting rate limiters
- Inflating/deflating the memory balloon
- Sending `InstanceHalt` actions

### SDKs

- **Go SDK (official):** `github.com/firecracker-microvm/firecracker-go-sdk` - Full-featured, supports CNI networking, snapshot management, used by firectl
- **Python SDK:** No official Python SDK exists. Interaction is via HTTP over Unix socket (trivially wrappable with `requests-unixsocket` or `aiohttp`)
- **Rust:** Direct API since Firecracker is in Rust

### Microvm Metadata Service (MMDS)

Firecracker includes a built-in metadata service accessible from within the guest:
- Configured via `/mmds` API endpoint
- JSON data store up to 51,200 bytes (configurable)
- Supports MMDS V1 and V2 (V2 adds token-based authentication)
- Useful for passing configuration data (e.g., repo URL, branch name) to the guest without needing network

---

## 4. Networking

### TAP Devices (Primary Mechanism)

Firecracker **only** supports Linux TAP devices for guest networking. No macvtap, no bridge mode, no userspace networking.

**Setup steps:**

```bash
# Create TAP device
sudo ip tuntap add tap0 mode tap
sudo ip addr add 172.16.0.1/24 dev tap0
sudo ip link set tap0 up

# Enable IP forwarding and NAT
sudo sysctl -w net.ipv4.ip_forward=1
sudo iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
sudo iptables -A FORWARD -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
sudo iptables -A FORWARD -i tap0 -o eth0 -j ACCEPT
```

Then configure the guest kernel boot args with the static IP:
```
ip=172.16.0.2::172.16.0.1:255.255.255.0::eth0:off
```

### CNI Plugin Integration

The Go SDK supports CNI-based networking with four required plugins:

1. **ptp** - Point-to-point networking (from standard CNI plugins repo)
2. **host-local** - IP address management (from standard CNI plugins repo)
3. **firewall** - Firewall rules (from standard CNI plugins repo)
4. **tc-redirect-tap** - Redirects traffic from a veth to a TAP device (from AWS Labs)

The `tc-redirect-tap` plugin is key: it can be chained after any CNI plugin that creates a network interface. It sets up the TAP device to mirror the CNI-created interface, and any IP configuration is applied statically to the VM's internal network interface on boot.

**Required capabilities:** `CAP_SYS_ADMIN` and `CAP_NET_ADMIN` for creating/configuring network namespaces.

**CNI config location:** `/etc/cni/conf.d/` (default, overridable)
**CNI plugin binary location:** `/opt/cni/bin` (default, overridable)

### Vsock (Host-Guest Communication Without Networking)

Firecracker implements **virtio-vsock** which provides a socket-based communication channel between host and guest that is **independent of network configuration**:

- Guest uses `AF_VSOCK` sockets
- Host uses `AF_UNIX` sockets
- Guest ports are mapped 1:1 to AF_UNIX sockets on the host
- **Guest-initiated connections:** Guest connects to a port; Firecracker forwards to an AF_UNIX socket on the host
- **Host-initiated connections:** Host connects to the Unix socket, sends `CONNECT PORT\n`, Firecracker forwards to guest

This is highly relevant for our use case -- vsock can be used to transfer files, send commands, or establish a control channel without any network configuration.

---

## 5. Block Device Attachment

### Drive Configuration

Drives are attached via the `/drives/{drive_id}` API endpoint. Key parameters:

| Parameter | Type | Description |
|-----------|------|-------------|
| `drive_id` | string | Unique identifier for the drive |
| `path_on_host` | string | Path to the file-backed block device on the host |
| `is_root_device` | bool | Whether this is the root device |
| `is_read_only` | bool | Whether the drive is read-only |
| `rate_limiter` | object | Optional bandwidth/IOPS limits |

### Important Constraints

- **All drives must be attached before VM start** - no hot-plug support
- Drives are presented as virtio-block devices (`/dev/vda`, `/dev/vdb`, etc.)
- Each drive is backed by a file on the host
- Maximum drives: limited by virtio-block device count

### Copy-on-Write Strategies for Shared Rootfs

For running multiple VMs from the same base rootfs (critical for our use case):

#### Option A: OverlayFS Inside the Guest

1. Set rootfs drive as `is_read_only: true`
2. Add a second writable ext4 drive for the overlay upper layer
3. Use `/sbin/overlay-init` as the init process (Firecracker provides this)
4. Boot args: `init=/sbin/overlay-init`
5. Optional: `overlay_root=vdb` to use a block device instead of tmpfs for the upper layer

```json
{
  "drives": [
    {
      "drive_id": "rootfs",
      "path_on_host": "base-rootfs.squashfs",
      "is_root_device": true,
      "is_read_only": true
    },
    {
      "drive_id": "overlay",
      "path_on_host": "vm1-overlay.ext4",
      "is_root_device": false,
      "is_read_only": false
    }
  ]
}
```

Boot args: `init=/sbin/overlay-init overlay_root=vdb`

#### Option B: Device Mapper on the Host

Use Linux device mapper to create CoW snapshots on the host before starting each VM:

```bash
# Create a loop device for the base image
losetup /dev/loop0 base-rootfs.ext4

# Create thin pool + CoW snapshot per VM
dmsetup create base --table "0 $(blockdev --getsz /dev/loop0) linear /dev/loop0 0"
# ... create thin provisioned snapshot
```

Each VM gets its own CoW snapshot. Only written blocks consume disk space.

#### Option C: Sparse File Copy per VM

Simplest approach: create a sparse copy of the rootfs per VM using `cp --reflink=auto` (on filesystems that support reflinks like btrfs/XFS) or use sparse files.

### Rate Limiting

Per-drive rate limiters use a **token bucket algorithm** with two buckets:
- **Bandwidth bucket** (bytes/sec)
- **Operations bucket** (IOPS)

Each bucket supports burst configuration. Rate limiters can be updated post-boot via the API.

---

## 6. Snapshotting

### Snapshot Capabilities

Firecracker snapshots capture the **full VM state**: CPU registers, memory contents, device states, and disk state. As of v1.14, snapshotting is **generally available** (previously "developer preview"). Incremental snapshots remain in developer preview.

### Snapshot Components

A snapshot produces two files:
1. **`snapshot.snap`** - Machine configuration (CPU template, disk, network devices, vCPU state)
2. **`memory.snap`** - VM memory contents

Combined with the disk image(s), these files contain everything needed to restore a VM to the exact point where the snapshot was taken.

### Creating a Snapshot

```bash
# Pause the VM first
curl --unix-socket /tmp/firecracker.socket -i \
  -X PATCH 'http://localhost/vm' \
  -d '{"state": "Paused"}'

# Create snapshot
curl --unix-socket /tmp/firecracker.socket -i \
  -X PUT 'http://localhost/snapshot/create' \
  -d '{
    "snapshot_type": "Full",
    "snapshot_path": "./snapshot.snap",
    "mem_file_path": "./memory.snap"
  }'
```

### Restoring from a Snapshot

```bash
# Start a new Firecracker process
firecracker --api-sock /tmp/firecracker-restored.socket

# Load snapshot
curl --unix-socket /tmp/firecracker-restored.socket -i \
  -X PUT 'http://localhost/snapshot/load' \
  -d '{
    "snapshot_path": "./snapshot.snap",
    "mem_backend": {
      "backend_type": "File",
      "backend_path": "./memory.snap"
    }
  }'

# Resume
curl --unix-socket /tmp/firecracker-restored.socket -i \
  -X PATCH 'http://localhost/vm' \
  -d '{"state": "Resumed"}'
```

### Performance Numbers

| Operation | Time |
|-----------|------|
| **Snapshot creation** | ~1 second per GB of memory |
| **Snapshot restore (resume)** | ~200-300ms |
| **Memory loading** | On-demand via mmap (lazy) |

### How Restore Works (Memory-Mapped)

When Firecracker loads a memory snapshot, it uses **mmap** to create a mapping from the snapshot file to memory. Pages are loaded on-demand as the guest accesses them (page faults). This means:
- Resume is fast (~200-300ms) because only actively-needed memory is loaded
- Fresh VMs use anonymous memory; restored VMs use file-backed memory + CoW to anonymous memory
- Pages that are never accessed after restore are never loaded

### Cloning from Snapshots

A snapshot can be restored **multiple times** to create clones. This is how production systems achieve fast VM cloning:

**CodeSandbox approach (2 second clone):**
1. Take a snapshot of a running VM
2. Use `userfaultfd` to lazily load memory for clones
3. Memory balloon inflated before snapshot to minimize snapshot size
4. Each clone gets its own CoW disk overlay

**Memory optimization with balloon:**
- Before snapshotting, inflate the balloon to maximum to mark free pages
- Free page reporting means freed pages don't need to be saved
- This can reduce snapshot size dramatically

### Snapshot Caveats

- Snapshots can only be restored with the **same Firecracker version** that created them
- Cross-version snapshot compatibility is not guaranteed
- The guest must handle potential issues with stale state (network connections, timers, RNG state)
- Firecracker provides `/dev/urandom` re-seeding guidance for clones

---

## 7. Boot Time

### Specification

Firecracker guarantees: **<=125ms** from receiving the `InstanceStart` API call to the start of the Linux guest user-space `/sbin/init` process.

### What This Includes

- Loading the kernel into memory
- Initializing the minimal device model
- Booting the kernel
- Reaching /sbin/init

### What This Does NOT Include

- Time for the guest init system to complete (systemd, etc.)
- Time to start application services
- Network configuration inside the guest
- Any file I/O in the guest

### What Affects Boot Time

| Factor | Impact |
|--------|--------|
| **Kernel size** | Larger kernels take longer to load; use Firecracker's minimal configs |
| **Serial console** | Disabling serial console improves boot time |
| **Rootfs size** | Smaller rootfs = less I/O at boot |
| **Guest init system** | Minimal init (custom script) vs systemd (adds seconds) |
| **Host CPU load** | Contention slows boot |
| **Host disk I/O** | SSD vs HDD matters for loading kernel and rootfs |
| **vCPU count** | Minimal impact on boot time itself |
| **Memory size** | Minimal impact on boot time |

### Practical Boot Times

| Scenario | Time |
|----------|------|
| Firecracker VMM boot to /sbin/init | ~125ms |
| With minimal Alpine Linux init | ~200-300ms total |
| With systemd-based init | ~1-3 seconds total |
| Snapshot restore | ~200-300ms |
| Full usable environment (network, services) | ~1-5 seconds depending on guest |

### Achieving Fastest Boot

For our use case, to minimize time-to-bash-shell:
1. Use Firecracker's tuned kernel config
2. Use a minimal rootfs (Alpine-based)
3. Custom init script instead of systemd
4. Disable serial console if not needed
5. Pre-configure network in boot args
6. Or: use **snapshot restore** of a pre-warmed environment (~200ms)

---

## 8. Security Model

### Defense-in-Depth Architecture

Firecracker implements multiple security layers:

```
Layer 4: Hardware virtualization (KVM) - hardware-enforced isolation
Layer 3: Minimal device model - reduced attack surface
Layer 2: Jailer - chroot, namespaces, cgroups, seccomp
Layer 1: Firecracker process - runs unprivileged after jailer setup
```

### The Jailer

The **jailer** is a separate binary that sets up the security sandbox for the Firecracker process:

1. Creates a chroot directory
2. Sets up cgroup isolation (CPU, memory limits)
3. Creates new namespaces (PID, network, mount)
4. Copies/bind-mounts required files into chroot
5. Drops all privileges
6. Applies seccomp filters
7. `exec()`s into the Firecracker binary

After the jailer executes, the Firecracker process:
- Runs as an **unprivileged user**
- Cannot see other host processes
- Cannot access the host filesystem outside the chroot
- Has no network access on the host
- Is limited to exactly **24 system calls** via seccomp

### Seccomp Filters

- Applied **per-thread** before any guest code executes
- Default filters allow only the bare minimum system calls
- Custom filters can be provided via `--seccomp-filter` flag
- Filters are BPF-based (seccomp-bpf)

### Jailer Usage Example

```bash
jailer --id vm1 \
  --exec-file /usr/bin/firecracker \
  --uid 1000 --gid 1000 \
  --chroot-base-dir /srv/jailer \
  --daemonize \
  -- --config-file vm_config.json
```

This creates the jail at `/srv/jailer/firecracker/vm1/root/`

### Security Boundary

Even if an attacker escapes the hardware virtualization boundary (requiring a CPU hardware bug or KVM vulnerability), they would still be:
- Inside a chroot jail
- With no network access
- With no view of other host processes
- Restricted to 24 system calls

### Practical Security Implications for Our Use Case

- Each sandbox VM is strongly isolated from the host and from other VMs
- The jailer adds overhead but is recommended for production use
- For development/local use, running without the jailer is acceptable
- The seccomp filters protect against kernel exploits from within the VM

---

## 9. File Sharing with Host

### The Problem

Firecracker does **NOT** support:
- **virtiofs** (FUSE-based shared filesystem)
- **9p/VirtFS** (Plan 9 filesystem protocol)
- Any form of shared filesystem passthrough

This is a **deliberate design decision** for security -- shared filesystems increase the attack surface significantly.

### Workarounds for Getting Files In/Out of VMs

#### Option 1: Block Device with Repo Content (Best for Our Use Case)

1. Create an ext4 image containing the git repo
2. Attach it as an additional drive (`/dev/vdb`)
3. Mount it inside the guest

```bash
# On host: create image with repo content
truncate -s 1G repo.ext4
mkfs.ext4 repo.ext4
mount repo.ext4 /tmp/repo
cp -a /path/to/git/repo/* /tmp/repo/
umount /tmp/repo

# Attach as additional drive in Firecracker config
# Guest mounts /dev/vdb at /workspace
```

**Pros:** Simple, fast, works with read-only base + CoW overlay
**Cons:** Need to create image each time, cannot see changes in real-time

#### Option 2: Vsock-Based File Transfer

Use virtio-vsock to transfer files between host and guest:

1. Run a file transfer agent on the host listening on the vsock Unix socket
2. Inside the guest, connect via AF_VSOCK to request/send files
3. Implement a simple protocol (e.g., tar stream over vsock)

**Pros:** Dynamic, bidirectional, no disk image creation needed
**Cons:** Requires custom agent software on both sides

#### Option 3: Network-Based (NFS/SSHFS/rsync)

Use the TAP network to mount NFS, use sshfs, or rsync:

```bash
# Inside the guest:
mount -t nfs 172.16.0.1:/path/to/repo /workspace
# or
sshfs user@172.16.0.1:/path/to/repo /workspace
# or
rsync -avz user@172.16.0.1:/path/to/repo/ /workspace/
```

**Pros:** Standard tools, bidirectional
**Cons:** Network overhead, requires running NFS/SSH server on host

#### Option 4: MMDS for Small Config Data

Use the Firecracker MMDS to pass configuration (repo URL, branch, etc.) to the guest. The guest then clones/fetches via network.

**Pros:** Built-in, no extra setup
**Cons:** Limited to 51KB, only for metadata, not file content

#### Option 5: Pre-baked Rootfs with Repo Content

Include the repo content directly in the rootfs image, using OverlayFS for writes:

1. Build rootfs with base OS + tools + repo snapshot
2. Boot with overlay-init for CoW writes
3. On VM shutdown, extract changed files from overlay

**Pros:** Fastest boot (everything is in the rootfs)
**Cons:** Need to rebuild rootfs for each repo state

### Recommended Approach for Our Use Case

**Hybrid:** Use a base rootfs (read-only, shared) with OverlayFS, plus an additional block device containing the git repo content. On VM shutdown, extract the overlay diff or the modified repo drive content.

---

## 10. Existing Tooling

### firectl

- **Repo:** [firecracker-microvm/firectl](https://github.com/firecracker-microvm/firectl)
- **Language:** Go
- **Purpose:** Simple CLI tool to launch Firecracker microVMs
- **Status:** Maintained (basic tool, not full management)
- **Usage:**

```bash
firectl \
  --kernel=vmlinux \
  --root-drive=rootfs.ext4 \
  --tap-device=tap0/06:00:AC:10:00:02 \
  -c 2 -m 1024 \
  --kernel-opts="console=ttyS0 reboot=k panic=1 pci=off root=/dev/vda rw"
```

**Options:** `--kernel`, `--root-drive`, `--add-drive`, `--tap-device`, `--vsock-device`, `-c` (CPUs), `-m` (memory MiB), `--cpu-template`, `--metadata`, `--firecracker-binary`, etc.

### Firecracker Go SDK

- **Repo:** [firecracker-microvm/firecracker-go-sdk](https://github.com/firecracker-microvm/firecracker-go-sdk)
- **Language:** Go 1.23+
- **Purpose:** Programmatic VM management, CNI networking, snapshot support
- **Status:** Actively maintained by AWS/Firecracker team
- **Key features:** Static and CNI-based networking, drive management, snapshot operations

### Ignite (Weaveworks)

- **Repo:** [weaveworks/ignite](https://github.com/weaveworks/ignite)
- **Purpose:** Docker/OCI UX for Firecracker VMs -- pull OCI images, run them as VMs
- **Status:** **Effectively abandoned** (Weaveworks shut down in 2024)
- **Key features:** `ignite run weaveworks/ignite-ubuntu` -- like Docker but with VMs
- **Architecture:** Uses containerd + CNI + Firecracker under the hood

### Flintlock

- **Repo:** [liquidmetal-dev/flintlock](https://github.com/liquidmetal-dev/flintlock)
- **Purpose:** gRPC service for creating/managing microVM lifecycle backed by containerd
- **Status:** Community-maintained (post-Weaveworks)
- **Supports:** Both Firecracker and Cloud Hypervisor as VMM backends

### firecracker-containerd

- **Repo:** [firecracker-microvm/firecracker-containerd](https://github.com/firecracker-microvm/firecracker-containerd)
- **Purpose:** Enables containerd to manage containers inside Firecracker microVMs
- **Status:** Maintained by AWS

### E2B (Cloud Service)

- **Website:** [e2b.dev](https://e2b.dev)
- **Purpose:** Cloud-hosted Firecracker sandboxes for AI agents
- **Scale:** 40,000 to 15 million sandbox creations per month (2024-2025 growth)
- **Boot time:** <200ms for sandboxes in same region
- **Key client:** Manus AI, Groq
- **Funding:** $32M total (Series A in July 2025)
- **Relevance:** Proves the "AI agent sandbox" use case at scale with Firecracker

---

## 11. Production Users

### AWS Lambda

- **Scale:** Tens of trillions of function invocations monthly
- **Architecture:** Lambda functions run inside individual Firecracker microVMs on bare-metal Nitro EC2 instances
- **Worker lifetime:** 14 hours, then terminated
- **Boot time:** ~125ms for cold starts
- **Memory:** 128MB to 10GB per function
- **Firecracker was literally built for this use case**

### Fly.io

- **Architecture:** Firecracker microVMs running at edge locations globally
- **Hardware:** Dedicated physical servers with 8-32 CPU cores, 32-256GB RAM
- **Product:** "Fly Machines" -- sub-second start and stop
- **Use case:** Application hosting, API servers, databases in microVMs
- **Key feature:** Strong hardware-virtualization-based isolation on shared hardware

### Koyeb

- **Architecture:** Moved from Kubernetes to Nomad + Firecracker + Kuma
- **Use case:** Serverless platform with autoscaling and scale-to-zero
- **Integration:** Nomad provisions Firecracker microVMs, containerd manages containers within them

### CodeSandbox

- **Architecture:** Firecracker microVMs for cloud development environments
- **Key innovation:** Clone a running VM in 2 seconds using snapshots + userfaultfd
- **Memory optimization:** Low-latency memory decompression for snapshot restore
- **Use case:** Interactive development environments with instant forking

### E2B

- **Architecture:** Firecracker sandboxes for AI agent code execution
- **Scale:** 15 million sandbox creations/month
- **Clients:** Manus AI, Groq, LangChain users
- **Innovation:** Sub-200ms sandbox creation for AI workloads

### Other Users

- **Weave (before shutdown):** Used Firecracker via Ignite for GitOps-managed VMs
- **Northflank:** Offers Firecracker-based sandboxes as part of their platform
- **Various CI/CD systems:** webapp.io claims 10x faster E2E tests with Firecracker

---

## 12. Comparison with Alternatives

### Firecracker vs Cloud Hypervisor

| Aspect | Firecracker | Cloud Hypervisor |
|--------|-------------|-----------------|
| **Language** | Rust | Rust |
| **Shared codebase** | rust-vmm components | rust-vmm components |
| **Boot time** | ~125ms | ~200ms |
| **Memory overhead** | <5 MiB | Slightly higher |
| **CPU/Memory hotplug** | Memory hotplug added v1.14 | Full CPU + memory hotplug |
| **Device hotplug** | No | Yes (vhost-user) |
| **Windows guests** | No | Yes (Windows 10/Server 2019) |
| **GPU passthrough** | No | Via VFIO |
| **Architecture support** | x86_64, aarch64 | x86_64, aarch64, riscv64 |
| **Organization** | AWS (Apache 2.0) | Linux Foundation |
| **Kata Containers** | Supported backend | Default backend |
| **I/O performance** | Good | Better write latency |
| **Best for** | Ephemeral serverless | Long-running cloud workloads |

### Firecracker vs QEMU microvm

| Aspect | Firecracker | QEMU microvm |
|--------|-------------|-------------|
| **Inspiration** | Original | Inspired by Firecracker |
| **Codebase** | 50K lines Rust | Part of 2M line QEMU C codebase |
| **PCI support** | Optional (v1.14+) | No PCI, no ACPI |
| **Device support** | 5-7 virtio devices | More devices available |
| **vhost support** | No | Yes |
| **Boot time** | ~125ms | Similar (slightly slower) |
| **GPU passthrough** | No | No (full QEMU has it) |
| **Security** | Rust + jailer + seccomp | Relies on QEMU sandboxing |

### Firecracker vs Kata Containers

| Aspect | Firecracker | Kata Containers |
|--------|-------------|----------------|
| **Level** | VMM (low-level) | Orchestration framework |
| **VMM backend** | Is the VMM | Uses Firecracker, Cloud Hypervisor, or QEMU |
| **Kubernetes** | No native integration | Full K8s CRI integration |
| **OCI compliance** | No | Yes |
| **Boot time** | ~125ms (VMM only) | 150-300ms (full stack) |
| **Management** | API + manual | Automatic via K8s |
| **Best for** | Custom serverless | K8s workload isolation |

### Firecracker vs gVisor

| Aspect | Firecracker | gVisor |
|--------|-------------|--------|
| **Isolation** | Hardware VM (KVM) | User-space kernel (syscall proxy) |
| **Boot time** | ~125ms | ~50-100ms |
| **Overhead** | <5 MiB | Lower |
| **Compatibility** | Full Linux kernel | Subset of syscalls |
| **Security model** | Hardware virtualization | Syscall interception |
| **File system** | Own kernel, own FS | Shares host FS (configurable) |
| **Best for** | Strong isolation | Container-like with better isolation |

### Recommendation for Our Use Case

**Firecracker** is the best fit because:
1. Strong isolation (hardware VM boundary) for running untrusted AI agent code
2. Sub-second boot times
3. Low memory overhead (many parallel VMs on one machine)
4. Snapshot/restore for fast cloning
5. Proven at scale (Lambda, Fly.io, E2B)
6. Simple API for programmatic control

---

## 13. Resource Requirements

### Per-VM Overhead

| Resource | Amount |
|----------|--------|
| **VMM memory overhead** | <=5 MiB (Firecracker process itself) |
| **Guest memory** | Configurable: 128 MiB minimum default, up to 32 GiB |
| **vCPUs** | 1-32 per VM |
| **Disk** | Size of rootfs image + overlay (sparse files help) |
| **File descriptors** | One Unix socket per VM + drive FDs |
| **Processes** | One Firecracker process per VM |

### Host Requirements

| Requirement | Details |
|-------------|---------|
| **CPU** | 64-bit Intel, AMD, or ARM with hardware virtualization (VT-x/AMD-V) |
| **KVM** | Must have `/dev/kvm` access |
| **Linux kernel** | 4.14+ (5.10+ recommended) |
| **RAM** | Depends on number of VMs; each VM uses its configured memory |
| **Disk** | SSD strongly recommended for rootfs I/O |

### VM Density

- Firecracker supports microVM creation rates of up to **150 microVMs per second per host**
- With memory oversubscription, thousands of microVMs can run on a single host
- AWS Lambda runs thousands of microVMs per bare-metal Nitro instance

### Resource for Our Use Case (Developer Laptop)

For running 5-10 parallel sandbox VMs on a developer machine:

| Resource | Per VM | 10 VMs |
|----------|--------|--------|
| VMM overhead | 5 MiB | 50 MiB |
| Guest memory (minimal) | 256-512 MiB | 2.5-5 GiB |
| Guest memory (comfortable) | 1-2 GiB | 10-20 GiB |
| vCPUs | 1-2 | 10-20 (oversubscribable) |
| Disk (shared rootfs) | 500MB shared | 500MB shared |
| Disk (per-VM overlay) | ~10-100MB | ~100MB-1GB |
| Boot time | ~200ms | ~200ms (parallel) |

A machine with 16-32GB RAM and 8+ cores can comfortably run 5-10 parallel sandbox VMs.

### Memory Balloon for Reclamation

Firecracker's virtio-balloon device allows dynamic memory management:
- Inflate balloon to reclaim memory from guest back to host
- Deflate on OOM to give memory back to guest
- Free page reporting (v1.14) for efficient memory snapshot size
- Configuration: `"amount_mib"`, `"deflate_on_oom"`, `"stats_polling_interval_s"`

---

## 14. Limitations

### Deliberate Omissions (By Design)

| Feature | Status | Reason |
|---------|--------|--------|
| **GPU passthrough** | Not supported | Attack surface, complexity, blocks memory oversubscription |
| **USB devices** | Not supported | Attack surface |
| **PCI passthrough** | Not supported (PCI transport added v1.14 for virtio only) | Security |
| **Sound devices** | Not supported | Not needed for serverless |
| **Graphics/display** | Not supported | Not needed |
| **Architecture emulation** | Not supported | KVM only (same arch) |
| **Windows guests** | Not supported | Linux only |
| **Hot-plug (devices)** | Not supported (memory hotplug added v1.14) | Complexity |
| **Live migration** | Not supported | Use snapshot/restore instead |
| **NUMA awareness** | Not supported | Complexity |
| **Virtio-fs / 9p** | Not supported | Security (host filesystem exposure) |
| **vhost kernel I/O** | Not supported | Security (keeps I/O in userspace) |

### Practical Limitations

| Limitation | Impact |
|-----------|--------|
| **KVM required** | Cannot run on hosts without hardware virtualization (e.g., some cloud VMs without nested virt) |
| **Linux host only** | No macOS or Windows host (WSL2 with nested virt can work) |
| **Linux guests only** | No Windows, FreeBSD, etc. |
| **No shared filesystem** | Must use block devices, network, or vsock for file transfer |
| **Snapshot version pinning** | Snapshots only work with same Firecracker version |
| **No cross-version compat** | API and configs may change between versions |
| **No vhost I/O** | Lower I/O throughput than QEMU with vhost |
| **Intel Skylake dropped** | Removed official support (still works, not tested) |
| **Kernel version support** | Only 5.10 and 6.1 officially supported |

### WSL2 Compatibility

Firecracker **can** run on WSL2 with configuration:
1. Enable nested virtualization: `.wslconfig` with `[wsl2] nestedVirtualization=true`
2. May need custom WSL2 kernel build with device mapper support
3. KVM access must work (`/dev/kvm` must be available)
4. Performance may be lower than bare-metal Linux

---

## 15. Relevance to Our Use Case

### Use Case Recap

A developer runs a single command in a folder with a git checkout. A Firecracker VM spawns with a copy of that repo inside it, drops the user into a bash shell. Multiple VMs run in parallel. When done, changes are committed back.

### Recommended Architecture

```
Developer runs: sandbox-run /path/to/repo
  |
  +-- Manager process (Python/Go)
  |     |
  |     |-- 1. Create per-VM overlay from shared base rootfs
  |     |-- 2. Create ext4 image from git repo working tree (or use block device + tar via vsock)
  |     |-- 3. Create TAP device (or use vsock-only for simplicity)
  |     |-- 4. Start Firecracker via API
  |     |-- 5. Connect user to serial console or SSH
  |     |
  |     +-- On exit:
  |           |-- 6. Extract modified files from overlay or repo drive
  |           +-- 7. Apply changes back to host git repo (git add/commit)
  |
  +-- Firecracker microVM
        |-- Base rootfs (read-only, shared via OverlayFS)
        |-- Overlay drive (per-VM, writable)
        |-- Repo drive (/dev/vdb mounted at /workspace)
        +-- Vsock for host<->guest communication
```

### Key Technical Decisions

1. **File transfer strategy:** Block device with repo content (fastest) or vsock-based tar streaming (most flexible)
2. **Networking:** Vsock for control channel; TAP + NAT only if internet access needed inside VM
3. **Rootfs:** Alpine-based minimal image with git, build tools, language runtimes
4. **Boot strategy:** Direct boot for first run; snapshot restore for subsequent runs (~200ms)
5. **Parallel VMs:** Each gets own overlay + repo drive; shared kernel and base rootfs
6. **Change extraction:** Mount the overlay/repo drive on host after VM exits, diff with original

### Performance Budget

| Phase | Time |
|-------|------|
| Create overlay + repo image | ~100-500ms |
| Boot Firecracker VM | ~125ms |
| Guest init to bash shell | ~200-500ms |
| **Total time to bash** | **~500ms-1.5s** |
| With snapshot restore | **~200-300ms total** |

### Open Questions for Implementation

1. How to handle the repo drive creation efficiently for large repos?
2. Should we use a persistent snapshot per-repo for instant boot?
3. How to handle submodules and large files (git-lfs)?
4. Should the VM have internet access (for package installs)?
5. How to handle multiple users/concurrent access to the same repo?

---

## Sources

### Official Documentation
- [Firecracker GitHub Repository](https://github.com/firecracker-microvm/firecracker)
- [Firecracker Getting Started](https://github.com/firecracker-microvm/firecracker/blob/main/docs/getting-started.md)
- [Firecracker Rootfs and Kernel Setup](https://github.com/firecracker-microvm/firecracker/blob/main/docs/rootfs-and-kernel-setup.md)
- [Firecracker Snapshot Support](https://github.com/firecracker-microvm/firecracker/blob/main/docs/snapshotting/snapshot-support.md)
- [Firecracker Network Setup](https://github.com/firecracker-microvm/firecracker/blob/main/docs/network-setup.md)
- [Firecracker Design Doc](https://github.com/firecracker-microvm/firecracker/blob/main/docs/design.md)
- [Firecracker Specification](https://github.com/firecracker-microvm/firecracker/blob/main/SPECIFICATION.md)
- [Firecracker Vsock](https://github.com/firecracker-microvm/firecracker/blob/main/docs/vsock.md)
- [Firecracker Ballooning](https://github.com/firecracker-microvm/firecracker/blob/main/docs/ballooning.md)
- [Firecracker MMDS User Guide](https://github.com/firecracker-microvm/firecracker/blob/main/docs/mmds/mmds-user-guide.md)
- [Firecracker Kernel Policy](https://github.com/firecracker-microvm/firecracker/blob/main/docs/kernel-policy.md)

### SDKs and Tools
- [Firecracker Go SDK](https://github.com/firecracker-microvm/firecracker-go-sdk)
- [firectl](https://github.com/firecracker-microvm/firectl)
- [Weaveworks Ignite](https://github.com/weaveworks/ignite)
- [Flintlock](https://github.com/liquidmetal-dev/flintlock)
- [firecracker-containerd](https://github.com/firecracker-microvm/firecracker-containerd)

### Production Usage and Case Studies
- [Seven Years of Firecracker - Marc Brooker](https://brooker.co.za/blog/2025/09/18/firecracker.html)
- [Fly.io Architecture](https://fly.io/docs/reference/architecture/)
- [CodeSandbox: How We Clone a Running VM in 2 Seconds](https://codesandbox.io/blog/how-we-clone-a-running-vm-in-2-seconds)
- [CodeSandbox: Cloning MicroVMs Using userfaultfd](https://codesandbox.io/blog/cloning-microvms-using-userfaultfd)
- [CodeSandbox: Low-Latency Memory Decompression](https://codesandbox.io/blog/how-we-scale-our-microvm-infrastructure-using-low-latency-memory-decompression)
- [Koyeb: From Kubernetes to Nomad and Firecracker](https://www.koyeb.com/blog/the-koyeb-serverless-engine-from-kubernetes-to-nomad-firecracker-and-kuma)
- [Firecracker: Lightweight Virtualization for Serverless (AWS Blog)](https://aws.amazon.com/blogs/aws/firecracker-lightweight-virtualization-for-serverless-computing/)
- [Behind the Scenes Lambda](https://www.bschaatsbergen.com/behind-the-scenes-lambda)
- [E2B: How Manus Uses E2B](https://e2b.dev/blog/how-manus-uses-e2b-to-provide-agents-with-virtual-computers)

### Comparisons and Analysis
- [Firecracker vs QEMU (E2B)](https://e2b.dev/blog/firecracker-vs-qemu)
- [Firecracker vs QEMU (Northflank)](https://northflank.com/blog/firecracker-vs-qemu)
- [Why We Replaced Firecracker with QEMU (Hocus)](https://hocus.dev/blog/qemu-vs-firecracker/)
- [Kata vs Firecracker vs gVisor (Northflank)](https://northflank.com/blog/kata-containers-vs-firecracker-vs-gvisor)
- [gVisor vs Kata vs Firecracker on VPS (2025)](https://onidel.com/blog/gvisor-kata-firecracker-2025)
- [Cloud Hypervisor Guide (Northflank)](https://northflank.com/blog/guide-to-cloud-hypervisor)
- [Firecracker vs Docker for Agentic Workloads](https://nextkicklabs.substack.com/p/firecracker-vs-docker-security-tradeoffs)

### Technical Deep Dives
- [Firecracker Internals (Tal Hoffman)](https://www.talhoffman.com/2021/07/18/firecracker-internals/)
- [E2B: Scaling Firecracker Using OverlayFS](https://e2b.dev/blog/scaling-firecracker-using-overlayfs-to-save-disk-space)
- [Space Efficient Filesystems for Firecracker](https://parandrus.dev/devicemapper/)
- [Using Device Mapper for Firecracker Images (Julia Evans)](https://jvns.ca/blog/2021/01/27/day-47--using-device-mapper-to-manage-firecracker-images/)
- [Firecracker: Start a VM in Less Than a Second (Julia Evans)](https://jvns.ca/blog/2021/01/23/firecracker--start-a-vm-in-less-than-a-second/)
- [Using Firecracker and Go for Untrusted Code (Stanislas)](https://stanislas.blog/2021/08/firecracker/)
- [Networking for a Firecracker Lab](https://blog.0x74696d.com/posts/networking-firecracker-lab/)
- [Firecracker MicroVMs on WSL2 (Veltishchev)](https://medium.com/@veltun/configuring-wsl2-to-support-firecracker-vms-i-e-for-containerlab-a3d36ca8ed8a)
- [GPU Discussion (Firecracker GitHub)](https://github.com/firecracker-microvm/firecracker/discussions/4845)
- [Shared Rootfs Discussion (Firecracker GitHub)](https://github.com/firecracker-microvm/firecracker/discussions/3061)
- [Cloud Virtualization Internals (Ubicloud)](https://www.ubicloud.com/blog/cloud-virtualization-red-hat-aws-firecracker-and-ubicloud-internals)
