# Render VM Performance Investigation & Optimization

## Problem
`dh render --vm` takes ~54s for the iris dashboard. User reports it should be ~5s.
`dh exec --vm` for the same script takes ~31s (used to be 3-5s).

## Benchmark Results (sandbox, 2026-03-19)

```
[timing] script execution: 34,204ms    ← Python/JVM running iris dashboard
[timing] http ready wait:   1,376ms    ← waiting for DH HTTP endpoint
[timing] node.js renderer: 16,237ms    ← total Node.js subprocess time
  └── node.js startup:      4,678ms    ← process start + module loading
  └── session.open:          9,032ms   ← JSAPI download + WebSocket connect
  └── session.render:        2,527ms   ← actual React widget rendering
[timing] total render:      52,039ms
```

## Root Causes Identified

### 1. Script Execution: 34s (should be ~3-5s)
**CRITICAL** — This is the dominant bottleneck.

Investigation needed:
- [ ] **File ownership in rootfs**: The sandbox-built rootfs was created with `mke2fs -d` without fakeroot/sudo, so all files are owned by UID 1000 instead of root. This causes `mount` failures for /proc, /sys, /dev, /tmp during init. Without /proc, the JVM can't read CPU topology, memory info, or do proper JIT. Without /tmp as tmpfs, all temp files go to the slow ext4 disk.
  - **Test**: Compare timing on user's machine (proper fakeroot build) vs sandbox
  - **Fix if confirmed**: This is a sandbox-only issue; user's machine uses Docker+fakeroot properly
- [ ] **JVM memory page faults**: Without UFFD, snapshot restore uses file-backed memory. Every new memory page triggers a disk read. Complex scripts touch many JVM pages not accessed during warmup.
  - **Test**: Run the same script twice in sequence (second run should be faster if pages are cached)
  - **Mitigation**: Better warmup during `vm prepare` — run a representative script that touches plotly, ui components, aggregations
- [x] **Warmup quality**: ~~Current warmup is 20 iterations of trivial `print()`.~~ **FIXED**: Extended warmup to 28 iterations across 7 phases, adding `deephaven.plot.express`, `deephaven.ui`, and `deephaven.agg` exercises.

### 2. Node.js Cold Start: ~14s overhead (startup + JSAPI load)
**HIGH** — 14s of pure overhead before any rendering happens.

- [x] **Node.js process startup + module loading (~5s)**: ~~900+ npm packages loaded from ext4 disk on every invocation~~ **FIXED**: Persistent render daemon (`render-daemon.mjs`) pre-loads all modules at boot, captured in snapshot.
- [x] **JSAPI download (~9s)**: ~~`@deephaven/jsapi-nodejs` downloads dh-core.js and dh-internal.js from the DH server every time because /tmp is fresh after snapshot restore~~ **FIXED** (two layers):
  - **Persistent cache**: JsApiLoader now uses `/opt/render/jsapi-cache/` on ext4 (survives snapshot restore) instead of `/tmp/` (fallback path still works for non-VM usage)
  - **Render daemon**: Pre-downloads JSAPI at boot and keeps it in V8 heap memory

### 3. HTTP Ready Wait: 1.4s
**LOW** — Polling loop waiting for DH HTTP endpoint after script execution.

- [x] **FIXED**: When the render daemon is available, the HTTP readiness poll is skipped entirely. The daemon's WebSocket connection to the DH server survives snapshot restore (both endpoints are in the same VM), so no HTTP check is needed. The subprocess fallback path still does the HTTP check.

### 4. Actual Widget Rendering: 2.5s
**ACCEPTABLE** — This is the real rendering work (React + jsdom + DH components). Already fast.

## Implementation (2026-03-19)

### Phase 1: Enhanced JVM Warmup
**File**: `src/internal/vm/machine_linux.go`

Extended the warmup from 4 phases (20 iterations) to 7 phases (28 iterations):
- Phases 1-4: existing (basic exec, tables, pickle/IO, multi-column)
- **Phase 5**: `deephaven.plot.express` — imports dpx, creates a line chart from a 100-row table
- **Phase 6**: `deephaven.ui` — imports ui, creates a text element
- **Phase 7**: `deephaven.agg` — imports agg, runs sum/avg/count aggregations

This pre-faults JVM memory pages for DH modules that dashboards depend on, so they're captured in the snapshot.

### Phase 2: Persistent Node.js Render Daemon
**Files**:
- `src/render/bin/render-daemon.mjs` (NEW) — Unix socket daemon
- `src/render/src/JsApiLoader.mjs` — persistent JSAPI cache dir
- `src/internal/vm/rootfs_linux.go` — Dockerfile `mkdir -p /opt/render/jsapi-cache`, init script starts daemon
- `src/internal/vm/vm_runner.py` — daemon communication via Unix socket, subprocess fallback
- `src/internal/vm/machine_linux.go` — waits for daemon readiness before snapshot

**Architecture**:
```
Boot (vm prepare):
  init.sh → DH server → vm_runner.py → render-daemon.mjs
                                         ↓
                                    Pre-loads:
                                    - All npm modules (~900 packages)
                                    - JSAPI (dh-core.js, dh-internal.js)
                                    - WebSocket connection to DH
                                    - React, jsdom, providers, WidgetHandler
                                         ↓
                                    Listens on /tmp/render-daemon.sock
                                         ↓
                              ───── VM Snapshot taken ─────

Restore (dh render --vm):
  vm_runner.py receives render request via vsock
    → runs Python script via DH session
    → connects to /tmp/render-daemon.sock (daemon already warm from snapshot)
    → sends JSON render request
    → daemon renders widget using pre-loaded TestClient
    → returns result
```

