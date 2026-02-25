# Plan: Go REPL TUI (`dhg repl`)

## Context

The `dhg` CLI already has `dhg exec` (batch code execution) and `dhg serve` (run script + keep server alive). Both use an embedded Python `runner.py` that starts a Deephaven server, connects via pydeephaven, executes code, and returns results.

The Python CLI (`deephaven_cli`) has a rich Textual-based REPL. We're building the Go equivalent using bubbletea/charm, informed by a wireframe mockup that specifies:

- **Input area** (top): auto-expanding, multi-line, syntax-highlighted, with history and reverse-i-search
- **Tab bar** (below input): first tab is always "log" (stdout/stderr), then one tab per Deephaven table (most-recent-first), truncating
- **Content area** (center): displays **either** the log view **or** a table view, depending on which tab is selected
- **Log view**: scrollable stdout/stderr output with errors in red — the default view (first tab always selected on startup)
- **Sidebar** (right): server details + shortcut help

## Architecture Overview

```
┌──────────────────────────────────────────────────────┐
│  Go (bubbletea TUI)                                  │
│  ┌──────────┐ ┌─────────────────────┐ ┌───────────┐  │
│  │InputArea  │ │ TabBar              │ │ Sidebar   │  │
│  └──────────┘ │ [log][t1][t2][t3]   │ └───────────┘  │
│               ├─────────────────────┤                │
│               │ ContentArea         │                │
│               │ (LogView or TableVP)│                │
│               └─────────────────────┘                │
│        │                       ▲                     │
│        ▼                       │                     │
│  ┌─────────────────────────────────────┐             │
│  │  REPLSession (Go)                   │             │
│  │  - manages Python subprocess        │             │
│  │  - sends commands as JSON on stdin  │             │
│  │  - reads JSON results from stdout   │             │
│  └──────────┬──────────────────────────┘             │
└─────────────┼────────────────────────────────────────┘
              │ stdin (JSON commands) / stdout (JSON results)
              ▼
┌─────────────────────────────────────────┐
│  Python (repl_runner.py)                │
│  - Starts DH server (embedded) or      │
│    connects (remote)                    │
│  - Long-running: reads commands from    │
│    stdin in a loop                      │
│  - Executes code via pydeephaven        │
│  - Returns structured JSON results      │
│  - Supports: execute, list_tables,      │
│    fetch_table_page, server_info        │
└─────────────────────────────────────────┘
```

## Communication Protocol

Go and Python communicate via **newline-delimited JSON** over stdin/stdout of the Python subprocess. The protocol is primarily request/response (Go sends command, Python replies with matching `id`), but Phase 4 adds **server-push** messages for table subscriptions where Python sends unsolicited `table_update` messages.

### Go → Python (stdin)

```jsonl
{"type":"execute","id":"1","code":"t = empty_table(5).update(['X = i'])"}
{"type":"list_tables","id":"2"}
{"type":"fetch_table","id":"3","name":"t","offset":0,"limit":50}
{"type":"server_info","id":"4"}
{"type":"shutdown","id":"5"}
{"type":"subscribe","id":"6","name":"t","offset":0,"limit":50}
{"type":"unsubscribe","id":"7","name":"t"}
```

### Python → Go (stdout)

```jsonl
{"type":"ready","port":10000,"version":"0.35.1","mode":"embedded"}
{"type":"result","id":"1","stdout":"","stderr":"","error":null,"result_repr":null,"assigned_tables":["t"],"all_tables":["t"],"elapsed_ms":42}
{"type":"tables","id":"2","tables":[{"name":"t","row_count":5,"is_refreshing":false,"columns":[{"name":"X","type":"long"}]}]}
{"type":"table_data","id":"3","name":"t","columns":["X"],"types":["long"],"rows":[[0],[1],[2],[3],[4]],"total_rows":5,"offset":0}
{"type":"server_info","id":"4","host":"localhost","port":10000,"version":"0.35.1","mode":"embedded","table_count":1}
{"type":"error","id":"5","message":"something went wrong"}
{"type":"shutdown_ack"}
{"type":"table_update","name":"t","columns":["X"],"types":["long"],"rows":[[0],[1],[2]],"total_rows":3,"offset":0}
```

## File Structure

