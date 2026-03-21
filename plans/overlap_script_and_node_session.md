# Overlap Script Execution with Node.js Session Setup

## Context

Pool render takes ~10.8s with two sequential phases:
1. **Python script execution** (~3700ms) — runs user code on the Deephaven server
2. **Node.js subprocess** (~6700ms) — session.open (~3600ms JSAPI load + WS connect) + session.render (~1400ms widget render)

The `session.open()` phase (JSAPI loading, WebSocket connection, jsdom setup, provider loading) is completely independent of the script's output. It only needs the Deephaven server to be running, which it already is in a pool VM. By starting Node.js before the script runs, these two phases overlap, saving ~3600ms.

**Expected result**: ~7200ms (down from ~10800ms)

## Changes

### 1. `src/internal/vm/vm_runner.py` — `handle_render_request()`

Replace the sequential flow with overlapped execution:

**Before:**
```
run_script(wrapper)          # blocks ~3700ms
_render_via_subprocess(...)  # blocks ~6700ms
                             # total: ~10400ms
```

**After:**
```
Popen(node_args)             # start Node.js (non-blocking)
run_script(wrapper)          # blocks ~3700ms (Node.js loading in parallel)
proc.communicate()           # wait for Node.js (~1400ms remaining for render)
                             # total: ~5100ms
```

Specifically in `handle_render_request()`:
- Move the HTTP readiness check and Node.js `Popen()` call BEFORE `session.run_script(wrapper)`
- Use `subprocess.Popen` instead of `subprocess.run` (non-blocking start)
- After the script completes, call `proc.communicate(timeout=...)` to collect output
- If the script fails, kill the Node.js process and return the script error
- Timing output adjusted to show overlapped vs sequential phases

The `_render_via_subprocess` function stays as-is for non-overlapped callers. A new helper `_start_node_renderer` returns the Popen handle, and `_collect_node_result` waits for it.

### 2. `src/internal/render/embedded/src/index.mjs` — `TestClient.render()`

Line 187: Change the `fetchVariableDefinition` timeout from hardcoded 3000ms to use the render timeout:

```js
// Before:
const definition = await this._widgetClient.fetchVariableDefinition(widgetName, 3000);

// After:
const definition = await this._widgetClient.fetchVariableDefinition(widgetName, timeout);
```

This is critical: with overlapped execution, Node.js may finish `session.open()` before the Python script creates the widget. The widget lookup must wait long enough for the script to complete. The render timeout (default 15s) is appropriate — it already represents the maximum acceptable wait.

### 3. No changes to `oneshot.mjs` or `session.mjs`

The Node.js process flow remains: `session.open()` → `session.render()`. The overlap is managed entirely by the Python runner starting Node.js earlier.

## Files

| File | Change |
|------|--------|
| `src/internal/vm/vm_runner.py` | Overlap script + Node.js in `handle_render_request()` |
| `src/internal/render/embedded/src/index.mjs` | Increase `fetchVariableDefinition` timeout |

## Verification

1. Run `dh render` with the iris dashboard script via pool: verify full snapshot output matches the pre-change output exactly
2. Compare timing: expect wall time drop from ~10800ms to ~7200ms
3. Run 3 consecutive pool renders to verify consistency
4. Run a cold render to verify no regression
