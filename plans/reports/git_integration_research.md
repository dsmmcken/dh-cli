# Git Integration Strategies for VM-Based Sandboxes

## Research Report

**Date:** 2026-02-11
**Context:** A developer has a git repo with a working directory (possibly with uncommitted changes). They want to spawn many parallel Firecracker VMs, each starting with the current state of the repo (including uncommitted changes and installed dependencies). When work in a VM is done, changes need to be committed back to the original repo. The original repo stays "clean" as the source of truth.

---

## Table of Contents

1. [Git Worktrees](#1-git-worktrees)
2. [Git Clone Strategies for Fast Local Copies](#2-git-clone-strategies-for-fast-local-copies)
3. [Git Bundle and Archive](#3-git-bundle-and-archive)
4. [Git Alternates Mechanism](#4-git-alternates-mechanism)
5. [Extracting Changes from VM Back to Host](#5-extracting-changes-from-vm-back-to-host)
6. [Handling Parallel Changes and Merge Conflicts](#6-handling-parallel-changes-and-merge-conflicts)
7. [Dependency Caching Strategies](#7-dependency-caching-strategies)
8. [Live Filesystem Sharing](#8-live-filesystem-sharing)
9. [Git Credentials and Configuration in VMs](#9-git-credentials-and-configuration-in-vms)
10. [Existing Solutions](#10-existing-solutions)
11. [Recommended Architecture](#11-recommended-architecture)

---

## 1. Git Worktrees

### How They Work

Git worktrees allow multiple working directories to share a single `.git` directory (the object store, refs, etc.). Each worktree gets its own:
- **HEAD** (can point to different branches or be detached)
- **Index** (staging area)
- **Working tree** (the actual files)

Everything else is shared: the object database, remote refs, config, hooks, etc. Commits made in any worktree immediately appear in the shared git database.

### Key Commands

```bash
# Create a worktree for a specific branch
git worktree add /path/to/worktree feature-branch

# Create a worktree with detached HEAD at a specific commit
git worktree add --detach /path/to/worktree HEAD

# List all worktrees
git worktree list

# Remove a worktree
git worktree remove /path/to/worktree

# Clean up stale worktree references
git worktree prune
```

### Can Worktrees Be Used as a Base for VM Filesystems?

**Partially.** Worktrees provide an efficient way to create multiple working copies without duplicating the object store. However, there are significant limitations for the VM use case:

1. **Firecracker uses block devices, not directory mounts.** Since Firecracker does not support virtiofs, you cannot directly mount a worktree directory into a VM. You would need to create a block device image containing the worktree contents.

2. **Branch exclusivity rule:** No two worktrees can check out the same branch simultaneously. For parallel VMs all starting from the same state, you would need to use detached HEADs or create temporary branches per VM.

3. **Uncommitted state cannot be directly shared:** You cannot create a worktree that includes the dirty (uncommitted) state of another worktree. The worktree starts clean at the specified commit.

### Workaround for Uncommitted State

To capture uncommitted changes for distribution to worktrees:

```bash
# 1. Create a temporary commit with all current changes (staged + unstaged + untracked)
git add -A
git stash create  # Returns a commit hash without modifying refs
# Or: create a temporary commit
git commit -m "TEMP: snapshot for VM"  # Remember to reset later

# 2. Create worktrees at this commit
git worktree add --detach /path/to/vm-worktree TEMP_COMMIT_SHA

# 3. Reset the temporary commit in the main worktree
git reset HEAD~1
```

### Performance with Many Worktrees

- There are no published benchmarks for 100+ worktrees, but git's architecture should handle it well since worktrees are lightweight (just a `.git` file pointing to the main repo plus the working tree).
- **Storage is the main concern:** Each worktree duplicates the working tree. For a Node.js project with 2GB of `node_modules`, 10 worktrees = 20GB. Using content-addressable package managers (pnpm) can reduce this by 60-80%.
- **Maintenance:** Worktrees accumulate and need periodic `git worktree prune` cleanup.
- Worktrees add overhead to operations that scan all worktrees (e.g., `git gc`).

### Verdict for Our Use Case

**Not ideal as the primary mechanism.** Worktrees are designed for a developer switching between a few branches, not for spawning hundreds of ephemeral VMs. Key issues:
- Cannot directly mount into Firecracker VMs
- Branch exclusivity complicates parallel identical checkouts
- No native uncommitted state sharing
- Storage duplication for dependencies

---

## 2. Git Clone Strategies for Fast Local Copies

### `git clone --local` (default for local paths)

```bash
git clone /path/to/source /path/to/destination
```

- **Mechanism:** Hardlinks files in `.git/objects/` from source to destination.
- **Speed:** Very fast (no data copy for objects).
- **Space:** Objects share disk space via hardlinks. Working tree is duplicated.
- **Independence:** Fully independent clone. Hardlinks are safe because git objects are immutable.
- **Limitation:** Only works on the same filesystem. Does not include uncommitted changes.

### `git clone --shared`

```bash
git clone --shared /path/to/source /path/to/destination
```

- **Mechanism:** Sets up `.git/objects/info/alternates` to point to source repo's objects directory. The clone starts with zero objects of its own.
- **Speed:** Near-instantaneous (no object copying at all).
- **Space:** Minimal -- only stores new objects created in the clone.
- **Danger:** If source repo runs `git gc` and prunes objects, the clone can become corrupt. Source repo's `git gc --auto` with `--local` flag is safe, but manual repacking or pruning is dangerous.
- **Breaking the dependency:** Run `git repack -a` in the clone to copy all borrowed objects locally.

### `git clone --reference`

```bash
git clone --reference /path/to/local-cache https://remote-server/repo.git /path/to/destination
```

- **Mechanism:** Uses the local repo as an alternate for fetching objects, but fetches the rest from remote. Keeps the alternates reference permanently.
- **Use case:** Speed up cloning a remote repo when you have a local copy.
- **Risk:** Same as `--shared` -- dependent on the reference repo for borrowed objects.

### `git clone --reference --dissociate`

```bash
git clone --reference /path/to/local-cache --dissociate https://remote-server/repo.git /path/to/destination
```

- **Mechanism:** Uses the local reference during clone, then copies all borrowed objects into the new clone, making it fully independent.
- **Speed:** Fast initial clone (reads from local reference), then a one-time copy of borrowed objects.
- **Safety:** Fully independent after clone completes. Best of both worlds.

### Comparison for Our Use Case

| Strategy | Speed | Space | Safety | Uncommitted Changes |
|----------|-------|-------|--------|-------------------|
| `--local` (default) | Fast | Medium (hardlinks) | Safe | No |
| `--shared` | Instant | Minimal | Dangerous | No |
| `--reference --dissociate` | Fast | Full | Safe | No |
| cp -a (filesystem copy) | Medium | Full | Safe | Yes |
| tar + extract | Medium | Full | Safe | Yes (if from worktree) |

### Key Insight

None of the git clone strategies include uncommitted changes. For our use case, we need a **filesystem-level approach** (tar, rsync, overlayfs) rather than a git-level clone to capture the complete working state.

---

## 3. Git Bundle and Archive

### Git Bundle

```bash
# Create a bundle of the entire repo
git bundle create repo.bundle --all

# Create a bundle of specific branches
git bundle create repo.bundle main feature-branch

# Create an incremental bundle (objects since a known commit)
git bundle create incremental.bundle HEAD~10..HEAD

# Clone from a bundle
git clone repo.bundle /path/to/new-repo

# Fetch from a bundle into existing repo
git fetch repo.bundle main:refs/remotes/bundle/main
```

**Characteristics:**
- Packages git objects and refs into a single file
- Can be cloned from or fetched into
- **Does NOT include uncommitted changes** -- only committed objects
- Good for offline transfer of repository history
- Incremental bundles are efficient for ongoing sync

### Git Archive

```bash
# Create a tarball of the current HEAD
git archive --format=tar.gz -o snapshot.tar.gz HEAD

# Archive a specific tree/commit
git archive --format=tar.gz -o snapshot.tar.gz abc1234

# Archive with a prefix (directory name)
git archive --prefix=project/ --format=tar.gz -o snapshot.tar.gz HEAD
```

**Characteristics:**
- Creates a tarball/zip of the working tree at a specific commit
- **Does NOT include .git directory** (no version history)
- **Does NOT include uncommitted changes**
- Very fast for creating snapshots of committed state
- Cannot be used for git operations (no objects, no refs)

### Including Uncommitted Changes with Bundles

The standard `git bundle` cannot include uncommitted changes. Workarounds:

**Approach A: Temporary commit + bundle**
```bash
# Stash everything including untracked files, get the stash commit
STASH_SHA=$(git stash create -u)
# Create a temporary ref
git update-ref refs/temp/vm-snapshot $STASH_SHA
# Bundle including the temp ref
git bundle create snapshot.bundle --all
# Clean up
git update-ref -d refs/temp/vm-snapshot
```

**Approach B: GitBundlePlus (third-party tool)**
```bash
pip install git-bundle-plus
git-bundle-plus create repo-with-stashes.bundle
```
This tool stashes and tags uncommitted changes before bundling.

### Verdict for Our Use Case

Bundles are useful for transferring repository state but add overhead and cannot natively include uncommitted changes. For our scenario of copying into block device images, a **filesystem-level snapshot** is more direct.

---

## 4. Git Alternates Mechanism

### How It Works

The alternates file (`.git/objects/info/alternates`) contains paths to other object directories. When git needs to read an object, it checks:
1. The local `.git/objects/` directory
2. Each path listed in the alternates file, in order

```bash
# Example: Set up alternates manually
echo "/path/to/shared-repo/.git/objects" >> .git/objects/info/alternates
```

### Sharing Object Storage Across Many Working Copies

This is how `git clone --shared` works internally. For many VMs sharing a common repo:

```bash
# Host repo at /repos/main
# Each VM clone has .git/objects/info/alternates pointing to /repos/main/.git/objects

# VM 1
echo "/repos/main/.git/objects" > /repos/vm-1/.git/objects/info/alternates

# VM 2
echo "/repos/main/.git/objects" > /repos/vm-2/.git/objects/info/alternates
```

### Safety Considerations

1. **Immutability is your friend:** Git objects are content-addressed and immutable. Once an object exists, its hash never changes and it never gets modified in place.

2. **Danger from garbage collection:** If the source repo runs `git gc` or `git prune`, it may delete objects that the dependent repos still reference. This causes corruption.

3. **Mitigation strategies:**
   - Never run `git gc` or `git prune` on the shared source while dependents exist
   - Use `git repack -a -d -l` (the `-l` flag means "only local objects") in dependent repos -- this is safe
   - For ephemeral VMs, this is actually fine: the VMs are short-lived, and we control the source repo

4. **Read-only safety:** If the source repo's objects directory is mounted read-only into VMs, and VMs only write new objects locally, this is safe.

### Applicability to Firecracker VMs

**Limited direct applicability.** Since Firecracker uses block devices (not directory mounts), we cannot easily share an alternates directory at the filesystem level. We would need to:
- Bake the objects into the block device image
- Or use a network filesystem (adds complexity and latency)

For our use case, it is more practical to include all needed objects in the VM's block device image.

---

## 5. Extracting Changes from VM Back to Host

This is one of the most critical areas. After a VM completes work, we need to get the changes back to the host.

### Approach A: git diff / git format-patch

```bash
# Inside VM: Create a diff of all changes (staged + unstaged)
git diff HEAD > changes.patch

# Include binary files
git diff HEAD --binary > changes.patch

# Include untracked files: stage them first
git add -A
git diff --cached --binary > changes.patch

# On host: Apply the patch
git apply changes.patch

# Or for format-patch (requires a commit):
# Inside VM:
git add -A && git commit -m "VM work"
git format-patch -1 HEAD --stdout > changes.patch
# On host:
git am < changes.patch
```

**Tradeoffs:**
- `git diff` is lightweight and works for text changes
- `--binary` flag needed for binary files
- Does not handle file deletions and renames perfectly in all cases
- `git format-patch` preserves commit metadata but requires committing first

### Approach B: git stash create + transfer

```bash
# Inside VM: Create a stash object without storing it
STASH_SHA=$(git stash create -u)  # -u includes untracked files
echo $STASH_SHA > /output/stash-sha.txt

# Inside VM: Create a bundle with just the stash
git bundle create /output/stash.bundle $STASH_SHA

# On host: Fetch the stash
git fetch /path/to/stash.bundle $STASH_SHA
git stash store $STASH_SHA -m "Changes from VM"
# Or: apply directly
git stash apply $STASH_SHA
```

**Tradeoffs:**
- Clean way to capture all changes including untracked files
- Portable via bundle
- Requires git infrastructure in the VM

### Approach C: git commit in VM + git fetch from host

```bash
# Inside VM: Commit all changes
git add -A
git commit -m "Work from VM $(hostname)"

# On host: Fetch from the VM's repo (requires network access or shared filesystem)
git fetch /path/to/vm-repo HEAD:refs/vm/vm-1-result

# Or: VM pushes to a shared bare repo
# Inside VM:
git push /shared/bare-repo.git HEAD:refs/vm/vm-1-result
```

**Tradeoffs:**
- Cleanest git workflow
- Requires network access between host and VM, or a shared filesystem/bare repo
- Each VM commits to a unique branch/ref to avoid conflicts

### Approach D: rsync-based

```bash
# Inside VM: After making changes
# (assumes SSH or network access from host to VM)

# From host:
rsync -avz --checksum vm-host:/workspace/ /path/to/staging/vm-1/

# Or: Inside VM, write changes to a shared volume
rsync -avz --delete /workspace/ /output/workspace/
```

**Tradeoffs:**
- Simple, no git knowledge needed
- Captures everything (including untracked, binary, etc.)
- No incremental efficiency for git objects
- Requires careful handling to avoid overwriting host state

### Approach E: OverlayFS Upper Layer as a Diff (Recommended for Firecracker)

This is the most elegant approach for Firecracker VMs using overlayfs:

```bash
# Setup (host side, before VM boot):
# lower = read-only base image with repo snapshot
# upper = empty ext4 sparse file for writes
# work = overlayfs work directory

# The VM boots with overlayfs:
# mount -t overlay overlay -o lowerdir=/base,upperdir=/upper,workdir=/work /workspace

# After VM completes:
# The upper directory contains ONLY the files that were modified/created/deleted

# On host: Extract the upper layer
# - New/modified files are present as regular files in upper/
# - Deleted files appear as character devices (whiteout files) in upper/
# - This is a perfect, minimal diff of what changed
```

**Extracting changes from the upper layer:**
```bash
# List all changed files
find /upper -type f  # Modified or new files

# List deleted files (whiteout entries)
find /upper -type c  # Character device = deleted file marker

# Copy changed files back to host repo
rsync -av /upper/ /host/repo/  # Careful: need to handle whiteouts

# Or: Create a tarball of changes
tar -czf changes.tar.gz -C /upper .

# Or: Use overlayfs-tools to compute diff
# https://github.com/kmxz/overlayfs-tools
overlayfs-tools diff /lower /upper /output
```

**Tradeoffs:**
- Most efficient: only captures actual changes at the filesystem level
- No git involvement needed in the VM
- Natural fit for Firecracker's block device model
- Need to handle overlayfs whiteout files for deletions
- Then convert the filesystem diff into git operations on the host

### Recommended Strategy

**Primary: OverlayFS upper layer extraction** -- for maximum efficiency and simplicity within the VM.

**Fallback: git commit + fetch** -- when you need proper git semantics (commit messages, authorship, etc.) and the VM has sufficient tooling.

**Conversion pipeline (host side):**
```bash
# 1. Extract upper layer from VM's overlay
# 2. Apply file changes to a working copy on the host
# 3. Use git to create a proper commit
rsync -av --delete vm-upper/ /host/staging/
cd /host/staging
git add -A
git commit -m "Changes from VM $VM_ID"
```

---

## 6. Handling Parallel Changes and Merge Conflicts

### Branch-Per-VM Strategy (Recommended)

Each VM works on a unique branch:

```bash
# Before spawning VMs, create the base snapshot
BASE_COMMIT=$(git rev-parse HEAD)

# Each VM gets a unique branch name
# VM 1 works on: vm/task-1
# VM 2 works on: vm/task-2
# etc.

# After all VMs complete, on the host:
git fetch /vm-results vm/task-1:refs/vm/task-1
git fetch /vm-results vm/task-2:refs/vm/task-2
```

### Merging Results

**Sequential merge (safest):**
```bash
git checkout main
git merge refs/vm/task-1 --no-ff -m "Merge VM task 1"
git merge refs/vm/task-2 --no-ff -m "Merge VM task 2"
```

**Octopus merge (for conflict-free changes):**
```bash
# Merge all VM results at once
git merge refs/vm/task-1 refs/vm/task-2 refs/vm/task-3
```

The octopus strategy only works when there are **no conflicts** between any of the branches. If any two VMs modified the same file in conflicting ways, the octopus merge will refuse to proceed. Limit to fewer than ~10 branches for manageability.

**Cherry-pick strategy (for selective integration):**
```bash
# Pick specific commits from VM results
git cherry-pick refs/vm/task-1
git cherry-pick refs/vm/task-2
```

### Conflict Detection

```bash
# Pre-check: test if a merge would succeed without actually merging
git merge --no-commit --no-ff refs/vm/task-1
# Check exit code: 0 = clean, non-zero = conflicts
git merge --abort  # Reset if we were just testing
```

### Presenting Conflicts to the User

For an automated system:
1. Attempt merges sequentially
2. If a conflict occurs, save the conflict state
3. Present the user with:
   - Which files conflict
   - The diff from each VM
   - Options: manual resolve, pick one version, abort

```bash
# Get conflicted files
git diff --name-only --diff-filter=U

# Show the conflict markers
git diff

# User resolves, then:
git add -A
git commit
```

### Conflict Minimization Strategies

1. **Partition work:** Assign different files/directories to different VMs
2. **Lock files:** Use a coordination mechanism so VMs claim files before editing
3. **Append-only patterns:** For log files or similar, use append-only operations that auto-merge
4. **Rebasing instead of merging:** Rebase VM results onto the latest main for a linear history

---

## 7. Dependency Caching Strategies

### The Problem

A typical project might have:
- `node_modules/`: 200MB-2GB
- `.venv/`: 100MB-500MB
- `target/` (Rust): 1-10GB
- `.gradle/`: 500MB-2GB

Copying these into every VM is expensive in time and space.

### Strategy A: Pre-Bake Dependencies into VM Image

```bash
# Build a "warm" base image:
# 1. Start with base OS image
# 2. Copy package-lock.json / requirements.txt / Cargo.lock
# 3. Run npm install / pip install / cargo build
# 4. Snapshot the image

# Cache key = hash of lock file
CACHE_KEY=$(sha256sum package-lock.json | cut -d' ' -f1)
IMAGE_PATH="/cache/images/${CACHE_KEY}.ext4"

if [ ! -f "$IMAGE_PATH" ]; then
    # Build new cached image
    create_base_image "$IMAGE_PATH"
    mount_and_install_deps "$IMAGE_PATH"
fi

# Use cached image as lower layer for overlayfs
```

**Tradeoffs:**
- Fastest VM boot time (deps already installed)
- Requires image rebuild when deps change
- Storage cost for multiple cached images (one per lock file hash)
- Best strategy for stable, well-defined dependency sets

### Strategy B: Read-Only Dependency Layer + Read-Write Project Overlay

```bash
# Layer architecture:
# [Read-only] Base OS + runtime
# [Read-only] Dependencies (node_modules, .venv, etc.)
# [Read-write] Project source code (overlayfs upper layer)

# Host setup:
mount -t overlay overlay \
    -o lowerdir=/deps-layer:/base-layer,upperdir=/project-upper,workdir=/work \
    /vm-root

# The VM sees a merged view:
# /workspace/
#   node_modules/  (from deps-layer, read-only but CoW if modified)
#   src/           (from project-upper, read-write)
#   package.json   (from project-upper, read-write)
```

**Tradeoffs:**
- Deps shared across all VMs (only stored once)
- If a VM needs to modify a dependency, overlayfs copies it (CoW)
- Requires careful layer management
- Most space-efficient for many VMs

### Strategy C: Hash-Based Cache Invalidation

```bash
# Compute cache key from dependency specification files
cache_key() {
    local files="package-lock.json yarn.lock Pipfile.lock requirements.txt Cargo.lock"
    local hash=""
    for f in $files; do
        if [ -f "$f" ]; then
            hash="${hash}$(sha256sum "$f" | cut -d' ' -f1)"
        fi
    done
    echo "$hash" | sha256sum | cut -d' ' -f1
}

CACHE_KEY=$(cache_key)
DEPS_IMAGE="/cache/deps-${CACHE_KEY}.squashfs"

if [ ! -f "$DEPS_IMAGE" ]; then
    # Install deps in a temp directory, create squashfs image
    tmpdir=$(mktemp -d)
    cp package-lock.json "$tmpdir/"
    (cd "$tmpdir" && npm ci --prefix .)
    mksquashfs "$tmpdir/node_modules" "$DEPS_IMAGE"
    rm -rf "$tmpdir"
fi

# Mount squashfs as read-only lower layer for VMs
```

### Strategy D: Shared Dependency Volume

```bash
# Use a single host directory with all deps, mount read-only into VMs
# Only works with filesystem sharing (not Firecracker's block device model)
# Would require NFS or similar network filesystem

# Host: /shared/deps/node_modules (read-only NFS export)
# VM: mount -t nfs host:/shared/deps/node_modules /workspace/node_modules -o ro
```

### Recommended Approach

**Layered block device with hash-based caching:**

1. **Base layer** (squashfs, read-only): OS + runtime + common tools
2. **Deps layer** (squashfs, read-only): Dependencies, keyed by lock file hash
3. **Project layer** (ext4, read-only): Git repo snapshot at specific state
4. **Write layer** (ext4, sparse): Empty, for VM writes (overlayfs upper)

Use device-mapper thin provisioning to compose these layers:

```bash
# Create thin pool
dmsetup create thin-pool \
    --table "0 $(blockdev --getsz /dev/loop0) thin-pool /dev/loop0 /dev/loop1 128 0"

# Create thin volume for VM (instant, uses no space)
dmsetup message thin-pool 0 "create_thin 1"

# Snapshot from base (CoW, instant)
dmsetup message thin-pool 0 "create_snap 2 1"
```

---

## 8. Live Filesystem Sharing

### virtiofs

**What it is:** A shared filesystem protocol designed for VM-host communication using VIRTIO transport and FUSE protocol.

**Performance:** 2x-8x faster than 9p (virtio-9p) depending on workload. Uses DAX (direct access) to allow file access without hypervisor communication.

**Firecracker support:** **NOT SUPPORTED.** Firecracker explicitly does not implement virtiofs due to security concerns (large attack surface). A WIP PR existed but was never merged.

### 9p (virtio-9p / Plan 9)

**What it is:** Network filesystem protocol adapted for VM-host communication over virtio transport.

**Performance:** Significantly slower than virtiofs (3-4x slower for file operations). Based on a network protocol not optimized for local VM use.

**Firecracker support:** **NOT SUPPORTED** natively.

### NFS

```bash
# Host: Export a directory
echo "/shared/repo 192.168.0.0/24(ro,no_subtree_check)" >> /etc/exports
exportfs -ra

# VM: Mount the NFS share
mount -t nfs host-ip:/shared/repo /workspace -o ro,vers=4
```

**Tradeoffs:**
- Works with any VM technology (network-based)
- Adds latency and complexity
- Need to run NFS server on the host
- Read-only mount is safe; read-write needs locking

### FUSE-based approaches

Custom FUSE filesystems could expose host directories to VMs over virtio-vsock:

```bash
# Concept: FUSE server on host, FUSE client in VM
# Host: fuse-server --socket /tmp/vm-1.sock --dir /path/to/repo
# VM: fuse-client --vsock host:port --mountpoint /workspace
```

**Tradeoffs:**
- Maximum flexibility
- Significant development effort
- Performance depends on implementation

### Direct Copy-In / Copy-Out (Recommended for Firecracker)

Since Firecracker only supports block devices:

```bash
# Copy-in: Before VM boot
# Create ext4 image with repo contents
truncate -s 1G /tmp/vm-repo.ext4
mkfs.ext4 /tmp/vm-repo.ext4
mount /tmp/vm-repo.ext4 /mnt/vm-repo
rsync -a /host/repo/ /mnt/vm-repo/
umount /mnt/vm-repo

# Attach as secondary block device to Firecracker VM
# In Firecracker config:
# "drives": [{ "drive_id": "repo", "path_on_host": "/tmp/vm-repo.ext4" }]

# Copy-out: After VM stops
# Mount the block device and extract changes
mount /tmp/vm-repo.ext4 /mnt/vm-result
rsync -a /mnt/vm-result/ /host/staging/
umount /mnt/vm-result
```

**Optimized with overlayfs + device mapper:**

```bash
# Read-only base image (shared across all VMs)
# Per-VM thin snapshot (CoW, instant creation, minimal space)
# After VM: read the thin snapshot's diff
```

### Comparison

| Method | Firecracker Support | Performance | Complexity | Safety |
|--------|-------------------|-------------|------------|--------|
| virtiofs | No | Excellent | Medium | Good |
| 9p | No | Poor | Medium | Good |
| NFS | Possible (network) | Good | High | Moderate |
| FUSE/vsock | Custom work | Variable | Very High | Variable |
| Block device copy | Yes (native) | Good | Low | Excellent |
| Block device + overlay | Yes (native) | Excellent | Medium | Excellent |

---

## 9. Git Credentials and Configuration in VMs

### Git Config

```bash
# Option A: Bake minimal git config into VM image
cat > /etc/gitconfig <<EOF
[user]
    name = Sandbox VM
    email = sandbox@example.com
[safe]
    directory = /workspace
EOF

# Option B: Copy host's git config (selective)
# During VM image preparation:
git config --global user.name > /vm-image/gitconfig-name
git config --global user.email > /vm-image/gitconfig-email
```

### SSH Key Forwarding

**Option A: SSH Agent Forwarding (for VMs accessible via SSH)**

```bash
# Host SSH config
Host vm-*
    ForwardAgent yes

# Requires: SSH server in VM, SSH agent on host
# Risk: VM can use your SSH keys while connected
```

**Option B: Inject a Scoped Deploy Key**

```bash
# Generate an ephemeral key pair per VM session
ssh-keygen -t ed25519 -f /tmp/vm-key -N "" -C "vm-ephemeral"

# Register as deploy key (read-only) on the repo
# Copy private key into VM image
cp /tmp/vm-key /vm-image/root/.ssh/id_ed25519

# Clean up after VM terminates
rm /tmp/vm-key /tmp/vm-key.pub
```

**Option C: HTTPS Access Tokens**

```bash
# Generate a scoped, short-lived personal access token
TOKEN=$(generate_scoped_pat --scope repo --expiry 1h)

# Inject into VM via environment or credential helper
cat > /vm-image/git-credential-helper.sh <<'EOF'
#!/bin/sh
echo "protocol=https"
echo "host=github.com"
echo "username=token"
echo "password=${GIT_TOKEN}"
EOF

# Git config to use the helper
git config --global credential.helper "/path/to/git-credential-helper.sh"
```

**Recommended:** Use short-lived HTTPS tokens for ephemeral VMs. They are easier to inject (environment variable in VM config), easier to scope (read-only or specific repos), automatically expire, and do not require SSH infrastructure in the VM.

### GPG Signing in VMs

GPG agent forwarding into VMs is complex and fragile. Alternatives:

1. **Skip signing in VMs:** VM commits are intermediate; sign the final merge commit on the host.
2. **Use SSH signing (Git 2.34+):**
   ```bash
   git config --global gpg.format ssh
   git config --global user.signingKey /path/to/ssh-key
   ```
   Easier to inject than GPG keys.
3. **Sign after the fact:** Use `git commit --amend -S` on the host after fetching VM commits.

---

## 10. Existing Solutions

### GitHub Codespaces

- **Architecture:** Each codespace is a Docker container running on a cloud VM.
- **Git handling:** Shallow clone of the repo into `/workspaces` directory, mounted persistently into the container.
- **Persistence:** `/workspaces` survives container rebuilds. Everything else is ephemeral.
- **Credentials:** SSH agent forwarding and HTTPS token injection via VS Code.
- **Key insight:** One workspace per user, not parallel VMs from same state.

### Gitpod

- **Architecture:** Kubernetes-based. Each workspace is a container on a K8s node.
- **Git handling:** Full clone into workspace. Uses devcontainer.json or gitpod.yml for environment setup.
- **Dependencies:** Prebuilt workspace images triggered by pushes to main. "Prebuild" concept similar to our warm image strategy.
- **Credentials:** Git credential forwarding from host to workspace.

### DevPod

- **Architecture:** Open-source, provider-agnostic. Can deploy to Docker, K8s, cloud VMs, etc.
- **Git handling:** Syncs git repo (or local path) into the workspace. Forwards git credentials.
- **Filesystem:** Mount-based for local providers, sync-based for remote.
- **Key insight:** Supports creating workspace from local path (not just git URL), which is relevant for our uncommitted changes scenario.

### E2B (Code Sandboxes)

- **Architecture:** Firecracker-based sandboxes, very similar to our use case.
- **Filesystem:** OverlayFS with shared read-only base layer and per-VM write layer.
- **Scaling:** Thousands of concurrent sandboxes using thin provisioning.
- **Boot time:** <125ms per sandbox due to Firecracker's minimal overhead.
- **Key insight:** Uses block device overlays (device mapper), not filesystem mounts. This is the closest analog to our architecture.

### Key Takeaways from Existing Solutions

1. **Codespaces/Gitpod** solve a different problem (persistent dev environments, one per user). Not designed for ephemeral parallel VMs.
2. **E2B** is the closest model. They use Firecracker + OverlayFS + device mapper for efficient multi-VM scaling.
3. **Pre-building images** (Gitpod prebuilds) is a proven pattern for fast startup with dependencies.
4. **Filesystem copy + overlay** (not git-level sharing) is the dominant pattern for VM-based solutions.

---

## 11. Recommended Architecture

Based on all research, here is the recommended architecture for our use case:

### Phase 1: Snapshot Creation (Host Side)

```
Developer's Repo (with uncommitted changes)
    |
    v
[1] Create temporary commit including ALL state:
    - git add -A
    - git stash create -u  (or temporary commit)
    |
    v
[2] Create base filesystem image:
    - tar of entire working directory (including .git, node_modules, etc.)
    - Or: rsync into an ext4 image
    |
    v
[3] Create squashfs (read-only, compressed) base image
    mksquashfs /path/to/snapshot base-image.squashfs
```

### Phase 2: VM Provisioning

```
Base Image (squashfs, read-only, shared)
    |
    v
[4] For each VM, create a thin CoW overlay:
    - Device mapper thin snapshot, OR
    - Sparse ext4 file for overlayfs upper layer
    |
    v
[5] Boot Firecracker VM:
    - Root drive: base OS image
    - Secondary drive: overlayfs composed of base + per-VM overlay
    - Guest boots, mounts overlay, workspace is ready
```

### Phase 3: Change Extraction (After VM Completes)

```
VM's overlay (upper layer) contains only changed files
    |
    v
[6] Extract changes:
    Option A (filesystem level):
    - Mount the overlay upper layer
    - rsync changed files to a staging area on host
    - Use git to commit changes

    Option B (git level):
    - VM runs: git add -A && git commit && git bundle create
    - Host extracts bundle

    Option C (hybrid):
    - VM writes a patch: git diff --binary HEAD > /output/changes.patch
    - Host applies: git apply changes.patch
```

### Phase 4: Integration (Host Side)

```
[7] Each VM's changes on a unique branch:
    refs/vm/task-1, refs/vm/task-2, ...
    |
    v
[8] Merge strategy:
    - Sequential merge for conflict detection
    - Present conflicts to user for resolution
    - Final result on main branch
```

### Key Design Decisions

| Decision | Recommendation | Rationale |
|----------|---------------|-----------|
| Repo transfer mechanism | Filesystem snapshot (tar/rsync into block device) | Captures uncommitted changes; simpler than git-level approaches |
| VM storage | Device mapper thin provisioning + overlayfs | Proven by E2B; space-efficient; instant VM creation |
| Dependency handling | Pre-baked layers keyed by lock file hash | Fastest boot; shared across VMs |
| Change extraction | OverlayFS upper layer + git commit on host | Minimal data transfer; proper git integration |
| Merge strategy | Branch-per-VM + sequential merge | Clean history; proper conflict handling |
| Credentials | Short-lived HTTPS tokens (if needed) | Simplest; auto-expire; easy to scope |
| Signing | Sign on host after merge | Avoid GPG complexity in ephemeral VMs |

### Performance Estimates

- **Snapshot creation:** 1-5 seconds (tar of working directory)
- **Per-VM provisioning:** <1 second (thin snapshot + Firecracker boot)
- **Change extraction:** <1 second (read overlay upper layer)
- **Total overhead per VM:** ~2-6 seconds for provisioning + extraction
- **Space per VM:** Only the bytes actually written (CoW), typically KB to low MB
- **Concurrent VMs:** Hundreds to thousands on a single host (limited by RAM, ~5MB per VM minimum)
