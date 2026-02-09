# Plan: Slim Install with nvm-like Version Manager

## Context

Currently `dh-cli` requires users to pre-install Python 3.13+, Java 17+, and bundles 250MB+ of Deephaven dependencies on first install. Users want a lightweight `uv tool install deephaven-cli` that gives them the `dh` command immediately, with heavy dependencies (deephaven-server, plugins) and Java managed lazily per-version -- similar to how `nvm` manages Node.js versions.

**Goals:**
- `uv tool install deephaven-cli` installs a thin wrapper (<5MB, no Deephaven deps)
- `dh install 41.1` creates an isolated environment with the heavy Deephaven deps for that version
- `dh` with no args launches an interactive TUI (first-run wizard on first use)
- Java auto-detected or downloaded to `~/.dh/java/` (no sudo needed)
- `.dhrc` files pin Deephaven versions per-project (like `.nvmrc`)
- All interactive features also work non-interactively (for AI agents)

---

## Architecture

### Two-Mode CLI

The same `deephaven-cli` package runs in two modes:

1. **Manager mode** (global thin wrapper, no Deephaven deps available): Handles `install`, `uninstall`, `use`, `versions`, `java`, `config`, TUI, and delegates runtime commands.
2. **Runtime mode** (after version activation): Handles `repl`, `exec`, `serve` with full Deephaven capabilities.

The key mechanism: when a runtime command is invoked, the thin wrapper resolves the target version, uses `site.addsitedir()` to add that version's venv site-packages to `sys.path`, then dispatches the command. Since all Deephaven imports are already lazy (function-level, not module-level), they resolve to the activated version's packages.

### Directory Layout

```
~/.dh/
├── config.toml              # default_version, plugin prefs
├── java/
│   └── temurin-21.0.x/      # Downloaded JDK (Adoptium API)
├── versions/
│   ├── 41.1/
│   │   ├── .venv/           # uv venv with deephaven-server, pydeephaven, plugins
│   │   └── meta.toml        # Installed packages/dates
│   └── 0.37.0/
│       ├── .venv/
│       └── meta.toml
└── cache/                   # Download cache (JDK tarballs)
```

### Source Layout (new files)

```
src/deephaven_cli/
├── cli.py                   # MODIFIED: two-phase dispatch (manager vs runtime)
├── server.py                # UNCHANGED
├── client.py                # UNCHANGED
├── discovery.py             # UNCHANGED
├── repl/                    # REWRITTEN: Textual-based TUI REPL (replaces prompt_toolkit)
│   ├── app.py               # Main Textual App for REPL
│   ├── widgets/
│   │   ├── output.py        # Output panel (RichLog + virtual DataTable)
│   │   ├── sidebar.py       # Global variables list
│   │   ├── log_panel.py     # Scrollable CLI logs
│   │   ├── input_bar.py     # Command input with history/completion
│   │   └── table_view.py    # Virtual-scrolling DataTable for Deephaven tables
│   ├── executor.py          # KEPT: code execution with output capture
│   └── console.py           # REWRITTEN: orchestrates Textual app
├── manager/                 # NEW
│   ├── __init__.py
│   ├── config.py            # ~/.dh/ paths, config.toml, .dhrc resolution
│   ├── versions.py          # install, uninstall, list versions via uv
│   ├── activate.py          # site.addsitedir() delegation
│   ├── java.py              # Java detection + Temurin download
│   └── pypi.py              # PyPI API for version discovery
└── tui/                     # NEW: management TUI (setup wizard, main menu)
    ├── __init__.py
    └── app.py               # First-run wizard + interactive menu
```

---

## Commands

### New Management Commands

| Command | Description |
|---------|-------------|
| `dh` (no args, interactive) | Launch TUI / first-run wizard |
| `dh install [VERSION]` | Install a Deephaven version (default: latest) |
| `dh uninstall VERSION` | Remove an installed version |
| `dh use VERSION` | Set global default version |
| `dh use VERSION --local` | Write `.dhrc` in current directory |
| `dh versions` | List installed versions |
| `dh versions --remote` | Also show available versions from PyPI |
| `dh java` | Show Java status |
| `dh java install` | Download Eclipse Temurin JDK 21 to `~/.dh/java/` |
| `dh doctor` | Check environment health (Java, versions, uv) |

