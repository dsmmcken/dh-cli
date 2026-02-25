# Phase 4: Live Table Subscriptions — Detailed Design

## Overview

Phase 4 adds live (ticking) table support to the REPL. When a user creates a refreshing table (e.g., `time_table("PT1S")`), the table view auto-updates as the underlying data changes. The tab bar shows a "LIVE" badge on refreshing table tabs.

## Key Design Decision: Dual-Mode Approach

**Critical insight from pydeephaven source analysis:** The `pydeephaven` client library (used in both embedded and remote modes) does NOT support push-based table listeners. Only the server-side `deephaven.table_listener` module supports callbacks, and it requires JVM access via `jpy` — only available in embedded mode.

**Decision: Use polling for both modes.** This keeps the implementation simple and uniform. The Python runner uses a background thread that periodically re-snapshots the subscribed table and emits updates when data changes. The polling interval is configurable (default 2 seconds).

Why not use `deephaven.table_listener` for embedded mode?
- The listener callback runs on the JVM update graph thread and must NOT perform I/O or heavy work
- Converting table data to JSON and writing to stdout from a JVM callback is unsafe and can deadlock
- Polling via `session.open_table(name).to_arrow()` is safe, simple, and works identically in both modes
- 2-second polling is fast enough for a TUI — the human eye won't notice the difference from true push

## Protocol Changes

### New Go -> Python Commands

```jsonl
{"type":"subscribe","id":"8","name":"t","offset":0,"limit":200}
{"type":"unsubscribe","id":"9","name":"t"}
```

### New Python -> Go Responses

```jsonl
{"type":"subscribe_ack","id":"8","name":"t"}
{"type":"unsubscribe_ack","id":"9","name":"t"}
{"type":"table_update","name":"t","columns":["Timestamp","X"],"types":["instant","long"],"rows":[...],"total_rows":42,"offset":0}
```

The `table_update` message has **no `id` field** — it is an unsolicited server-push message, not a response to a specific command. This is the first time the protocol supports unsolicited messages from Python to Go (previously, `ready` was the only non-ID message, handled specially during startup).

---

## File-by-File Changes

### 1. `repl/protocol.go` — New command constructors and response type check

**Add new command constructors:**

```go
// NewSubscribeCmd creates a subscribe command for live table updates.
func NewSubscribeCmd(name string, offset, limit int) Command {
    return Command{Type: "subscribe", ID: nextID(), Name: name, Offset: &offset, Limit: &limit}
}

// NewUnsubscribeCmd creates an unsubscribe command.
func NewUnsubscribeCmd(name string) Command {
    return Command{Type: "unsubscribe", ID: nextID(), Name: name}
}
```

**Add response type checks:**

```go
func (r *Response) IsTableUpdate() bool    { return r.Type == "table_update" }
func (r *Response) IsSubscribeAck() bool   { return r.Type == "subscribe_ack" }
func (r *Response) IsUnsubscribeAck() bool { return r.Type == "unsubscribe_ack" }
```

No new fields needed on `Response` — `table_update` reuses the existing `Name`, `Columns`, `Types`, `Rows`, `TotalRows`, `Offset` fields already used by `table_data`.

---

### 2. `repl/session.go` — Handle unsolicited `table_update` messages

The current `readLoop` only dispatches messages that have an `id` field (request-response pattern). For `table_update`, there is no `id` because it's server-pushed.

**Add a push channel to `Session`:**

```go
type Session struct {
    cmd      *exec.Cmd
    stdin    io.WriteCloser
    stdout   io.ReadCloser
    mu       sync.Mutex
    pending  map[string]chan *Response
    readyCh  chan *Response
    readDone chan struct{}
    ready    *Response
    pushCh   chan *Response  // NEW: channel for unsolicited server-push messages
}
```

**Initialize in `NewSession`:**

```go
s := &Session{
    // ... existing fields ...
    pushCh: make(chan *Response, 16), // buffered to avoid blocking readLoop
}
```

**Modify `readLoop` to route `table_update` messages to `pushCh`:**

In the existing `readLoop`, after the `IsReady()` and `IsShutdownAck()` checks, and before the ID-based dispatch, add:

```go
// Handle unsolicited server-push messages (no ID)
if resp.IsTableUpdate() {
    select {
    case s.pushCh <- &resp:
    default:
        // Drop update if consumer is too slow — next poll will catch up
    }
    continue
}
```

