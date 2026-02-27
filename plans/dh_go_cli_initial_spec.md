# dhg — Go CLI for Deephaven (Phase 1 Specification)

## Context

The existing `dh` CLI is a ~4600-line Python tool built with argparse and Textual. We're migrating to Go for faster startup, single-binary distribution, and the Charm TUI ecosystem (Bubbletea, Bubbles, Huh, Lipgloss). During migration, the new tool lives as `dh` with config in `~/.dh`, running alongside `dh` without conflict.

Source code goes in `go_src/`, tests in `go_behaviour_tests/` and `go_unit_tests/`. The Go binary is packaged as a Python wheel via `go-to-wheel` and installed with `uv tool install`.

Phase 1 covers: setup wizard, version management, Java management, server discovery, config, doctor. No REPL, no exec, no serve.

---

## Two Interaction Modes

Every feature is accessible both ways:

1. **Interactive TUI** — `dh` with no args (or `dh setup`). Bubbletea app with Huh forms, Bubbles components, Lipgloss styling. For humans at a terminal.

2. **CLI commands with flags** — `dh <command> [flags]`. Non-interactive, structured output. Every command supports `--json` for machine-parseable output. For AI agents and scripts.

---

## Global Flags

Available on every subcommand:

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--json` | `-j` | `false` | JSON to stdout. Errors as JSON to stderr. |
| `--verbose` | `-v` | `false` | Extra detail to stderr. |
| `--quiet` | `-q` | `false` | Suppress non-essential output. |
| `--no-color` | | auto | Disable ANSI. Auto-off when stdout is not a TTY. |
| `--help` | `-h` | | Help for the command. |
| `--version` | | | Print dhg version (root only). |

Environment variables: `DH_HOME` (config dir), `NO_COLOR` (standard), `DH_JSON=1`.

`--json` implies `--quiet` for human text. `--verbose` and `--quiet` are mutually exclusive.

---

## Command Tree

```
dhg                              # No args → TUI (setup wizard or main menu)
dh setup                        # Run setup wizard explicitly
  --non-interactive              # Auto-detect Java, install latest DH, for CI

dh install [VERSION]            # Install a Deephaven version
  --no-plugins                   # Skip plugin installation
  --python <ver>                 # Python version for venv (default: 3.13)

dh uninstall <VERSION>          # Remove an installed version
  --force                        # Skip confirmation

dh use <VERSION>                # Set default version
  --local                        # Write .dhrc in cwd instead of global config

dh versions                     # List installed versions
  --remote                       # Also show PyPI versions
  --limit <n>                    # Max remote versions (default: 20)
  --all                          # Show all remote versions

dh java                         # Show Java status
dh java install                 # Download Eclipse Temurin JDK 21
  --jdk-version <ver>            # JDK version (default: 21)
  --force                        # Re-download if already present

dh list                         # List running Deephaven servers
dh kill <PORT>                  # Stop a server by port

dh doctor                       # Check environment health
  --fix                          # Attempt auto-fix

