# Plan: `dh kill <port>` — Stop a Deephaven server by port

## Overview

Add a `dh kill` subcommand that stops a Deephaven server running on a given port. Uses `discover_servers()` to find the server, then kills it appropriately based on source type:
- **dh-cli processes** (`dh serve`, `dh repl`, `dh exec`): `os.kill(pid, SIGTERM)`
- **Docker containers**: `docker stop <container_id>`
- **Standalone Java**: `os.kill(pid, SIGTERM)`

## Changes

### 1. Add `container_id` to `ServerInfo` (`src/deephaven_cli/discovery.py`)

Add optional `container_id: str | None = None` field to `ServerInfo` dataclass. Populate it in `_parse_docker_ps_output()` — the container ID is already parsed but currently discarded.

### 2. Add `kill_server()` function (`src/deephaven_cli/discovery.py`)

```python
def kill_server(port: int) -> tuple[bool, str]:
    """Kill the Deephaven server on the given port. Returns (success, message)."""
```

Logic:
1. Call `discover_servers()` to find server on that port
2. If not found → return `(False, "No Deephaven server found on port {port}")`
3. If `source == "docker"` → run `docker stop <container_id>`
4. Otherwise → `os.kill(pid, signal.SIGTERM)`
5. Return `(True, "Stopped {source} on port {port}")`

### 3. Add `kill` subparser and routing (`src/deephaven_cli/cli.py`)

- Add `kill` subparser with required `port` positional argument (type=int)
- Add `run_kill(port)` function that calls `kill_server()` and prints result
- Wire into command routing

### 4. Add tests (`tests/test_discovery.py`)

Unit test `kill_server` with mocked `discover_servers`:
- Port not found → error message
- Docker source → verify `docker stop` called with container_id
- dh-cli source → verify `os.kill` called with pid and SIGTERM

### 5. Update README

Add `dh kill` to docs.

## Files to modify

- `src/deephaven_cli/discovery.py` — add `container_id` field, `kill_server()` function
- `src/deephaven_cli/cli.py` — add `kill` subparser, `run_kill()`, routing
- `tests/test_discovery.py` — unit tests for kill logic
- `README.md` — document `dh kill`

## Verification

1. Run unit tests: `uv run pytest tests/test_discovery.py -v -k "not integration"` — all must pass
2. Manual: `dh list` → note ports
3. Manual: `dh kill 10000` → stops Docker container, confirms with message
4. Manual: `dh kill 46505` → stops dh repl process
5. Manual: `dh kill 99999` → "No Deephaven server found on port 99999"