### Modified Runtime Commands (add `--version` flag)

```
dh repl --version 41.1
dh exec --version 0.37.0 script.py
dh serve --version 41.1 app.py
```

### Existing Commands (unchanged behavior)

`dh list`, `dh kill`, `dh lint`, `dh format`, `dh typecheck` -- these don't need Deephaven deps and work as-is.

---

## Version Resolution Precedence

1. `--version` CLI flag
2. `DH_VERSION` environment variable
3. `.dhrc` file (walk up from cwd, like `.nvmrc`)
4. `~/.dh/config.toml` `default_version`
5. Latest installed version
6. Nothing installed → first-run wizard (interactive) or error (non-interactive)

### `.dhrc` Format

```toml
version = "41.1"
```

### `config.toml` Format

```toml
default_version = "41.1"

[plugins]
deephaven-plugin-ui = "latest"
deephaven-plugin-plotly-express = "latest"
deephaven-plugin-theme-pack = "latest"
```

---

## Activation Mechanism (`manager/activate.py`)

```python
def activate_version(version: str) -> None:
    """Add version's site-packages to sys.path so lazy imports find Deephaven packages."""
    venv_dir = DH_HOME / "versions" / version / ".venv"
    site_packages = find_site_packages(venv_dir)  # .venv/lib/python3.*/site-packages
    site.addsitedir(str(site_packages))            # Also processes .pth files
    set_java_home_if_needed()
```

**Why this works:** The existing codebase has zero module-level Deephaven imports:
- `server.py:46` — `from deephaven_server import Server` inside `start()`
- `client.py:36` — `from pydeephaven import Session` inside `connect()`
- `discovery.py` — no Deephaven imports at all

After `site.addsitedir()`, these lazy imports resolve to the activated version's packages.

---

## Java Management (`manager/java.py`)

1. **Detection**: Check `JAVA_HOME` → `java` on PATH → `~/.dh/java/` (require Java 17+)
2. **Installation**: Download Eclipse Temurin JDK 21 via Adoptium API (`https://api.adoptium.net/v3/assets/latest/21/hotspot`)
3. **Platform detection**: `platform.system()` (Linux/Darwin) + `platform.machine()` (x64/aarch64)
4. **Extract** to `~/.dh/java/jdk-21.0.x/`
5. **Set** `JAVA_HOME` in `os.environ` before any Deephaven import

No sudo required. Download uses stdlib `urllib.request`.

---

## Version Installation (`manager/versions.py`)

`dh install 41.1` runs:
```bash
uv venv ~/.dh/versions/41.1/.venv --python 3.13
uv pip install --python ~/.dh/versions/41.1/.venv/bin/python \
    "deephaven-server==41.1" \
    "pydeephaven==41.1" \
    deephaven-plugin-ui \
    deephaven-plugin-plotly-express \
    deephaven-plugin-theme-pack
```

Plugin versions are not pinned — `uv` resolves compatible versions automatically. Exact installed versions are recorded in `meta.toml`.

Available versions are fetched from PyPI JSON API (`https://pypi.org/pypi/deephaven-server/json`) using stdlib only.

---

## First-Run Experience

### Interactive — Textual Wizard (`dh` or `dh repl`, no versions installed)

A multi-screen Textual wizard guides the user through setup:

**Screen 1: Welcome**
```
┌────────────────────────────────────────────────────┐
│                                                    │
│            ▄▄▄   Deephaven CLI                     │
│           ▀▀▀▀▀  v0.2.0                            │
│                                                    │
│   Welcome! Let's get your environment ready.       │
│                                                    │
│   This wizard will:                                │
│     1. Check for Java (or install it)              │
│     2. Install a Deephaven engine version          │
│     3. Get you into a REPL                         │
│                                                    │
│                        ┌───────────────────┐       │
│                        │   Get Started →   │       │
│                        └───────────────────┘       │
└────────────────────────────────────────────────────┘
```