dh config                       # Show all config
dh config set <KEY> <VALUE>     # Set a value
dh config get <KEY>             # Get a value (raw, for scripts)
dh config path                  # Print config file path
```

---

## Command Details

### `dh` (no args)

If TTY attached and no versions installed → launch setup wizard.
If TTY attached and versions installed → launch main menu TUI.
If not TTY → print help and exit.

### `dh setup`

Walk through: Java detection → Java install if needed → version picker → install → done.

```bash
dh setup                           # Interactive wizard
dh setup --non-interactive --json  # CI: auto-setup, JSON result
```

JSON output:
```json
{
  "java": {"found": true, "version": "21.0.5", "path": "/usr/lib/jvm/java-21/bin/java", "source": "JAVA_HOME"},
  "deephaven": {"version": "42.0", "installed": true, "set_as_default": true}
}
```

### `dh install [VERSION]`

Install into `~/.dh/versions/<VERSION>/.venv`. Default: latest from PyPI.

```bash
dh install              # Latest
dh install 42.0 --json  # Specific version, JSON output
```

JSON output:
```json
{
  "version": "42.0",
  "status": "installed",
  "path": "/home/user/.dh/versions/42.0",
  "set_as_default": true,
  "elapsed_seconds": 42.3
}
```

### `dh uninstall <VERSION>`

Remove a version. Prompts for confirmation unless `--force`.

### `dh use <VERSION>`

Set default. `--local` writes `.dhrc` in cwd.

JSON: `{"version": "42.0", "scope": "global", "config_path": "~/.dh/config.toml"}`

### `dh versions`

List installed versions. `--remote` adds PyPI versions.

JSON:
```json
{
  "installed": [
    {"version": "42.0", "is_default": true, "installed_at": "2025-02-10T15:30:00Z"}
  ],
  "default_version": "42.0",
  "remote": ["42.0", "41.1", "41.0"]
}
```

### `dh java`

Show detection result. Checks JAVA_HOME, PATH, `~/.dh/java/`.

JSON: `{"found": true, "version": "21.0.5", "path": "...", "home": "...", "source": "JAVA_HOME"}`

### `dh java install`

Download Eclipse Temurin JDK 21 to `~/.dh/java/`.

### `dh list`

Discover running Deephaven servers. Linux: `/proc/net/tcp`. macOS: `lsof`. Docker: `docker ps`.

JSON:
```json
{
  "servers": [
    {"port": 10000, "pid": 12345, "source": "dh serve", "script": "dashboard.py"},
    {"port": 8080, "pid": 0, "source": "docker", "container_id": "abc123"}
  ]
}
```

### `dh kill <PORT>`

Stop server on port. SIGTERM for processes, `docker stop` for containers.

### `dh doctor`

Check: uv installed, Java found, versions installed, default set, disk space.

JSON:
```json
{
  "healthy": true,
  "checks": [
    {"name": "uv", "status": "ok", "detail": "/home/user/.local/bin/uv"},
    {"name": "java", "status": "ok", "detail": "Java 21.0.5"},
    {"name": "versions", "status": "ok", "detail": "2 installed"},
    {"name": "default_version", "status": "ok", "detail": "42.0"}
  ]
}
```

### `dh config` / `config set` / `config get` / `config path`

View and modify `~/.dh/config.toml`.

---

## TUI Screens

All screens follow Bubbletea conventions: three-part layout (header → content → help bar), `j`/`k` + arrow keys for navigation, `enter` to select, `q`/`esc`/`ctrl+c` to go back or quit. The Bubbles `help` component auto-generates the keybinding bar. Lipgloss adaptive colors for light/dark terminal themes.

### Main Menu (versions already installed)

Simple cursor-based list (not the full filterable `list` bubble — overkill for 6 items). Each item has a title and description line. The header shows the ASCII logo, active version, and Java status at a glance.

```
    ____                 __
   / __ \___  ___  ____ / /_  ____ ___  _____  ____
  / / / / _ \/ _ \/ __ \/ __ \/ __ `/ | / / _ \/ __ \
 / /_/ /  __/  __/ /_/ / / / / /_/ /| |/ /  __/ / / /
/_____/\___/\___/ .___/_/ /_/\__,_/ |___/\___/_/ /_/
               /_/

  Active: v42.0 (default)  ╎  Java 21.0.5  ╎  2 servers running

  > Manage versions
    Install, remove, and switch between Deephaven versions

    Running servers
    View and manage active Deephaven processes

    Java status
    Check or install Java runtime

    Environment doctor
    Diagnose and fix setup issues

    Configuration
    View and edit settings

  ↑/k up • ↓/j down • enter select • ? more • q quit
```

Press `?` to expand full help:
```
  Navigation              Actions
  ↑/k  up                 enter  open             ? toggle help
  ↓/j  down                                       q quit
```

**Design notes:**
- **Header**: ASCII logo rendered with Lipgloss adaptive color (purple `#7D56F4` dark / `#874BFD` light). Status line uses dim separators (`╎`). Green for healthy status, yellow for warnings.
- **Cursor**: `>` prefix on selected item title, bold + primary color. Unselected items indented to align. Description line always dim regardless of selection.
- **Help bar**: Bubbles `help` component with `ShortHelp` / `FullHelp` convention:
  - **ShortHelp** (default, one line): Navigation + enter + `?` + quit. Just enough to orient the user.
  - **FullHelp** (press `?`): All keybindings in grouped columns. This is where users discover action shortcuts. Columns organized semantically (Navigation, Actions, etc.).
  - Keys defined as `key.Binding` with `key.WithHelp("d", "set default")` — the help component renders them automatically.
  - Context-sensitive: keys enable/disable based on screen state via `SetEnabled()`, help bar auto-hides disabled keys.
