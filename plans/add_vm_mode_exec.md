# Plan: Add `--vm` Flag to `dhg exec` (Firecracker MicroVM Mode)

## Context

The `dhg exec` command currently starts a Deephaven server embedded in a Python/JVM process, which takes several seconds for JVM startup. This plan adds an experimental `--vm` flag that uses Firecracker microVM snapshot-restore to achieve near-instant (~10-20ms) Deephaven server startup. A pre-booted Deephaven server is snapshotted, and each `exec --vm` invocation restores a fresh VM from that snapshot.

**Key insight:** `runner.py` already supports `--mode remote` which connects to an existing Deephaven server. The VM just provides the server; runner.py connects to it over the network without any changes.

## Architecture

```
dhg exec --vm -c "..."
  1. Restore Firecracker VM from snapshot (~10ms)
     -> VM resumes with Deephaven already running at 172.16.0.2:10000
  2. Launch runner.py --mode remote --host 172.16.0.2 --port 10000
     -> Connects via pydeephaven Session, executes code, returns results
  3. Destroy VM + cleanup TAP device
```

## New Files

```
go_src/internal/vm/
  vm.go               # Types, constants, VMPaths
  prereqs.go          # Prerequisite checks (/dev/kvm, firecracker binary, etc.)
  rootfs.go           # Docker-based rootfs building (ext4 image)
  network.go          # TAP device setup/teardown
  machine.go          # Firecracker boot, snapshot create, snapshot restore
  cleanup.go          # Stale instance cleanup, signal handler registration
  vm_unsupported.go   # //go:build !linux stubs

go_src/internal/cmd/
  vm.go               # `dhg vm prepare|status|clean` subcommands

go_src/internal/exec/
  exec_vm_linux.go    # runVM() function, //go:build linux
  exec_vm_other.go    # //go:build !linux stub returning error
```

## Modified Files

| File | Change |
|------|--------|
| `go_src/internal/exec/exec.go` | Add `VMMode bool` to `ExecConfig`; add VM branch in `Run()` |
| `go_src/internal/cmd/exec.go` | Add `--vm` flag, wire into config |
| `go_src/internal/cmd/root.go` | Register `addVMCommands(cmd)` |
| `go_src/go.mod` | Add `firecracker-go-sdk` dependency |
| `go_src/internal/exec/runner.py` | **No changes** (remote mode reused as-is) |

## Artifact Layout

```
~/.dhg/vm/
  firecracker          # Binary (auto-downloaded by `dhg vm prepare`)
  vmlinux              # Kernel (auto-downloaded)
  rootfs/
    deephaven-0.36.0.ext4
  snapshots/
    0.36.0/
      snapshot_mem       # Memory snapshot
      snapshot_vmstate   # VM state
      disk.ext4          # Disk backing
      metadata.json      # Version, IP, port, timestamps
  run/                   # Ephemeral per-invocation state
    exec-<timestamp>/
      firecracker.sock
      firecracker.pid
      instance.json
      cow_disk.ext4
```

## CLI Changes

### New flag on exec
```
dhg exec --vm -c "print('hello')"    # Execute using VM mode
dhg exec --vm script.py              # Works with all existing flags
```

`--vm` is mutually exclusive with `--host` (both connect remotely; `--vm` manages its own server).

### New `dhg vm` subcommand group
```
dhg vm prepare [--version X]   # Build rootfs + create snapshot
dhg vm status                  # Show prerequisites + available snapshots
dhg vm clean [--version X]     # Remove VM artifacts
```

## Implementation Phases

### Phase 1: Package skeleton + prerequisites
- Create `internal/vm/` with types, paths, constants (`vm.go`)
- Implement `CheckPrerequisites()` — checks Linux, /dev/kvm, firecracker binary, kernel, iproute2 (`prereqs.go`)
- Add `dhg vm status` command (`cmd/vm.go`)
- Add `--vm` flag to exec (initially returns "snapshot not found" error)
- Platform stubs for non-Linux (`vm_unsupported.go`, `exec_vm_other.go`)

