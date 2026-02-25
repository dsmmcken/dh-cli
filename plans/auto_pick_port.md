# Plan: Auto-Pick Port When Default Port is Occupied

**Status: ✅ IMPLEMENTED**

## Overview

When the default port 10000 is already in use, automatically find and use an available port instead of failing. Using port 0 tells the OS to assign an available ephemeral port.

## Current Behavior

- Default port is hardcoded as 10000 across CLI, server, and client
- If port 10000 is occupied, the server fails to start with an error
- User must manually specify `--port` to use a different port

## Proposed Behavior

- Default remains 10000 for predictability
- If port 10000 (or any specified port) is occupied, automatically try port 0 to get an available port
- Print the actual port being used so users know where to connect

## Implementation Steps

### 1. Update `DeephavenServer` class (`src/deephaven_cli/server.py`)

- Add port availability check before starting
- If specified port is unavailable, fall back to port 0
- After server starts, retrieve the actual bound port from the `Server` instance
- Store the actual port for later access

```python
def _is_port_available(self, port: int) -> bool:
    """Check if a port is available for binding."""
    import socket
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        try:
            s.bind(('localhost', port))
            return True
        except OSError:
            return False

def start(self) -> int:
    """Start server, returns actual port used."""
    port_to_use = self.port
    if not self._is_port_available(port_to_use):
        print(f"Port {port_to_use} is occupied, finding available port...")
        port_to_use = 0  # Let OS assign

    self._server = Server(port=port_to_use, jvm_args=self.jvm_args)
    self._server.start()

    # Get actual port (need to check deephaven_server API for this)
    self.actual_port = self._server.port  # or similar attribute
    return self.actual_port
```

### 2. Verify `deephaven_server.Server` API

- Check if `Server` instance exposes the actual bound port after starting
- If using port 0, we need to retrieve what port was assigned
- Look at deephaven_server documentation/source for the attribute name

### 3. Update CLI commands (`src/deephaven_cli/cli.py`)

- After server starts, get the actual port
- Pass the actual port (not requested port) to client and console
- Print the actual port being used

```python
with DeephavenServer(port=port, ...) as server:
    actual_port = server.actual_port
    if actual_port != port:
        print(f"Using port {actual_port}")
    with DeephavenClient(port=actual_port) as client:
        # ...
```

### 4. Update tests

- Add test for port fallback behavior
- Test that occupied port triggers auto-selection
- Test that actual port is correctly propagated

## Files to Modify

1. `src/deephaven_cli/server.py` - Add port checking and fallback logic
2. `src/deephaven_cli/cli.py` - Use actual port from server for client/console
3. `tests/test_phase2_server.py` - Add tests for port fallback

## Confirmed Findings

**✅ `deephaven_server.Server.port` returns actual bound port:**
```
$ uv run python -c "from deephaven_server import Server; s = Server(port=0); s.start(); print(s.port)"
Server started on port 41631
41631
```

The server already prints "Server started on port XXXX" so users will see the port.

## Remaining Decisions

1. Should auto-port be default behavior, or require `--port 0` explicitly?
2. Should we add a property `DeephavenServer.actual_port` for clarity?

## Alternative Approaches

### A. Sequential port scanning
Instead of port 0, try 10001, 10002, etc. until finding an available port. More predictable but slower.

### B. Fail fast with helpful message
Keep current behavior but improve error message: "Port 10000 in use. Try: dh repl --port 0"

### C. Always use port 0 (rejected)
Would break predictability - users wouldn't know which port to connect to.