**Screen 2: Java Detection**
```
┌────────────────────────────────────────────────────┐
│  Step 1 of 3 — Java                     ━━━○━━━━  │
├────────────────────────────────────────────────────┤
│                                                    │
│   Checking for Java 17+...                         │
│                                                    │
│   ✗ No Java installation found                     │
│                                                    │
│   Deephaven requires a JDK to run its engine.      │
│   We'll install Eclipse Temurin 21 (LTS) to:       │
│   ~/.dh/java/jdk-21.0.5+11                         │
│                                                    │
│   No sudo required. ~190MB download.               │
│                                                    │
│   ┌──────────────┐  ┌─────────────────────┐       │
│   │   Install →  │  │  I have Java (skip) │       │
│   └──────────────┘  └─────────────────────┘       │
└────────────────────────────────────────────────────┘
```

If Java IS found:
```
│   ✓ Java 21.0.5 found at /usr/lib/jvm/java-21      │
│                                                      │
│                         ┌──────────┐                 │
│                         │  Next →  │                 │
│                         └──────────┘                 │
```

If installing, shows a ProgressBar:
```
│   Downloading Eclipse Temurin 21...                │
│   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━  72%      │
│   138 MB / 192 MB  •  12.4 MB/s  •  ~4s remaining │
```

**Screen 3: Version Selection**
```
┌────────────────────────────────────────────────────┐
│  Step 2 of 3 — Deephaven Version        ━━━━━○━━  │
├────────────────────────────────────────────────────┤
│                                                    │
│   Select a version to install:                     │
│                                                    │
│   ┌────────────────────────────────────────────┐   │
│   │  ▸ 41.1          latest                    │   │
│   │    41.0                                    │   │
│   │    0.40.9                                  │   │
│   │    0.40.8                                  │   │
│   │    0.39.8                                  │   │
│   │    0.38.0                                  │   │
│   │    0.37.6                                  │   │
│   │    ↓ Show older versions...                │   │
│   └────────────────────────────────────────────┘   │
│                                                    │
│   ┌──────────────┐                                 │
│   │  Install →   │                                 │
│   └──────────────┘                                 │
└────────────────────────────────────────────────────┘
```

Then progress:
```
│   Installing Deephaven 41.1...                     │
│   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━  45%      │
│                                                    │
│   ✓ deephaven-server==41.1                         │
│   ✓ pydeephaven==41.1                              │
│   ◔ deephaven-plugin-ui                            │
│   ○ deephaven-plugin-plotly-express                │
│   ○ deephaven-plugin-theme-pack                    │
```

**Screen 4: Done**
```
┌────────────────────────────────────────────────────┐
│  Setup Complete                          ━━━━━━━●  │
├────────────────────────────────────────────────────┤
│                                                    │
│   ✓ Java 21.0.5            ~/.dh/java/jdk-21.0.5  │
│   ✓ Deephaven 41.1         set as default          │
│   ✓ Plugins                ui, plotly, theme-pack  │
│                                                    │
│   Quick start:                                     │
│     dh repl              Interactive REPL           │
│     dh exec script.py    Run a script               │
│     dh serve app.py      Start a server             │
│                                                    │
│   ┌──────────────────┐  ┌────────────────┐         │
│   │  Launch REPL →   │  │     Done       │         │
│   └──────────────────┘  └────────────────┘         │
└────────────────────────────────────────────────────┘
```

### Non-interactive (`dh repl` piped/scripted, no versions installed)

Plain text error — no TUI:

```
Error: No Deephaven version installed.

  dh install latest    Install the latest version
  dh install 41.1      Install a specific version
  dh                   Interactive setup wizard
```

---

## Dependency Changes (`pyproject.toml`)

**Before (current):**
```toml
dependencies = [
    "deephaven-server>=0.37.0",      # 250MB
    "pydeephaven>=0.37.0",
    "deephaven-plugin-ui>=0.32.1",
    "deephaven-plugin-plotly-express>=0.18.0",
    "deephaven-plugin-theme-pack>=0.2.0",
    "prompt_toolkit>=3.0.0",
    "pygments>=2.0.0",
    "ruff>=0.8.0",
    "ty>=0.0.1a7",
]
```

