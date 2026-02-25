# Plan: `dh list` — Discover Running Deephaven Servers

## Overview

Add a `dh list` subcommand that discovers all running Deephaven servers on the machine (dh-cli instances, Docker, standalone Java) using `/proc` on Linux and `lsof` on macOS. No new dependencies.

## Output Format

```
PORT   PID    SOURCE     SCRIPT              CWD
10000  67647  dh serve   dashboard.py        ~/projects/demo
10001  12345  docker     —                   —
10002  78901  dh repl    —                   ~/work
```

## Discovery Logic

### Linux (`/proc` parsing)

1. **Find listening sockets:** Parse `/proc/net/tcp` and `/proc/net/tcp6` for entries with state `0A` (LISTEN). Extract port (hex decode) and inode for each.

2. **Map inodes to PIDs:** For each `/proc/<pid>/fd/*`, `os.readlink()` to find `socket:[<inode>]` entries. Match against listening socket inodes.

3. **Identify Deephaven processes:** For each matched PID, read `/proc/<pid>/cmdline` and `/proc/<pid>/comm`:
   - `comm == "dh"` → dh-cli instance. Parse cmdline to get subcommand (`serve`/`repl`/`exec`) and script path.
   - `"io.deephaven"` in cmdline → Java Deephaven server. Check if running in Docker by looking for `containerd` or `/docker` in `/proc/<pid>/cgroup`.
   - Everything else → skip (not Deephaven).

4. **Get working directory:** `os.readlink(f"/proc/{pid}/cwd")` for dh-cli instances.

### macOS (`lsof` fallback)

1. Run `lsof -iTCP -sTCP:LISTEN -P -n -F pcn` (machine-parseable output).
2. For each listening process, check command name and full command line.
3. Same identification heuristics as Linux.

## Files to Modify

- **`src/deephaven_cli/cli.py`** — Add `list` subparser and `run_list()` routing
- **`src/deephaven_cli/discovery.py`** *(new)* — Discovery logic:
  - `discover_servers() -> list[ServerInfo]` — main entry point
  - `_discover_linux()` — `/proc` parsing
  - `_discover_macos()` — `lsof` parsing
  - `_parse_proc_net_tcp(content: str) -> list[ListeningSocket]` — pure parser, easily testable
  - `_classify_process(comm: str, cmdline: str) -> ServerInfo | None` — pure classifier, easily testable
  - `ServerInfo` dataclass: `port`, `pid`, `source` (dh serve/dh repl/dh exec/docker/java), `script` (optional), `cwd` (optional)
- **`tests/test_discovery.py`** *(new)* — Unit tests for discovery
- **`README.md`** — Document `dh list`

## CLI Interface

```
dh list                  # List all running Deephaven servers
```

No arguments needed. Exits with 0 always (empty list is not an error).

## Testing Strategy

### Unit tests (`tests/test_discovery.py`) — no server needed

The key parsing/classification functions are pure and can be tested with fake data:

**1. `/proc/net/tcp` parsing** — feed real-format strings, verify port/inode extraction:
```python
def test_parse_proc_net_tcp_listen_entry():
    # Real format from /proc/net/tcp with a LISTEN socket on port 10000
    content = "  sl  local_address ...\n   0: 00000000:2710 00000000:0000 0A ..."
    result = _parse_proc_net_tcp(content)
    assert result[0].port == 10000
    assert result[0].state == "0A"

def test_parse_proc_net_tcp_skips_non_listen():
    # State 01 = ESTABLISHED, should be skipped
    content = "  sl  local_address ...\n   0: 00000000:2710 0100007F:1234 01 ..."
    result = _parse_proc_net_tcp(content)
    assert len(result) == 0

def test_parse_hex_port():
    assert _hex_to_port("2710") == 10000
    assert _hex_to_port("6EC1") == 28353
```

**2. Process classification** — test the heuristics with fake cmdline/comm data:
```python
def test_classify_dh_serve():
    info = _classify_process(comm="dh", cmdline="dh\x00serve\x00dashboard.py")
    assert info.source == "dh serve"
    assert info.script == "dashboard.py"

def test_classify_dh_repl():
    info = _classify_process(comm="dh", cmdline="dh\x00repl")
    assert info.source == "dh repl"
    assert info.script is None

def test_classify_java_deephaven():
    info = _classify_process(comm="java", cmdline="java\x00-cp\x00...\x00io.deephaven.server.jetty.JettyMain")
    assert info.source == "java"

def test_classify_unrelated_process():
    info = _classify_process(comm="node", cmdline="node\x00server.js")
    assert info is None
```

**3. Output formatting** — verify table rendering:
```python
def test_format_empty():
    assert "No Deephaven servers found" in format_server_list([])

def test_format_single_server():
    output = format_server_list([ServerInfo(port=10000, pid=123, source="dh serve", script="app.py", cwd="/home/user")])
    assert "10000" in output
    assert "dh serve" in output
```

### Integration test (`tests/test_discovery.py`) — with live server

Follow the existing pattern from `test_phase8_app.py` (subprocess + timeout):

```python
@pytest.mark.integration
def test_list_finds_running_serve():
    """Start dh serve, then verify dh list finds it."""
    proc = subprocess.Popen(["dh", "serve", script_path, "--no-browser"], ...)
    time.sleep(30)  # wait for startup
    result = subprocess.run(["dh", "list"], capture_output=True, text=True, timeout=10)
    assert "dh serve" in result.stdout
    proc.terminate()

@pytest.mark.integration
def test_list_empty_when_no_servers():
    result = subprocess.run(["dh", "list"], capture_output=True, text=True, timeout=10)
    assert result.returncode == 0
```

## Manual Verification

1. Start `dh serve` on a port, run `dh list`, confirm it appears
2. Run `dh list` with no servers running — should show "No Deephaven servers found"
3. Run unit tests: `uv run pytest tests/test_discovery.py -v`