- **Window resize**: `tea.WindowSizeMsg` recalculates layout. Logo hidden if terminal height < 20 rows. Description lines hidden if height < 15.
- **Wrapping**: Cursor wraps — down from last item goes to first, up from first goes to last.

### Setup Wizard Flow (first run or `dh setup`)

Uses Huh forms for the multi-step wizard. Each step is a Huh group. Progress shown as `Step N of M` in header.

**Screen 1: Welcome**
```
    ____                 __
   / __ \___  ___  ____ / /_  ____ ___  _____  ____
  / / / / _ \/ _ \/ __ \/ __ \/ __ `/ | / / _ \/ __ \
 / /_/ /  __/  __/ /_/ / / / / /_/ /| |/ /  __/ / / /
/_____/\___/\___/ .___/_/ /_/\__,_/ |___/\___/_/ /_/
               /_/

  Welcome! Let's get your environment ready.

  This wizard will:
    1. Check for Java (or install it)
    2. Install a Deephaven engine version
    3. Get you started

  > Get Started

  enter continue • q quit
```

**Screen 2: Java Check** (`Step 1 of 3`)
```
  Step 1 of 3 — Java

  Checking for Java 17+...  ⣾

  ────────────────────────────────

  ✓ Java 21.0.5 found
    /usr/lib/jvm/java-21/bin/java (JAVA_HOME)

  > Next

  enter continue • q quit
```

If Java missing:
```
  Step 1 of 3 — Java

  ✗ No compatible Java found

  Deephaven requires Java 17+.
  We can install Eclipse Temurin 21 to ~/.dh/java/
  (no sudo required).

  > Install Java
    Skip for now

  ↑/k up • ↓/j down • enter select • q quit
```

**Screen 3: Version Picker** (`Step 2 of 3`)
```
  Step 2 of 3 — Deephaven Version

  Select a version to install:

  > 42.0    latest
    41.1
    41.0
    0.37.1
    0.37.0
    0.36.1

  ↑/k up • ↓/j down • enter install • q quit
```

Uses Bubbles `list` with filtering if >10 versions shown. Versions fetched async from PyPI with spinner while loading.

**Screen 4: Install Progress** (`Step 3 of 3`)
```
  Step 3 of 3 — Installing

  Installing Deephaven 42.0...

  ████████████████████░░░░░░░░░░  67%

  Installing deephaven-plugin-ui...
```

Bubbles `progress` bar. Installation runs in a goroutine via `tea.Cmd`.

**Screen 5: Done**
```
  ✓ Setup Complete

  Deephaven 42.0 installed and set as default.

  Quick start:
    dh versions       Manage versions
    dh list           See running servers
    dh doctor         Check environment

  > Done

  enter finish • q quit
```

### Sub-screens

All sub-screens share the same layout pattern: title header, content area, help bar at bottom. Each screen defines its own `keyMap` implementing `ShortHelp()` and `FullHelp()`. Action shortcuts live in the help bar — not inline with items.

**Versions screen** — Bubbles `list` for installed versions. Action keys are context-sensitive (e.g. `d` disabled on the item already marked default).

```
  Installed Versions

  > 42.0  ★ default       installed 2025-02-10
    41.1                   installed 2025-02-01
    41.0                   installed 2025-01-15

  ↑/k up • ↓/j down • d default • x uninstall • ? more • esc back
```

Press `?`:
```
  Navigation              Actions
  ↑/k  up                 d  set default           ? toggle help
  ↓/j  down               x  uninstall             esc back
                           a  add new version       q quit
                           r  toggle remote
```

**Servers screen** — Discovered servers with per-server actions in help bar.

```
  Running Servers

  > :10000  pid 12345  dh serve   dashboard.py
    :8080   docker     ghcr.io/deephaven/server:latest

  ↑/k up • ↓/j down • enter details • ? more • esc back
```

Press `?`:
```
  Navigation              Actions
  ↑/k  up                 enter  details           ? toggle help
  ↓/j  down               x      kill              esc back
                           o      open in browser   q quit
