# Git Workflow Design: User Does Git Inside the VM

**Date:** 2026-02-11
**Context:** This addendum revises the architecture combinations report. The original report assumed the host would extract filesystem diffs and create git commits. Instead, **the user does all git operations (branch, add, commit) inside the VM** as part of their normal workflow. The host's job is to receive the finished commits.

---

## Design Principle

The VM should feel like a **normal development environment**. The user:
1. Gets dropped into a bash shell in a full git working copy
2. Makes changes, runs tests, iterates
3. Uses `git checkout -b`, `git add`, `git commit` normally
4. Exits the VM
5. Their commits appear in the host repo

The host never needs to parse overlayfs upper layers or construct commits from file diffs. Git handles all of that inside the VM.

---

## What the VM Needs

For this to work, the VM must contain a **complete, functional git repository**, not just a file snapshot:

| Requirement | Why |
|-------------|-----|
| Full `.git` directory | `git log`, `git branch`, `git diff` all need it |
| All objects reachable from HEAD | `git commit` needs parent commit objects |
| Working tree matching the host's current state | Including uncommitted changes |
| Git binary + config | Obviously |
| User identity (name/email) | For `git commit` authorship |

This means the repo transfer mechanism must copy the `.git` directory, not just the working tree files.

---

## The Commit-Back Problem

The interesting design question is: **how do commits made inside the VM get back to the host repo?**

### Option A: Shared Block Device (Firecracker)

Since Firecracker uses block devices, the repo lives on an ext4 image. After the VM exits:

```
1. VM shuts down
2. Host mounts the VM's repo block device (ext4 image)
3. Host runs: git -C /mnt/vm-repo log --oneline HEAD ^<base-commit>
   → Shows all commits the user made in the VM
4. Host runs: git -C /host/repo fetch /mnt/vm-repo HEAD:refs/sandbox/<vm-id>
   → Fetches the VM's commits into the host repo as a ref
5. Host unmounts the block device
6. User can now: git merge refs/sandbox/<vm-id>
   Or: the tool auto-merges if fast-forward is possible
```

**Key insight:** `git fetch` from a local path works perfectly for this. The host fetches objects + refs from the mounted VM repo into itself. No network needed.

```bash
# After VM exits, on host:
MOUNT=$(mktemp -d)
mount /path/to/vm-repo.ext4 "$MOUNT"

# What did the user commit?
BASE_COMMIT=$(cat /path/to/vm-metadata/base-commit)
NEW_COMMITS=$(git -C "$MOUNT/workspace" log --oneline "$BASE_COMMIT"..HEAD)

if [ -n "$NEW_COMMITS" ]; then
    # Fetch their commits into the host repo
    git -C /host/repo fetch "$MOUNT/workspace" HEAD:refs/sandbox/vm-$VM_ID
    echo "Commits from sandbox vm-$VM_ID:"
    echo "$NEW_COMMITS"
    echo ""
    echo "To merge: git merge refs/sandbox/vm-$VM_ID"
fi

umount "$MOUNT"
```

### Option B: virtiofs Direct Mount (Cloud Hypervisor)

With virtiofs, the repo directory is shared between host and guest. Git operations in the guest write directly to the host filesystem. **Commits appear in the host repo in real-time.**

```
1. Host creates a working copy for the VM:
   cp --reflink=always -a /host/repo /tmp/sandbox/vm-1
2. virtiofsd shares /tmp/sandbox/vm-1 into the VM at /workspace
3. User does git operations inside the VM
4. Commits are written through virtiofs to /tmp/sandbox/vm-1/.git on host
5. On VM exit:
   git -C /host/repo fetch /tmp/sandbox/vm-1 HEAD:refs/sandbox/vm-1
```

**Advantage:** No mount/unmount step. The repo on the host is always up-to-date.

**Consideration:** The working copy must be an independent clone (not a worktree) because the user might check out different branches. Using `cp --reflink=always` on Btrfs/XFS gives an instant independent copy.

### Option C: vsock Push (Firecracker, Agent-Based)

