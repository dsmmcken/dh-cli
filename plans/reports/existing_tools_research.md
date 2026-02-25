# Existing Tools & Approaches Research Report

**Date:** 2026-02-11
**Purpose:** Deep research into existing tools, projects, and approaches for sandboxed development environments, with focus on applicability to a local Firecracker-based VM sandbox system for AI agents.

**Target Use Case:** Developer runs a command, a Firecracker VM spawns with a copy of their repo (dependencies pre-installed), they get a bash shell, work in isolation, and commit changes back. Many VMs run in parallel.

---

## Table of Contents

1. [VM-Based Container Runtimes](#1-vm-based-container-runtimes)
   - [Kata Containers](#kata-containers)
   - [gVisor](#gvisor)
   - [Cloud Hypervisor](#cloud-hypervisor)
2. [AI Agent Sandboxing Approaches](#2-ai-agent-sandboxing-approaches)
   - [E2B](#e2b)
   - [Daytona](#daytona)
   - [Modal](#modal)
   - [OpenHands (formerly OpenDevin)](#openhands)
   - [Devin](#devin)
   - [Claude Code Sandbox](#claude-code-sandbox)
   - [Cursor Background Agents](#cursor-background-agents)
   - [Codex CLI Sandbox](#codex-cli-sandbox)
3. [Dev Environment Tools](#3-dev-environment-tools)
   - [Hocus.dev](#hocus)
   - [Devcontainers Spec](#devcontainers)
   - [Gitpod / Ona](#gitpod)
   - [GitHub Codespaces](#github-codespaces)
4. [Lightweight Sandboxing (Non-VM)](#4-lightweight-sandboxing-non-vm)
   - [Bubblewrap (bwrap)](#bubblewrap)
   - [Firejail](#firejail)
   - [systemd-nspawn](#systemd-nspawn)
   - [LXC/LXD](#lxclxd)
   - [Security Comparison](#security-comparison-non-vm-vs-vm)
5. [Nix/NixOS](#5-nixnixos)
6. [Orchestration & Management](#6-orchestration--management)
   - [Weaveworks Ignite](#weaveworks-ignite)
   - [Flintlock](#flintlock)
   - [containerd + Firecracker Snapshotter](#containerd--firecracker-snapshotter)
   - [Podman Machine](#podman-machine)
7. [Comparative Analysis](#7-comparative-analysis)
8. [Recommendations for Our Use Case](#8-recommendations-for-our-use-case)

---

## 1. VM-Based Container Runtimes

### Kata Containers

**How it works:**
Kata Containers is an open-source project that provides lightweight VMs that feel and perform like containers but provide stronger workload isolation using hardware virtualization (KVM). The architecture consists of:

- **kata-runtime**: Deploys containers using a hypervisor of choice (QEMU, Cloud Hypervisor, or Firecracker), which creates a VM to host the kata-agent.
- **kata-agent**: Runs inside each VM as the supervisor, managing container processes on behalf of the runtime on the host.
- **kata-shim**: Bridges the container runtime interface with the VM-based execution.

When using Firecracker as the VMM, the container rootfs is a device mapper snapshot, hot-plugged as a virtio block device. The kata-agent inside the VM finds the mount point and uses libcontainerd to create and spawn the container. This requires containerd's devmapper snapshotter.

**What problem it solves:** Provides VM-level isolation for container workloads while maintaining the container UX and Kubernetes integration.

**Applicability to our use case:**
- Kata is designed for Kubernetes orchestration of containers -- overkill for a local dev sandbox tool.
- However, the *pattern* of using a runtime + in-VM agent is directly applicable.
- The Firecracker integration demonstrates how to use devmapper for rootfs provisioning.
- Cloud Hypervisor is now the default VMM (v3.3.0+), offering virtiofs support for filesystem sharing.

**Pros:**
- Mature, production-proven architecture
- Multi-VMM support (can swap Firecracker, Cloud Hypervisor, QEMU)
- Well-documented agent protocol
- Active community (CNCF project)

**Cons:**
- Heavy dependency on Kubernetes ecosystem
- Complex setup for non-K8s use cases
- Firecracker mode lacks virtiofs (must copy files)
- Not designed for long-lived interactive development sessions

**Maintenance status:** Active. CNCF project. Version 3.3.0+ recommended. Cloud Hypervisor is now the default VMM.

---

### gVisor

**How it works:**
gVisor provides a userspace kernel (the "Sentry") that intercepts system calls via ptrace or KVM platforms. Instead of passing syscalls to the host kernel, the Sentry provides its own Go-based reimplementation of the Linux syscall interface, memory management, filesystems, network stack, etc. The "Gofer" is a separate host process that manages filesystem access, communicating with the Sentry via 9P protocol.

**What problem it solves:** Reduces kernel attack surface by never letting applications directly interact with the host kernel, without requiring hardware virtualization.

**Applicability to our use case:**
- gVisor provides weaker isolation than Firecracker VMs (software sandbox vs hardware boundary).
- Faster startup (milliseconds, no VM boot) but 10-30% overhead on I/O-heavy workloads.
- Modal uses gVisor for their sandbox platform, proving it works at scale for AI agents.
- Not ideal if we want VM-level isolation as a hard requirement.

**Pros:**
- Millisecond startup (no VM boot process)
- Easy integration with existing container workflows
- Strong syscall filtering (reimplemented Linux interface)
- Used in production at Google (GKE Sandbox)
- Lower overhead than full VM for CPU-bound work

**Cons:**
- Not VM-level isolation (software boundary, not hardware)
- 10-30% I/O performance overhead due to syscall interception
- Incomplete syscall coverage (some Linux features unsupported)
- Still shares some host resources
- More complex to reason about security boundary

**Maintenance status:** Active. Maintained by Google. Used in GKE Sandbox. Regular releases.

---

### Cloud Hypervisor

**How it works:**
Cloud Hypervisor is a Rust-based VMM built on the rust-vmm project (same foundation as Firecracker). It targets the middle ground between Firecracker's minimalism and QEMU's feature-completeness. Key features over Firecracker:

- **virtiofs support**: Shared filesystem between host and guest (critical for our use case)
- **CPU/memory hotplug**: Can add/remove resources to running VMs
- **PCI hotplug**: Dynamic device attachment
- **VFIO passthrough**: Direct hardware access (GPU support)
- **Memory ballooning**: Dynamic RAM management (return unused memory to host)
- **vhost-user**: Offload device emulation to separate processes

Codebase is ~50k lines of Rust (vs Firecracker's similar size, vs QEMU's ~2M lines of C).

**What problem it solves:** Provides a secure, minimal VMM with enough features for long-running workloads (unlike Firecracker which is optimized for ephemeral serverless functions).

**Applicability to our use case:** **Highly relevant.** Cloud Hypervisor addresses the key limitations of Firecracker for dev environments:
- virtiofs enables sharing the repo directory between host and guest without copying
- Memory ballooning means idle VMs don't waste host RAM
- CPU/memory hotplug allows dynamic resource adjustment
- Still fast boot (~200ms vs Firecracker's ~125ms)

**Pros:**
- virtiofs for filesystem sharing (huge advantage over Firecracker)
- Memory ballooning (return unused RAM to host)
- Rust codebase, same security posture as Firecracker
- Default VMM for Kata Containers
- Active development, good community
- Better suited for long-running workloads

**Cons:**
- Slightly slower boot than Firecracker (~200ms vs ~125ms)
- Larger attack surface than Firecracker (more features = more code)
- Less battle-tested at scale than Firecracker (not powering Lambda)
- Documentation not as extensive as Firecracker

**Maintenance status:** Active. Default VMM for Kata Containers. Regular releases. Strong community.

---

## 2. AI Agent Sandboxing Approaches

### E2B

**How it works:**
E2B is an open-source cloud platform for running AI-generated code in Firecracker microVMs. The workflow:

1. Define environment via a standard `Dockerfile`
2. `e2b template build` converts the Dockerfile into a microVM snapshot
3. When a sandbox is requested, E2B restores the snapshot (not rebuild from scratch)
4. Each sandbox gets its own Firecracker microVM with hardware-level isolation
5. SDKs (JS/Python) provide programmatic control over sandboxes
6. Kubernetes + Terraform handle dynamic scaling

**Key specs:**
- Boot time: <200ms (snapshot restore)
- Memory overhead: <5 MiB per microVM
- Max session duration: 24 hours
- Multi-language support (anything that runs on Linux)

**Infrastructure repo:** [github.com/e2b-dev/infra](https://github.com/e2b-dev/infra) - open-source infrastructure powering E2B Cloud.

**Applicability to our use case:** **Most directly relevant.** E2B's architecture is essentially what we want to build locally:
- Dockerfile-to-snapshot pipeline is exactly our dependency pre-installation story
- Firecracker-based isolation is our target
- The snapshot/restore pattern for fast startup is the right approach
- We could study their infrastructure code for implementation patterns

**Pros:**
- Open source (Apache-2.0), can study and adapt
- Proven Firecracker + snapshot architecture
- Dockerfile-based environment definition (familiar DX)
- Well-documented SDK patterns
- Self-hosting possible
- Active community, used by Manus AI and others

**Cons:**
- Cloud-first design (self-hosting requires significant effort)
- No virtiofs (Firecracker limitation) -- uses snapshot approach instead
- 24-hour session limit in cloud version
- Scaling infrastructure is complex (K8s + Terraform)
- Not designed for local-only use

**Maintenance status:** Very active. Well-funded startup. Apache-2.0 license. Regular releases.

---

### Daytona

**How it works:**
Daytona started as a dev environment manager (self-hosted Codespaces alternative) and pivoted in February 2025 to become infrastructure for running AI-generated code. Architecture:

- Control plane manages sandbox lifecycle
- Default isolation: Docker containers (Kata Containers and Sysbox optional for enhanced isolation)
- Sandboxes run on customer-managed compute (cloud or on-prem)
- 90ms environment creation via pre-built snapshots
- Comprehensive API for process execution, file operations, Git integration
- Supports Dev Containers standard for environment configuration

**Applicability to our use case:**
- Daytona's fastest-in-class cold starts (27-90ms) are impressive
- However, default Docker isolation is weaker than VM-based
- The Dev Containers standard support is interesting for environment definition
- Their API patterns for agent interaction are worth studying

**Pros:**
- Extremely fast cold starts (27-90ms)
- Dev Containers standard support
- On-prem deployment option
- Good API for programmatic control
- Free tier with $200 compute credits

**Cons:**
- Docker containers by default (weaker isolation than VMs)
- Youngest platform (pivoted 2025), still maturing
- Less battle-tested than E2B
- VM-level isolation requires extra configuration (Kata/Sysbox)

**Maintenance status:** Active. Pivoted to AI code execution. Growing community.

---

### Modal

**How it works:**
Modal built a custom container runtime from scratch with gVisor for isolation. Key architecture:

- Custom container runtime (not Docker)
- gVisor-based isolation (stronger than standard runc, weaker than VM)
- Custom filesystem layer for fast image unpacking
- Snapshot/volume primitives for state management
- Autoscale from zero to 10,000+ concurrent sandboxes
- Sub-second cold starts
- Python-first SDK

**Applicability to our use case:**
- Modal demonstrates that gVisor can work well at scale for AI agents
- Their snapshot/volume approach is relevant for state management
- Custom runtime shows the value of building purpose-built systems
- However, cloud-only (no self-hosting) limits direct adoption

**Pros:**
- Proven at massive scale (10,000+ concurrent)
- Sub-second cold starts
- Excellent Python SDK
- Memory snapshot for state preservation
- Strong autoscaling

**Cons:**
- Cloud-only (no self-hosting, no BYOC)
- gVisor isolation (not VM-level)
- Python-centric
- Memory snapshots still in early development
- Closed source runtime

**Maintenance status:** Very active. Well-funded ($80M raise). Regular updates.

---

### OpenHands

**How it works:**
OpenHands (formerly OpenDevin) uses Docker containers for sandbox isolation:

1. Each task session gets a new Docker container
2. Agent connects via REST API server running inside the container
3. Only project-specific files are exposed via workspace mounting
4. Agents execute bash commands, Python code via IPython
5. Containers support arbitrary base Docker images
6. Containers are torn down post-session

**Applicability to our use case:**
- Simpler approach: just Docker containers with mounted workspaces
- Good reference for the agent-sandbox interaction protocol
- REST API inside container pattern is worth studying
- Docker isolation is weaker than VM for untrusted code

**Pros:**
- Simple, well-understood architecture
- Flexible Docker image support
- Clean agent-sandbox API
- Open source, active community
- Easy to set up locally

**Cons:**
- Docker-only isolation (shared kernel)
- No VM-level security boundary
- Less suitable for parallel untrusted workloads
- Container startup slower than snapshot restore

**Maintenance status:** Very active. Published at ICLR 2025. Growing community.

---

### Devin

**How it works:**
Devin (by Cognition Labs) runs each session in its own isolated VM with a complete development environment: shell, code editor, and browser. Devin 2.0 (April 2025) enhanced isolation with per-session VMs and parallelization support.

**Applicability to our use case:**
- Validates the VM-per-session model for AI agent work
- Demonstrates that full IDE environments can run in isolated VMs
- Limited public technical details about implementation

**Pros:**
- Proves VM-per-session model works
- Full development environment per session
- Production-proven with paying customers

**Cons:**
- Closed source, limited technical details
- Cloud-only
- Cannot study implementation

**Maintenance status:** Active. Commercial product by Cognition Labs.

---

### Claude Code Sandbox

**How it works:**
Anthropic built a lightweight OS-level sandboxing tool called `sandbox-runtime` (open-sourced as `@anthropic-ai/sandbox-runtime`). It enforces filesystem and network restrictions without containers:

**Linux:** Uses bubblewrap (bwrap) for namespace isolation + seccomp BPF for syscall filtering. Creates empty mount namespace on tmpfs, bind-mounts only allowed directories. Network isolation via removing network namespace and routing through a proxy.

**macOS:** Uses Apple's `sandbox-exec` with dynamically generated Seatbelt profiles. Monitors sandbox violations in real-time.

**Key innovation:** Network isolation through a Unix domain socket proxy that enforces domain allowlists.

**Impact:** Reduces permission prompts by 84% in internal usage.

**Applicability to our use case:**
- Demonstrates that lightweight process-level sandboxing is effective for many use cases
- The proxy-based network isolation pattern is clever and reusable
- However, this is *process-level* sandboxing, not VM-level isolation
- Good for sandboxing a single agent on a developer's machine, but not for multi-tenant parallel VMs

**Pros:**
- Open source (research preview)
- No container/VM overhead
- Works on both Linux and macOS
- Proven in production (Claude Code)
- Simple filesystem + network isolation model

**Cons:**
- Process-level only (weaker than VM isolation)
- Not designed for multi-tenant scenarios
- Shares host kernel and resources
- Cannot provide different OS environments

**Maintenance status:** Active. Open-source research preview by Anthropic.

---

### Cursor Background Agents

**How it works:**
Cursor 2.0 runs background agents in isolated environments, described as "AI pair programmers in isolated Ubuntu VMs with internet access." Key architecture:

- Up to 8 agents in parallel
- Each agent gets its own isolated environment (git worktrees or remote worker sandboxes)
- Sandbox Mode restricts terminal commands: no network by default, filesystem limited to workspace + /tmp
- Background agents can work on separate branches and open PRs
- Uses bubblewrap on Linux, similar to Claude Code's approach for local mode
- Remote background agents run in full VM environments

**Applicability to our use case:**
- Validates the parallel-agents-in-isolated-environments model
- Git worktree approach for branch isolation is relevant
- Sandbox-by-default with allowlisting is a good security model

**Pros:**
- Proven parallel agent model
- Git worktree integration
- Both local (bwrap) and remote (VM) modes
- Good UX for managing multiple agents

**Cons:**
- Closed source
- Specific to Cursor IDE
- Limited technical details on VM provisioning

**Maintenance status:** Active. Part of Cursor 2.0 product.

---

### Codex CLI Sandbox

**How it works:**
OpenAI's Codex CLI implements multi-layered OS-level sandboxing:

- **macOS:** Seatbelt policies via `sandbox_init()`, compiled at runtime, kernel-enforced
- **Linux:** Landlock (default) + seccomp, with optional bubblewrap pipeline for enhanced filesystem isolation
- Default: no network access, write permissions limited to workspace
- Vendored bubblewrap for Linux (recent addition)

**Applicability to our use case:**
- Similar approach to Claude Code sandbox (OS-level primitives)
- Demonstrates Landlock as an alternative to bubblewrap on Linux
- Process-level sandboxing, not VM-level

**Pros:**
- Multiple isolation layers (Landlock + seccomp + optional bwrap)
- Open source
- Lightweight, no container overhead

**Cons:**
- Process-level only
- Not suitable for multi-tenant/multi-VM scenarios
- Linux-specific nuances with Landlock kernel version requirements

**Maintenance status:** Active. Open source by OpenAI.

---

## 3. Dev Environment Tools

### Hocus

**How it works:**
Hocus was a self-hosted alternative to Gitpod and GitHub Codespaces that spun up disposable dev environments on your own servers. Architecture evolution:

1. **Initially used Firecracker** for VM isolation
2. **Replaced Firecracker with QEMU** after weeks of testing due to:
   - **No dynamic RAM management**: Once Firecracker allocates RAM, it never returns it to host. An idling VM that once used 32GB still consumes 32GB.
   - **No GPU support**: Required for some dev workflows
   - **Disk I/O bottleneck**: virtio-blk implementation has limited throughput with multiple drives
   - **No virtiofs**: Cannot share filesystem between host and guest

These limitations exist because Firecracker is designed for short-lived Lambda functions, not long-running dev environments.

**Applicability to our use case:** **Critical lessons learned:**
- Firecracker's RAM management is a real problem for running many parallel long-lived VMs
- virtiofs absence means file copying overhead
- QEMU configuration took 2 months to get right as an alternative
- Cloud Hypervisor offers a middle ground (virtiofs + memory ballooning without QEMU complexity)

**Maintenance status:** **Defunct.** Repository archived September 2024. MIT license. Startup dissolved.

---

### Devcontainers

**How it works:**
The Development Container Specification provides a standard way to define containerized development environments:

- Central config file: `devcontainer.json` (JSON with comments)
- Runs on Docker or Podman
- **Features**: Self-contained, shareable installation units (e.g., "install Python 3.11")
- **Templates**: Reusable environment starting points
- Supported by VS Code, GitHub Codespaces, Jetbrains, Devcontainer CLI

**Applicability to our use case:**
- Could serve as the environment definition format (instead of or alongside Dockerfiles)
- Well-established standard with broad tooling support
- Features system could modularly compose VM environments
- Already understood by developers

**Pros:**
- Industry standard for dev environment definition
- Rich feature ecosystem
- Editor/IDE integration
- Declarative and shareable

**Cons:**
- Designed for containers, not VMs (would need adaptation)
- Docker/Podman dependency
- Some features assume container runtime semantics

**Maintenance status:** Active. Maintained by Microsoft/community. Broad industry adoption.

---

### Gitpod

**How it works:**
Gitpod (rebranded to Ona in September 2025) provides ephemeral, Docker-based development environments. Architecture evolution:

- **Classic:** Container-based with Linux namespace isolation (multi-tenant) or VM-level isolation (single-tenant/enterprise)
- **Flex/Ona (2025+):** VM-level isolation for all tiers, OS-level isolation for AI workloads
- Pods operate through secondary network interfaces for network isolation
- Fully sandboxed environments for high-autonomy AI agents

**Applicability to our use case:**
- Validates the shift from container to VM isolation for dev environments
- Their pivot to AI agent sandboxing mirrors our use case
- Network isolation via secondary interfaces is a useful pattern

**Maintenance status:** Active. Rebranded to Ona. Funded startup.

---

### GitHub Codespaces

**How it works:**
Each codespace runs as a Docker container inside a dedicated VM:

- Each codespace gets its own newly-built VM (never co-located)
- Docker devcontainer runs inside the VM
- Isolated virtual network with firewalls blocking inter-codespace communication
- TLS-encrypted tunnel for connection
- Resources: 2-core/8GB to 32-core/128GB
- Uses devcontainer.json for environment configuration

**Applicability to our use case:**
- VM-per-workspace model is exactly our target
- devcontainer.json integration shows how to define environments
- The "never co-located" security model is the right approach
- GitHub's scale proves this model works

**Pros:**
- Proven at massive scale
- Strong isolation (VM per workspace)
- devcontainer.json integration
- Automatic token management

**Cons:**
- Cloud-only, proprietary
- Cannot study implementation
- Tied to GitHub ecosystem

**Maintenance status:** Active. Core GitHub product.

---

## 4. Lightweight Sandboxing (Non-VM)

### Bubblewrap

**How it works:**
Bubblewrap (bwrap) creates isolated environments using Linux namespaces without requiring root access:

- Creates new mount namespace on temporary filesystem
- User specifies exactly which parts of filesystem are visible
- Supports: cgroup, IPC, mount, network, PID, user, UTS namespaces
- Uses `PR_SET_NO_NEW_PRIVS` to disable setuid binaries
- Drops all capabilities within sandbox
- Seccomp filtering support
- Bind-mounts specific directories (read-only or read-write)

**Applicability to our use case:**
- Too lightweight for our multi-VM use case
- But useful as an inner sandbox layer (sandbox within VM)
- Used by Claude Code and Codex CLI for process-level sandboxing
- Good for the "local development" mode where VMs aren't needed

**Pros:**
- No root required (unprivileged)
- Minimal overhead
- Fine-grained namespace control
- Used by Flatpak, Claude Code, Codex
- Well-maintained

**Cons:**
- Process-level only (shared kernel)
- No hardware isolation boundary
- Requires careful configuration
- Not a complete sandbox policy by itself

**Maintenance status:** Active. Used by Flatpak and other major projects.

---

### Firejail

**How it works:**
Firejail is a SUID program that uses Linux namespaces, seccomp-bpf, and cgroups to restrict application environments. Comes with pre-built profiles for common applications (Firefox, VLC, etc.).

**Applicability to our use case:** Limited. Designed for desktop application sandboxing, not VM/container orchestration. More useful as a convenience tool than a building block.

**Pros:**
- Easy to use (pre-built profiles)
- Good for desktop app isolation
- Combines namespaces + seccomp + cgroups

**Cons:**
- SUID binary (potential security concerns)
- Desktop-focused, not server/dev workflows
- Not designed for programmatic use

**Maintenance status:** Active. Community-maintained.

---

### systemd-nspawn

**How it works:**
systemd-nspawn is a lightweight container tool that spawns a new namespace for debugging, testing, and building. It's often described as "chroot on steroids."

**Applicability to our use case:** Limited. Simple but lacks resource isolation (no cgroup management), no network isolation by default, and not designed for multi-tenant scenarios.

**Maintenance status:** Active. Part of systemd. Stable but limited features.

---

### LXC/LXD

**How it works:**
LXC provides system containers (full Linux OS inside a container) using namespaces, cgroups, and chroot. LXD adds a REST API daemon layer for management.

- System containers run full init systems (systemd)
- Unprivileged containers map root to limited host user
- LXD provides clustering, live migration, snapshot/restore
- AppArmor/SELinux integration for enhanced security

**Applicability to our use case:**
- System containers are closer to our needs than application containers
- Full OS environment inside container (like a lightweight VM)
- Snapshot/restore capability useful for fast provisioning
- But still shares host kernel (weaker isolation than VM)

**Pros:**
- Full OS environment (systemd, SSH, multiple services)
- Snapshot/restore for fast provisioning
- Lower overhead than VMs
- LXD provides good management API
- Unprivileged containers for better security

**Cons:**
- Shared kernel (not VM-level isolation)
- Security depends heavily on configuration
- Less isolation than Firecracker/Cloud Hypervisor
- Kernel vulnerability = all containers exposed

**Maintenance status:** Active. LXD is now under Canonical management (moved from Linux Containers project).

---

### Security Comparison: Non-VM vs VM

| Approach | Isolation Level | Kernel Sharing | Startup | Overhead |
|----------|----------------|---------------|---------|----------|
| Bubblewrap | Process-level (namespaces) | Shared | Instant | Negligible |
| Firejail | Process-level (namespaces + seccomp) | Shared | Instant | Negligible |
| systemd-nspawn | Container-level (namespaces) | Shared | Fast | Low |
| LXC/LXD | System container (namespaces + cgroups) | Shared | Fast | Low |
| gVisor | Userspace kernel (syscall interception) | Filtered | Instant | 10-30% I/O |
| Firecracker | Hardware VM (KVM) | Separate | ~125ms | <5 MiB |
| Cloud Hypervisor | Hardware VM (KVM) | Separate | ~200ms | Low |

**For our use case:** VM-level isolation (Firecracker or Cloud Hypervisor) is recommended because multiple parallel VMs running potentially untrusted AI-generated code need a hard security boundary.

---

## 5. Nix/NixOS

### How it Applies

**Reproducible environments with nix-shell/devShell:**
- `shell.nix` or `flake.nix` defines exact dependencies
- Pin nixpkgs to a specific commit for true reproducibility
- `devenv` adds ergonomic abstractions (services, language helpers, pre-commit hooks)
- Could define VM environments declaratively

**NixOS as minimal VM guest OS via microvm.nix:**
- [microvm.nix](https://github.com/microvm-nix/microvm.nix) is a Nix Flake for building NixOS MicroVMs
- Supports Firecracker, Cloud Hypervisor, QEMU, and others
- Builds read-only root filesystem (squashfs or erofs) containing only required /nix/store paths
- Optional writable overlay on top
- Can mount host's /nix/store into VM for shared dependency cache

**Key capability:** microvm.nix can isolate your /nix/store into exactly what is required for the guest's NixOS. This means each VM gets a minimal, purpose-built rootfs.

**Applicability to our use case:**
- microvm.nix is directly applicable for building minimal VM images with pre-installed dependencies
- Nix store sharing across VMs could massively reduce disk usage
- Declarative VM definitions enable reproducible environments
- Read-only rootfs + writable overlay is the right filesystem model

**Pros:**
- Truly reproducible environments
- Minimal VM images (only required packages)
- Nix store deduplication across VMs
- Declarative, version-controlled definitions
- Active microvm.nix project
- Multi-VMM support (Firecracker, Cloud Hypervisor, etc.)

**Cons:**
- Steep learning curve for Nix
- Nix ecosystem can be complex
- Build times for custom images
- Less familiar to most developers than Dockerfiles

**Maintenance status:** Active. Nix ecosystem is growing rapidly. microvm.nix is actively maintained.

---

## 6. Orchestration & Management

### Weaveworks Ignite

**How it works:**
Ignite was a Firecracker VM manager with Docker/OCI container UX:
- Run OCI images as Firecracker microVMs
- ~125ms boot with default Linux kernel
- GitOps-first management (ignited gitops)
- Docker-like CLI (ignite run, ignite exec, etc.)

**Applicability to our use case:**
- The "OCI image as VM" concept is exactly our model
- Docker-like UX for VM management is the right abstraction level
- GitOps support aligns with our git-based workflow
- **However: project is archived and unmaintained**

**Maintenance status:** **Archived.** Repository archived December 2023. Weaveworks shut down February 2024. Read-only. Last release: v0.10.0.

---

### Flintlock

**How it works:**
Flintlock manages the lifecycle of microVMs on bare-metal hosts:
- Supports both Firecracker and Cloud Hypervisor
- Uses containerd's devmapper snapshotter for filesystem provisioning
- OCI images for VM volumes, kernel, and initrd
- Cloud-init/ignition for VM metadata configuration
- gRPC API for VM lifecycle management

Originally built for creating microVMs as Kubernetes nodes, but applicable to other lightweight virtualization needs.

**Applicability to our use case:** **Relevant as a building block:**
- Provides the VM lifecycle management layer we need
- OCI-to-VM volume pipeline is exactly our workflow
- Supports both Firecracker and Cloud Hypervisor (flexibility)
- Cloud-init for VM configuration is standard
- gRPC API is programmable

**Pros:**
- Supports Firecracker + Cloud Hypervisor
- OCI image-based VM provisioning
- containerd integration for image management
- gRPC API for programmatic control
- Being revived as community project

**Cons:**
- Originally designed for K8s node creation
- Weaveworks origin (company defunct)
- Community revival still early
- May need adaptation for dev sandbox use case

**Maintenance status:** **Being revived.** Not archived. Community effort to restart after Weaveworks shutdown. Active as of 2025.

---

### containerd + Firecracker Snapshotter

**How it works:**
The firecracker-containerd project enables containerd to manage containers as Firecracker microVMs:
- Devmapper snapshotter creates filesystem images as block devices
- Block devices are hot-plugged to Firecracker as virtio block devices
- Thin provisioning via device-mapper for efficient storage
- BoltDB for devmapper metadata

**Applicability to our use case:** **Core infrastructure component.**
- The devmapper snapshotter is the standard way to provision Firecracker rootfs from OCI images
- Thin provisioning enables efficient storage for many parallel VMs
- containerd provides image pulling and layer management
- This is the "plumbing" layer that E2B, Kata, and others build on

**Maintenance status:** Active. Part of firecracker-microvm GitHub organization.

---

### Podman Machine

**How it works:**
Podman Machine creates Linux VMs for running containers on macOS/Windows:
- QEMU-based VMs running Fedora CoreOS
- SSH-based communication between host and VM
- gvproxy for port mapping
- Socket-activated services
- Rootless by design

**Applicability to our use case:** Limited. Podman Machine is designed to provide a Linux container runtime on non-Linux hosts, not for multi-VM sandbox orchestration. The SSH-based communication pattern is worth noting though.

**Maintenance status:** Active. Part of the Podman project (Red Hat).

---

## 7. Comparative Analysis

### Architecture Patterns Observed

| Pattern | Used By | Description |
|---------|---------|-------------|
| Dockerfile to VM Snapshot | E2B | Build from Dockerfile, snapshot the VM, restore for fast start |
| OCI Image to Block Device | Kata, Flintlock, Ignite | Use containerd + devmapper to turn images into VM rootfs |
| Nix to Minimal Rootfs | microvm.nix | Declaratively build minimal VM images with exact dependencies |
| Docker Container + Mounted Workspace | OpenHands, Daytona | Mount repo into container, run agent inside |
| OS-level Process Sandbox | Claude Code, Codex CLI | bwrap/seatbelt/Landlock for lightweight per-process isolation |
| In-VM Agent + Host Runtime | Kata, E2B | Agent process inside VM communicates with host orchestrator |
| Git Worktree for Branch Isolation | Cursor | Each agent works on separate branch via git worktree |

### VMM Comparison for Our Use Case

| Feature | Firecracker | Cloud Hypervisor | QEMU |
|---------|-------------|-----------------|------|
| Boot time | ~125ms | ~200ms | ~500ms+ |
| Memory overhead | <5 MiB | Low | Higher |
| virtiofs | No | Yes | Yes |
| Memory ballooning | No | Yes | Yes |
| GPU passthrough | No | Yes (VFIO) | Yes |
| Codebase | ~50k Rust | ~50k Rust | ~2M C |
| CPU/mem hotplug | No | Yes | Yes |
| Snapshot/restore | Yes | Limited | Yes |
| Best for | Ephemeral (<minutes) | Medium-lived (hours) | Long-lived (days+) |

### Isolation Strength Comparison

| Approach | Isolation | Multi-tenant Safe | Our Need |
|----------|-----------|-------------------|----------|
| bwrap/Landlock | Process namespaces | No (shared kernel) | Insufficient alone |
| Docker/LXC | Container namespaces + cgroups | Risky (shared kernel) | Insufficient for untrusted code |
| gVisor | Userspace kernel | Moderate (Modal uses it) | Acceptable if VM not required |
| Firecracker | Hardware VM (KVM) | Yes | Strong fit |
| Cloud Hypervisor | Hardware VM (KVM) | Yes | Strong fit |

---

## 8. Recommendations for Our Use Case

### Primary Recommendation: Cloud Hypervisor + OCI/Nix-based Images

**Why Cloud Hypervisor over Firecracker:**
1. **virtiofs**: Share repo directory between host and guest without copying. This is critical for the workflow where changes in the VM are immediately available on the host.
2. **Memory ballooning**: When running many parallel VMs, idle ones can return RAM to the host.
3. **Still fast**: ~200ms boot is fast enough for interactive use.
4. **Kata Containers default**: Proven in production at scale.

**Environment Definition Options (ranked):**
1. **Dockerfile + snapshot**: Most familiar to developers. Build once, snapshot, restore quickly. (E2B model)
2. **microvm.nix**: Most minimal and reproducible. Steeper learning curve but better long-term. (microvm.nix model)
3. **devcontainer.json**: Industry standard, but needs adaptation for VM use. (GitHub Codespaces model)

### Key Components to Build/Adopt

| Layer | Recommendation | Alternative |
|-------|---------------|-------------|
| VMM | Cloud Hypervisor | Firecracker (if virtiofs not needed) |
| VM Image Build | Dockerfile to rootfs (like E2B) | microvm.nix |
| Filesystem Sharing | virtiofs (Cloud Hypervisor) | Block device + devmapper (Firecracker) |
| VM Lifecycle | Custom (study Flintlock) | Direct Cloud Hypervisor API |
| In-VM Agent | Custom agent (study Kata agent) | SSH-based (simpler) |
| Environment Spec | Dockerfile | devcontainer.json, Nix flake |
| Dependency Cache | Shared /nix/store or overlay layers | OCI layer deduplication |
| Git Integration | virtiofs mount + git operations in VM | Git worktree per VM on host |
| Network Isolation | VM network namespace | Proxy-based (like Claude Code) |

### Architecture Worth Studying in Depth

1. **E2B infrastructure** ([github.com/e2b-dev/infra](https://github.com/e2b-dev/infra)) - Most directly relevant open-source implementation
2. **microvm.nix** ([github.com/microvm-nix/microvm.nix](https://github.com/microvm-nix/microvm.nix)) - Best approach for minimal, reproducible VM images
3. **Flintlock** ([github.com/liquidmetal-dev/flintlock](https://github.com/liquidmetal-dev/flintlock)) - VM lifecycle management with containerd integration
4. **Kata Containers agent** ([github.com/kata-containers/kata-containers](https://github.com/kata-containers/kata-containers)) - In-VM agent communication patterns
5. **Anthropic sandbox-runtime** ([github.com/anthropic-experimental/sandbox-runtime](https://github.com/anthropic-experimental/sandbox-runtime)) - Lightweight process sandboxing for complementary local mode
6. **Hocus blog posts** ([hocus.dev/blog](https://hocus.dev/blog)) - Lessons learned from Firecracker limitations

### Critical Lessons from Existing Tools

1. **Firecracker is not ideal for long-lived dev environments** (Hocus learned this the hard way). Cloud Hypervisor or QEMU are better for sessions lasting hours.
2. **virtiofs vs block device** is a fundamental architectural choice. virtiofs enables real-time file sharing; block devices require copy-in/copy-out.
3. **Snapshot/restore is key to fast startup**. Building from scratch each time is too slow. E2B, Modal, and Daytona all use some form of snapshotting.
4. **Memory management matters at scale**. Running 10+ parallel VMs without memory ballooning will exhaust host RAM quickly.
5. **The agent-in-VM pattern is proven**. Kata, E2B, and Devin all run an agent inside the VM that communicates with the host orchestrator.
6. **Git integration can be filesystem-based or protocol-based**. virtiofs mount lets git operations happen on shared files; alternatively, git push/pull through the network.