**Important:** The `pushCh` is buffered (capacity 16) and uses a non-blocking send. If the Go TUI is too slow to consume updates, we drop them. This is safe because the next polling cycle will send fresh data. This prevents the readLoop goroutine from blocking, which would stall all protocol communication.

**Add new session methods:**

```go
// Subscribe tells the Python runner to start polling a table and sending updates.
func (s *Session) Subscribe(name string, offset, limit int) (*Response, error) {
    return s.sendAndWait(NewSubscribeCmd(name, offset, limit))
}

// Unsubscribe tells the Python runner to stop polling a table.
func (s *Session) Unsubscribe(name string) (*Response, error) {
    return s.sendAndWait(NewUnsubscribeCmd(name))
}

// PushChannel returns the channel that receives unsolicited table_update messages.
func (s *Session) PushChannel() <-chan *Response {
    return s.pushCh
}
```

---

### 3. `repl/app.go` — Subscription lifecycle + push message handling

This is the most complex set of changes. The app must:
1. Subscribe to refreshing tables when viewing them
2. Unsubscribe when switching away
3. Listen for push messages and dispatch them to the table view
4. Manage a "listening" goroutine via `tea.Cmd`

**Add new message types:**

```go
// TableUpdateMsg is sent when a live table pushes an update.
type TableUpdateMsg struct {
    Name     string
    Response *Response
}
```

**Add subscription tracking to `REPLModel`:**

```go
type REPLModel struct {
    // ... existing fields ...
    subscribedTable string // name of the currently subscribed table, or ""
}
```

**The push listener pattern:**

The standard bubbletea approach for external event sources is the "wait-on-channel" `tea.Cmd`. We create a `tea.Cmd` that blocks reading from `session.PushChannel()`, converts the received `*Response` into a `TableUpdateMsg`, and returns it. After the message is processed by `Update()`, we return a *new* `tea.Cmd` that waits again — creating a continuous subscription loop.

Add this method to `REPLModel`:

```go
// listenForPush returns a tea.Cmd that waits for the next push message.
func (m REPLModel) listenForPush() tea.Cmd {
    session := m.session
    if session == nil {
        return nil
    }
    return func() tea.Msg {
        select {
        case resp, ok := <-session.PushChannel():
            if !ok {
                return nil // channel closed
            }
            return TableUpdateMsg{Name: resp.Name, Response: resp}
        case <-session.readDone:
            return nil // session ended
        }
    }
}
```

**Note:** `session.readDone` needs to be exposed. Add a method to `Session`:

```go
// Done returns a channel that is closed when the session's read loop exits.
func (s *Session) Done() <-chan struct{} {
    return s.readDone
}
```

**Start the push listener after session starts:**

In the `SessionStartedMsg` handler, after the existing code:

```go
case SessionStartedMsg:
    // ... existing code ...
    return m, m.listenForPush()  // start listening for push messages
```

**Handle `TableUpdateMsg` in `Update()`:**

```go
case TableUpdateMsg:
    if msg.Response == nil {
        return m, m.listenForPush() // re-listen even on nil
    }
    resp := msg.Response
    tv, exists := m.tableviews[msg.Name]
    if exists {
        tv.SetData(resp)
        m.tabbar.UpdateTableTab(msg.Name, resp.TotalRows, true)
    }
    return m, m.listenForPush() // always re-listen for next update
```

**Key pattern:** Every `TableUpdateMsg` handler must return `m.listenForPush()` as a `tea.Cmd` to continue receiving updates. If we forget this, the push listener stops.

**Subscription lifecycle in `switchToView`:**

Modify `switchToView` to manage subscriptions:

```go
func (m *REPLModel) switchToView(name string) tea.Cmd {
    // Blur the old table view
    if old, ok := m.tableviews[m.activeView]; ok {
        old.Blur()
    }

    // Unsubscribe from old table if it was subscribed
    var cmds []tea.Cmd
    if m.subscribedTable != "" && m.subscribedTable != name {
        oldName := m.subscribedTable
        session := m.session
        if session != nil {
            cmds = append(cmds, func() tea.Msg {
                session.Unsubscribe(oldName)
                return nil // we don't need the ack
            })
        }
        m.subscribedTable = ""
    }

    m.activeView = name
    m.tabbar.SetActiveByName(name)

    if tv, ok := m.tableviews[name]; ok {
        tv.Focus()

        // Subscribe if the table is refreshing and not already subscribed
        if tv.isRefreshing && m.subscribedTable != name && m.session != nil {
            m.subscribedTable = name
            session := m.session
            cmds = append(cmds, func() tea.Msg {
                session.Subscribe(name, 0, 200)
                return nil // ack is consumed by readLoop
            })
        }
    }

    if len(cmds) > 0 {
        return tea.Batch(cmds...)
    }
    return nil
}
```