```
go_src/internal/
├── repl/                          # NEW - REPL package
│   ├── repl_runner.py             # NEW - Embedded long-running Python REPL runner
│   ├── session.go                 # NEW - Manages Python subprocess + JSON protocol
│   ├── protocol.go                # NEW - JSON message types (Go structs)
│   ├── app.go                     # NEW - Top-level bubbletea REPL model
│   ├── input.go                   # NEW - Input area component (textarea-based)
│   ├── tabbar.go                  # NEW - Tab bar component
│   ├── logview.go                 # NEW - Log/output viewport (stdout/stderr with colored errors)
│   ├── tableview.go               # NEW - Table viewport component
│   ├── sidebar.go                 # NEW - Sidebar component (server info + help)
│   ├── history.go                 # NEW - Command history (file-backed)
│   └── styles.go                  # NEW - REPL-specific lipgloss styles
├── cmd/
│   └── repl.go                    # NEW - `dhg repl` cobra command
```

## Implementation Scope

All 4 phases will be implemented. Input starts as plain text (no syntax highlighting); highlighting can be added later as an enhancement.

## Phased Implementation

### Phase 1: Foundation — Working REPL with Log Tab

Goal: Submit Python code, see output in the log tab. Tab bar with permanent "log" tab, no table tabs yet, no sidebar.

**Files:**

1. **`repl/protocol.go`** — JSON message structs
   - `Command` struct: `{Type, ID, Code, Name, Offset, Limit}`
   - `Response` struct: `{Type, ID, Stdout, Stderr, Error, ResultRepr, AssignedTables, AllTables, ...}`
   - Helper constructors: `NewExecuteCmd(code)`, `NewShutdownCmd()`, etc.

2. **`repl/repl_runner.py`** — Long-running Python REPL runner (embedded via `//go:embed`)
   - Reuses server startup logic from existing `runner.py` (embedded/remote modes)
   - Main loop: `for line in sys.stdin: handle(json.loads(line))`
   - `execute` handler: wraps code with output capture (reusing `build_wrapper` pattern from runner.py), executes, returns result JSON
   - `list_tables` handler: returns `session.tables` with metadata
   - `fetch_table` handler: opens table, slices with offset/limit, returns column data as JSON arrays
   - `server_info` handler: returns connection details
   - `shutdown` handler: closes session, exits
   - Emits `{"type":"ready",...}` on startup once server is connected

3. **`repl/session.go`** — Go-side subprocess manager
   - `Session` struct: holds `*exec.Cmd`, stdin writer, stdout scanner, response channel
   - `NewSession(cfg SessionConfig) (*Session, error)` — resolves version, finds python, starts subprocess, waits for `ready` message
   - `Execute(code string) (*ExecuteResult, error)` — sends execute command, blocks for result
   - `ListTables() ([]TableMeta, error)`
   - `FetchTable(name string, offset, limit int) (*TableData, error)`
   - `ServerInfo() (*ServerInfo, error)`
   - `Close()` — sends shutdown, kills process
   - Background goroutine reads stdout lines, dispatches to waiting callers by message ID

4. **`repl/logview.go`** — Log/output view component
   - `LogViewModel` struct: `entries []LogEntry`, `viewport viewport.Model`
   - `LogEntry`: `{Type string, Text string, Timestamp time.Time}` — types: "stdout", "stderr", "error", "command", "info"
   - Appends new entries after each command execution:
     - The submitted command (dimmed, prefixed with `> `)
     - stdout text (default color)
     - stderr text (yellow)
     - errors/tracebacks (red)
     - result repr (if non-nil)
   - Scrollable via viewport (up/down, mouse wheel)
   - Auto-scrolls to bottom on new output

5. **`repl/tabbar.go`** — Tab bar component (log tab only in Phase 1)
   - `TabBarModel` struct: `tabs []TabInfo`, `activeIdx int`
   - `TabInfo`: `{Name string, Type string, RowCount int, IsRefreshing bool}` — Type is "log" or "table"
   - First tab is always `{Name: "log", Type: "log"}`, permanently pinned
   - Renders horizontal row of tab labels styled with lipgloss (active tab highlighted)
   - Emits `TabSelectedMsg{Name string, Type string}` when active tab changes

