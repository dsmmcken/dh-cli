# Optimize Cold Render Time (Implemented)

## Context

Current benchmarks:
- **Cold render: 9472ms** (script 2850ms sequential, then Node.js 5883ms)
- Pool render: 4324ms (daemon, already optimized)

Cold renders are 2.2x slower than pool because:
1. Script and Node.js run **sequentially** (overlap was removed when daemon was added)
2. No pre-loaded daemon — fresh Node.js subprocess pays full startup + JSAPI + provider loading
3. warmup.mjs only caches JsApiLoader + JSAPI, not providers/GoldenLayout/WidgetHandler

**Target: ~5000ms cold render** (close to pool's 4300ms)

## Changes

### 1. Start daemon on cold renders too (`vm_runner.py`)

The render daemon takes ~3.5s to start (first `createTestClient`) but once ready,
each render is only ~1s. By starting the daemon in the background while the script
runs (~2.8s), most of the startup overlaps. After the script finishes, poll briefly
for daemon readiness, then use it.

In `handle_render_request`, before running the script:
- Check if daemon socket exists (pool VMs: yes, cold: no)
- If not, start it in the background via `os.system()`
- Run script normally
- After script, poll `/tmp/render_daemon_ready` for up to 5s
- Try daemon; if still not ready, fall back to `_render_via_subprocess`

### 2. Enhance warmup.mjs to pre-cache more modules

Extended warmup.mjs to also import `react-dom/client` and `@deephaven/js-plugin-ui`
after loading JSAPI, populating V8 compile cache for the heaviest dynamic imports.

Note: using `createTestClient()` in warmup was tried but regressed pool renders
because the WebSocket connection competes with JVM warmup for CPU. The lighter
approach (module imports without server connections) avoids this.

### 3. Parallelize JSAPI load with jsdom creation in createTestClient

Split `JsApiLoader.load()` into `loadJSAPI()` (async, slow) and `createDom()` (sync, fast).
`createTestClient` now runs them concurrently via `Promise.all()`.
Small win (~100-200ms) but stacks with the other changes.

## Files

| File | Change |
|------|--------|
| `src/internal/vm/vm_runner.py` | Start daemon on cold renders, poll for readiness |
| `src/render/bin/warmup.mjs` | Import heavy dynamic modules for compile cache |
| `src/render/src/index.mjs` | Parallelize JSAPI load with jsdom creation |
| `src/render/src/JsApiLoader.mjs` | Add `loadJSAPI()` and `createDom()` methods |

## Results

Benchmark (3 runs each, iris dashboard):

| Metric | Before | After | Change |
|--------|--------|-------|--------|
| Cold render wall | 10021ms | 6230ms | **-38%** |
| Pool render avg | 4510ms | 4113ms | **-9%** |

Cold render breakdown: script=2900ms + daemon wait=1406ms + render=1028ms.
The daemon starts in background during script execution; after script, daemon
is ready within ~1.4s. Poll timeout is 2s (not 5s) to limit fallback penalty
if daemon fails. Pool improvement is from lighter warmup.mjs (less JVM warmup
contention at boot time).