**Important:** `switchToView` now returns a `tea.Cmd` instead of being void. Update all callers to handle the returned cmd. Current callers:

1. `TabSelectedMsg` handler — change to:
   ```go
   case TabSelectedMsg:
       cmd := m.switchToView(msg.Tab.Name)
       return m, cmd
   ```

2. `TabSearchMsg` handler — change to:
   ```go
   case TabSearchMsg:
       cmd := m.switchToView(msg.Selected)
       return m, cmd
   ```

3. `ExecuteResultMsg` handler (auto-switch to last assigned table) — change to:
   ```go
   lastTable := resp.AssignedTables[len(resp.AssignedTables)-1]
   switchCmd := m.switchToView(lastTable)
   // ... existing fetchCmds ...
   if switchCmd != nil {
       fetchCmds = append(fetchCmds, switchCmd)
   }
   ```

**Also subscribe after `TableDataMsg` when first data arrives for a refreshing table:**

In the `TableDataMsg` handler, after `tv.SetData(resp)` and focus logic, add:

```go
// Auto-subscribe if this table is refreshing and currently active
if m.activeView == msg.Name && tv.isRefreshing && m.subscribedTable != msg.Name && m.session != nil {
    m.subscribedTable = msg.Name
    session := m.session
    return m, func() tea.Msg {
        session.Subscribe(msg.Name, 0, 200)
        return nil
    }
}
```

This handles the case where the table view was created on `TableDataMsg` and we now know `isRefreshing` from the response metadata. (`isRefreshing` is populated from the `handle_fetch_table` response or from the `TableMeta` when the TableViewModel was created.)

**Wait — `handle_fetch_table` doesn't currently include `is_refreshing`.** We need to add it. See Python changes below. The `table_data` response will include an `is_refreshing` field, and the Go code needs to read it. Add to the existing `TableDataMsg` handling:

```go
// After tv.SetData(resp):
if resp.IsRefreshing {
    tv.isRefreshing = true
}
```

And add `IsRefreshing` to the `Response` struct (see protocol.go changes).

---

### 4. `repl/protocol.go` — Add `IsRefreshing` field to Response

Add to the `Response` struct in the "table_data" fields section:

```go
// "table_data" fields
Name         string   `json:"name,omitempty"`
Columns      []string `json:"columns,omitempty"`
Types        []string `json:"types,omitempty"`
Rows         [][]any  `json:"rows,omitempty"`
TotalRows    int      `json:"total_rows,omitempty"`
Offset       int      `json:"offset,omitempty"`
IsRefreshing bool     `json:"is_refreshing,omitempty"` // NEW
```

This field is populated by `handle_fetch_table` in the Python runner. For `table_update` messages it is implicitly true (only refreshing tables can be subscribed).

---

### 5. `repl/tableview.go` — Live update support + LIVE status bar

**Modify `SetData` to preserve cursor position:**

Currently `SetData` calls `m.table.SetRows(rows)` which resets the cursor to 0. For live updates, we want to preserve the cursor position (or at least keep it reasonable).

```go
func (m *TableViewModel) SetData(resp *Response) {
    if resp == nil {
        return
    }

    // Remember current cursor position
    prevCursor := m.table.Cursor()

    // ... existing row conversion code ...

    m.table.SetRows(rows)
    m.totalRows = resp.TotalRows
    m.dataOffset = resp.Offset
    m.loading = false

    // Restore cursor position, clamped to new row count
    if prevCursor > 0 && len(rows) > 0 {
        if prevCursor >= len(rows) {
            prevCursor = len(rows) - 1
        }
        m.table.SetCursor(prevCursor)
    }
}
```

**Note:** Check that `bubbles/table.Model` has `Cursor() int` and `SetCursor(int)` methods. Looking at the bubbles table source:

```go
func (m Model) Cursor() int { return m.cursor }
func (m *Model) SetCursor(n int) { m.cursor = clamp(n, 0, len(m.rows)-1) }
```

Yes, these exist.

**Modify `statusBar` to show LIVE indicator:**