```

**Doctor screen** — Static checklist, no cursor navigation. Minimal help bar.

```
  Environment Health

  ✓ uv         /home/user/.local/bin/uv
  ✓ Java       21.0.5 (JAVA_HOME)
  ✓ Versions   2 installed
  ✓ Default    42.0
  ⚠ Disk       2.1 GB free in ~/.dh

  Everything looks good (1 warning).

  r refresh • esc back • q quit
```

---

## `~/.dh/` Directory Structure

```
~/.dh/
  config.toml              # User config
  state.json               # Tool state (first_run_completed, etc.)
  versions/
    42.0/
      .venv/               # Python venv (created by uv)
      meta.toml            # Install metadata
  java/
    jdk-21.0.5+11/         # Managed JDK
  cache/                   # Download cache, PyPI cache
```

### config.toml

```toml
default_version = "42.0"

[install]
plugins = [
  "deephaven-plugin-ui",
  "deephaven-plugin-plotly-express",
  "deephaven-plugin-theme-pack",
]
python_version = "3.13"
```

### Version Resolution Precedence

1. `--version` flag
2. `DH_VERSION` env var
3. `.dhrc` in cwd (walk up)
4. `config.toml` default_version
5. Latest installed version

---

## Output Conventions

### JSON Mode (`--json`)

- **stdout**: Single JSON object per command
- **stderr**: Progress/warnings as JSON lines: `{"level": "info", "message": "..."}`
- No ANSI escapes in JSON mode

### Error JSON

```json
{"error": "version_not_found", "message": "Version 99.0 not found on PyPI"}
```

Error codes: `version_not_found`, `version_not_installed`, `java_not_found`, `uv_not_found`, `install_failed`, `network_error`, `server_not_found`, `permission_denied`, `config_error`

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Network error |
| 3 | Timeout |
| 4 | Resource not found |
| 130 | Interrupted (Ctrl+C) |

---

## First Launch: CWD Venv Detection

On first run, `dh` also checks if the current directory has a venv with deephaven-server:

1. Look for `.venv/` or `venv/` in cwd
2. Check for `deephaven_server` in site-packages
3. If found: report in setup wizard, offer to use it or install a managed version

---

## Go Project Structure

```
go_src/
  go.mod
  go.sum
  cmd/dhg/
    main.go
    root.go                # Root cobra command + global flags
    setup.go
    install.go
    uninstall.go
    use.go
    versions.go
    java.go                # java command group
    java_install.go
    list.go
    kill.go
    doctor.go
    config.go              # config command group
  internal/
    config/                # Config loading, .dhrc, version resolution
    java/                  # Java detection, Adoptium download
    versions/              # Install/uninstall, PyPI client, list
    discovery/             # Server discovery (/proc, lsof, docker)
    output/                # JSON helpers, table formatting, progress
    tui/
      app.go               # Top-level Bubbletea model + screen stack
      screens/             # welcome, java_check, version_picker, etc.
      components/          # Reusable styled components
```

## Packaging

1. Go binary cross-compiled with `CGO_ENABLED=0` (static)
2. `go-to-wheel` packages into platform-specific Python wheels
3. Wheel contains the binary, Python wrapper calls `os.execvp`
4. Install: `uv tool install dh` or `pip install dh`
5. `dh` appears on PATH

---

## Implementation Phases

This spec is broken into phases with detailed plans in `plans/dhg_phase*.md`. Each phase is independently testable with its own unit and behaviour tests.

### Dependency Graph

```
Phase 0: Scaffold (must complete first)
  │
  ├── Phase 1a: Config     ─┐
  ├── Phase 1b: Java         ├── all parallel after Phase 0
  └── Phase 1c: Discovery  ─┘
          │
          ├── Phase 2: Versions (needs 1a)
          │
          └── Phase 3: Doctor (needs 1a + 1b + 1c + 2)
                │
                Phase 4: TUI (needs all internal packages)
                │
                Phase 5: Packaging (needs working binary)