**After (thin wrapper):**
```toml
dependencies = [
    "textual[syntax]>=1.0.0",          # TUI framework + tree-sitter syntax highlighting (includes rich)
    "textual-fastdatatable>=0.10.0",   # Virtual-scrolling DataTable with Arrow backend
]

[project.optional-dependencies]
dev = ["pytest>=8.0", "pytest-timeout>=2.0", "ruff>=0.8.0", "ty>=0.0.1a7"]
```

Notes:
- `textual[syntax]` includes tree-sitter for Python syntax highlighting in the REPL input
- `textual` includes `rich` and `pygments` transitively
- `prompt_toolkit` is dropped — Textual replaces it for both the REPL and the management TUI
- `textual-fastdatatable` provides ArrowBackend for virtual-scrolling tables (pairs with Deephaven's `table.to_arrow()`)
- `ruff` and `ty` move to dev deps. `dh lint`/`dh format`/`dh typecheck` check availability at runtime

---

## TUI Framework

Use [Textual](https://textual.textualize.io/) for **everything interactive** — both the REPL and the management TUI. This replaces `prompt_toolkit` entirely.

Textual provides:
- Full terminal app with widgets, layouts, CSS-like styling
- Reactive UI components (DataTable, TextArea, Input, RichLog, ProgressBar)
- Tree-sitter syntax highlighting via `textual[syntax]`
- Built on Rich for beautiful rendering
- Pure Python, ~5MB installed

---

## REPL TUI (`dh repl`)

The REPL is a full Textual application, not a line-based prompt.

### Layout

```
┌─────────────────────────────────┬──────────────────┐
│                                 │ Variables         │
│  Output Area                    │                   │
│  (last command result:          │  t          Table │
│   text, DataTable, or error)    │  stocks     Table │
│                                 │  df      DataFrame│
│                                 │  x           int  │
│                                 │  config     dict  │
│                                 │                   │
│                                 │                   │
├─────────────────────────────────┴──────────────────┤  ← flex
│ [12:01:03] Server started on port 10000            │
│ [12:01:05] Created table 't' (1000 rows, 3 cols)  │
│ [12:01:07] Query completed in 0.3s                 │  ← 3 rows, scrollable
├────────────────────────────────────────────────────┤
│ >>> t = empty_table(1000).update("X = i")          │  ← 1 row, input
├────────────────────────────────────────────────────┤
│ localhost:10000 │ v41.1 │ 3 tables │ 1000 rows     │  ← footer
└────────────────────────────────────────────────────┘
```

| Region | Widget | Height |
|--------|--------|--------|
| Output + Sidebar | `Horizontal(OutputPanel, Sidebar)` | Flex (fills remaining) |
| CLI logs | `RichLog` (scrollable) | 3 rows, docked |
| CLI input | `Input` or `TextArea` with syntax highlighting | 1 row, docked |
| Footer | `Footer` (custom) | 1 row, docked |

### Output Panel (`repl/widgets/output.py`)

Displays the result of the last command:
- **Text output**: Rendered in a `RichLog` (stdout, print statements, repr values)
- **Table output**: When the result is a Deephaven table, renders a **virtual-scrolling DataTable**
- **Errors**: Rendered with syntax-highlighted tracebacks
- **Mixed**: Log-style interleaving of commands and their outputs (scrollable history)

### Virtual-Scrolling DataTable (`repl/widgets/table_view.py`)

For displaying Deephaven tables of any size (including billions of rows):

**Strategy**: Use `textual-fastdatatable` with ArrowBackend, fed by Deephaven's server-side slicing.

```python
# Key APIs from Deephaven:
table.size          # Row count — no data transfer
table.slice(start, stop)  # Server-side row range — only transfers viewport
table.to_arrow()    # Convert (sliced) table to Arrow — for fastdatatable
table.is_refreshing # Detect ticking/live tables
```

**Virtual scrolling flow**:
1. Get `table.size` for total row count (metadata only, instant)
2. Report virtual size to Textual's scroll system
3. On scroll event, compute visible row range from scroll offset
4. Call `table.slice(start, end).to_arrow()` — only fetches visible rows
5. Feed Arrow data to `textual-fastdatatable` ArrowBackend for rendering
6. Cache nearby pages for smooth scrolling

**For ticking/live tables** (`table.is_refreshing == True`):
- Poll `table.size` periodically to update virtual size
- Re-fetch visible viewport on each tick
- Show a "live" indicator in the footer

This approach handles billion-row tables because only ~50-100 rows are ever in memory at once.

### Sidebar (`repl/widgets/sidebar.py`)

Displays all global variables in the Deephaven session:
- Variable name + type (Table, DataFrame, int, str, etc.)
- Tables show row count and column count
- Clicking/selecting a variable opens it in the output panel
- For Tables: opens the virtual DataTable view
- For other types: shows `repr()` in the output panel
- Live-updates after each command execution

**Implementation**: Query the server's namespace after each command:
```python
# Server-side script to list globals
client.run_script("""
import json
_dh_vars = {k: type(v).__name__ for k, v in globals().items() if not k.startswith('_')}
""")
```

### Log Panel (`repl/widgets/log_panel.py`)

- Textual `RichLog` widget, 3 rows tall, scrollable
- Shows timestamped events: server start, table creation, query timing, errors
- Auto-scrolls to latest entry
- Distinct from command output (this is operational logs, not command results)

### Input Bar (`repl/widgets/input_bar.py`)

- Textual `TextArea` (1 row default, expands for multi-line with Shift+Enter)
- Python syntax highlighting via tree-sitter
- **Command history**: Up/Down arrows cycle through previous commands (stored in `~/.dh/history`)
- **Tab completion**: Query server namespace for variable names, table columns
- **Submit**: Enter executes the command
- **Multi-line**: Shift+Enter adds a newline (auto-detects incomplete statements)

### Footer

Custom Textual `Footer` showing:
- Connection: `localhost:10000` or `remote.example.com:8080`
- Version: `v41.1`
- Table count: `3 tables`
- Current table row count (if a table is displayed): `1,000,000 rows`
- Mode indicator: `[local]` or `[remote]`

---

## Management TUI (`dh` with no args)

Also a Textual app, separate from the REPL.

### First-Run Wizard (no versions installed)

Screens:
1. **Welcome** — splash with Deephaven branding
2. **Java Check** — detect Java, offer install if missing (ProgressBar for download)
3. **Version Picker** — OptionList of available versions from PyPI (highlight latest)
4. **Plugin Selection** — Checkbox list (defaults pre-checked: ui, plotly, theme-pack)
5. **Install Progress** — ProgressBar for version installation
6. **Done** — tips for getting started, key commands

### Main Menu (versions installed)

```
┌────────────────────────────────────────────────────┐
│  Deephaven CLI                    v41.1 │ Java 21  │
├────────────────────────────────────────────────────┤
│                                                    │
│  [r] Start REPL          [s] Serve a script        │
│  [e] Execute a script    [v] Manage versions       │
│  [l] Running servers     [j] Java                  │
│  [c] Config              [q] Quit                  │
│                                                    │
├────────────────────────────────────────────────────┤
│  Running servers:                                  │
│  PORT   PID    TYPE     UPTIME                     │
│  10000  12345  dh-cli   2h 15m                     │
│  10001  12346  docker   45m                        │
│                                                    │
└────────────────────────────────────────────────────┘
```

Submenus:
- **Manage versions**: DataTable of installed versions, install new, uninstall, set default
- **Running servers**: Live DataTable from `discover_servers()`, kill with keybinding
- **Config**: Edit default version, plugin prefs, Java path

---

## Implementation Phases

### Phase 1: Manager scaffolding + dependency split
- Create `manager/` package: `config.py`, `activate.py`, `__init__.py`
- Create `tui/` package stub
- Update `pyproject.toml`: drop deephaven deps + prompt_toolkit, add textual + textual-fastdatatable
- Restructure `cli.py` for two-phase dispatch (manager commands vs runtime commands)
- Add `--version` flag to repl/exec/serve parsers

### Phase 2: Version management
- Implement `manager/pypi.py` (query available versions from PyPI via stdlib urllib)
- Implement `manager/versions.py` (install, uninstall, list via uv subprocess)
- Implement `manager/config.py` (config.toml, .dhrc discovery, version resolution)
- Add `dh install`, `dh uninstall`, `dh versions`, `dh use` commands

### Phase 3: Activation + delegation
- Implement `manager/activate.py` (`site.addsitedir()` + JAVA_HOME)
- Wire version resolution into runtime command dispatch
- End-to-end test: install version → activate → run command

### Phase 4: Java management
- Implement `manager/java.py` (detect, download Temurin via Adoptium API, extract)
- Add `dh java`, `dh java install`, `dh doctor` commands
- Wire Java auto-detection into activation

### Phase 5: REPL TUI (Textual rewrite)
- Implement REPL as a Textual App replacing the prompt_toolkit REPL
- Build widgets: OutputPanel, Sidebar, LogPanel, InputBar, Footer
- Build virtual-scrolling TableView using textual-fastdatatable + Deephaven `table.slice()`
- Implement command history, tab completion, multi-line input
- Wire into `dh repl` command

### Phase 6: Management TUI
- Implement `tui/app.py` with first-run wizard (Java + version install)
- Implement interactive main menu
- Wire `dh` (no args) → management TUI

### Phase 7: Polish
- Update README.md
- `dh config` command
- Progress bars for downloads
- Non-interactive mode testing for AI agents

---

## Key Files to Modify

| File | Change |
|------|--------|
| `pyproject.toml` | Drop deephaven deps + prompt_toolkit, add textual + textual-fastdatatable |
| `src/deephaven_cli/cli.py` | Two-phase dispatch, new subcommands, --version flag |
| `src/deephaven_cli/manager/config.py` | NEW: paths, config.toml, .dhrc |
| `src/deephaven_cli/manager/versions.py` | NEW: install/uninstall/list via uv |
| `src/deephaven_cli/manager/activate.py` | NEW: site.addsitedir() delegation |
| `src/deephaven_cli/manager/java.py` | NEW: Java detect + Temurin download |
| `src/deephaven_cli/manager/pypi.py` | NEW: PyPI version discovery |
| `src/deephaven_cli/tui/app.py` | NEW: first-run wizard + management menu |
| `src/deephaven_cli/repl/app.py` | NEW: Textual REPL app (replaces prompt_toolkit REPL) |
| `src/deephaven_cli/repl/widgets/output.py` | NEW: Output panel (RichLog + table display) |
| `src/deephaven_cli/repl/widgets/sidebar.py` | NEW: Global variables list |
| `src/deephaven_cli/repl/widgets/table_view.py` | NEW: Virtual-scrolling DataTable for billion-row tables |
| `src/deephaven_cli/repl/widgets/input_bar.py` | NEW: Command input with history + completion |
| `src/deephaven_cli/repl/widgets/log_panel.py` | NEW: Timestamped operational logs |

**Files to remove** (replaced by Textual):
- `src/deephaven_cli/repl/prompt/` (entire directory — prompt_toolkit UI)
- `src/deephaven_cli/repl/console.py` (replaced by `repl/app.py`)

## Verification

1. `uv tool install -e .` installs without downloading deephaven-server
2. `dh versions --remote` shows available versions from PyPI
3. `dh install 41.1` creates `~/.dh/versions/41.1/.venv/` with Deephaven packages
4. `dh use 41.1` sets the default
5. `dh repl` launches the full Textual REPL TUI with output, sidebar, logs, input, footer
6. In the REPL, creating a billion-row table displays it with virtual scrolling (only viewport rows fetched)
7. Sidebar shows all session variables, clicking one opens it in output panel
8. `.dhrc` with `version = "0.37.0"` + `dh exec -c 'print("hello")'` uses that version
9. `dh java install` downloads Temurin to `~/.dh/java/`
10. `dh` (no args) launches management TUI
11. `dh repl` without a version installed gives clear error with instructions
12. All management commands work non-interactively (no TTY required)