### Phase 2: Auto-download + rootfs building
- `EnsureFirecracker()` — download Firecracker binary from `https://github.com/firecracker-microvm/firecracker/releases` if missing at `~/.dhg/vm/firecracker` (`prereqs.go`)
- `EnsureKernel()` — download pre-built vmlinux from Firecracker's CI artifacts or a hosted kernel if missing (`prereqs.go`)
- Both functions are called by `dhg vm prepare` before rootfs build
- `BuildRootfs()` using Docker: write Dockerfile + init.sh to temp dir, `docker build`, `docker export`, create ext4 image (`rootfs.go`)
- Init script inside VM: configures network, starts Deephaven server, signals readiness

### Phase 3: Network
- TAP device create/configure/teardown using `ip` commands (`network.go`)
- IP pair allocation (172.16.0.0/16 range, /30 subnets for isolation)
- Support concurrent VMs with unique TAP names and IP pairs

### Phase 4: Snapshot creation
- `BootAndSnapshot()` using firecracker-go-sdk: boot VM, wait for DH port reachable, pause, create snapshot (`machine.go`)
- Complete `dhg vm prepare` end-to-end
- Write `metadata.json` with version, IP, port

### Phase 5: Snapshot restore + exec integration
- `RestoreFromSnapshot()`: restore VM from snapshot files (~10ms) (`machine.go`)
- `runVM()` in `exec_vm_linux.go`: restore VM, launch runner.py in remote mode targeting VM IP, cleanup on exit
- Wire into `exec.Run()` — if `cfg.VMMode`, call `runVM()` instead of normal path
- Signal handling: SIGINT/SIGTERM triggers VM destroy + TAP teardown
- Stale instance cleanup on startup (`cleanup.go`)

### Phase 6: Polish
- `dhg vm clean` command
- Verbose output (`--verbose` shows restore timing, VM IP, etc.)
- JSON output mode compatibility
- Error messages with actionable hints

## Key Design Decisions

1. **runner.py unchanged** — VM mode is just remote mode with an auto-managed server
2. **Fresh VM per invocation** — restored from snapshot, destroyed after. Clean isolation, no state leakage
3. **Docker for rootfs** — reproducible, no custom build tooling needed
4. **TAP networking** — pydeephaven connects via TCP, which already works over TAP. No vsock proxy needed
5. **firecracker-go-sdk** — official Go SDK, production-ready, handles snapshot create/restore API
6. **Linux only** — Firecracker requires KVM. Build constraints ensure clean compilation on other platforms
7. **Separate prepare step** — `dhg vm prepare` is explicit (2-5 min), `dhg exec --vm` is fast (~10ms restore + network)

## Validation / Sequence of Operations for `dhg exec --vm`

```
1. Check prereqs (Linux, /dev/kvm, firecracker binary)
2. Resolve version, check snapshot exists
3. Cleanup stale instances from previous crashed runs
4. Create instance directory in ~/.dhg/vm/run/<id>/
5. Copy disk.ext4 as COW overlay
6. Setup TAP device with unique name + IP pair
7. Restore VM from snapshot (firecracker-go-sdk, ~10ms)
8. Verify DH port reachable on VM IP (should be instant since server was snapshotted running)
9. Launch runner.py --mode remote --host <VM_IP> --port 10000
10. Stream output (same as normal exec)
11. On completion/signal: stop VMM, teardown TAP, remove instance dir
```

## Testing

- `dhg vm status` — verify prerequisite detection works
- `dhg vm prepare --version <installed_version>` — end-to-end snapshot creation
- `dhg exec --vm -c "print('hello')"` — basic execution
- `dhg exec --vm -c "from deephaven import empty_table; t = empty_table(5)"` — table creation
- `dhg exec --vm --json -c "1+1"` — JSON mode
- `dhg exec --vm` on non-Linux — clean error message
- `dhg exec --vm` without snapshot — clear error pointing to `dhg vm prepare`
- Kill `dhg exec --vm` mid-execution — verify TAP cleanup
- Two concurrent `dhg exec --vm` — verify no resource collision

## New Dependency

```
github.com/firecracker-microvm/firecracker-go-sdk v1.1.1
```
