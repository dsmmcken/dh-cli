# Project Instructions for Claude Code

## Project Structure

- **`src/`** — Main Go CLI source (module: `github.com/dsmmcken/dh-cli/src`)
- **`unit_tests/`** — Unit tests (uses `replace ../src` directive)
- **`behaviour_tests/`** — Black-box CLI/TUI tests (testscript .txtar files)
- **`plans/`** — Design docs / implementation plans

## Build & Install

### Standard (local machine with `uvx` available)

```bash
make install-local
```

This builds a wheel for the current platform and installs it via `uv tool install` to `~/.local/bin/dh`.

### Sandbox / CI fallback (when Python `os.getcwd()` fails)

If the project lives on a virtiofs or FUSE mount, Python's `os.getcwd()` will fail, breaking `uv`, `go-to-wheel`, and `make install-local`. Use direct Go build instead:

```bash
CGO_ENABLED=0 make build && cp dh ~/.local/bin/dh
```

**How to tell:** If you see `Current directory does not exist` from uv/go-to-wheel, or `FileNotFoundError` from Python's `os.getcwd()`, use the fallback build.

Do **not** use `sudo cp`, `make install`, or `go install` directly.

### Running `dh exec` in the sandbox

The sandbox has no Java and no `dh install`, so the only way to run Deephaven code is via the VM snapshot path. Build the binary first, then use `DH_HOME` to point at the persisted workspace artifacts.

```bash
# Build (needed each sandbox session — binary is ephemeral)
CGO_ENABLED=0 make build && cp dh ~/.local/bin/dh

# Run code (auto-detects latest snapshot, no --version needed)
DH_HOME=/workspace/.dh dh exec --vm -c 'print("hello world")'
DH_HOME=/workspace/.dh dh exec --vm script.py
echo 'print("hi")' | DH_HOME=/workspace/.dh dh exec --vm -
```

If no snapshot exists yet, build one first (requires Docker):

```bash
DH_HOME=/workspace/.dh dh vm prepare -v    # ~2-5 min, artifacts persist in /workspace/.dh/
```

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
make test    # unit + behaviour tests
make vet     # vet all source
```

## Plan File Location

When creating plans in plan mode, always save them to the `plans/` directory in this project.

**IMPORTANT:** Do NOT use the default three-random-words naming convention. Instead, always name plan files with a clear, descriptive name that reflects the plan's purpose (e.g., `plans/add_user_authentication.md`, `plans/refactor_database_layer.md`, `plans/fix_video_encoding_bug.md`).

## Committing Plans

When committing implementation work, include any relevant plan files from `plans/` in the commit. Plans serve as documentation of the design decisions and should be versioned alongside the code they describe.
