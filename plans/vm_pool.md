# VM Pool: Pre-warmed Firecracker Instance Pool

## Problem

`dh exec --vm` currently restores a Firecracker snapshot on every invocation (~700-1000ms). The dominant costs are UFFD eager page loading (~600ms) and Firecracker process startup (~150ms). For interactive and batch workflows, this latency compounds. A pool of pre-restored, idle VMs would reduce per-invocation latency to ~20ms (vsock round-trip only).

## Design

### Architecture

A long-running **pool daemon** maintains N warm Firecracker VMs, each already restored from snapshot and idle in `accept()` on their vsock runner port. When `dh exec --vm` runs, it connects to the daemon over a Unix socket, receives a pre-warmed VM, executes code, and the VM is destroyed. The daemon immediately begins restoring a replacement.

```
dh exec --vm -c "..."
  │
  ├─ Probe /tmp/dh-pool-{uid}.sock
  │   ├─ Connected → send exec request → get warm VM → execute → done (~20ms)
  │   └─ ECONNREFUSED → fork daemon → wait for ready → retry (~1s first call)
  │
Pool daemon
  ├─ Unix socket: /tmp/dh-pool-{uid}.sock
  ├─ Warm VM queue: [vm-0, vm-1, ..., vm-N-1]
  ├─ On request: dequeue VM, start file server, proxy exec, destroy VM
  ├─ Backfill: restore replacement VM in background
  └─ Idle timer: no requests for {timeout} → destroy all VMs → exit
```

### Vsock Path Per Instance

The main technical constraint is vsock socket path contention. Currently all restores use `{snapDir}/vsock.sock` — two VMs from the same snapshot would collide on that path.

**Fix**: Each pool member gets its own vsock path in its instance directory:
- `~/.dh/vm/run/pool-0/vsock.sock`
- `~/.dh/vm/run/pool-1/vsock.sock`
- etc.

This requires `RestoreFromSnapshot` to accept a per-instance vsock UDS path instead of always deriving it from the snapshot directory. The vsock CID (guest=3) and ports (10000, 10001) remain the same — only the host-side UDS path changes. Firecracker's snapshot_vmstate captures device state but the UDS path is a host-side machine config parameter set at restore time.

### Pool Lifecycle

#### Starting

**Default: auto-start on first `dh exec --vm` call.**

`dh exec --vm` probes the daemon socket. If absent or connection refused, it forks the daemon process in the background and waits for it to signal readiness (daemon writes to a pipe or touches a ready file). The first call pays the cold start cost (~1s, same as today). All subsequent calls within the idle window get a pre-warmed VM.

Explicit start is also available for pre-warming:
```bash
dh vm pool start              # Start with defaults (N=1, idle=5m)
dh vm pool start -n 5         # 5 warm VMs
dh vm pool start --idle-timeout 30m  # 30 minute idle timeout
dh vm pool start --no-idle-timeout   # Keep alive until explicit stop
```

#### Stopping

**Default: auto-shutdown after idle timeout.**

The daemon tracks the timestamp of the last completed request. A background goroutine checks periodically; when `time.Since(lastRequest) > idleTimeout`, it:
1. Stops accepting new connections
2. Waits for in-flight requests to complete (with a hard deadline)
3. Destroys all warm VMs (`DestroyInstance` for each)
4. Removes the Unix socket
5. Exits

Explicit stop is also available:
```bash
dh vm pool stop               # Graceful shutdown (drain + destroy)
dh vm pool status             # Show pool state: size, idle time, version
```

#### Dynamic Resizing

The pool can be resized while running:
```bash
dh vm pool scale 3            # Scale up to 3 warm VMs
dh vm pool scale 1            # Scale back down to 1
```

The daemon receives the scale request over its Unix socket. Scaling up starts restoring additional VMs in background goroutines. Scaling down marks excess VMs for drain — they are destroyed as they become idle (not mid-request). The new size persists until the daemon restarts.