```

### Phase Summary

| Phase | Name | Depends On | Parallel With | Deliverable |
|-------|------|-----------|---------------|-------------|
| **0** | [Scaffold](dhg_phase0_scaffold.md) | — | — | Go module, Cobra root, global flags, `--version`, `--help`, test harnesses |
| **1a** | [Config](dhg_phase1a_config.md) | 0 | 1b, 1c | `~/.dh/`, config.toml, `.dhrc`, `dh config` commands |
| **1b** | [Java](dhg_phase1b_java.md) | 0 | 1a, 1c | Java detection, Temurin install, `dh java` commands |
| **1c** | [Discovery](dhg_phase1c_discovery.md) | 0 | 1a, 1b | Server discovery (/proc, lsof, docker), `dh list`, `dh kill` |
| **2** | [Versions](dhg_phase2_versions.md) | 1a | 1b, 1c | Install/uninstall/use/versions via uv, PyPI client |
| **3** | [Doctor](dhg_phase3_doctor.md) | 1a, 1b, 1c, 2 | — | `dh doctor` integrating all subsystem checks |
| **4** | [TUI](dhg_phase4_tui.md) | all above | — | Bubbletea main menu, setup wizard, sub-screens |
| **5** | [Packaging](dhg_phase5_packaging.md) | all above | — | go-to-wheel → Python wheel → `uv tool install` |

### Parallelism

- **Phases 1a, 1b, 1c** are fully independent and can be implemented simultaneously
- **Phase 2** can start as soon as Phase 1a (config) is done — no need to wait for 1b/1c
- **Phase 3** is a thin integration layer; fast to implement once its dependencies land
- **Phase 4** (TUI) and **Phase 5** (packaging) are sequential

### Future Phases (deferred, beyond this spec)

- `dh exec`, `dh serve`, `dh repl` — runtime commands that launch Deephaven
- `dh lint`, `dh format`, `dh typecheck` — dev tool wrappers

---

## Testing

Two test suites with strict separation of concerns.

### Directory Layout

```
go_unit_tests/          # White-box tests — imports internal packages
  config_test.go        # Config loading, version resolution, .dhrc
  java_test.go          # Java detection, version parsing
  versions_test.go      # Install/uninstall logic, PyPI client
  discovery_test.go     # Server discovery, /proc parsing, classification
  output_test.go        # JSON envelope, table formatting
  tui_test.go           # Bubbletea model Update/View via teatest
  cmd_test.go           # Cobra command wiring, flag parsing, output capture

go_behaviour_tests/     # Black-box tests — only invokes the compiled binary
  CLAUDE.md             # Rules for writing behaviour tests (see below)
  cli_test.go           # Non-interactive CLI command tests (testscript)
  tui_test.go           # Interactive TUI tests (go-expect + vt10x)
  helpers_test.go       # Shared test helpers (binary path, PTY setup)
  testdata/
    scripts/            # testscript .txtar files (non-interactive)
      version.txtar
      install.txtar
      java.txtar
      config.txtar
      doctor.txtar
      list.txtar
      kill.txtar
      setup.txtar
      json_output.txtar
      error_codes.txtar
      help.txtar
      global_flags.txtar
    golden/             # Golden files for TUI screen snapshots
      main_menu.golden
      setup_welcome.golden
      setup_java_found.golden
      setup_java_missing.golden
      setup_version_picker.golden
      setup_done.golden
      versions_screen.golden
      servers_screen.golden
      doctor_screen.golden
```

---

### Unit Tests (`go_unit_tests/`)

Follow standard Go + Bubbletea + Cobra conventions. These tests import internal packages and test implementation details.

#### Cobra Command Tests

Use `cmd.SetOut()`, `cmd.SetArgs()`, and output capture. Table-driven.

```go
func TestVersionsCommand(t *testing.T) {
    tests := []struct {
        name    string
        args    []string
        wantOut string
        wantErr bool
    }{
        {"no flags", []string{"versions"}, "No versions installed", false},
        {"json flag", []string{"versions", "--json"}, `"installed":[]`, false},
        {"remote flag", []string{"versions", "--remote"}, "PyPI", false},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            cmd := NewRootCmd()
            buf := bytes.NewBufferString("")
            cmd.SetOut(buf)
            cmd.SetArgs(tt.args)
            err := cmd.Execute()
            // assert on buf.String() and err
        })
    }
}
```

#### Bubbletea TUI Tests

Use `teatest` with golden files and fixed terminal sizes. Set `lipgloss.SetColorProfile(termenv.Ascii)` in test init to prevent CI/CD flakiness.

```go
func TestMainMenuView(t *testing.T) {
    m := NewMainMenu(testConfig())
    tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
    // verify initial render
    out, _ := io.ReadAll(tm.FinalOutput(t))
    teatest.RequireEqualOutput(t, out)  // golden file comparison
}