If an in-VM agent is running, the user's commits can be pushed over vsock:

```
1. User commits inside the VM
2. User runs: sandbox push  (or it happens automatically on exit)
3. Agent creates a git bundle: git bundle create /tmp/changes.bundle <base>..HEAD
4. Agent sends the bundle over vsock to the host
5. Host receives and fetches: git fetch /tmp/changes.bundle HEAD:refs/sandbox/vm-1
```

**Advantage:** Works without mounting block devices. Works even if the VM is still running (incremental push).

---

## Handling Uncommitted Changes at Spawn Time

The user's repo may have uncommitted changes (dirty working tree, staged files). These must be present in the VM. Approaches:

### Approach 1: Filesystem-Level Copy (Recommended)

Copy the entire working directory including `.git`:

```bash
# For block device approach:
truncate -s 10G /tmp/vm-repo.ext4
mkfs.ext4 /tmp/vm-repo.ext4
mount /tmp/vm-repo.ext4 /mnt/vm-repo
rsync -a --exclude='.git/objects' /host/repo/ /mnt/vm-repo/
# Hardlink .git/objects (immutable, safe to share)
cp -al /host/repo/.git/objects/ /mnt/vm-repo/.git/objects/
umount /mnt/vm-repo

# For virtiofs approach:
cp --reflink=always -a /host/repo /tmp/sandbox/vm-1
```

The `cp -al` (hardlink) trick for `.git/objects` is safe because git objects are immutable and content-addressed. This avoids copying potentially gigabytes of git history.

### Approach 2: Git Stash Snapshot

```bash
# Create a stash-like commit capturing ALL current state
SNAPSHOT=$(cd /host/repo && git stash create -u)
if [ -z "$SNAPSHOT" ]; then
    SNAPSHOT=$(git rev-parse HEAD)  # Clean tree, just use HEAD
fi

# Clone into VM working copy
git clone --local /host/repo /tmp/vm-repo
cd /tmp/vm-repo
git checkout --detach $SNAPSHOT
# Or: git stash apply $SNAPSHOT
```

**Limitation:** `git stash create -u` can be slow for repos with many untracked files.

### Approach 3: Two-Phase (Git Objects + Working Tree Overlay)

```bash
# Phase 1: Create a git clone (fast, uses hardlinks)
git clone --local /host/repo /tmp/vm-repo
cd /tmp/vm-repo && git checkout $(cd /host/repo && git rev-parse HEAD)

# Phase 2: Overlay the dirty working tree on top
rsync -a --exclude='.git' /host/repo/ /tmp/vm-repo/
# Now the VM repo has: committed history (from clone) + dirty state (from rsync)
```

---

## Handling the Base Commit Reference

We need to know what commit the VM started from so we can identify which commits are new. Record this at spawn time:

```bash
# At VM creation:
BASE_COMMIT=$(git -C /host/repo rev-parse HEAD)
echo "$BASE_COMMIT" > /path/to/vm-metadata/base-commit

# At VM exit, new commits = everything after BASE_COMMIT:
git -C /mnt/vm-repo log --oneline $BASE_COMMIT..HEAD
```

If the user started with uncommitted changes, the base commit is still HEAD — their first commit in the VM will include those changes plus whatever else they did.

---

## Parallel VMs: Branch Management

Each VM works independently. The host uses refs to track each VM's work:

```
refs/sandbox/vm-1  → VM 1's final HEAD
refs/sandbox/vm-2  → VM 2's final HEAD
refs/sandbox/vm-3  → VM 3's final HEAD
```

The user merges results as they see fit:

```bash
# List all sandbox results
git branch --list 'sandbox/*'

# Merge one
git merge refs/sandbox/vm-1

# Cherry-pick specific commits from another
git cherry-pick refs/sandbox/vm-2~3..refs/sandbox/vm-2

# Or: interactive rebase to clean up
git rebase -i refs/sandbox/vm-1
```

The tool could also offer convenience commands:

