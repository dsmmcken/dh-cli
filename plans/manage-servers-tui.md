# Manage Servers TUI Screen

## Context

The "Running servers" option in the `dh` main menu currently just refreshes a static text display in the footer of the main menu. There's no way to interact with individual servers. The user wants to select a server from a list and either kill it or open its browser URL.

## Changes

### File: `src/deephaven_cli/tui/app.py`

**1. New `ServersScreen`** — replaces the inline `_refresh_servers()` text with a dedicated screen.

- Use an `OptionList` to display discovered servers (one per row, showing port, pid, source, script)
- Up/down to navigate, then:
  - `k` or `delete` — kill the selected server (confirm with status message, then refresh list)
  - `o` or `enter` — open `http://localhost:{port}` in browser
  - `escape` — back to main menu
- On mount, call `discover_servers()` and populate the list
- After a kill, re-run `discover_servers()` and rebuild the list
- Show "[dim]No running servers.[/dim]" when the list is empty
- Show a status line at the bottom for feedback ("Stopped server on port 10000", "Opened browser", etc.)

Layout:
```
┌──────────────────────────────────────────────────┐
│  Running Servers                                 │
│                                                  │
│  > :10000  pid 12345  dh-serve  dashboard.py     │
│    :10001  pid 12346  docker                     │
│    :8080   pid 99999  java                       │
│                                                  │
│  [dim]enter: open browser  k: kill  esc: back[/dim]    │
│  [status message here]                           │
└──────────────────────────────────────────────────┘
```

**2. Update `MainMenuScreen._handle_selection`** — change the `"servers"` case from `self._refresh_servers()` to `self.app.push_screen(ServersScreen())`.

**3. Remove `_refresh_servers()` and `#servers-info`** from `MainMenuScreen` — the server list is now its own screen, not inline in the main menu.

### Existing code to reuse

| What | Where |
|------|-------|
| `discover_servers() → list[ServerInfo]` | `src/deephaven_cli/discovery.py:30` |
| `kill_server(port) → (bool, str)` | `src/deephaven_cli/discovery.py:78` |
| `_open_browser(url)` | `src/deephaven_cli/cli.py:32` (handles WSL) |
| `ServerInfo` dataclass (port, pid, source, script, cwd, container_id) | `src/deephaven_cli/discovery.py:12` |
| `VersionsScreen` pattern (Screen + OptionList + escape binding) | `src/deephaven_cli/tui/app.py:518` |

### Implementation detail

- Each `Option` in the OptionList gets `id=str(server.port)` so we can look up which server is selected
- Keep a `_servers: list[ServerInfo]` on the screen so we can map selection back to the full object
- `_open_browser` is a module-level function in `cli.py` — move it to a shared location or import it directly (it's already top-level, not inside a class)

## Verification

1. `uv run dh` — open management TUI, press `l` to open servers screen
2. In another terminal: `uv run dh serve <script>` to start a server
3. Back in the TUI: servers screen should show the running server
4. Press `enter` on it — browser opens to `http://localhost:{port}`
5. Press `k` — server is killed, list refreshes, status shows confirmation
6. Press `escape` — returns to main menu
7. Run `uv run pytest tests/test_phase567_features.py -x -q` — existing tests still pass
