# dhg

A command-line tool for managing [Deephaven](https://deephaven.io/) installations, servers, and configuration.

> [!WARNING]
> This is an unofficial experimental project. APIs may change without notice. Not recommended for production use. This project was developed with AI assistance (Claude).

## What is Deephaven?

[Deephaven](https://deephaven.io/) is a real-time data engine that combines the power of a database with the flexibility of Python. It's designed for streaming data, time-series analysis, and building real-time dashboards.

This CLI (`dhg`) manages the Deephaven environment on your machine: installing versions, detecting Java, discovering running servers, and providing an interactive TUI for first-time setup.

## Features

- **Version management** — Install, uninstall, and switch between multiple Deephaven versions
- **Interactive TUI** — Setup wizard for first-time users, main menu for daily operations
- **Java detection** — Auto-detect Java from JAVA_HOME, PATH, or managed installs, with one-command install
- **Server discovery** — Find running Deephaven processes and Docker containers across the system
- **Environment doctor** — Diagnose and fix common setup problems
- **Per-directory versions** — Pin a project to a specific Deephaven version with `.dhgrc`
- **JSON output** — Every command supports `--json` for scripting and automation
- **Cross-platform** — Linux and macOS, with cross-compilation for 6 OS/arch targets

## Requirements

- Go 1.24+ (build only)
- [uv](https://docs.astral.sh/uv/) (runtime — used for creating Python venvs)
- Java 17+ (for running Deephaven servers)
- Linux or macOS (Windows server discovery not supported)

## Installation

### Install with uv (recommended)

```bash
cd go_src
make install-local            # Builds wheel + installs via uv tool install
dhg --version                 # Verify
```

This uses [go-to-wheel](https://pypi.org/project/go-to-wheel/) (via `uvx`) to compile the Go binary into a platform-specific Python wheel, then installs it with `uv tool install`. The `dhg` command is placed on your PATH.

To uninstall:

```bash
cd go_src
make uninstall                # uv tool uninstall dhg-cli
```

### Build binary directly

```bash
cd go_src
make build                    # Build for current OS/arch
./dhg --version               # Verify
sudo cp dhg /usr/local/bin/   # Manual install
```

### Build targets

| Target | Description |
|--------|-------------|
| `make install-local` | Package wheel + `uv tool install` (recommended) |
| `make uninstall` | `uv tool uninstall dhg-cli` |
| `make package` | Build Python wheel for current platform via `uvx go-to-wheel` |
| `make package-all` | Build wheels for all 6 OS/arch targets |
| `make package VERSION=X.Y.Z` | Package with custom version string |
| `make build` | Build `dhg` binary for current platform (`CGO_ENABLED=0`) |
| `make build-all` | Cross-compile binaries for linux/darwin/windows × amd64/arm64 |
| `make test` | Run unit tests and behaviour tests |
| `make clean` | Remove binary and `dist/` directory |

## Quick Start

```bash
dhg                           # Launch interactive TUI
dhg install                   # Install the latest Deephaven version
dhg versions                  # List installed versions
dhg doctor                    # Check environment health
dhg list                      # Show running Deephaven servers
dhg exec -c "t = empty_table(5)" # Execute Python code
dhg serve dashboard.py        # Run script and keep server alive
```

---

## Commands

### `dhg` — Interactive TUI

When run in a terminal with no subcommand, launches an interactive interface.

- **First run** (no versions installed): Setup wizard walks through Java detection, version selection, and installation
- **Subsequent runs**: Main menu with access to version management, server list, Java status, doctor, and configuration

Navigation: `j`/`k` or arrow keys to move, `Enter` to select, `Esc` to go back, `q` to quit, `?` for help.

### `dhg install` — Install a Deephaven version

Downloads `deephaven-server` and default plugins into an isolated venv managed by uv at `~/.dhg/versions/`.

```bash
dhg install                   # Install the latest version from PyPI
dhg install 0.35.1            # Install a specific version
dhg install --no-plugins      # Skip default plugin installation
dhg install --python 3.12     # Use a specific Python version for the venv
```

| Option | Description | Default |
|--------|-------------|---------|
| `VERSION` | Version to install (omit for latest) | latest from PyPI |
| `--no-plugins` | Skip installing default plugins | plugins installed |
| `--python VERSION` | Python version for the venv | from config or `3.13` |

### `dhg uninstall` — Remove an installed version

```bash
dhg uninstall 0.35.1          # Remove with confirmation prompt
dhg uninstall 0.35.1 --force  # Skip confirmation
```

| Option | Description |
|--------|-------------|
| `VERSION` | Version to remove (required) |
| `--force` | Skip confirmation prompt |

If the uninstalled version was the default, the default is updated to the next latest installed version.

### `dhg use` — Set the default version

```bash
dhg use 0.35.1                # Set global default in ~/.dhg/config.toml
dhg use 0.35.1 --local        # Write .dhgrc in current directory
```

| Option | Description |
|--------|-------------|
| `VERSION` | Version to set as default (required, must be installed) |
| `--local` | Write `.dhgrc` in current directory instead of global config |

### `dhg versions` — List installed versions

```bash
dhg versions                  # Show locally installed versions
dhg versions --remote         # Also show available versions from PyPI
dhg versions --remote --all   # Show all remote versions (no limit)
dhg versions --limit 20       # Show top 20 remote versions
```

| Option | Description | Default |
|--------|-------------|---------|
| `--remote` | Also query PyPI for available versions | off |
| `--limit N` | Number of remote versions to show | 20 |
| `--all` | Show all remote versions | off |

The default version is marked with `*` in human output or `"default": true` in JSON.

### `dhg java` — Show Java status

Detects Java from multiple sources and reports the version and location.

```bash
dhg java                      # Show Java detection result
dhg java --json               # JSON output
```

**Detection order**: `$JAVA_HOME/bin/java` → `java` on `$PATH` → `~/.dhg/java/*/bin/java` (managed installs)

### `dhg java install` — Install Java

Downloads Eclipse Temurin JDK into `~/.dhg/java/`.

```bash
dhg java install              # Install JDK 21 (default)
dhg java install --jdk-version 17   # Install a specific JDK version
dhg java install --force      # Force reinstall
```

| Option | Description | Default |
|--------|-------------|---------|
| `--jdk-version N` | JDK major version to install | `21` |
| `--force` | Force reinstall even if already present | off |

### `dhg exec` — Execute Python code on a Deephaven server

Runs Python code in batch mode on a Deephaven server. In embedded mode (default), starts a local server automatically. In remote mode (`--host`), connects to an existing server.

Code can be provided via `-c` flag, a script file, or stdin (use `-` for stdin).

```bash
dhg exec -c "print('hello')"                       # Inline code
dhg exec script.py                                  # Script file
echo "print('hi')" | dhg exec -                     # From stdin
dhg exec -c "from deephaven import empty_table; t = empty_table(5)"  # Table creation
dhg exec -c "print('remote')" --host remote.example.com             # Remote server
dhg exec script.py --json                           # JSON output
```

| Option | Description | Default |
|--------|-------------|---------|
| `-c CODE` | Python code to execute | |
| `SCRIPT` | Path to script file (positional arg) | |
| `--port N` | Server port | `10000` |
| `--jvm-args ARGS` | JVM arguments (quoted string) | `-Xmx4g` |
| `--timeout N` | Execution timeout in seconds (0 = none) | `0` |
| `--no-show-tables` | Do not show table previews | off |
| `--no-table-meta` | Do not show column types and row counts | off |
| `--version VERSION` | Deephaven version to use | resolved |
| `--host HOST` | Remote server host (enables remote mode) | |
| `--auth-type TYPE` | Authentication type for remote connection | |
| `--auth-token TOKEN` | Authentication token for remote connection | |
| `--tls` | Use TLS for remote connection | off |
| `--tls-ca-cert PATH` | Path to CA certificate for TLS | |
| `--tls-client-cert PATH` | Path to client certificate for TLS | |
| `--tls-client-key PATH` | Path to client private key for TLS | |

By default, table previews are shown when tables are created or assigned. The embedded Python runner uses AST-based variable capture, stdout/stderr multiplexing, and exception handling with full tracebacks. Exit code is propagated from user code.

### `dhg serve` — Run script and keep server alive

Runs a script and keeps the Deephaven server running for dashboards, visualizations, and long-running data pipelines.

```bash
dhg serve dashboard.py                    # Run and open browser
dhg serve dashboard.py --port 8080        # Custom port
dhg serve dashboard.py --iframe my_widget # Open browser to iframe URL
dhg serve dashboard.py --no-browser       # Don't open browser
```

| Option | Description | Default |
|--------|-------------|---------|
| `SCRIPT` | Path to script file (required, positional) | |
| `--port N` | Server port | `10000` |
| `--jvm-args ARGS` | JVM arguments (quoted string) | `-Xmx4g` |
| `--no-browser` | Don't open browser automatically | off |
| `--iframe NAME` | Open browser to iframe URL for the given widget name | |
| `--version VERSION` | Deephaven version to use | resolved |

Opens the browser automatically when the server is ready. Server runs until Ctrl+C (first signal graceful shutdown, second force kill).

### `dhg list` — List running Deephaven servers

Discovers all running Deephaven servers on this machine, including processes and Docker containers.

```bash
dhg list                      # Human-readable table
dhg list --json               # JSON array of servers
```

**Discovery methods**:
- Linux: `/proc/net/tcp` + `/proc/*/fd/` inode matching
- macOS: `lsof -iTCP -sTCP:LISTEN`
- Docker: `docker ps` with image name filtering

### `dhg kill` — Stop a running server

```bash
dhg kill 10000                # Stop server on port 10000
```

| Argument | Description |
|----------|-------------|
| `PORT` | Port of the server to stop (required) |

Sends SIGTERM to native processes. Runs `docker stop` for Docker containers.

### `dhg config` — Manage configuration

Reads and writes `~/.dhg/config.toml`.

```bash
dhg config                    # Show all config values
dhg config get default_version        # Get a single value
dhg config set default_version 0.35.1 # Set a value
dhg config path               # Show config file path
```

| Subcommand | Description |
|------------|-------------|
| *(none)* | Show all configuration |
| `get KEY` | Get a single config value |
| `set KEY VALUE` | Set a config value |
| `path` | Print the config file path |

**Config keys**: `default_version`, `install.plugins`, `install.python_version`

### `dhg doctor` — Check environment health

Runs 5 diagnostic checks and reports their status.

```bash
dhg doctor                    # Human-readable report
dhg doctor --json             # JSON report
dhg doctor --fix              # Show suggested fixes
dhg doctor --no-color         # No ANSI colors
```

| Check | What it verifies |
|-------|-----------------|
| uv | `uv` is installed and on PATH |
| Java | Java 17+ is detected |
| Versions | At least one Deephaven version is installed |
| Default | `default_version` is set and the directory exists |
| Disk | Free disk space at `~/.dhg/` is above 5 GB |

### `dhg setup` — Run setup wizard

```bash
dhg setup                     # Launch interactive setup wizard
dhg setup --non-interactive   # Auto-detect Java, install latest, output JSON
```

---

## Global Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--json` | `-j` | Output as JSON (implies `--quiet`) |
| `--verbose` | `-v` | Extra detail to stderr |
| `--quiet` | `-q` | Suppress non-essential output |
| `--no-color` | | Disable ANSI colors |
| `--config-dir DIR` | | Override config directory (default: `~/.dhg`) |

`--verbose` and `--quiet` are mutually exclusive.

## Environment Variables

| Variable | Description |
|----------|-------------|
| `DHG_HOME` | Override config directory (same as `--config-dir`) |
| `DHG_VERSION` | Override default version for resolution |
| `DHG_JSON` | Set to `1` to enable JSON output |
| `NO_COLOR` | Disable ANSI colors (any value) |
| `JAVA_HOME` | Java detection — checked first |

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Network error |
| 3 | Timeout |
| 4 | Not found |
| 130 | Interrupted (Ctrl+C) |

## Version Resolution

When a command needs to determine which Deephaven version to use, it follows this precedence:

1. `--version` flag (if the command supports it)
2. `DHG_VERSION` environment variable
3. `.dhgrc` file — walks up from the current directory to the filesystem root
4. `default_version` in `~/.dhg/config.toml`
5. Latest installed version (by semver sort)
6. Error if nothing found

## Configuration

### Global config: `~/.dhg/config.toml`

```toml
default_version = "0.35.1"

[install]
python_version = "3.13"
plugins = ["deephaven-plugin-ui", "deephaven-plugin-plotly-express"]
```

### Local version pin: `.dhgrc`

A plain-text file containing a single version string. Create it with `dhg use --local`:

```
0.35.1
```

### Directory layout

```
~/.dhg/
├── config.toml                 # Global configuration
├── versions/
│   ├── 0.35.1/
│   │   ├── .venv/             # Isolated Python virtual environment
│   │   └── meta.toml          # Installation metadata
│   └── 0.36.0/
│       ├── .venv/
│       └── meta.toml
└── java/
    └── jdk-21.0.2+13/         # Managed JDK installation
        └── bin/java
```

## TUI Screens

### Setup Wizard (first run)

```
Welcome → Java Check → Version Picker → Install Progress → Done
```

Guides new users through initial environment setup with automatic Java detection, version selection from PyPI, and installation with progress tracking.

### Main Menu (versions installed)

```
Main Menu
  ├── Manage versions    (install, uninstall, set default)
  ├── Running servers    (list, kill)
  ├── Java status        (detect, install)
  ├── Environment doctor (health checks)
  └── Configuration      (view settings)
```

All screens support `Esc` to go back and `q` to quit.

## Development

### Project structure

```
go_src/                         # Main source module
├── cmd/dhg/main.go            # Entry point
├── internal/
│   ├── cmd/                   # Cobra command definitions
│   ├── config/                # TOML config, .dhgrc, version resolution
│   ├── discovery/             # Server discovery (linux, darwin, docker)
│   ├── exec/                  # Code execution engine (embedded Python runner)
│   ├── java/                  # Java detection, version parsing, install
│   ├── output/                # JSON/text output, exit codes
│   ├── tui/                   # Bubbletea TUI app
│   │   ├── components/        # Reusable TUI components
│   │   └── screens/           # Individual TUI screens
│   └── versions/              # Install, uninstall, list, PyPI client
├── go.mod
└── Makefile

go_unit_tests/                  # White-box unit tests
├── cmd_test.go
├── config_test.go
├── discovery_test.go
├── doctor_test.go
├── exec_test.go
├── java_test.go
├── output_test.go
├── tui_test.go
├── versions_test.go
└── go.mod                     # Separate module with replace directive

go_behaviour_tests/             # Black-box CLI tests
├── cli_test.go                # testscript runner
├── helpers_test.go            # Test helpers
├── tui_test.go                # TUI tests (go-expect + vt10x)
├── testdata/scripts/          # .txtar test scripts
│   ├── config.txtar
│   ├── doctor.txtar
│   ├── error_codes.txtar
│   ├── exec.txtar
│   ├── global_flags.txtar
│   ├── help.txtar
│   ├── install.txtar
│   ├── java.txtar
│   ├── kill.txtar
│   ├── list.txtar
│   ├── serve.txtar
│   ├── setup.txtar
│   ├── uninstall.txtar
│   ├── use.txtar
│   ├── version.txtar
│   └── versions.txtar
└── go.mod
```

### Running tests

```bash
cd go_src
make test                     # Run all unit + behaviour tests

# Or individually:
cd go_unit_tests && go test ./... -v
cd go_behaviour_tests && go test ./... -v
```

### Dependencies

| Library | Purpose |
|---------|---------|
| [cobra](https://github.com/spf13/cobra) | CLI command framework |
| [bubbletea](https://github.com/charmbracelet/bubbletea) | TUI framework (Elm architecture) |
| [bubbles](https://github.com/charmbracelet/bubbles) | TUI components (spinners, lists, progress bars) |
| [lipgloss](https://github.com/charmbracelet/lipgloss) | Terminal styling and layout |
| [go-toml/v2](https://github.com/pelletier/go-toml) | TOML config parsing |
| [testify](https://github.com/stretchr/testify) | Test assertions (unit tests) |
| [go-internal/testscript](https://github.com/rogpeppe/go-internal) | CLI integration testing (behaviour tests) |
