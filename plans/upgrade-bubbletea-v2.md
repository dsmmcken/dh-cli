# Upgrade Bubbletea v1.3.10 → v2.0.1

## Context

Bubbletea v2 was released with a new import domain (`charm.land/`), a declarative `View() tea.View` return type replacing `View() string`, `KeyMsg` split into `KeyPressMsg`/`KeyReleaseMsg`, and terminal features (alt screen, mouse mode) moved from program options to View fields. This plan covers migrating the entire Charm stack: bubbletea, bubbles, and lipgloss.

## Blocker: Bubbline Compatibility

`github.com/knz/bubbline` (used for the REPL's multi-line editor in `input.go`) depends on bubbletea v1. No v2-compatible version exists upstream. The `editline.Model` implements v1's `tea.Model` interface, so it cannot be used with v2 directly.

**Strategy: Inline the editline code.** Copy bubbline's `editline.Model` (~600 LOC) and the `history` package into `src/internal/repl/editline/` (or similar), then port it to v2 in-tree. This eliminates the external dependency entirely. The port involves the same mechanical changes as the rest of the migration: import paths, `View() string` → `View() tea.View`, `tea.KeyMsg` → `tea.KeyPressMsg`.

Key bubbline code to inline:
- `editline/` package — the `Model` struct, `InputCompleteMsg`, `ErrInterrupted`
- `history/` package — `LoadHistory()`, `SaveHistory()`
- Internal textarea wrapper (if bubbline wraps `bubbles/textarea`)

After inlining, remove the `github.com/knz/bubbline` dependency from `go.mod`.

## Dependency Updates

**src/go.mod** and **unit_tests/go.mod**:

| Old | New |
|-----|-----|
| `github.com/charmbracelet/bubbletea v1.3.10` | `charm.land/bubbletea/v2 v2.0.1` |
| `github.com/charmbracelet/bubbles v1.0.0` | `charm.land/bubbles/v2 v2.0.0` |
| `github.com/charmbracelet/lipgloss v1.1.0` | `charm.land/lipgloss/v2 v2.0.0` |
| `github.com/knz/bubbline` (indirect) | removed — code inlined into `src/internal/repl/` |

## Import Path Changes (all 25 files)

Mechanical find-and-replace across every file that imports charmbracelet packages:

| Old | New |
|-----|-----|
| `tea "github.com/charmbracelet/bubbletea"` | `tea "charm.land/bubbletea/v2"` |
| `"github.com/charmbracelet/bubbles/key"` | `"charm.land/bubbles/v2/key"` |
| `"github.com/charmbracelet/bubbles/help"` | `"charm.land/bubbles/v2/help"` |
| `"github.com/charmbracelet/bubbles/viewport"` | `"charm.land/bubbles/v2/viewport"` |
| `"github.com/charmbracelet/bubbles/table"` | `"charm.land/bubbles/v2/table"` |
| `"github.com/charmbracelet/bubbles/spinner"` | `"charm.land/bubbles/v2/spinner"` |
| `"github.com/charmbracelet/bubbles/progress"` | `"charm.land/bubbles/v2/progress"` |
| `"github.com/charmbracelet/lipgloss"` | `"charm.land/lipgloss/v2"` |

## API Changes

### 1. `View() string` → `View() tea.View` (12 models)

Only types that implement `tea.Model` need updating. Sub-components (`InputModel`, `TabBarModel`, `LogViewModel`, `TableViewModel`, `SidebarModel`) return their own type from `Update()`, NOT `tea.Model`, so their `View() string` methods stay as-is.

**Models that implement `tea.Model` (return `(tea.Model, tea.Cmd)` from Update):**

| File | Type |
|------|------|
| `src/internal/tui/app.go` | `App` |
| `src/internal/repl/app.go` | `REPLModel` |
| `src/internal/tui/screens/welcome.go` | `WelcomeScreen` |
| `src/internal/tui/screens/mainmenu.go` | `MainMenu` |
| `src/internal/tui/screens/javacheck.go` | `JavaCheckScreen` |
| `src/internal/tui/screens/doctor.go` | `DoctorScreen` |
| `src/internal/tui/screens/servers.go` | `ServersScreen` |
| `src/internal/tui/screens/versionpicker.go` | `VersionPickerScreen` |
| `src/internal/tui/screens/versions.go` | `VersionsScreen` |
| `src/internal/tui/screens/installprogress.go` | `InstallProgressScreen` |
| `src/internal/tui/screens/config.go` | `ConfigScreen` |
| `src/internal/tui/screens/done.go` | `DoneScreen` |

**Pattern for screens** (child models rendered by App):
```go
// Before
func (m WelcomeScreen) View() string {
    return content
}

// After
func (m WelcomeScreen) View() tea.View {
    return tea.NewView(content)
}
```

**Pattern for `App`** (top-level, owns AltScreen):
```go
// Before
func (a App) View() string {
    if len(a.stack) > 0 {
        return a.stack[len(a.stack)-1].View()
    }
    return ""
}

// After
func (a App) View() tea.View {
    var v tea.View
    if len(a.stack) > 0 {
        v = a.stack[len(a.stack)-1].View()
    }
    v.AltScreen = true
    return v
}
```

**Pattern for `REPLModel`** (top-level, owns AltScreen + mouse):
```go
// Before
func (m REPLModel) View() string {
    // ... builds string ...
    return mainArea
}

// After
func (m REPLModel) View() tea.View {
    // ... builds string (unchanged) ...
    v := tea.NewView(mainArea)
    v.AltScreen = true
    v.MouseMode = tea.MouseModeCellMotion
    return v
}
```

### 2. Program Options Removal (3 files)

Remove `tea.WithAltScreen()` and `tea.WithMouseCellMotion()` from `tea.NewProgram()` calls (now set declaratively in View):

- `src/internal/cmd/repl.go:116` — remove both options
- `src/internal/cmd/setup.go:39` — remove `tea.WithAltScreen()`
- `src/internal/cmd/root.go:81` — remove `tea.WithAltScreen()`

### 3. `tea.KeyMsg` → `tea.KeyPressMsg` (15 files)

Every `case tea.KeyMsg:` in type switches becomes `case tea.KeyPressMsg:`. The `msg.String()` method and `key.Matches(msg, binding)` work identically on `KeyPressMsg`.

Files with KeyMsg switches:
- `src/internal/repl/app.go` (line 100)
- `src/internal/repl/input.go` (lines 172, 195)
- `src/internal/repl/tabbar.go`
- `src/internal/tui/app.go` (line 72)
- All screen files that handle keyboard input

### 4. Lipgloss Color Changes

**`lipgloss.AdaptiveColor`** — removed in lipgloss v2. Use the compat shim:
```go
import "charm.land/lipgloss/v2/compat"

ColorPrimary = compat.AdaptiveColor{Light: "#2F71F2", Dark: "#4A90FF"}
```

Affects `src/internal/tui/styles.go` (5 color definitions).

**`lipgloss.TerminalColor`** type — removed in lipgloss v2. Replace with `lipgloss.Color` or `color.Color` where used as a variable type (e.g., `src/internal/repl/tabbar.go`).

### 5. Test Updates (`unit_tests/tui_test.go`)

**KeyMsg struct literals** (~16 occurrences):
```go
// Before
tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}
tea.KeyMsg{Type: tea.KeyEnter}

// After
tea.KeyPressMsg{Code: 'j', Text: "j"}
tea.KeyPressMsg{Code: tea.KeyEnter}
```

**View assertions** — `View()` now returns `tea.View` not `string`:
```go
// Before
view := updated.View()
assert.Contains(t, view, "text")

// After — extract string content from tea.View
view := updated.View()
assert.Contains(t, view.String(), "text")  // or view.Content depending on API
```

## Unchanged APIs

These are used in the codebase and remain compatible in v2:
- `tea.Batch()`, `tea.Quit`, `tea.Model`, `tea.Cmd`, `tea.Msg`
- `tea.WindowSizeMsg` (same fields: Width, Height)
- `tea.NewProgram()`, `p.Run()`
- `key.NewBinding()`, `key.Matches()`, `key.WithKeys()`, `key.WithHelp()`
- `lipgloss.NewStyle()`, all style methods, `JoinHorizontal/Vertical`, border functions
- `lipgloss.Color("#hex")` — still works
- All bubbles components (viewport, table, help, spinner, progress) have v2 equivalents with compatible APIs

## Execution Order

The project won't compile between import path changes and API updates, so these must be done together in one session:

1. Inline bubbline editline/history code into `src/internal/repl/editline/`
2. Update `go.mod` files (both `src/` and `unit_tests/`), drop `github.com/knz/bubbline`
3. Import path replacements (mechanical, all 25 files)
4. `KeyMsg` → `KeyPressMsg` (all switch statements)
5. `View()` signature changes (12 models + content extraction in App)
6. Remove program options from cmd files
7. Lipgloss color migration (`AdaptiveColor` → compat, `TerminalColor` removal)
8. Test updates (KeyMsg literals, View assertions)
9. `go mod tidy` in both modules
10. Build and test

## Verification

```bash
CGO_ENABLED=0 make build    # compiles
CGO_ENABLED=0 make vet      # no warnings
CGO_ENABLED=0 make test     # unit + behaviour tests pass
```

Manual smoke tests (require Java/VM environment):
- `dh` — main menu launches with alt screen
- `dh setup` — wizard flow works
- `dh repl` — input, history, tabs, mouse scroll, Ctrl+C quit