6. **`repl/app.go`** — Bubbletea REPL model
   - `REPLModel` struct with: `input InputModel`, `tabbar TabBarModel`, `logview LogViewModel`, `session *Session`
   - `Init()`: returns cmd to show "Connecting..." spinner, then starts session in background via `tea.Cmd`
   - `Update()`: handles key events, session responses
     - `Enter` (no shift): submit code → send to session → append result to log view
     - `Shift+Enter`: insert newline in textarea
     - `Ctrl+C` / `Ctrl+D`: quit
   - `View()`: layout is `[input] [tabbar] [content area]` vertically stacked
   - Content area shows log view (only option in Phase 1)

7. **`repl/input.go`** — Input component wrapping textarea
   - Customized `textarea.Model` with:
     - Single-line height by default, grows on Shift+Enter up to max 6 lines, then becomes scrollable within that 6-line area
     - `>` prompt character
     - Custom key handling: Enter submits, Shift+Enter newline
   - Emits `SubmitMsg{Code string}` message on Enter

8. **`cmd/repl.go`** — Cobra command
   - Same flags as `exec` (port, jvm-args, version, host, auth, tls)
   - `runRepl()`: builds `SessionConfig` from flags, creates `REPLModel`, runs `tea.NewProgram` with alt-screen

9. **`cmd/root.go`** — Add `addReplCommand(cmd)` call

### Phase 2: Table Display + Table Tabs

Goal: Code that creates tables adds table tabs. Clicking a table tab shows its data; clicking "log" returns to the log view.

**Files:**

1. **`repl/tableview.go`** — NEW — Table viewport component using `bubbles/table`
   - `TableViewModel` wraps `table.Model` from `github.com/charmbracelet/bubbles/table`
   - Configures `table.Model` with columns (from server metadata) and rows (from fetched data)
   - `table.Model` provides: column headers, aligned columns, row selection/highlighting, scrollable viewport, keyboard navigation (up/down/pgup/pgdn), styled header row
   - Additional state: `totalRows int`, `isRefreshing bool`, `dataOffset int`
   - Shows row count and "refreshing"/"static" badge above or below the table
   - Pagination: when user scrolls past loaded rows, fetches next page from session and appends to table rows

2. **Update `repl/tabbar.go`**:
   - Add table tabs after the pinned "log" tab, most recently created first
   - Left/Right arrow keys or click to switch active tab
   - Truncates with `...` if tabs overflow terminal width
   - Table tabs show name + row count

3. **Update `repl/app.go`**:
   - Content area now switches based on active tab:
     - Log tab selected → render `LogViewModel`
     - Table tab selected → render `TableViewModel` for that table
   - After execute result, if `assigned_tables` is non-empty:
     - Add new table tabs (prepend after "log")
     - Auto-switch to the most recently created table tab
     - Fetch first page of table data
   - Maintain a map of `tableName → TableViewModel` so each table preserves its scroll position

4. **Update `repl/repl_runner.py`**:
   - `fetch_table` returns paginated row data as JSON-serializable arrays
   - Handle type conversion (Arrow types → JSON-safe: timestamps as ISO strings, bytes as base64, etc.)

### Phase 3: Sidebar + History + Polish

Goal: Full mockup layout with sidebar, persistent history, and keyboard shortcuts.

**Files:**

1. **`repl/sidebar.go`** — Sidebar component
   - `SidebarModel` struct: `serverInfo ServerInfo`, `width int`
   - Top section: Server details (host:port, version, mode, table count)
   - Bottom section: Keybinding help (styled list)
   - Fixed width (~25 chars), right-aligned
   - Updates when server info changes

2. **`repl/history.go`** — Command history manager
   - `History` struct: `entries []string`, `path string`, `cursor int`, `draft string`
   - Loads from `~/.dhg/repl_history` on startup
   - `Add(cmd)`: appends, deduplicates consecutive, writes to file
   - `Up()` / `Down()`: navigate with cursor, preserves draft
   - Max 500 entries
   - `Search(query)`: reverse-i-search matching

3. **Update `repl/input.go`**:
   - Wire Up/Down arrows to history navigation
   - `Ctrl+R` enters reverse-i-search mode (overlay showing matches)
   - `Ctrl+T` enters tab-search mode (fuzzy search table names, Enter selects)

4. **Update `repl/app.go`**:
   - Layout becomes: `[main area | sidebar]` horizontal split
   - Main area is: `[input] [tabbar] [content: log or table]` vertical stack
   - Sidebar width fixed, main area fills remaining space

