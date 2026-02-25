# Viable Architecture Combinations for AI Agent VM Sandbox

**Date:** 2026-02-11
**Purpose:** Synthesize research findings into concrete architecture options for a local sandbox system where: a developer runs a command in a git repo, a VM spawns with a copy of the repo (dependencies pre-installed), they get a bash shell, work in isolation, and commit changes back. Many VMs run in parallel.

---

## Overview

After researching Firecracker, Cloud Hypervisor, filesystem CoW strategies, git integration, and existing tools, we identified **5 viable architecture combinations** ranging from simple to sophisticated. Each is a complete end-to-end solution.

---

## Architecture 1: Firecracker + OverlayFS + Block Device Copy

**Inspiration:** E2B's production architecture

### Components

| Layer | Technology |
|-------|-----------|
| VMM | Firecracker |
| Base image | Squashfs-compressed rootfs (read-only) |
| Per-VM writes | Sparse ext4 image as OverlayFS upper layer |
| Repo transfer | tar/rsync into ext4 block device image |
| Git integration | OverlayFS upper layer extraction → git commit on host |
| Networking | virtio-vsock for control channel; TAP + NAT for internet |
| Dependency cache | Pre-baked squashfs layers keyed by lock file hash |
| Fast restart | Firecracker snapshot/restore (~200ms) |

### How It Works End-to-End

```
1. User runs: sandbox spawn /path/to/repo
2. Manager checks if a cached base image exists (keyed by lock file hash)
   - If not: create ext4, install deps, compress to squashfs
   - If yes: reuse existing squashfs
3. Create per-VM sparse ext4 file (a few KB, grows on write)
4. Start Firecracker VM with:
   - Drive 1: base rootfs squashfs (read-only, shared)
   - Drive 2: sparse ext4 for overlay writes
   - Drive 3: ext4 image containing git repo snapshot
   - Boot args: init=/sbin/overlay-init overlay_root=vdb
5. Guest init script mounts overlay, drops user into bash at /workspace
6. User works in the VM...
7. On exit (Ctrl-D or explicit command):
   - Mount the per-VM ext4 overlay on host
   - Extract changed files from upper/ directory
   - Apply to a staging copy of the repo
   - git add -A && git commit on unique branch
   - Present result to user (branch name, diff summary)
8. Clean up: delete sparse overlay file, release TAP device
```

### Pros
- **Proven at scale**: E2B runs 15M+ sandboxes/month with this pattern
- **Strong isolation**: Hardware VM boundary (KVM)
- **Space efficient**: Squashfs compression (2-3x) + CoW overlays
- **Simple diff extraction**: Just read the OverlayFS upper directory
- **Fast boot with snapshots**: ~200ms restore of a pre-warmed snapshot
- **Minimal dependencies**: Just Firecracker binary + Linux kernel image

### Cons
- **No live filesystem sharing**: Changes in VM not visible on host until extraction
- **Image creation overhead**: First run requires building squashfs base (minutes)
- **No memory reclamation**: Firecracker doesn't return unused RAM to host (Hocus learned this the hard way)
- **Block device creation for repo**: Must tar/rsync repo into ext4 image each time (~1-5s for large repos)
- **No virtiofs**: Cannot mount host directories directly

### Performance

| Metric | Value |
|--------|-------|
| First run (cold, no cached image) | 2-5 minutes (build base image) |
| Subsequent runs (cached base) | ~1-3 seconds (create overlay + copy repo + boot) |
| With snapshot restore | ~200-500ms |
| Memory per VM | Configured guest memory (256MB-2GB) + 5MB VMM overhead |
| Disk per VM | Only bytes written (typically KB-MB) + repo image |
| Parallel VMs on 32GB host | 10-30 (depending on per-VM memory) |
| Change extraction | <1 second |

### Complexity to Implement
**Medium.** Requires building: overlay-init script for guest, base image builder, Firecracker API wrapper, change extraction pipeline. Well-documented patterns from E2B.