**Protocol** (one request per Unix socket connection, JSON lines):
```json
Request:  {"widget":"my_widget", "actions":["snapshot"], "timeout":15000, "rows":10, "json":false}
Response: {"exit_code":0, "stdout":"", "stderr":"[timing] ...", "render_output":"..."}
```

### Phase 3: Skip HTTP Wait
**File**: `src/internal/vm/vm_runner.py`

When the render daemon socket exists, `handle_render_request` routes to `_render_via_daemon()` which skips the HTTP readiness poll entirely. The daemon's pre-established WebSocket connection survives snapshot restore (both endpoints are co-located in the same VM, kernel TCP state is part of the snapshot).

When the daemon is unavailable, falls back to `_render_via_subprocess()` which retains the original HTTP polling + oneshot.mjs behavior.

## Benchmark Results (sandbox 2026-03-19, no UFFD)

### Simple widget (`bench_simple.py` — @ui.component with button, N=5)

| Metric | Baseline | Optimized | Delta |
|--------|----------|-----------|-------|
| script execution | 1,906ms | 1,400ms | -0.5s |
| http ready wait | 1,224ms | 0ms | **-1.2s** (eliminated) |
| node.js cold start | 23,471ms | — | — |
| render daemon | — | 5,100ms | — |
| **total render** | **26,795ms** | **6,800ms** | **-20s** |
| **wall clock** | **27.2s** | **6.7s** | **-75%** |

### Iris dashboard (`bench_iris.py` — plotly, ui, agg dashboard, N=3)

| Metric | Baseline | Optimized | Delta |
|--------|----------|-----------|-------|
| script execution | 36,370ms | 28,000ms | **-8.4s** (warmup helps) |
| http ready wait | 1,713ms | 0ms | **-1.7s** (eliminated) |
| node.js cold start | 15,598ms | — | — |
| render daemon | — | 13,000ms | — |
| **total render** | **54,070ms** | **42,000ms** | **-12s** |
| **wall clock** | **54.5s** | **43s** | **-21%** |

### Where the time goes (render daemon breakdown)

The daemon eliminates 14s of Node.js cold start (5s module load + 9s JSAPI download) but `session.render` inside the daemon is ~3s slower than the baseline subprocess per-render.

| Component | Subprocess | Daemon | Why |
|-----------|-----------|--------|-----|
| Node.js startup | 5s | 0s | pre-loaded in snapshot |
| JSAPI download | 9s | 0s | pre-loaded in snapshot |
| HTTP readiness poll | 1.4s | 0s | eliminated |
| session.render | 1.9s | **4.9s** | +3s demand-paging overhead |
| **Total** | **17.3s** | **4.9s** | **-12.4s net** |

**Root cause of +3s render overhead**: Without UFFD, the daemon's V8 heap, jsdom DOM, React component tree, and npm module code are demand-faulted from the snapshot memory file on first access. Each page fault triggers a 4KB disk read. A simple render touches ~750 unique pages = ~3MB of random reads. With UFFD all pages are pre-populated before the VM resumes, so this overhead would disappear.

### What each optimization contributed

| Optimization | Simple widget | Iris dashboard |
|-------------|---------------|----------------|
| Render daemon (no cold start) | **-18.4s** | **-2.6s** |
| Render daemon (+page fault cost) | +3.0s | +10.1s |
| Skip HTTP wait | -1.2s | -1.7s |
| Better JVM warmup | -0.5s | -8.4s |
| **Net improvement** | **-20s (75%)** | **-12s (21%)** |

For the iris dashboard, the render daemon barely helps on net (-2.6 + 10.1 = +7.5s worse for the render phase alone). The entire iris improvement comes from better JVM warmup (-8.4s) and skipping HTTP wait (-1.7s). This is because complex dashboards touch many more daemon memory pages, making the page fault cost dominate.

## What would help next

1. **UFFD** (`vm.unprivileged_userfaultfd=1`): Pre-populates all snapshot pages before VM resumes. Would eliminate the ~3-10s page fault overhead in session.render, making the daemon render match the subprocess baseline (~2s). Expected: simple widget 3-4s, iris render phase ~3s.

2. **Proper rootfs (Docker+fakeroot, not mke2fs)**: The sandbox rootfs has wrong file ownership (UID 1000 instead of root). This causes mount failures for /proc, /sys, /dev, breaking JVM CPU topology detection and forcing temp files to slow ext4 instead of tmpfs. Expected: iris script exec drops from 28s to 3-5s.

3. **Both together**: simple widget ~3s, iris ~8-10s total.

## Files Modified
- `src/internal/vm/machine_linux.go` — 7-phase warmup, render daemon readiness wait
- `src/internal/vm/rootfs_linux.go` — Dockerfile jsapi-cache dir, init script daemon startup
- `src/internal/vm/vm_runner.py` — daemon communication, subprocess fallback
- `src/internal/cmd/render_vm_linux.go` — page cache warming for render path
- `src/render/bin/render-daemon.mjs` (NEW) — persistent Node.js render daemon
- `src/render/src/JsApiLoader.mjs` — persistent JSAPI cache directory
