# Pre-start render daemon in pool VMs

## Context

Current pool render takes ~6.5s with this breakdown:

| Phase | Time | Can reduce? |
|-------|------|-------------|
| Node.js startup (module loading) | ~1500ms | Yes — eliminate |
| session.open (JSAPI + WS connect) | ~3600ms | Yes — eliminate |
| session.render (widget render) | ~970ms | Partially |
| Script execution | ~3000ms | No |

Node.js total (~6000ms) dominates because it starts a fresh process for every
render. The script overlaps but only hides ~3000ms of the ~6000ms Node.js time.

**Target**: ~4000ms (script + daemon render only)

## Approach

Start the `render-daemon.mjs` process during pool fill, so by checkout time
Node.js modules and the JSAPI connection are already loaded. Each render
request then goes through the daemon (~1200ms) instead of a fresh subprocess
(~6000ms).

This reuses existing infrastructure that was previously reverted because the
daemon was started *before* snapshot (V8 heap state didn't survive restore).
Starting it *after* restore avoids that problem entirely.

### Why the daemon is fast

The daemon calls `createTestClient()` once at startup, loading all npm modules
into the Node.js module cache and JSAPI files from the disk cache. For each
render request it creates a fresh `DaemonSession` → `createTestClient()`, but
this second call is fast (~200ms) because every `import()` and `loadDhModules()`
hits the in-process cache. Only a fresh WebSocket handshake is needed (~100ms).

## Changes

### 1. `src/internal/vm/pool_linux.go` — `fillOne()`

Extend the init request to also start the render daemon in the background:

```python
import os
os.system("mount -t tmpfs tmpfs /root/.cache/deephaven 2>/dev/null")
os.system("NODE_COMPILE_CACHE=/opt/render/.compile-cache node --no-warnings --import /opt/render/src/css-loader.mjs /opt/render/bin/render-daemon.mjs </dev/null >/dev/null 2>/dev/null &")
```

After the init request returns, poll for the daemon's ready marker file
(`/tmp/render_daemon_ready`) via a lightweight vsock exec. This ensures the VM
enters the ready queue only after the daemon is fully loaded. Timeout after ~10s
and put the VM in the queue anyway (renders fall back to subprocess).

### 2. `src/internal/vm/vm_runner.py` — `handle_render_request()`

Try the daemon first, fall back to the overlapped subprocess:

```python
def handle_render_request(session, request):
    # ... parse request, build stderr_lines ...

    # Run the script first (daemon or subprocess both need the widget to exist)
    run_script(wrapper)

    # Try daemon (fast path: ~1200ms)
    result = _render_via_daemon(widget, actions, ...)
    if result is not None:
        return result

    # Fall back to overlapped subprocess (slow path: ~6000ms)
    return _render_via_subprocess(widget, actions, ...)
```

Note: the script must run BEFORE sending to the daemon, because the daemon's
`session.render()` needs the widget to exist. The overlap optimization from
the previous commit doesn't apply here — the daemon handles session.open
internally and doesn't benefit from parallel script execution (it's already
pre-loaded). The script is the sequential bottleneck, then the daemon renders.

### 3. `src/internal/vm/vm_runner.py` — `_render_via_daemon()`

Add the `verbose` flag to the daemon request so the daemon returns its own
timing lines (`[timing] connect:`, `[timing] session.render:`).

### 4. No changes to `render-daemon.mjs`

The existing daemon code handles everything: pre-loading, socket listening,
per-request fresh session, action pipeline, output formatting.

## Files

| File | Change |
|------|--------|
| `src/internal/vm/pool_linux.go` | Start daemon in fillOne init + poll for readiness |
| `src/internal/vm/vm_runner.py` | Try daemon first in handle_render_request |

## Expected timeline

```
Pool fill:
  T=0    VM restore (~300ms)
  T=300  Init request: mount tmpfs + start daemon (background)
  T=400  Init returns → poll for daemon readiness
  T=2500 Daemon ready (/tmp/render_daemon_ready exists) → VM in ready queue

Render request:
  T=0    Script execution starts (~3000ms)
  T=3000 Script done → send to daemon
  T=3000 Daemon creates fresh session (~200ms) + renders (~970ms)
  T=4170 Done
```

**Expected wall time: ~4200ms** (down from ~6500ms, 35% faster)

## Verification

1. `DH_HOME=/workspace/.dh ./scripts/bench-render.sh --runs 3`
2. All renders must show `[OK]` (full snapshot validated)
3. Pool avg wall time should be ~4000-4500ms
4. Verify daemon timing lines appear in verbose output
5. Verify cold render still works (falls back to subprocess)