### Best For
- **Maximum isolation** with hardware VM boundary
- Teams that need to run **untrusted AI-generated code**
- Workloads that are **ephemeral** (minutes to low hours)
- Cases where **no live sync** between host and VM is acceptable

---

## Architecture 2: Cloud Hypervisor + virtiofs + Direct Mount

**Inspiration:** Kata Containers, Hocus lessons learned

### Components

| Layer | Technology |
|-------|-----------|
| VMM | Cloud Hypervisor |
| Base image | ext4 or squashfs rootfs |
| Filesystem sharing | virtiofs (host directory mounted in guest) |
| Repo transfer | Direct mount via virtiofs (no copying!) |
| Git integration | Git operations in VM work directly on shared files |
| Memory management | virtio-balloon for dynamic RAM reclamation |
| Networking | virtio-net with TAP + NAT |
| Dependency cache | Shared host directory mounted read-only via virtiofs |

### How It Works End-to-End

```
1. User runs: sandbox spawn /path/to/repo
2. Manager creates a CoW working copy on host:
   - cp --reflink=always (on Btrfs/XFS) or rsync to temp dir
   - This gives each VM an independent copy of the repo
3. Start virtiofsd daemon pointing at the VM's working copy
4. Start Cloud Hypervisor VM with:
   - Root drive: base OS rootfs
   - virtiofs share: /path/to/vm-working-copy → /workspace in guest
   - virtio-balloon: configured for dynamic memory
5. Guest boots, /workspace is immediately available via virtiofs
6. User works in the VM... changes are written through virtiofs to host
7. On exit:
   - Git operations happen directly on the host files (via virtiofs)
   - Or: host reads the working copy directory for changes
   - git add -A && git commit on unique branch
   - No extraction step needed — files are already on host!
8. Clean up: stop virtiofsd, delete working copy
```

### Pros
- **Live filesystem sharing**: Changes in VM immediately visible on host
- **No image creation for repo**: Direct directory mount, zero copy overhead
- **Memory ballooning**: Idle VMs return RAM to host (critical for many parallel VMs)
- **Simpler git integration**: Git operations in VM work on real host files
- **No extraction step**: Files are already on host when VM exits
- **CPU/memory hotplug**: Adjust resources without restart

### Cons
- **Slightly slower boot**: ~200ms vs Firecracker's ~125ms
- **Larger attack surface**: virtiofs adds code complexity vs Firecracker's minimal model
- **Less battle-tested**: Not powering Lambda-scale workloads
- **virtiofsd per VM**: Each VM needs its own virtiofsd daemon process
- **Requires Btrfs/XFS for efficient CoW copies** (or fall back to rsync)
- **Less compression**: No squashfs base layer benefit since files are on host FS

### Performance

| Metric | Value |
|--------|-------|
| First run | ~1-2 seconds (CoW copy + boot) |
| With reflink copy | ~200-500ms (instant copy + boot) |
| Memory per VM | Guest memory + balloon gives back unused |
| Disk per VM | Only changed blocks (with reflink) or full copy |
| Parallel VMs on 32GB host | 15-50+ (balloon reclaims idle VM memory) |
| File I/O throughput | ~643 MB/s with DAX mode, ~35 MB/s without |
| Change extraction | Instant (files already on host) |

### Complexity to Implement
**Medium-Low.** Cloud Hypervisor has a clear API. virtiofs setup is well-documented. Main complexity is managing virtiofsd processes and CoW copies.

