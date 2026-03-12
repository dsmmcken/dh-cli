# Plan: Fix missing stdout/stderr in `dh serve` scripts

## Problem

When a user runs `dh serve dashboard.py` and their script contains `print(2+2)`,
the output is never visible in the Deephaven web console. The print output
disappears into `/dev/null`.

## Root Cause

In `runner.py`, `run_serve()` suppresses both OS file descriptors AND Python's
`sys.stdout`/`sys.stderr` before starting the JVM server. The problem is that
during `server.start()`, the Deephaven server captures a reference to the current
`sys.stdout` for its web console output mechanism. If `sys.stdout` is a devnull
file at that point, the web console's output capture gets a dead reference.

Even after restoring `sys.stdout` in the finally block, the server's internal
reference still points to the devnull file.

## Solution (Implemented)

### 1. FD-only suppression in `run_serve()`

Only redirect OS file descriptors 1/2 to `/dev/null` during `server.start()`.
Do NOT replace `sys.stdout`/`sys.stderr` at the Python level. This way:
- JVM native output during startup is suppressed (via FD redirect)
- Python print() during startup is also suppressed (stdout writes to FD 1 = /dev/null)
- But the Deephaven server captures the **real** sys.stdout object
- After FDs are restored, the server's captured sys.stdout works correctly

### 2. Bypass sys.stdout for runner messages

After startup, the Deephaven server has captured `sys.stdout` for its web console.
Our runner's own messages (sentinel, status) must bypass this capture to avoid
appearing in the web console. Use `os.write(1, ...)` to write directly to FD 1
(the Go parent's stdout pipe), bypassing `sys.stdout` entirely.

### 3. Shutting down message to stderr

The "Shutting down..." message on Ctrl+C is sent to `sys.stderr` since it's an
operational message, not user output.

## Files Changed

- `src/internal/exec/runner.py` — `run_serve()`:
  - Removed `sys.stdout = devnull_file` / `sys.stderr = devnull_file` (and restore)
  - Replaced `print()` for sentinel/status with `os.write(1, ...)`
  - Changed "Shutting down" to use `sys.stderr`

## Verified

Tested via browser automation:
- `print(2+2)` in the web console now outputs `4`
- Sentinel and status messages no longer appear in the web console
- "Server started on port 10000" (Deephaven's own message) still appears (expected)
