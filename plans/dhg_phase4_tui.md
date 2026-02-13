# Phase 4: Interactive TUI

**Depends on:** All Phase 1 packages + Phase 2 (versions) + Phase 3 (doctor)
**This is the final feature phase.**

## Goal

Implement the Bubbletea interactive TUI: main menu, setup wizard, and sub-screens. After this phase, `dhg` with no args launches a full interactive experience.

## Files to create/modify

```
go_src/
  internal/
    tui/
      app.go               # Top-level Bubbletea model, screen stack management
      keymap.go             # Shared keyMap definitions, help bindings
      styles.go             # Lipgloss styles (adaptive colors, shared constants)
      screens/
        mainmenu.go         # Main menu screen
        welcome.go          # Setup wizard: welcome
        javacheck.go        # Setup wizard: Java detection
        versionpicker.go    # Setup wizard: version selection
        installprogress.go  # Setup wizard: install progress
        done.go             # Setup wizard: completion
        versions.go         # Versions sub-screen
        servers.go          # Servers sub-screen
        doctor.go           # Doctor sub-screen
      components/
        logo.go             # ASCII Deephaven logo rendering
  cmd/dhg/
    setup.go               # dhg setup command (launches wizard)
    root.go                # Modify: no-args TTY path launches TUI
```

## Architecture

### Screen stack (`app.go`)
- `App` model holds a stack of `Screen` interfaces
- Push/pop screens for navigation (enter → push, esc → pop)
- Each `Screen` implements `Init()`, `Update()`, `View()`
- Terminal resize (`tea.WindowSizeMsg`) propagated to active screen
- Top-level catches `q`/`ctrl+c` for quit when at root screen

### Shared styles (`styles.go`)
- Adaptive colors: primary (`#7D56F4` dark / `#874BFD` light), success (green), warning (yellow), error (red), dim (gray)
- Header style, selected item style, dim description style, help bar style
- Border styles for boxes

### Shared keymap (`keymap.go`)
- Navigation: up/k, down/j, enter
- Global: ?, q, esc, ctrl+c
- Each screen extends with its own action keys

## Screens

### Main Menu (`mainmenu.go`)
- Shows when versions are installed
- ASCII logo in header (hidden if height < 20)
- Status line: active version, Java status, server count
- 5 items with title + description (description hidden if height < 15)
- Simple cursor model (not full `list` bubble)
- Cursor wraps around
- Enter pushes corresponding sub-screen
- Help bar: ShortHelp (navigate, enter, ?, q) / FullHelp (grouped columns)

### Setup Wizard
Five screens, linear flow. Driven by `dhg setup` or auto-launched on first run.

**welcome.go** — Logo, welcome text, "Get Started". Enter → push javacheck.

**javacheck.go** — Runs `java.Detect()` async via `tea.Cmd`. Shows spinner during detection. Result: found (show info, "Next" button) or missing (offer install/skip). Install runs `java.Install()` with progress.

**versionpicker.go** — Fetches versions from PyPI async. Shows Bubbles `list` with filtering. Enter on a version → push installprogress.

**installprogress.go** — Runs `versions.Install()` in goroutine. Shows Bubbles `progress` bar. Updates via `tea.Cmd` messages. On complete → push done.

**done.go** — Summary + quick start hints. Enter/q → quit.

### Sub-screens

**versions.go** — Lists installed versions via `versions.ListInstalled()`. Default marked with star. Action keys in help bar: `d` set default, `x` uninstall, `a` add new, `r` toggle remote. Context-sensitive: `d` disabled on current default.

**servers.go** — Lists servers via `discovery.Discover()`. Action keys: `x` kill, `o` open browser. Refreshes on each visit.

**doctor.go** — Runs doctor checks, displays results. `r` to refresh. No cursor navigation (static display).

### Root command integration (`root.go`)
- No args + TTY + no versions installed → `tea.NewProgram(tui.NewApp(wizardMode))`
- No args + TTY + versions installed → `tea.NewProgram(tui.NewApp(menuMode))`
- `dhg setup` → always launches wizard regardless of installed versions

## Tests

### Unit tests (`go_unit_tests/tui_test.go`)
- teatest with `WithInitialTermSize(80, 24)`
- `lipgloss.SetColorProfile(termenv.Ascii)` in test init
- Main menu: initial render golden file, cursor moves on j/k, enter returns correct screen ID
- Wizard: each screen model responds to expected messages
- Screen stack: push/pop works correctly
- Logo hidden at small terminal size

### Behaviour tests — TUI (`go_behaviour_tests/tui_test.go`)
Using go-expect + vt10x (pattern from CLAUDE.md):
- Main menu renders all items
- j/k navigation moves cursor
- Enter opens correct sub-screen
- Esc returns to previous screen
- q quits cleanly
- ? toggles help bar
- Setup wizard full flow: welcome → java → version → install → done
- Each wizard screen shows expected content
- Cursor wrapping (down from last → first)
- Golden file snapshots for each screen

### Behaviour tests — CLI (`go_behaviour_tests/testdata/scripts/setup.txtar`)
- `dhg setup --non-interactive --json` → JSON output with java + deephaven status

## Verification

```bash
./dhg                     # launches TUI (wizard or menu depending on state)
./dhg setup               # launches wizard explicitly
# Navigate through all screens manually
# Run test suites
cd go_unit_tests && go test ./...
cd go_behaviour_tests && go test ./...
```
