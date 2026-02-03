# Plan: Fix Quiet Flag to Suppress All Server Notices

## Problem

When running `dh exec -q`, the following unwanted output still appears:

```
1 compiler directives added
# Starting io.deephaven.python.server.EmbeddedServer
deephaven.cacheDir=/home/dsmmcken/.cache/deephaven
deephaven.configDir=/home/dsmmcken/.config/deephaven
deephaven.dataDir=/home/dsmmcken/.local/share/deephaven
# io.deephaven.internal.log.LoggerFactoryServiceLoaderImpl: searching for...
# io.deephaven.internal.log.LoggerFactoryServiceLoaderImpl: found...
Server started on port 10000
```

These messages come from:
1. **JVM** - `1 compiler directives added`
2. **Deephaven Java server** - All the `#` prefixed lines and config paths
3. **Server startup notice** - `Server started on port 10000`

## Solution

Temporarily redirect stdout/stderr to `/dev/null` during server startup when quiet mode is enabled, then restore them for script execution.

## Files to Modify

- `src/deephaven_cli/cli.py` - Pass `quiet` flag to `DeephavenServer`
- `src/deephaven_cli/server.py` - Accept `quiet` flag and suppress output during `start()`

## Implementation

### 1. Modify `DeephavenServer` to accept `quiet` parameter

In `server.py`:
- Add `quiet: bool = False` parameter to `__init__`
- In `start()`, when `quiet=True`:
  - Save original `sys.stdout` and `sys.stderr`
  - Redirect both to `os.devnull` before calling `Server.start()`
  - Restore original streams after server is started

### 2. Pass `quiet` flag through `run_exec()`

In `cli.py` line 264:
- Change `DeephavenServer(port=port, jvm_args=jvm_args)`
- To `DeephavenServer(port=port, jvm_args=jvm_args, quiet=quiet)`

## Code Changes

### server.py

```python
import os

class DeephavenServer:
    def __init__(self, port: int = 10000, jvm_args: list[str] | None = None, quiet: bool = False):
        self.port = port
        self.jvm_args = jvm_args or ["-Xmx4g"]
        self.quiet = quiet
        self._server: Server | None = None
        self._started = False

    def start(self) -> DeephavenServer:
        if self._started:
            raise RuntimeError("Server already started")

        from deephaven_server import Server

        # Suppress JVM/server output when quiet
        if self.quiet:
            original_stdout = sys.stdout
            original_stderr = sys.stderr
            devnull = open(os.devnull, 'w')
            sys.stdout = devnull
            sys.stderr = devnull

        try:
            self._server = Server(port=self.port, jvm_args=self.jvm_args)
            self._server.start()
        finally:
            if self.quiet:
                sys.stdout = original_stdout
                sys.stderr = original_stderr
                devnull.close()

        self._started = True
        atexit.register(self._cleanup)
        return self
```

### cli.py

Line 264, change:
```python
with DeephavenServer(port=port, jvm_args=jvm_args) as server:
```
to:
```python
with DeephavenServer(port=port, jvm_args=jvm_args, quiet=quiet) as server:
```

## Verification

Run:
```bash
echo "print('hello')" | uv run dh exec -q -
```

Expected output (only):
```
hello
```

No JVM notices, no server startup messages, no config paths.