func TestMainMenuNavigation(t *testing.T) {
    m := NewMainMenu(testConfig())
    tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
    // press j to move down
    tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
    // press enter to select
    tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
    fm := tm.FinalModel(t)
    // assert model state
}
```

Golden files stored in `go_unit_tests/testdata/*.golden`, updated with `go test -update`.

#### Internal Package Tests

- **config**: Test TOML parsing, version resolution precedence, `.dhrc` walk-up
- **java**: Test version string parsing, path detection logic (mock filesystem)
- **versions**: Test `meta.toml` read/write, version sorting. Mock `exec.Command` for `uv` calls using the standard Go pattern (package-level `var execCommand = exec.Command`, replace in tests)
- **discovery**: Test `/proc/net/tcp` parsing with fixture data, `lsof` output parsing, process classification
- **output**: Test JSON envelope construction, table column alignment, error code mapping

---

### Behaviour Tests (`go_behaviour_tests/`)

Black-box tests that invoke the compiled `dh` binary via `os/exec` or `testscript`. These tests have **zero knowledge of implementation** — they only know the CLI's public interface (commands, flags, stdout, stderr, exit codes, files written to `~/.dh/`).

Uses `testscript` (`github.com/rogpeppe/go-internal/testscript`) for declarative test scenarios in `.txtar` files.

#### testscript Setup

```go
// go_behaviour_tests/dhg_test.go
func TestMain(m *testing.M) {
    os.Exit(testscript.RunMain(m, nil))
}

func TestBehaviour(t *testing.T) {
    testscript.Run(t, testscript.Params{
        Dir:                 "testdata/scripts",
        RequireExplicitExec: true,
    })
}
```

#### Example Test Scenarios

**`version.txtar`** — version flag and versions command:
```
# --version prints version string and exits 0
exec dh --version
stdout '^dhg v[0-9]+\.[0-9]+\.[0-9]+'
! stderr .

# versions with nothing installed shows empty
exec dh versions --json
stdout '"installed":\[\]'
```

**`config.txtar`** — config commands:
```
# config path prints the config file location
exec dh config path
stdout '\.dhg/config\.toml'

# config set writes a value
exec dh config set default_version 42.0
exec dh config get default_version
stdout '^42\.0$'

# config shows all values
exec dh config --json
stdout '"default_version":"42.0"'
```

**`json_output.txtar`** — JSON mode contract:
```
# --json flag produces valid JSON on stdout
exec dh doctor --json
stdout '^\{'
stdout '"healthy"'
stdout '"checks"'

# --json errors produce JSON error objects
! exec dh kill 99999 --json
stdout '"error"'
stdout '"message"'
```

**`error_codes.txtar`** — exit code contract:
```
# unknown command exits 1
! exec dhg nonexistent
stderr 'unknown command'

# kill nonexistent server exits 4
! exec dh kill 99999
```

**`help.txtar`** — help output:
```
# --help shows usage
exec dh --help
stdout 'Usage:'
stdout 'dh \[command\]'

# subcommand help
exec dh install --help
stdout 'Install a Deephaven version'
stdout '\-\-no-plugins'
```

**`global_flags.txtar`** — global flag behaviour:
```
# --no-color disables ANSI
exec dh doctor --no-color
! stdout '\x1b\['

# --quiet suppresses non-essential output
exec dh doctor --quiet
! stdout 'Environment'

# --json implies --quiet for human text
exec dh doctor --json
! stdout 'Environment Health'
stdout '"healthy"'
```

**`java.txtar`** — Java detection:
```
# java command reports status as JSON
exec dh java --json
stdout '"found"'
stdout '"version"'
stdout '"path"'
```

**`doctor.txtar`** — doctor checks:
```
# doctor runs all checks
exec dh doctor
stdout 'uv'
stdout 'Java'

# doctor --json returns structured checks
exec dh doctor --json
stdout '"checks"'
stdout '"name"'
stdout '"status"'
```

#### TUI Behaviour Tests

Interactive TUI tests use **Netflix/go-expect** (`github.com/Netflix/go-expect`) with **vt10x** (`github.com/hinshun/vt10x`) to spawn the `dh` binary in a pseudo-terminal. go-expect drives keystrokes, vt10x parses ANSI escape sequences into a virtual screen buffer, and assertions run against the rendered text.

```go
// tui_test.go — helper pattern
func spawnTUI(t *testing.T, args ...string) (*expect.Console, *vt10x.VT) {
    t.Helper()
    vt := vt10x.New(vt10x.WithSize(80, 24))
    console, err := expect.NewConsole(
        expect.WithStdout(vt),
        expect.WithDefaultTimeout(5 * time.Second),
    )
    require.NoError(t, err)
    cmd := exec.Command(dhgBinary, args...)
    cmd.Stdin = console.Tty()
    cmd.Stdout = console.Tty()
    cmd.Stderr = console.Tty()
    cmd.Env = append(os.Environ(), "DH_HOME="+t.TempDir())
    require.NoError(t, cmd.Start())
    t.Cleanup(func() { console.Close(); cmd.Process.Kill() })
    return console, vt
}
```

**Main menu tests:**
```go
func TestTUI_MainMenu_Renders(t *testing.T) {
    console, vt := spawnTUI(t)
    // Wait for menu to appear
    console.ExpectString("Manage versions")
    // Assert all menu items present in screen buffer
    screen := vt.String()
    assert.Contains(t, screen, "Running servers")
    assert.Contains(t, screen, "Java status")
    assert.Contains(t, screen, "Environment doctor")
    assert.Contains(t, screen, "Configuration")
}

func TestTUI_MainMenu_NavigateDown(t *testing.T) {
    console, vt := spawnTUI(t)
    console.ExpectString("Manage versions")
    // Press j twice to move to "Java status"
    console.Send("j")
    console.Send("j")
    time.Sleep(50 * time.Millisecond)
    // The cursor indicator should be on the third item
    screen := vt.String()
    // Assert cursor position (> prefix on Java status line)
}

func TestTUI_MainMenu_SelectVersions(t *testing.T) {
    console, vt := spawnTUI(t)
    console.ExpectString("Manage versions")
    // Enter selects current item → versions screen
    console.SendLine("")
    console.ExpectString("Installed Versions")
}

func TestTUI_MainMenu_QuitWithQ(t *testing.T) {
    console, _ := spawnTUI(t)
    console.ExpectString("Manage versions")
    console.Send("q")
    // Process should exit cleanly
    // (cmd.Wait() returns nil)
}

func TestTUI_MainMenu_HelpToggle(t *testing.T) {
    console, vt := spawnTUI(t)
    console.ExpectString("Manage versions")
    // Press ? to expand full help
    console.Send("?")
    time.Sleep(50 * time.Millisecond)
    screen := vt.String()
    assert.Contains(t, screen, "Navigation")
    // Press ? again to collapse
    console.Send("?")
}
```

**Setup wizard tests:**
```go
func TestTUI_SetupWizard_WelcomeScreen(t *testing.T) {
    // With no versions installed, dhg launches wizard
    console, vt := spawnTUI(t)
    console.ExpectString("Welcome")
    screen := vt.String()
    assert.Contains(t, screen, "Get Started")
}

func TestTUI_SetupWizard_JavaCheck(t *testing.T) {
    console, vt := spawnTUI(t)
    console.ExpectString("Get Started")
    console.SendLine("") // press enter
    console.ExpectString("Java")
    screen := vt.String()
    // Should show Java found or missing
    assert.True(t,
        strings.Contains(screen, "found") ||
        strings.Contains(screen, "No compatible Java"),
    )
}

func TestTUI_SetupWizard_FullFlow(t *testing.T) {
    console, _ := spawnTUI(t)
    // Welcome → enter
    console.ExpectString("Get Started")
    console.SendLine("")
    // Java check → next
    console.ExpectString("Java")
    console.ExpectString("Next")
    console.SendLine("")
    // Version picker → select first and enter
    console.ExpectString("Select a version")
    console.SendLine("")
    // Install progress
    console.ExpectString("Installing")
    // Done screen
    console.ExpectString("Setup Complete")
}
```

**Sub-screen tests:**
```go
func TestTUI_VersionsScreen_EscGoesBack(t *testing.T) {
    console, _ := spawnTUI(t)
    console.ExpectString("Manage versions")
    console.SendLine("") // enter versions screen
    console.ExpectString("Installed Versions")
    console.Send("\x1b") // ESC
    console.ExpectString("Manage versions") // back at main menu
}

func TestTUI_DoctorScreen_ShowsChecks(t *testing.T) {
    console, vt := spawnTUI(t)
    console.ExpectString("Manage versions")
    // Navigate to doctor (4th item)
    console.Send("jjj")
    console.SendLine("")
    console.ExpectString("Environment Health")
    screen := vt.String()
    assert.Contains(t, screen, "uv")
    assert.Contains(t, screen, "Java")
}
```

**Golden file snapshots** for visual regression: capture the vt10x screen buffer as text, compare against `testdata/golden/*.golden`. Update with `go test -update`.

---

#### What Behaviour Tests Cover

Every feature is tested through the CLI and TUI surface:

**CLI commands** (testscript):

| Feature | What to Assert |
|---------|---------------|
| `dh --version` | Version string format, exit 0 |
| `dh --help` | Usage text, all subcommands listed |
| `dh <cmd> --help` | Per-command help, flags documented |
| `dh --json` | Valid JSON on stdout, no ANSI |
| `dh --quiet` | Suppressed human output |
| `dh --no-color` | No ANSI escape sequences |
| `dh versions` | Lists installed versions |
| `dh versions --json` | JSON array of versions |
| `dh versions --remote` | Includes PyPI versions |
| `dh install <ver>` | Creates `~/.dh/versions/<ver>/` |
| `dh uninstall <ver>` | Removes version dir |
| `dh use <ver>` | Updates config.toml |
| `dh use <ver> --local` | Creates `.dhrc` in cwd |
| `dh java` | Reports Java status |
| `dh java --json` | JSON with found/version/path |
| `dh java install` | Creates `~/.dh/java/` |
| `dh config` | Shows config |
| `dh config set K V` | Persists value |
| `dh config get K` | Returns raw value |
| `dh config path` | Returns file path |
| `dh list` | Discovers servers |
| `dh list --json` | JSON array of servers |
| `dh kill <port>` | Stops server, exit 0 |
| `dh kill <bad>` | Exit 4, error message |
| `dh doctor` | All checks reported |
| `dh doctor --json` | Structured check results |
| Error cases | Correct exit codes per table |
| Unknown commands | Exit 1, helpful error |
| Mutual exclusivity | `--verbose` + `--quiet` rejected |

**TUI screens** (go-expect + vt10x):

| Screen | What to Assert |
|--------|---------------|
| Main menu | All 5 items render, cursor navigation (j/k/arrows), enter selects, q quits, ? toggles help, esc quits |
| Main menu header | ASCII logo present, active version shown, Java status shown |
| Main menu resize | Logo hidden when height < 20, descriptions hidden when height < 15 |
| Setup wizard welcome | Logo, welcome text, "Get Started" prompt |
| Setup wizard Java check | Spinner during detection, result shown (found or missing), install option when missing |
| Setup wizard version picker | List of versions from PyPI, cursor navigation, enter installs |
| Setup wizard install progress | Progress bar advances, completion message |
| Setup wizard done | Summary, quick start hints, "Done" prompt |
| Versions sub-screen | Lists installed versions, default marked, actions in help bar (d/x/a/r), esc returns to menu |
| Servers sub-screen | Lists discovered servers, actions in help bar (x/o), esc returns to menu |
| Doctor sub-screen | All checks rendered with status indicators, esc returns to menu |
| Full wizard flow | Welcome → Java → Version → Install → Done (end-to-end) |
| Cursor wrapping | Down from last item wraps to first, up from first wraps to last |
| Screen transitions | Enter on menu item opens correct sub-screen, esc always returns |

---

## Verification

After implementation:

1. `cd go_src && go build ./cmd/dh` — binary compiles
2. `cd go_unit_tests && go test ./...` — all unit tests pass
3. `cd go_behaviour_tests && go test ./...` — all behaviour tests pass
4. `./dh --version` — prints version
5. `./dh doctor --json` — JSON output with all checks
6. `./dh java --json` — detects Java
7. `./dh versions --json` — lists (empty) installed versions
8. `./dh` (in terminal) — launches TUI setup wizard
9. Walk through wizard: Java check → version pick → install → done
10. `./dh versions` — shows installed version
11. `./dh list --json` — discovers servers
12. Package with go-to-wheel, install via `uv tool install`, verify `dh` on PATH
