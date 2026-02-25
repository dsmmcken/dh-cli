# Project Instructions for Claude Code

## Project Structure

- **`go_src/`** — Main Go CLI source (module: `github.com/dsmmcken/dh-cli/go_src`)
- **`go_unit_tests/`** — Unit tests (uses `replace ../go_src` directive)
- **`go_behaviour_tests/`** — Black-box CLI/TUI tests (testscript .txtar files)
- **`src/deephaven_cli/`** — Legacy Python CLI
- **`plans/`** — Design docs / implementation plans

## Build & Install

### Standard (local machine with `uvx` available)

```bash
cd go_src && make install-local
```

This builds a wheel for the current platform and installs it via `uv tool install` to `~/.local/bin/dhg`.

### Sandbox / CI fallback (when Python `os.getcwd()` fails)

If the project lives on a virtiofs or FUSE mount, Python's `os.getcwd()` will fail, breaking `uv`, `go-to-wheel`, and `make install-local`. Use direct Go build instead:

```bash
cd go_src && CGO_ENABLED=0 go build -ldflags="-X github.com/dsmmcken/dh-cli/go_src/internal/cmd.Version=0.1.0" -o dhg ./cmd/dhg && cp dhg ~/.local/bin/dhg
```

**How to tell:** If you see `Current directory does not exist` from uv/go-to-wheel, or `FileNotFoundError` from Python's `os.getcwd()`, use the fallback build.

Do **not** use `sudo cp`, `make install`, or `go install` directly.

## Go Toolchain Setup

Current Go version: **1.26.0**

If the sandbox doesn't have the right Go version, install via `golang.org/dl`:

```bash
go install golang.org/dl/go1.26.0@latest
~/go/bin/go1.26.0 download
```

Then persist in `/etc/sandbox-persistent.sh`:

```bash
export PATH="$HOME/go/bin:$HOME/sdk/go1.26.0/bin:$PATH"
export GOTOOLCHAIN=go1.26.0
```

Note: `CGO_ENABLED=0` is required — the sandbox has no gcc.

## Running Tests

```bash
# Unit tests
cd go_unit_tests && go test ./...

# Behaviour tests
cd go_behaviour_tests && go test ./...

# Vet all source
cd go_src && go vet ./...
```

## Plan File Location

When creating plans in plan mode, always save them to the `plans/` directory in this project.

**IMPORTANT:** Do NOT use the default three-random-words naming convention. Instead, always name plan files with a clear, descriptive name that reflects the plan's purpose (e.g., `plans/add_user_authentication.md`, `plans/refactor_database_layer.md`, `plans/fix_video_encoding_bug.md`).

## Committing Plans

When committing implementation work, include any relevant plan files from `plans/` in the commit. Plans serve as documentation of the design decisions and should be versioned alongside the code they describe.