### Best For
- **Long-running sessions** (hours) where memory reclamation matters
- Workflows needing **live sync** between host and VM
- Cases where **simple git integration** is prioritized
- Teams comfortable with Cloud Hypervisor (vs Firecracker's larger ecosystem)

---

## Architecture 3: Firecracker + dm-thin Provisioning + Snapshot/Restore

**Inspiration:** firecracker-containerd, CodeSandbox

### Components

| Layer | Technology |
|-------|-----------|
| VMM | Firecracker |
| Storage backend | Device mapper thin provisioning (dm-thin) |
| Base image | Thin volume in shared pool |
| Per-VM storage | Thin snapshot (instant, block-level CoW) |
| Fast boot | Firecracker snapshot/restore (~200ms) |
| Repo transfer | Pre-baked into base thin volume |
| Git integration | Mount thin snapshot after VM exit → git diff |
| Networking | vsock for control; optional TAP for internet |

### How It Works End-to-End

```
1. ONE-TIME SETUP:
   - Create a dm-thin pool (sparse files, e.g., 100GB virtual)
   - Create base thin volume (ID=0) with OS + tools + deps + repo snapshot
   - Boot a Firecracker VM from this volume
   - After everything is warm (deps installed, services started), take a
     Firecracker snapshot (memory.snap + snapshot.snap)
   - This snapshot becomes the "golden image"

2. User runs: sandbox spawn /path/to/repo
3. Manager:
   a. Creates instant thin snapshot from base volume (sub-millisecond)
   b. Optionally updates the snapshot with latest repo state
      (mount snapshot, rsync changed files, unmount)
   c. Copies the golden Firecracker snapshot files
   d. Patches the snapshot config to point to the new thin volume
   e. Starts Firecracker and loads the snapshot (~200ms)
4. VM resumes exactly where the golden image was snapshotted:
   - Already booted, deps installed, shell ready
   - User is dropped into bash at /workspace
5. User works in the VM...
6. On exit:
   - Mount the thin snapshot on host
   - Use thin_delta or filesystem diff to identify changes
   - Apply changes to host repo
   - git add -A && git commit on unique branch
7. Clean up: delete thin snapshot, release resources
```

### Pros
- **Fastest possible startup**: ~200ms snapshot restore (already booted!)
- **Most space-efficient**: Block-level CoW means only written blocks use space
- **Proven pattern**: firecracker-containerd uses dm-thin in production
- **Instant "clone"**: Thin snapshot creation is sub-millisecond
- **Efficient for large repos**: Block-level CoW is better than file-level for large binaries
- **Scale**: dm-thin handles hundreds of snapshots efficiently

### Cons
- **Most complex setup**: dm-thin pool management, snapshot lifecycle, Firecracker snapshot handling
- **Device mapper complexity**: Need to manage loop devices, thin pools, metadata
- **Snapshot version coupling**: Firecracker snapshots are version-pinned
- **Change extraction is harder**: Block-level diff requires thin_delta tool or mounting + comparing
- **No memory reclamation**: Firecracker limitation still applies
- **Stale state risk**: Memory snapshot may have stale network connections, timers

### Performance

| Metric | Value |
|--------|-------|
| First run (cold) | 2-5 minutes (build base volume + initial snapshot) |
| Subsequent runs | ~200-300ms (thin snapshot + memory restore) |
| Memory per VM | Configured guest memory + 5MB VMM; snapshot memory mmap'd lazily |
| Disk per VM | Only written blocks (typically KB-few MB) |
| Parallel VMs on 32GB host | 10-30+ (memory pages loaded on demand) |
| Thin snapshot creation | <1ms |
| Change extraction | 1-5 seconds (mount + diff) |

### Complexity to Implement
**High.** Requires: dm-thin pool management, Firecracker snapshot lifecycle, snapshot patching, block-level change extraction. But delivers the best performance.

### Best For
- **Fastest possible startup** is the top priority
- Running **many parallel VMs** (50+) where space efficiency matters
- **Ephemeral workloads** (snapshot restore is ideal for short-lived VMs)
- Teams willing to invest in **infrastructure complexity** for performance

---

## Architecture 4: Cloud Hypervisor + microvm.nix + Nix Store Sharing

**Inspiration:** microvm.nix project, NixOS reproducible builds

### Components

| Layer | Technology |
|-------|-----------|
| VMM | Cloud Hypervisor (or Firecracker via microvm.nix) |
| Environment definition | Nix flake (declarative, reproducible) |
| Base image | NixOS minimal rootfs (squashfs/erofs, only required /nix/store paths) |
| Dependency management | Nix store (content-addressed, deduplicated) |
| Filesystem sharing | virtiofs for /nix/store (read-only) + repo directory |
| Per-VM writes | OverlayFS on top of shared base |
| Git integration | Direct via virtiofs or block device + extraction |

### How It Works End-to-End

```
1. User defines environment in flake.nix:
   {
     inputs.microvm.url = "github:microvm-nix/microvm.nix";
     outputs = { self, nixpkgs, microvm }: {
       nixosConfigurations.sandbox = nixpkgs.lib.nixosSystem {
         modules = [
           microvm.nixosModules.microvm
           {
             microvm = {
               hypervisor = "cloud-hypervisor";
               shares = [{
                 tag = "workspace";
                 source = "/path/to/repo";
                 mountPoint = "/workspace";
               }];
               volumes = [{
                 image = "overlay.img";
                 mountPoint = "/";
                 size = 1024;  # MB
               }];
             };
             # Include only needed packages
             environment.systemPackages = with pkgs; [
               git nodejs python3 gcc
             ];
           }
         ];
       };
     };
   }

2. Build the VM: nix build .#nixosConfigurations.sandbox.config.microvm.runner
3. Run: result/bin/microvm-run
4. VM boots with minimal NixOS, /workspace mounted via virtiofs
5. /nix/store is shared read-only from host (if using virtiofs)
6. User works in /workspace...
7. On exit: changes in /workspace are on host via virtiofs (no extraction)
```

### Pros
- **Truly reproducible**: Nix guarantees identical environments across machines
- **Minimal images**: Only required packages included (no bloat)
- **Deduplication**: /nix/store is content-addressed; shared across all VMs
- **Multi-VMM**: microvm.nix supports Firecracker, Cloud Hypervisor, QEMU, kvmtool
- **Declarative**: Environment defined in version-controlled flake.nix
- **virtiofs support**: Via Cloud Hypervisor for direct file sharing
- **Cache sharing**: All VMs share the same /nix/store

### Cons
- **Steep Nix learning curve**: Nix language and ecosystem are complex
- **Build times**: First build can take minutes (subsequent builds cached)
- **Nix store size**: Can grow large if many different package versions are needed
- **Not Dockerfile-based**: Less familiar to most developers
- **NixOS-specific**: Guest must be NixOS (can't use arbitrary distros)
- **microvm.nix maturity**: Smaller community than Firecracker/E2B

### Performance

| Metric | Value |
|--------|-------|
| First build | 5-15 minutes (download + build NixOS rootfs) |
| Subsequent builds (cached) | 1-5 seconds |
| VM boot | ~200ms (Cloud Hypervisor) or ~125ms (Firecracker) |
| Disk per VM (rootfs) | 100-500MB (only needed packages) |
| Shared /nix/store | Deduplicated across all VMs |
| Memory per VM | Balloon-managed (Cloud Hypervisor) |
| Parallel VMs on 32GB host | 15-50+ |

### Complexity to Implement
**Medium-High.** Nix expertise required. But microvm.nix handles most of the VM plumbing. The main work is defining the flake and building the orchestrator.

### Best For
- Teams already using or willing to adopt **Nix**
- Environments that need **strict reproducibility**
- Projects with **many different language runtimes** (Nix handles polyglot well)
- Long-term investment in **declarative infrastructure**

---

## Architecture 5: Firecracker + Dockerfile-to-Snapshot Pipeline + vsock Agent

**Inspiration:** E2B infrastructure, Kata Containers agent pattern

### Components

| Layer | Technology |
|-------|-----------|
| VMM | Firecracker |
| Environment definition | Dockerfile (familiar to all developers) |
| Image pipeline | Dockerfile → Docker build → extract rootfs → ext4 image → Firecracker snapshot |
| Per-VM storage | dm-snapshot (simpler than dm-thin) |
| In-VM agent | Custom agent communicating over virtio-vsock |
| Git integration | Agent handles git operations; communicates diffs over vsock |
| Control plane | Host-side orchestrator managing VM lifecycle |

### How It Works End-to-End

```
1. User creates a Dockerfile for their sandbox environment:
   FROM ubuntu:22.04
   RUN apt-get update && apt-get install -y git nodejs npm python3
   COPY package.json package-lock.json /workspace/
   RUN cd /workspace && npm ci
   # Agent binary is included in base
   COPY sandbox-agent /usr/local/bin/

2. Build pipeline:
   a. docker build -t sandbox-env .
   b. Extract rootfs: docker export $(docker create sandbox-env) | tar -C rootfs -xf -
   c. Create ext4 image from rootfs
   d. Boot Firecracker from this image
   e. Take snapshot (golden image with deps installed, agent running)

3. User runs: sandbox spawn /path/to/repo
4. Manager:
   a. Create dm-snapshot from golden image (instant CoW)
   b. Start Firecracker, restore from snapshot
   c. Agent inside VM wakes up, connects to host via vsock
   d. Host streams repo tarball to agent via vsock
   e. Agent extracts repo to /workspace
   f. Agent spawns bash shell, connects to user's terminal via vsock
5. User works in the VM...
6. On exit (user exits bash):
   a. Agent detects shell exit
   b. Agent runs: git add -A && git diff --binary HEAD
   c. Agent sends diff to host over vsock
   d. Host applies diff to a staging copy
   e. Host creates git commit on unique branch
7. Clean up: kill Firecracker, delete dm-snapshot
```

### Pros
- **Dockerfile-based**: Most developers already know Docker
- **Robust communication**: vsock is reliable, no network setup needed
- **Agent pattern**: In-VM agent can handle complex operations (git, file sync, health checks)
- **Simpler storage**: dm-snapshot is easier than dm-thin (fewer moving parts)
- **Snapshot restore**: ~200ms to a fully warm environment
- **Extensible**: Agent can be extended with new capabilities over time

### Cons
- **Custom agent required**: Must build and maintain an in-VM agent binary
- **vsock complexity**: Need a protocol for host-agent communication
- **Docker as build dependency**: Requires Docker for image building (though only at build time)
- **dm-snapshot scaling**: Write amplification with many concurrent snapshots of same origin
- **Repo streaming overhead**: Tar over vsock adds latency at VM start (~1-5s for large repos)
- **No live sync**: Same as Architecture 1

### Performance

| Metric | Value |
|--------|-------|
| Image build (cold) | 2-10 minutes (Docker build + snapshot) |
| VM start (warm) | ~200-300ms (snapshot restore) |
| Repo injection | 1-5 seconds (tar stream over vsock) |
| Total time to bash | ~2-6 seconds |
| Memory per VM | Guest memory + 5MB overhead |
| Disk per VM | dm-snapshot overhead (only written blocks) |
| Change extraction | <1 second (diff over vsock) |
| Parallel VMs on 32GB host | 10-30 |

### Complexity to Implement
**High.** Requires: Docker-to-rootfs pipeline, dm-snapshot management, Firecracker snapshot lifecycle, vsock agent protocol, agent binary. But it's the most complete and extensible solution.

### Best For
- Teams wanting **Docker-familiar** environment definitions
- Scenarios needing **rich in-VM control** (not just bash access)
- Building a **product** (agent is extensible for future features)
- Cases where a **control protocol** between host and VM is valuable

---

## Comparison Matrix

| | Arch 1: FC + Overlay | Arch 2: CH + virtiofs | Arch 3: FC + dm-thin + snap | Arch 4: CH + Nix | Arch 5: FC + Docker + vsock |
|---|---|---|---|---|---|
| **VMM** | Firecracker | Cloud Hypervisor | Firecracker | Cloud Hypervisor | Firecracker |
| **Boot time** | 1-3s (boot) / 200ms (snap) | 200ms-1s | ~200ms (snap) | 200ms-5s | 200ms + 1-5s repo inject |
| **Live file sync** | No | Yes (virtiofs) | No | Yes (virtiofs) | No (agent-mediated) |
| **Memory efficiency** | Fixed allocation | Balloon reclaim | Lazy mmap loading | Balloon reclaim | Fixed allocation |
| **Disk efficiency** | Good (squashfs + overlay) | Moderate (reflink) | Excellent (block CoW) | Good (Nix dedup) | Good (dm-snapshot) |
| **Diff extraction** | Easy (read upper dir) | Instant (on host) | Medium (block diff) | Instant (on host) | Easy (agent sends diff) |
| **Max parallel VMs** | 10-30 | 15-50+ | 50+ | 15-50+ | 10-30 |
| **Setup complexity** | Medium | Medium-Low | High | Medium-High | High |
| **Environment def** | Custom scripts | Any + virtiofs | Custom/Docker | Nix flake | Dockerfile |
| **Isolation** | Strong (KVM) | Strong (KVM) | Strong (KVM) | Strong (KVM) | Strong (KVM) |
| **Battle-tested** | Yes (E2B) | Moderate (Kata) | Yes (containerd) | Low (microvm.nix) | Partial (E2B-like) |

---

## Recommendations

### For Quickest MVP
**Architecture 2: Cloud Hypervisor + virtiofs** — Least infrastructure to build. No image pipelines, no extraction step. Just create a CoW copy of the repo directory, mount via virtiofs, boot a VM. Changes are immediately on host when VM exits.

### For Maximum Scale
**Architecture 3: Firecracker + dm-thin + snapshot/restore** — Sub-millisecond VM "cloning" via thin snapshots, lazy memory loading from snapshot restore, block-level CoW. This is the CodeSandbox/E2B-level approach.

### For Best Developer Experience
**Architecture 5: Firecracker + Dockerfile + vsock agent** — Dockerfile is the most familiar environment definition. The agent pattern allows rich interactions. This is what a product would look like.

### For Reproducibility Purists
**Architecture 4: Cloud Hypervisor + microvm.nix** — Nix guarantees byte-identical environments. Best for polyglot projects with complex dependency trees.

### For Simplest Implementation
**Architecture 1: Firecracker + OverlayFS** — Fewest moving parts. Well-documented E2B patterns. Good starting point that can evolve into Architecture 3 or 5.

---

## Suggested Development Roadmap

### Phase 1: Proof of Concept (Architecture 1)
1. Build a minimal Alpine-based rootfs with git + common tools
2. Write a script that creates a repo ext4 image from current working directory
3. Boot Firecracker with rootfs + repo image + overlay
4. Connect to serial console (bash shell)
5. On exit, mount overlay and extract changes

### Phase 2: Performance Optimization (Architecture 1 → 3)
1. Add dm-thin for block-level CoW
2. Implement Firecracker snapshot/restore for fast boot
3. Add dependency caching (squashfs layers keyed by lock file hash)
4. Parallelize VM operations

### Phase 3: Rich UX (→ Architecture 5)
1. Build vsock-based agent for host↔VM communication
2. Implement Dockerfile-to-snapshot pipeline
3. Add automated change extraction and git commit pipeline
4. Handle merge conflicts for parallel VM results

### Phase 4: Advanced Features
1. Memory ballooning (consider Cloud Hypervisor upgrade)
2. Live sync (virtiofs via Cloud Hypervisor)
3. Pre-built environment marketplace
4. CI/CD integration

---

## References

See the detailed individual research reports:
- [Firecracker Research](firecracker_research.md)
- [Filesystem CoW Research](filesystem_cow_research.md)
- [Existing Tools Research](existing_tools_research.md)
- [Git Integration Research](git_integration_research.md)