```go
func (m TableViewModel) statusBar() string {
    rowInfo := fmt.Sprintf("%d rows", m.totalRows)
    if m.totalRows == 0 {
        rowInfo = "empty"
    }

    var refreshInfo string
    if m.isRefreshing && m.isSubscribed {
        refreshInfo = lipgloss.NewStyle().
            Foreground(lipgloss.Color("#00FF00")).
            Bold(true).
            Render("LIVE")
    } else if m.isRefreshing {
        refreshInfo = "refreshing"
    } else {
        refreshInfo = "static"
    }

    if m.loading {
        return tui.StyleDim.Render(fmt.Sprintf("  %s | %s | loading...", m.name, rowInfo))
    }

    return tui.StyleDim.Render(fmt.Sprintf("  %s | %s | ", m.name, rowInfo)) + refreshInfo
}
```

**Add `isSubscribed` field to `TableViewModel`:**

```go
type TableViewModel struct {
    // ... existing fields ...
    isSubscribed bool // NEW: whether this table has an active subscription
}
```

**Add setter:**

```go
// SetSubscribed updates the subscription status for the LIVE indicator.
func (m *TableViewModel) SetSubscribed(subscribed bool) {
    m.isSubscribed = subscribed
}
```

Call this from `app.go` when subscribing/unsubscribing.

---

### 6. `repl/tabbar.go` — LIVE badge on refreshing tabs

Modify `View()` to show a green "LIVE" badge on refreshing table tabs:

In the existing render loop, change the label construction:

```go
label := t.Name
if t.Type == TabTable && t.RowCount >= 0 {
    label = fmt.Sprintf("%s (%d)", t.Name, t.RowCount)
}
if t.Type == TabTable && t.IsRefreshing {
    liveBadge := lipgloss.NewStyle().
        Foreground(lipgloss.Color("#00FF00")).
        Bold(true).
        Render(" LIVE")
    label = label + liveBadge
}
```

**Note on rendering:** The LIVE badge is appended to the label string before the tab style is applied. Because lipgloss applies background color to the entire string, the LIVE badge foreground might get overridden by the active tab's background. To handle this correctly, the badge should be rendered separately and concatenated AFTER the tab style is applied:

```go
// Better approach: render label and badge separately
tabRendered := style.Render(label)
if t.Type == TabTable && t.IsRefreshing {
    badge := lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(true).Render(" LIVE")
    tabRendered = tabRendered + badge
}
tabs = append(tabs, tabRendered)
```

This way the LIVE badge is always green, regardless of whether the tab is active or inactive.

---

### 7. `repl/repl_runner.py` — Subscribe/unsubscribe handlers with polling thread

This is the most significant Python-side change. Add a subscription manager that uses a background thread to periodically snapshot the subscribed table and emit `table_update` messages.

**Add subscription state (module-level):**

```python
import threading

# --- Subscription state ---
_subscription_lock = threading.Lock()
_active_subscription = None  # dict with keys: name, offset, limit, stop_event, thread
```

**Add `handle_subscribe`:**

```python
def handle_subscribe(session, cmd_id, cmd):
    """Start polling a refreshing table and emitting updates."""
    global _active_subscription
    name = cmd.get("name", "")
    offset = cmd.get("offset", 0)
    limit = cmd.get("limit", 200)

    # Stop any existing subscription first
    _stop_subscription()

    stop_event = threading.Event()

    def poll_loop():
        """Background thread that periodically snapshots the table."""
        last_hash = None
        while not stop_event.is_set():
            stop_event.wait(2.0)  # 2-second interval
            if stop_event.is_set():
                break
            try:
                t = session.open_table(name)
                arrow = t.to_arrow()
                total = arrow.num_rows
                sliced = arrow.slice(offset, min(limit, total - offset) if total > offset else 0)

                # Simple change detection: hash of (total_rows, num_rows, first+last values)
                current_hash = (total, sliced.num_rows)
                if sliced.num_rows > 0:
                    # Add first and last row values for change detection
                    first_vals = tuple(sliced.column(col)[0].as_py() for col in sliced.column_names)
                    last_vals = tuple(sliced.column(col)[-1].as_py() for col in sliced.column_names)
                    current_hash = (total, sliced.num_rows, first_vals, last_vals)

                if current_hash == last_hash:
                    continue  # No change, skip emit
                last_hash = current_hash

                columns = [f.name for f in sliced.schema]
                types = [str(f.type) for f in sliced.schema]
                rows = []
                for i in range(sliced.num_rows):
                    row = []
                    for col in columns:
                        val = sliced.column(col)[i].as_py()
                        if isinstance(val, (bytes, bytearray)):
                            val = base64.b64encode(val).decode("ascii")
                        elif hasattr(val, 'isoformat'):
                            val = val.isoformat()
                        row.append(val)
                    rows.append(row)

                emit({
                    "type": "table_update",
                    "name": name,
                    "columns": columns,
                    "types": types,
                    "rows": rows,
                    "total_rows": total,
                    "offset": offset,
                })
            except Exception as e:
                # Table may have been deleted or session closed — stop polling
                print(f"Subscription error for {name}: {e}", file=sys.stderr)
                break

    thread = threading.Thread(target=poll_loop, daemon=True)
    thread.start()

    with _subscription_lock:
        _active_subscription = {
            "name": name,
            "offset": offset,
            "limit": limit,
            "stop_event": stop_event,
            "thread": thread,
        }

    emit({"type": "subscribe_ack", "id": cmd_id, "name": name})
```

