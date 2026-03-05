# Plan: Replace REPL InputModel with bubbline

## Context

The current REPL input is a custom wrapper around `bubbles/textarea` with hand-rolled history navigation, reverse-i-search, and multi-line handling. The multi-line approach (alt+enter for newlines) is a workaround for terminal limitations. `knz/bubbline` is a purpose-built REPL input library (used by CockroachDB's SQL shell) that handles all of this natively — multi-line auto-grow, history, reverse-i-search, conditional enter, and proper prompt rendering. Adopting it simplifies the codebase and gives us a more polished input experience.

## Approach

Use `editline.Model` (the embeddable tea.Model layer of bubbline) as a sub-component inside a thin `InputModel` wrapper. The wrapper handles: border rendering, executing state visuals, tab search (Ctrl+T), and height reporting for layout.

Adopt bubbline's keybinding conventions:
- **Enter** → submit (CheckInputComplete returns true)
- **Ctrl+O** → always insert newline (bubbline built-in)
- **Alt+Enter** → always submit (bubbline built-in)
- **Up/Down** → history navigation (bubbline handles first/last line detection)
- **Ctrl+R** → reverse-i-search (bubbline built-in)
- **Ctrl+C** on empty input → quit (via ErrInterrupted)
- **Ctrl+D** on empty input → quit (via io.EOF)

## Files to Change

### 1. Add dependency
```
cd src && go get github.com/knz/bubbline
```

### 2. Delete: `src/internal/repl/history.go`
Bubbline manages history internally via `editline.Model.AddHistoryEntry()`, `SetHistory()`, `GetHistory()`. Persistence handled via `github.com/knz/bubbline/history` package (`LoadHistory` / `SaveHistory`).

### 3. Rewrite: `src/internal/repl/input.go`

New `InputModel` struct:
```go
type InputModel struct {
    editor     *editline.Model
    maxHeight  int        // 6
    totalWidth int
    mode       InputMode  // InputNormal or InputTabSearch (remove InputHistorySearch — bubbline handles it)
    tabNames   []string
    searchQuery   string
    searchMatches []string
    searchIdx     int
    executing  bool
}
```

**NewInput(historyPath string):**
- Create `editline.New(80, 10)` (width, maxHeight)
- Set `editor.Prompt = "> "`, `editor.NextPrompt = "  "`
- Set `editor.CheckInputComplete` to always return `true` (Enter = submit)
- Load history from file via `history.LoadHistory(path)` → `editor.SetHistory(h)`
- No `*History` parameter needed

**Key interception in Update (before forwarding to editline):**
- `ctrl+t` → enter TabSearch mode (same overlay logic as current)
- `tab`, `shift+tab` → consume (return nil), tab bar handles these
- All other keys → forward to `editor.Update(msg)`

**Remove entirely:**
- `InputHistorySearch` mode and `updateHistorySearch()` — bubbline handles Ctrl+R
- All `history *History` references
- `canNavigateHistory()` — bubbline auto-detects first/last line

**Height():**
```go
func (m InputModel) Height() int {
    lines := strings.Count(m.editor.Value(), "\n") + 1
    if lines < 3 { lines = 3 }
    if lines > m.maxHeight { lines = m.maxHeight }
    return lines + 2 // +2 for border
}
```

**View():**
- In TabSearch mode: render search overlay (same as current)
- In Normal mode: render `m.editor.View()`, pad to min 3 lines, wrap in rounded border box
- Border color: primary when not executing, dim when executing

**SetWidth(w):**
- `m.editor.SetSize(w - 2, maxHeight)` — subtract 2 for border

**Reset():**
- `m.editor.Reset()` — bubbline clears and refocuses

**SaveHistory(path):**
- New method: `history.SaveHistory(m.editor.GetHistory(), path)` — called after each submit

### 4. Modify: `src/internal/repl/app.go`

**REPLModel struct changes:**
- Remove `history *History` field
- Keep `historyPath string` for persistence

**NewREPLModel:**
- Remove `NewHistory(cfg.DHHome)`
- Pass history path to `NewInput(historyPath)`
- Compute `historyPath` from `cfg.DHHome`

**Update — new message handler:**
```go
case editline.InputCompleteMsg:
    // Check for quit conditions
    if m.input.editor.Err != nil {
        // io.EOF (Ctrl+D) or ErrInterrupted (Ctrl+C on empty) → quit
        if m.session != nil { m.session.Close() }
        return m, tea.Quit
    }
    code := strings.TrimSpace(m.input.editor.Value())
    if code == "" {
        m.input.Reset()
        return m, nil
    }
    m.executing = true
    m.input.SetExecuting(true)
    m.input.editor.AddHistoryEntry(code)
    m.input.SaveHistory()
    m.input.Reset()
    m.logview.AppendEntry(LogEntry{Type: LogCommand, Text: code})
    return m, m.executeCode(code)
```

**Remove SubmitMsg handler** — replaced by InputCompleteMsg above.

**Simplify Ctrl+C/Ctrl+D handling:**
- Remove the special-case `ctrl+c` interception at lines 100-113
- Let bubbline handle Ctrl+C/Ctrl+D natively:
  - During search: bubbline cancels search internally
  - On empty input: bubbline emits InputCompleteMsg with Err set → we quit
  - With content: bubbline clears the input
- Only keep quit interception for `ctrl+c` during tab search mode (InputModel handles this)

**Layout — no changes needed** to height calculation logic; `input.Height()` still works the same way.

### 5. Modify: `src/internal/repl/sidebar.go`

Update `defaultREPLKeyMap` to match bubbline conventions:
```go
defaultREPLKeyMap = replKeyMap{
    Submit:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "submit")),
    Newline:    key.NewBinding(key.WithKeys("ctrl+o"), key.WithHelp("ctrl+o", "newline")),
    History:    key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "history")),
    SearchHist: key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "search hist")),
    SearchTabs: key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl+t", "search tabs")),
    NextTab:    key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next tab")),
    PrevTab:    key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev tab")),
    Quit:       key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
}
```
Only change: `alt+ret` → `ctrl+o` for newline.

### 6. Modify: `src/internal/repl/styles.go`

Remove `stylePrompt` and `styleExecuting` if no longer used (bubbline has its own prompt styling).

### 7. Delete: `src/internal/repl/session_windows.go` / `session_unix.go`

No changes needed — these are session management, not input.

## Tab Search Preservation

Tab search (Ctrl+T) stays in the InputModel wrapper. When `mode == InputTabSearch`:
- InputModel renders its own overlay (same `renderSearchOverlay` code as current)
- Keys are handled by `updateTabSearch()` (unchanged)
- On selection, emits `TabSearchMsg` (unchanged)
- On cancel, returns to InputNormal and re-focuses editline

## History Migration

bubbline's `history.LoadHistory` reads one entry per line (plain text). Our current format is JSON-encoded strings per line. Options:
- Write a small migration in `NewInput` that detects JSON format and converts
- Or just start fresh (history is low-value data)

Recommend: attempt to load with bubbline's loader; if entries look JSON-encoded (start with `"`), decode them. Simple and backwards-compatible.

## Verification

1. `CGO_ENABLED=0 make build` — compiles cleanly
2. `CGO_ENABLED=0 make vet` — no issues
3. `CGO_ENABLED=0 make test` — unit + behaviour tests pass
4. Manual test with `DH_HOME=/workspace/.dh dh repl --vm`:
   - Type single-line code, press Enter → submits
   - Press Ctrl+O → inserts newline, input grows
   - Type multi-line code, press Enter → submits all lines
   - Press Up/Down → navigates history
   - Press Ctrl+R → reverse-i-search works
   - Press Ctrl+T → tab search overlay works
   - Press Ctrl+C on empty → quits
   - Border changes color during execution