```bash
# Auto-merge if fast-forward possible, otherwise show diff
sandbox merge vm-1

# Show what each VM changed
sandbox diff vm-1
sandbox diff vm-2

# Discard a VM's work
sandbox drop vm-3
```

---

## Updated Architecture Implications

### For Architecture 1 & 3 (Firecracker + Block Device)

**Repo goes in the block device as a full git clone.** After VM exit:
1. Mount the block device
2. `git fetch` from it into host repo
3. Unmount and clean up

The overlay extraction approach from the original report is **no longer needed** for git-tracked files. OverlayFS is still used for the rootfs CoW, but the repo diff comes from git itself.

**Optimization:** The repo block device can be a dm-thin snapshot of a "base repo image" that gets updated periodically. Each VM gets a thin snapshot. Git objects written by the user only consume the thin snapshot's CoW space.

### For Architecture 2 & 4 (Cloud Hypervisor + virtiofs)

**Simplest model.** The VM writes directly to a host directory via virtiofs. Git commits appear on the host immediately. After VM exit, just `git fetch` from the working copy.

No block device management needed for the repo — only the rootfs uses a block device.

### For Architecture 5 (vsock Agent)

The agent can implement `sandbox push` and `sandbox sync` commands that use `git bundle` over vsock. This allows incremental commit transfer while the VM is still running.

---

## Recommended End-to-End Flow

```
┌─ HOST ────────────────────────────────────────────────┐
│                                                        │
│  $ sandbox spawn                                       │
│    │                                                   │
│    ├─ Record BASE_COMMIT = HEAD                        │
│    ├─ Create VM working copy (reflink or ext4 image)   │
│    │   └─ Full .git + dirty working tree               │
│    ├─ Boot VM (Firecracker or Cloud Hypervisor)        │
│    └─ Attach user's terminal to VM shell               │
│                                                        │
│  ┌─ VM ──────────────────────────────────────────┐     │
│  │                                                │     │
│  │  $ cd /workspace                               │     │
│  │  $ git checkout -b my-feature                  │     │
│  │  $ vim src/main.py                             │     │
│  │  $ python test.py                              │     │
│  │  $ git add -A                                  │     │
│  │  $ git commit -m "Add feature X"               │     │
│  │  $ exit                                        │     │
│  │                                                │     │
│  └────────────────────────────────────────────────┘     │
│                                                        │
│    VM exits, sandbox tool:                             │
│    ├─ Fetch VM commits: git fetch <vm-repo>            │
│    │   HEAD:refs/sandbox/<vm-id>                       │
│    ├─ Display: "1 new commit on refs/sandbox/<vm-id>"  │
│    ├─ If fast-forward: auto-merge to current branch    │
│    ├─ If diverged: show diff, ask user                 │
│    └─ Clean up VM resources                            │
│                                                        │
│  $ git log --oneline  # Shows the VM's commits        │
│                                                        │
└────────────────────────────────────────────────────────┘
```

---

## Edge Cases

### User Doesn't Commit Anything
If the VM exits without new commits, there's nothing to fetch. The tool reports "No changes" and cleans up.

### User Has Uncommitted Changes at VM Exit
The tool can detect this: `git -C /vm-repo status --porcelain` is non-empty but HEAD hasn't moved. Options:
1. Warn the user and ask if they want to auto-commit
2. Create a stash in the host repo from the VM's dirty state
3. Just discard (the VM's block device/directory can be kept for recovery)

### User Rebases or Rewrites History
If the user runs `git rebase` in the VM, the base commit may no longer be an ancestor of HEAD. The tool detects this (base commit not in `git log HEAD`) and falls back to creating a patch or showing a manual merge.

### Multiple VMs Modify the Same Files
Standard git merge conflict. The tool presents this to the user just like any other merge. The branch-per-VM model means conflicts only arise at merge time, not during VM execution.

### Large Repos (>1GB .git)
For the block device approach: use `cp -al` for .git/objects (hardlinks) to avoid copying gigabytes of pack files. For virtiofs: `cp --reflink=always` handles this efficiently on Btrfs/XFS.