**Add `handle_unsubscribe`:**

```python
def handle_unsubscribe(session, cmd_id, cmd):
    """Stop polling the currently subscribed table."""
    name = cmd.get("name", "")
    _stop_subscription()
    emit({"type": "unsubscribe_ack", "id": cmd_id, "name": name})
```

**Add `_stop_subscription` helper:**

```python
def _stop_subscription():
    """Stop the active subscription if any."""
    global _active_subscription
    with _subscription_lock:
        if _active_subscription is not None:
            _active_subscription["stop_event"].set()
            # Don't join — daemon thread will exit on its own
            _active_subscription = None
```

**Thread safety for `emit`:**

The `emit` function writes to stdout. Both the main thread (handling commands) and the subscription background thread call `emit`. Python's `print` with `flush=True` is **not** thread-safe at the application level — two concurrent `print` calls can interleave their output.

**Solution:** Add a lock around `emit`:

```python
_emit_lock = threading.Lock()

def emit(obj):
    """Write JSON line to stdout (the protocol channel). Thread-safe."""
    line = json.dumps(obj)
    with _emit_lock:
        print(line, flush=True)
```

**Modify `handle_fetch_table` to include `is_refreshing`:**

In the existing `handle_fetch_table`, add `is_refreshing` to the emitted response:

```python
emit({
    "type": "table_data",
    "id": cmd_id,
    "name": name,
    "columns": columns,
    "types": types,
    "rows": rows,
    "total_rows": total,
    "offset": offset,
    "is_refreshing": t.is_refreshing,  # NEW
})
```

**Add command handlers to `run_loop`:**

```python
elif cmd_type == "subscribe":
    handle_subscribe(session, cmd_id, cmd)
elif cmd_type == "unsubscribe":
    handle_unsubscribe(session, cmd_id, cmd)
```

**Stop subscription on shutdown:**

In the `shutdown` handler, add `_stop_subscription()` before closing the session:

```python
elif cmd_type == "shutdown":
    _stop_subscription()
    emit({"type": "shutdown_ack"})
    try:
        session.close()
    except Exception:
        pass
    sys.exit(0)
```

**Change detection design:**

The polling loop uses a simple hash-based change detection: `(total_rows, visible_row_count, first_row_values, last_row_values)`. This avoids re-emitting identical data on every poll. For most ticking tables (like `time_table`), the total row count changes on every tick, so the hash will differ. For tables with in-place modifications, the first/last row check provides a reasonable heuristic. If needed, this can be made more sophisticated later (e.g., full data hash), but for a TUI the simple approach is sufficient.

---

### 8. `repl/session.go` — Expose `readDone` channel

Add this method so `app.go` can use it in the push listener:

```go
// Done returns a channel that is closed when the read loop exits.
func (s *Session) Done() <-chan struct{} {
    return s.readDone
}
```

---

## Interaction Flow

### Happy Path: User creates a time_table

