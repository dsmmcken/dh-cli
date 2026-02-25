# dh-cli

A command-line tool for running Python scripts with embedded [Deephaven](https://deephaven.io/) core servers.

> [!WARNING]
> This is an unofficial experimental project. APIs may change without notice. Not recommended for production use. This project was developed with AI assistance (Claude).

## What is Deephaven?

[Deephaven](https://deephaven.io/) is a real-time data engine that combines the power of a database with the flexibility of Python. It's designed for streaming data, time-series analysis, and building real-time dashboards.

This CLI (`dh`) provides a simple way to run Deephaven scripts from your terminal without needing to set up a full server environment.

## Features

- **Version management** - Install and switch between multiple Deephaven versions
- **Interactive REPL** - Textual TUI with syntax highlighting, tab completion, variable sidebar, and real DataTable rendering
- **Batch execution** - Run scripts and exit with clean stdout/stderr separation (ideal for automation and AI agents)
- **Inline code** - Execute one-liners with `dh -c $'print("hello")'`
- **Serve mode** - Run scripts with a persistent server for dashboards and long-running services (auto-opens browser)
- **Remote connections** - Connect to existing Deephaven servers with `--host`, including auth and TLS support
- **Server discovery** - List and stop running Deephaven servers
- **Dev tools** - Lint, format, and type-check from the CLI

## Requirements

- Python 3.13+
- Java 11+ (must be on PATH, or use `dh java-install`)
- Linux or macOS (Windows not fully supported)

## Installation

```bash
# Install globally with uv (recommended)
uv tool install -e . --python 3.13

# Or install in a virtual environment
uv pip install -e .

# Or with pip
pip install -e .
```

## Quick Start

```bash
dh install                # Install the latest Deephaven version
dh repl                   # Start interactive REPL
dh -c $'print("hello")'  # Run inline code
dh exec script.py         # Run a script file
dh serve dashboard.py     # Serve a dashboard with web UI
```

---

## Commands

### `dh install` - Install a Deephaven version

Downloads deephaven-server, pydeephaven, and default plugins into an isolated venv managed by uv at `~/.dh/versions/`.

```bash
dh install                # Install the latest version
dh install 41.1           # Install a specific version
dh install latest         # Same as 'dh install'
```

| Argument | Description |
|----------|-------------|
| `VERSION` | Version to install (default: latest) |

### `dh uninstall` - Remove an installed version

```bash
dh uninstall 41.1
```

| Argument | Description |
|----------|-------------|
| `VERSION` | Version to remove (required) |

### `dh use` - Set the default version

Sets the global default version used by `dh repl`, `dh exec`, and `dh serve`. Can also write a per-directory `.dhrc` file.

```bash
dh use 41.1               # Set global default in ~/.dh/config.toml
dh use 41.1 --local       # Write .dhrc in current directory
```

| Argument | Description |
|----------|-------------|
| `VERSION` | Version to set as default (required) |
| `--local` | Write `.dhrc` in current directory instead of global config |

### `dh versions` - List installed versions

```bash
dh versions               # Show locally installed versions
dh versions --remote      # Also show versions available from PyPI
```

| Option | Description |
|--------|-------------|
| `--remote` | Also query PyPI for available versions |

### `dh java` - Show Java status

Detects and displays information about the current Java installation.

```bash
dh java
```

### `dh java-install` - Install Java

Downloads Eclipse Temurin JDK 21 into `~/.dh/java/`.

```bash
dh java-install
```

### `dh doctor` - Check environment health

Runs diagnostic checks on the Deephaven CLI environment: Java installation, installed versions, and uv availability.

```bash
dh doctor
```

### `dh config` - Show or edit configuration

Reads and writes `~/.dh/config.toml`.

```bash
dh config                              # Show all config
dh config --set default_version 41.1   # Set a config value
```

| Option | Description |
|--------|-------------|
| `--set KEY VALUE` | Set a configuration key to a value |

### `dh repl` - Interactive REPL

Starts an interactive Python session with a Deephaven server. When running in a TTY, launches a Textual TUI with:

- Python syntax highlighting in the input bar
- Tab completion for variable names
- Variable sidebar showing all session variables with types
- Real DataTable rendering for Deephaven tables (via `textual-fastdatatable`)
- Command history (persisted to `~/.dh/history`)
- Log panel with timestamped server events
- Status footer with connection info and table counts

When piped or in a non-TTY context, falls back to a simple `input()` console.

```bash
dh repl                                # Start with embedded server
dh repl -v                             # Verbose mode (show startup messages)
dh repl --port 8080                    # Custom port
dh repl --jvm-args -Xmx8g             # Custom JVM memory
dh repl --vi                           # Vi key bindings (default: Emacs)
dh repl --version 41.1                 # Use a specific Deephaven version
dh repl --host myserver.com            # Connect to remote server
dh repl --host myserver.com --port 8080
```

| Option | Description | Default |
|--------|-------------|---------|
| `--port PORT` | Server port | `10000` |
| `--jvm-args ARGS...` | JVM arguments for embedded server | `-Xmx4g` |
| `-v, --verbose` | Show startup/connection messages | off |
| `--vi` | Vi key bindings | Emacs |
| `--version VERSION` | Deephaven version to use | auto-resolved |
| `--host HOST` | Connect to remote server (skips embedded) | embedded |
| `--auth-type TYPE` | Auth type: `Anonymous`, `Basic`, or custom | `Anonymous` |
| `--auth-token TOKEN` | Auth token (or `DH_AUTH_TOKEN` env var) | — |
| `--tls` | Enable TLS/SSL | off |
| `--tls-ca-cert PATH` | CA certificate PEM file | — |
| `--tls-client-cert PATH` | Client certificate for mutual TLS | — |
| `--tls-client-key PATH` | Client private key for mutual TLS | — |

### `dh exec` - Execute a script

Runs a Python script in batch mode and exits. Best for automation, CI/CD pipelines, and AI agents. Tables created during execution are displayed by default.

```bash
dh exec script.py                      # Run script and exit
dh exec script.py -v                   # Verbose mode
dh exec script.py --timeout 30         # Timeout after 30 seconds
dh exec script.py --no-show-tables     # Suppress table preview output
dh exec script.py --no-table-meta      # Suppress column types / row count
echo "print(2+2)" | dh exec -          # Read script from stdin
dh exec --host remote.example.com script.py  # Execute on remote server
```

| Option | Description | Default |
|--------|-------------|---------|
| `script` | Python script to execute (use `-` for stdin) | — |
| `-c CODE` | Execute code string instead of a file | — |
| `--port PORT` | Server port | `10000` |
| `--jvm-args ARGS...` | JVM arguments for embedded server | `-Xmx4g` |
| `-v, --verbose` | Show startup/connection messages | off |
| `--timeout SECONDS` | Max execution time (exit code 3 on timeout) | none |
| `--no-show-tables` | Suppress table preview output | tables shown |
| `--no-table-meta` | Suppress column types and row count | metadata shown |
| `--version VERSION` | Deephaven version to use | auto-resolved |
| `--host HOST` | Connect to remote server (skips embedded) | embedded |
| `--auth-type TYPE` | Auth type: `Anonymous`, `Basic`, or custom | `Anonymous` |
| `--auth-token TOKEN` | Auth token (or `DH_AUTH_TOKEN` env var) | — |
| `--tls` | Enable TLS/SSL | off |
| `--tls-ca-cert PATH` | CA certificate PEM file | — |
| `--tls-client-cert PATH` | Client certificate for mutual TLS | — |
| `--tls-client-key PATH` | Client private key for mutual TLS | — |

### `dh -c` - Inline code execution

Shorthand for `dh exec -c`. Execute code directly without a script file:

```bash
dh -c $'print("hello")'
dh -c $'from deephaven import empty_table\nt = empty_table(5).update("X = i")'
dh -c $'t.where("Sym = \`DOG\`")'
```

> **Note:** Always use ANSI-C quoting (`$'...'`) with `-c` to avoid shell interpretation issues with backticks and special characters.

### `dh serve` - Serve mode

Runs a script and keeps the Deephaven server alive for dashboards, visualizations, and long-running services. Opens the web UI in your browser automatically.

```bash
dh serve dashboard.py                  # Run script, open browser, keep alive
dh serve dashboard.py --port 8080      # Custom port
dh serve dashboard.py --iframe my_widget  # Open browser to widget iframe
dh serve dashboard.py --no-browser     # Don't open browser
dh serve dashboard.py -v               # Verbose startup
```

| Option | Description | Default |
|--------|-------------|---------|
| `script` | Python script to execute (required) | — |
| `--port PORT` | Server port | `10000` |
| `--jvm-args ARGS...` | JVM arguments | `-Xmx4g` |
| `-v, --verbose` | Show startup messages | off |
| `--no-browser` | Don't open browser automatically | browser opens |
| `--iframe WIDGET` | Open browser to iframe URL for the given widget name | — |
| `--version VERSION` | Deephaven version to use | auto-resolved |

### `dh list` - List running servers

Discovers all running Deephaven servers on this machine, including those started via `dh serve`, `dh repl`, Docker, or standalone Java.

```bash
dh list
```

### `dh kill` - Stop a server

Stops a Deephaven server by port number. Works with dh-cli processes (sends SIGTERM) and Docker containers (`docker stop`).

```bash
dh kill 10000
dh kill 8080
```

| Argument | Description |
|----------|-------------|
| `port` | Port of the server to stop (required) |

### `dh lint` - Run linter

Runs `ruff check` on the current directory or a specific path.

```bash
dh lint                    # Lint current directory
dh lint src/               # Lint specific directory
dh lint --fix              # Auto-fix lint issues
dh lint src/ -- --select E501  # Pass extra args to ruff
```

| Option | Description |
|--------|-------------|
| `path` | File or directory to lint (default: current directory) |
| `--fix` | Automatically fix lint issues |
| `extra` | Extra args passed to ruff (after `--`) |

### `dh format` - Run formatter

Runs `ruff format` on the current directory or a specific path.

```bash
dh format                  # Format current directory
dh format src/             # Format specific directory
dh format --check          # Check without making changes
```

| Option | Description |
|--------|-------------|
| `path` | File or directory to format (default: current directory) |
| `--check` | Check formatting without making changes |
| `extra` | Extra args passed to ruff (after `--`) |

### `dh typecheck` - Run type checker

Runs `ty check` on the current directory or a specific path.

```bash
dh typecheck               # Type-check current directory
dh typecheck src/          # Type-check specific directory
```

| Option | Description |
|--------|-------------|
| `path` | File or directory to check (default: current directory) |
| `extra` | Extra args passed to ty (after `--`) |

---

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Script error |
| 2 | Connection error |
| 3 | Timeout |
| 130 | Interrupted (Ctrl+C) |

## Shell Quoting

Deephaven query strings use backticks (`` ` ``) for string literals:

```python
# In a .py file - works correctly
stocks.where('Sym = `DOG`')
```

When passing code via the shell, backticks are interpreted as command substitution. Solutions:

**1. Use a script file (most reliable):**
```bash
dh exec my_script.py
```

**2. Use `-c` with ANSI-C quoting:**
```bash
dh -c $'stocks.where("Sym = \`DOG\`")'
```

**3. Pipe with ANSI-C quoting:**
```bash
echo $'stocks.where("Sym = \`DOG\`")' | dh exec -
```

## Examples

### Quick One-Liner

```bash
dh -c $'print(2 + 2)'

dh -c $'from deephaven import empty_table\nt = empty_table(5).update("X = i")'
```

### Run a Script

**query.py:**
```python
from deephaven import empty_table

t = empty_table(1000).update([
    "X = i",
    "Y = X * 2",
    "Z = Math.sin(X / 100.0)"
])

result = t.where("X > 500").sum_by()
print(result.to_string())
```

```bash
dh exec query.py
```

### Interactive Session

```bash
dh repl

>>> from deephaven import empty_table
>>> t = empty_table(10).update("X = i", "Y = X * X")
>>> t.to_string()
```

### Dashboard

**dashboard.py:**
```python
from deephaven import time_table

ticking = time_table("PT1S").update([
    "Value = Math.random()",
    "RunningSum = cumsum(Value)"
])
```

```bash
dh serve dashboard.py
```

### Automation with Timeout

```bash
dh exec validate_data.py --timeout 60
# Exit code: 0 = success, 1 = error, 3 = timeout
```

### Remote Server

```bash
dh repl --host myserver.com
dh exec script.py --host myserver.com --port 8080
dh repl --host myserver.com --auth-type Basic --auth-token user:pass
dh repl --host myserver.com --tls --tls-ca-cert /path/to/ca.pem
```

## How It Works

1. **Version management** - `dh install` creates isolated venvs in `~/.dh/versions/` using uv
2. **Server startup** - Launches an embedded Deephaven server with JVM
3. **Client connection** - Connects via gRPC using `pydeephaven`
4. **Code execution** - Wraps user code to capture stdout/stderr and expression results
5. **Result transfer** - Uses pickle + base64 encoding through Deephaven tables for safe string transfer
6. **Table display** - REPL uses `textual-fastdatatable` with Arrow backend for real virtual-scrolling tables; exec mode uses pandas text rendering
7. **Cleanup** - Shuts down server on exit

## Development

```bash
git clone <repo>
cd dh-cli
uv venv --python 3.13
uv pip install -e ".[dev]"

uv run pytest                          # Run tests
uv run pytest tests/test_phase4_executor.py -v  # Specific test file

dh lint                                # ruff check
dh lint --fix                          # auto-fix
dh format                              # ruff format
dh typecheck                           # ty check
```