#### Idle Timeout Defaults

| Context | Default | Rationale |
|---------|---------|-----------|
| Auto-started | 5 minutes | Interactive use — don't waste memory if user walks away |
| Explicit start | 5 minutes | Same default, overridable with `--idle-timeout` or `--no-idle-timeout` |

Each warm VM costs ~4.5GB memory. At N=1 that's 4.5GB idle, which is reasonable for a default. Scale up with `-n` for batch workloads.

### Request Flow (Hot Path)

1. `dh exec --vm` connects to daemon Unix socket
2. Sends JSON request: `{"code": "...", "cwd": "/current/dir", "show_tables": true, ...}`
3. Daemon dequeues a warm VM from the pool
4. Daemon starts a file server bound to the VM's vsock path, rooted at the request's `cwd`
5. Daemon sends exec request to the VM's runner over vsock port 10000
6. Runner executes code, returns JSON response
7. Daemon relays response back to client, destroys the VM, closes file server
8. Daemon begins restoring a replacement VM in the background

**Expected hot-path latency**: ~20-30ms (vsock round-trip + file server startup + JSON marshaling). Compare to ~700-1000ms today.

### Request Flow (Cold Path — No Pool Running)

1. `dh exec --vm` probes daemon socket → ECONNREFUSED
2. Forks daemon process: `dh vm pool start --internal-autostart`
3. Daemon restores first VM, signals readiness
4. `dh exec --vm` retries connection, proceeds as hot path
5. Daemon continues restoring remaining N-1 VMs in background

**Expected cold-path latency**: ~1s (same as current non-pool behavior, since one VM must restore).

### Pool Backfill

When a VM is consumed, the daemon immediately starts restoring a replacement in a background goroutine. Backfill is the same as initial pool fill: `RestoreFromSnapshot` + UFFD setup + wait for vsock readiness.

If all VMs are consumed and a request arrives before backfill completes, the request blocks until the next VM is ready. This is equivalent to current non-pool latency (~1s). The daemon could optionally queue requests with a configurable `--max-wait` timeout.

### File Server Binding

The file server must be per-request, not per-VM, because:
- VMs are pre-created before the request arrives (CWD unknown)
- Different requests may come from different working directories
- The guest LD_PRELOAD library connects to the file server lazily (on first `/workspace/*` access)

Sequence: dequeue VM → start file server at `{vm.vsockPath}_10001` with `rootDir=request.cwd` → execute → stop file server.

The file server starts in ~5ms (just a Unix socket listen), which is negligible on the hot path.

### Version Pinning

The pool is tied to one Deephaven version (the resolved version at daemon start time). If a request specifies a different `--version`, the daemon has two options:

1. **Reject**: Return an error telling the client to use cold path or restart the pool
2. **Fallback**: Client detects version mismatch, bypasses pool, does cold restore

Option 2 is simpler and preserves backward compatibility. The daemon can log a warning.

If the user runs `dh vm pool start --version X`, the pool is explicitly pinned to version X.

### Daemon Process Model

The daemon is a single Go process (the `dh` binary itself, invoked as `dh vm pool start`). It:

- Writes its PID to `~/.dh/vm/pool.pid` (for `dh vm pool stop` to find it)
- Writes pool metadata to `~/.dh/vm/pool.json`: `{version, pool_size, idle_timeout, started_at, socket_path}`
- Redirects stdout/stderr to `~/.dh/vm/pool.log` when auto-started
- Handles SIGTERM gracefully (drain + destroy)
- Handles SIGINT the same as SIGTERM

Instance directories for pool VMs use a `pool-` prefix:
```
~/.dh/vm/run/
├── pool-0/          # Warm VM instance
│   ├── instance.json
│   ├── vsock.sock
│   └── firecracker.sock
├── pool-1/          # Warm VM instance
│   ├── instance.json
│   ├── vsock.sock
│   └── firecracker.sock
└── exec-1709012345/ # Non-pool ephemeral instance (backward compat)
```