1. User types: `t = time_table("PT1S")` and presses Enter
2. `app.go` sends `execute` command, receives `ExecuteResultMsg` with `assigned_tables: ["t"]`
3. `app.go` calls `AddTableTab("t", -1, false)`, `fetchTableData("t", 0)`, and `switchToView("t")`
4. Python responds with `table_data` including `is_refreshing: true`
5. `app.go` processes `TableDataMsg`, creates `TableViewModel`, calls `tv.SetData()`
6. `app.go` detects `isRefreshing=true` and active view is "t" — sends `subscribe` command
7. Python starts background polling thread, emits `subscribe_ack`
8. Every 2 seconds, Python snapshots the table and emits `table_update`
9. Go's `readLoop` routes `table_update` to `pushCh`
10. `listenForPush` `tea.Cmd` receives from `pushCh`, returns `TableUpdateMsg`
11. `app.go` handles `TableUpdateMsg`, calls `tv.SetData()` (preserving cursor), returns `listenForPush` again
12. Tab bar shows "LIVE" badge, status bar shows "LIVE"

### Tab Switch: User switches to "log" tab

1. User presses Tab to switch to "log"
2. `switchToView("log")` detects `subscribedTable == "t"`, sends `unsubscribe` for "t"
3. Python stops the polling thread, emits `unsubscribe_ack`
4. `subscribedTable` set to ""
5. No more `table_update` messages for "t"

### Tab Switch Back: User returns to "t" tab

1. User presses Tab to switch back to "t"
2. `switchToView("t")` detects `tv.isRefreshing` and `subscribedTable == ""` — sends `subscribe` for "t"
3. Python starts a new polling thread
4. Updates resume

### Edge Cases

- **Table deleted during subscription:** The polling thread catches the exception from `session.open_table()` and stops. The LIVE badge remains on the tab but no more updates arrive. When the user next fetches data or executes code, the error will surface naturally.

- **Session disconnect:** `readDone` is closed, `listenForPush` returns `nil`, the loop stops cleanly.

- **Multiple tables assigned at once:** Only the last table is auto-switched to and subscribed. Other refreshing tables remain in the tab bar without active subscriptions until the user navigates to them.

- **Static table with subscribe:** The polling thread will repeatedly snapshot the same data. The hash check prevents re-emitting identical data. No harm, just a small CPU cost. However, `app.go` should only subscribe to tables with `isRefreshing == true`, so this shouldn't happen.

- **Rapid tab switching:** Each `switchToView` call unsubscribes from the old and subscribes to the new. The `_stop_subscription()` call sets a `threading.Event` which is checked at the start of each poll cycle, so the old thread stops within one poll interval.

---

## Summary of Changed Files

| File | Changes |
|------|---------|
| `repl/protocol.go` | Add `NewSubscribeCmd`, `NewUnsubscribeCmd`, `IsTableUpdate`, `IsSubscribeAck`, `IsUnsubscribeAck`, `IsRefreshing` field on Response |
| `repl/session.go` | Add `pushCh` field, handle `table_update` in `readLoop`, add `Subscribe()`, `Unsubscribe()`, `PushChannel()`, `Done()` methods |
| `repl/app.go` | Add `subscribedTable` field, `TableUpdateMsg` type, `listenForPush()` method, subscription lifecycle in `switchToView` (now returns `tea.Cmd`), handle `TableUpdateMsg`, auto-subscribe on `TableDataMsg` for refreshing tables |
| `repl/tableview.go` | Add `isSubscribed` field, `SetSubscribed()`, preserve cursor in `SetData()`, LIVE indicator in `statusBar()` |
| `repl/tabbar.go` | Add LIVE badge rendering for refreshing tabs |
| `repl/repl_runner.py` | Add `_emit_lock` for thread safety, `handle_subscribe`/`handle_unsubscribe` handlers, `_stop_subscription` helper, background polling thread, change detection, `is_refreshing` in `handle_fetch_table`, stop subscription on shutdown |

## Testing

```bash
cd go_src && make build

# Test 1: Static table (no subscription)
./dhg repl
> from deephaven import empty_table
> t = empty_table(5).update(["X = i"])
# Tab bar shows "t (5)" with no LIVE badge
# Status bar shows "t | 5 rows | static"

# Test 2: Time table (live subscription)
> from deephaven import time_table
> tt = time_table("PT1S")
# Tab bar shows "tt (N) LIVE" with green LIVE badge
# Status bar shows "tt | N rows | LIVE"
# Table data auto-updates every ~2 seconds
# Row count increases by 1 each second

# Test 3: Tab switching
# Press Tab to switch to "log"
# LIVE badge remains on "tt" tab but no updates
# Press Tab to switch back to "tt"
# Updates resume

# Test 4: Multiple tables
> t2 = time_table("PT2S")
# Auto-switches to t2, subscribes to t2
# Press Shift+Tab to go back to tt — subscribes to tt, unsubscribes from t2
```