### Phase 4: Live Tables via Subscription

Goal: Ticking/refreshing tables stream updates to the viewport via pydeephaven subscriptions (not polling).

The communication model becomes **bidirectional** — Python can push unsolicited `table_update` messages when subscribed table data changes.

**New protocol messages:**

```jsonl
Go → Python:  {"type":"subscribe","id":"6","name":"t","offset":0,"limit":50}
Go → Python:  {"type":"unsubscribe","id":"7","name":"t"}
Python → Go:  {"type":"table_update","name":"t","columns":["X"],"types":["long"],"rows":[[0],[1]],"total_rows":5,"offset":0}
```

1. **Update `repl/repl_runner.py`**:
   - `subscribe` handler: uses `pydeephaven` table listener/callback mechanism
     - Opens table, registers a listener for data changes
     - On each update, re-fetches the current viewport window (offset/limit) and emits a `table_update` JSON message to stdout
     - Only one subscription active at a time (the currently viewed table)
   - `unsubscribe` handler: removes the active listener
   - When user switches tabs, Go sends `unsubscribe` for old + `subscribe` for new

2. **Update `repl/session.go`**:
   - Background reader goroutine now also handles unsolicited `table_update` messages
   - Dispatches `table_update` as a bubbletea `Cmd` (via a callback channel) so the TUI updates

3. **Update `repl/app.go`**:
   - When a tab is selected and the table `is_refreshing`, send `subscribe` command
   - Handle incoming `TableUpdateMsg` to refresh the table viewport
   - Visual indicator: "LIVE" badge on refreshing table tabs (pulsing via spinner)
   - On tab switch: unsubscribe old, subscribe new (if refreshing)

4. **Update `repl/tableview.go`**:
   - Accept `TableUpdateMsg` to update the `table.Model` rows without losing scroll position
   - Re-set rows on the `table.Model` while preserving the cursor/selection index

## Key Existing Code to Reuse

| What | Where | Reuse How |
|------|-------|-----------|
| Version resolution | `config.ResolveVersion()` | Call from `cmd/repl.go` |
| Find venv Python | `exec.FindVenvPython()` | Call from `session.go` |
| Ensure pydeephaven | `exec.EnsurePydeephaven()` | Call from `session.go` |
| Java detection | `java.Detect()` | Call from `cmd/repl.go` |
| Lipgloss color palette | `tui.Color*`, `tui.Style*` | Import in `repl/styles.go` |
| Server start + output capture | `runner.py` `run_embedded/run_remote` | Adapt into `repl_runner.py` |
| Code wrapping + result reading | `runner.py` `build_wrapper/read_result_table` | Reuse in `repl_runner.py` |
| Process group handling | `exec/exec_unix.go` | Same pattern in `session.go` |

## New Dependencies

No new external dependencies needed — `bubbles` already provides textarea, viewport, spinner, and help components.

## Verification

### Phase 1 Testing
```bash
cd go_src && make build
./dhg repl                           # Embedded mode
./dhg repl --host localhost:10000    # Remote mode

# In the REPL:
# Tab bar shows [log] tab (always selected by default)
> print("hello")                     # "hello" appears in log view (white text)
> 2 + 2                              # "4" appears in log view
> x = 1/0                            # Traceback appears in log view (red text)
> import sys; print("err", file=sys.stderr)  # "err" in yellow
> [Ctrl+C]                           # Clean exit
```

### Phase 2 Testing
```bash
> from deephaven import empty_table
> t = empty_table(5).update(["X = i", "Y = X * 2"])
# Tab bar: [log] [t] — auto-switches to "t" tab, table data displayed
> t2 = empty_table(3).update(["Name = `hello`"])
# Tab bar: [log] [t2] [t] — auto-switches to "t2" tab
# Click "log" tab → see stdout/stderr history
# Click "t" tab → see t's data (scroll position preserved)
```

### Phase 3 Testing
```bash
# Up arrow recalls history; Ctrl+R reverse-i-search
# Ctrl+T tab search; sidebar shows server info + keybindings
```

### Phase 4 Testing
```bash
> from deephaven import time_table
> t = time_table("PT1S")
# "LIVE" badge, table auto-updates every 2 seconds
```
