# Phase 0: Project Scaffold

**Must complete before all other phases.**

## Goal

Set up the Go module, directory structure, Cobra root command with global flags, and the test harnesses for both test suites. After this phase, `dh --version`, `dh --help`, and `go test` in both test directories work.

## Deliverables

### Go module and directory structure

```
go_src/
  go.mod                   # module github.com/dsmmcken/dh-cli/go_src
  go.sum
  cmd/dhg/
    main.go                # func main() { cmd.Execute() }
    root.go                # Root Cobra command + global flags
  internal/
    output/
      output.go            # Stub: JSON/human output helpers, exit code constants

go_unit_tests/
  go.mod                   # Separate module, depends on go_src
  cmd_test.go              # Root command tests
  output_test.go           # Output helper tests

go_behaviour_tests/
  go.mod                   # Separate module, no dependency on go_src
  CLAUDE.md                # Already exists
  cli_test.go              # testscript runner
  tui_test.go              # go-expect + vt10x runner (empty, placeholder)
  helpers_test.go          # Binary path resolution, test setup
  testdata/
    scripts/
      version.txtar        # --version test
      help.txtar            # --help test
      global_flags.txtar    # --json, --quiet, --no-color, --verbose
      error_codes.txtar     # Unknown command exits 1
```

### Root command (`root.go`)

- `dh` with version string (injected via `-ldflags -X`)
- Global persistent flags: `--json`, `--verbose`, `--quiet`, `--no-color`, `--config-dir`
- `--version` flag on root only
- Environment variable bindings: `DH_HOME`, `NO_COLOR`, `DH_JSON`
- Mutual exclusivity: `--verbose` + `--quiet` returns error
- `--json` implies `--quiet`
- When no subcommand and not TTY → print help
- When no subcommand and TTY → stub message ("TUI not yet implemented") for now

### Output package (`internal/output/`)

- Exit code constants (0, 1, 2, 3, 4, 130)
- `PrintJSON(w io.Writer, v any)` — marshal and write
- `PrintError(w io.Writer, code string, message string)` — JSON error envelope
- `IsJSON()` / `IsQuiet()` / `IsVerbose()` — read from global flags

### Dependencies

```
go_src:
  github.com/spf13/cobra
  github.com/charmbracelet/lipgloss  (for --no-color auto-detection)

go_unit_tests:
  go_src (replace directive)
  github.com/stretchr/testify

go_behaviour_tests:
  github.com/rogpeppe/go-internal    (testscript)
  github.com/Netflix/go-expect       (TUI tests, placeholder)
  github.com/hinshun/vt10x           (TUI tests, placeholder)
  github.com/stretchr/testify
```

## Tests for this phase

### Unit tests
- Root command with `--version` returns version string
- Root command with `--help` lists usage
- `--verbose` + `--quiet` together returns error
- `--json` flag sets JSON mode
- Unknown subcommand returns exit 1
- Output helpers: `PrintJSON` produces valid JSON, `PrintError` produces error envelope

### Behaviour tests
- `version.txtar`: `dh --version` → stdout matches `^dhg v\d+\.\d+\.\d+`, no stderr
- `help.txtar`: `dh --help` → stdout contains `Usage:` and `dh [command]`
- `global_flags.txtar`: `--verbose` + `--quiet` → exit 1
- `error_codes.txtar`: `dh nonexistent` → exit 1, stderr contains error

## Verification

```bash
cd go_src && go build -o dhg ./cmd/dhg && ./dh --version
cd go_unit_tests && go test ./...
cd go_behaviour_tests && go test ./...
```