### Pool Status

```bash
$ dh vm pool status
Pool: running (PID 12345)
Version: 0.36.0
Pool size: 2/2 ready
Idle: 45s (timeout: 5m0s)
Socket: /tmp/dh-pool-1000.sock

$ dh vm pool status --json
{"running":true,"pid":12345,"version":"0.36.0","ready":2,"target":2,"idle_seconds":45,"idle_timeout_seconds":300}
```

## Implementation Plan

### Phase 1: Per-instance vsock paths

Modify `RestoreFromSnapshot` to accept a vsock UDS path parameter instead of always using `{snapDir}/vsock.sock`. This is the prerequisite for running multiple VMs from the same snapshot simultaneously.

**Files**:
- `src/internal/vm/machine_linux.go` — add `VsockUDSPath` field to `VMConfig` or as parameter to `RestoreFromSnapshot`
- `src/internal/exec/exec_vm_linux.go` — pass instance-specific vsock path
- `src/internal/vm/fileserver_linux.go` — derive file server socket from instance vsock path

### Phase 2: Pool daemon core

Implement the pool daemon with warm VM queue, backfill, and Unix socket protocol.

**Files** (new):
- `src/internal/vm/pool_linux.go` — pool manager: VM queue, backfill goroutine, idle timer
- `src/internal/vm/pool_protocol.go` — Unix socket JSON protocol between client and daemon
- `src/internal/cmd/vm_pool.go` — `dh vm pool {start,stop,status}` commands

### Phase 3: Auto-start integration

Modify `dh exec --vm` to probe for pool daemon and use it when available, with fallback to cold restore.

**Files**:
- `src/internal/exec/exec_vm_linux.go` — add pool probe + request path before cold restore path

### Phase 4: Idle auto-shutdown

Add idle timer to the daemon. Configurable timeout with `--idle-timeout` flag.

**Files**:
- `src/internal/vm/pool_linux.go` — idle timer goroutine, graceful shutdown sequence

## Resource Budget

| Pool Size | Memory | vCPUs (shared) | Warm-up Time |
|-----------|--------|-----------------|--------------|
| 1 (default) | 4.5 GB | 2 | ~1s |
| 2 | 9 GB | 4 | ~2s |
| 3 | 13.5 GB | 6 | ~3s |
| 5 | 22.5 GB | 10 | ~5s |

Pool size of 1 is the default — one warm VM ready, backfill starts immediately after use. For rapid-fire batch workloads, `-n 2` keeps one executing while one warms up.

## CLI Surface

```
dh vm pool start [flags]     Start the VM pool daemon
  -n, --pool-size int         Number of warm VMs (default 1)
  --idle-timeout duration     Auto-shutdown after idle period (default 5m, 0 to disable)
  --version string            Deephaven version (default: resolved)
  -v, --verbose               Verbose logging

dh vm pool stop              Stop the pool daemon gracefully
dh vm pool scale N           Resize the pool (add or drain VMs live)
dh vm pool status            Show pool state
dh vm pool status --json     JSON output
```

## Open Questions

1. **Pool size auto-tuning**: Should the daemon adjust pool size based on request rate? Probably not for v1 — keep it simple.
2. **Multiple versions**: Should one daemon support multiple version pools? Probably not for v1 — one version per daemon, restart to switch.
3. **Disk overlay**: Currently each VM shares the same `disk.ext4`. Firecracker opens it read-write but changes are ephemeral (VM is destroyed). With concurrent VMs, we may need per-instance CoW overlays (e.g., device-mapper snapshot or a copy). Needs testing — Firecracker may handle this via its own block device layer.
4. **UFFD sharing**: Each pool member currently gets its own UFFD handler + mmap of snapshot_mem. The page cache is shared at the kernel level, so the actual I/O cost is only paid once. The mmap + handler goroutines are the per-instance overhead (~10MB each, negligible).
