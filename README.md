# dh-cli

A command-line tool for running Python scripts with embedded [Deephaven](https://deephaven.io/) core servers.

> [!WARNING]
> This is an unofficial experimental project. APIs may change without notice. Not recommended for production use. This project was developed with AI assistance (Claude).

## What is Deephaven?

[Deephaven](https://deephaven.io/) is a real-time data engine that combines the power of a database with the flexibility of Python. It's designed for streaming data, time-series analysis, and building real-time dashboards.

This CLI (`dh`) provides a simple way to run Deephaven scripts from your terminal without needing to set up a full server environment.

## Features

- **Interactive REPL** - Python shell with Deephaven context, tab completion, and automatic table display
- **Batch execution** - Run scripts and exit with clean stdout/stderr separation (ideal for automation and AI agents)
- **Inline code** - Execute one-liners with `dh -c $'print("hello")'`
- **Serve mode** - Run scripts with a persistent server for dashboards and long-running services (auto-opens browser)
- **Remote connections** - Connect to existing Deephaven servers with `--host`, including auth and TLS support
- **Automatic table preview** - Tables created during execution are displayed by default
- **Quiet by default** - JVM and server startup messages suppressed for clean output

## Requirements

- Python 3.13+
- Java 11+ (must be on PATH)
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

## Usage

### Interactive REPL

```bash
dh repl                              # Start interactive session (quiet by default)
dh repl -v                           # Verbose mode with startup messages
dh repl --port 8080                  # Custom port
dh repl --jvm-args -Xmx8g           # Custom JVM memory
dh repl --vi                         # Vi key bindings (default: Emacs)
dh repl --host myserver.com          # Connect to remote server
```

The REPL provides:
- Full Python environment with Deephaven imports available
- Tab completion
- Automatic display of expression results
- Automatic preview of newly created tables

### Execute Scripts (Batch Mode)

Best for automation, CI/CD pipelines, and AI agents:

```bash
dh exec script.py                    # Run script and exit (tables shown by default)
dh exec script.py -v                 # Verbose mode (show startup messages)
dh exec script.py --timeout 30       # Timeout after 30 seconds
dh exec script.py --no-show-tables   # Suppress table preview output
dh exec script.py --no-table-meta    # Suppress column types/row count in output
echo "print(2+2)" | dh exec -        # Read script from stdin
dh exec --host remote.example.com script.py  # Execute on remote server
```

### Inline Code Execution

Execute code directly without a script file, similar to `python -c`:

```bash
dh -c $'print("hello")'                          # Shorthand for dh exec -c
dh exec -c $'from deephaven import empty_table\nt = empty_table(5)'
dh -c $'t.where("Sym = \`DOG\`")'                # Backticks work in $'...'
```

> **Note:** Always use ANSI-C quoting (`$'...'`) with `-c` to avoid shell interpretation issues with backticks and special characters.

### Serve Mode

For dashboards, visualizations, and long-running services:

```bash
dh serve dashboard.py            # Run script, open browser, keep server alive
dh serve dashboard.py --port 8080
dh serve dashboard.py --no-browser  # Don't open browser automatically
```

The server stays running after script execution, allowing you to:
- View the web UI (opened automatically in your browser)
- Keep data pipelines running
- Serve real-time dashboards

### Remote Server Connections

Connect to an existing Deephaven server instead of starting an embedded one:

```bash
dh repl --host myserver.com                    # Connect to remote server
dh exec script.py --host myserver.com          # Execute on remote server
dh repl --host myserver.com --port 8080        # Custom port
dh repl --host myserver.com --auth-type Basic --auth-token user:pass
```

Authentication and TLS options are available for remote connections:

| Option | Description |
|--------|-------------|
| `--host HOST` | Connect to remote server (skips embedded server) |
| `--auth-type TYPE` | Authentication type: `Anonymous`, `Basic`, or custom (default: `Anonymous`) |
| `--auth-token TOKEN` | Auth token (for Basic: `user:password`). Can also use `DH_AUTH_TOKEN` env var |
| `--tls` | Enable TLS/SSL encryption |
| `--tls-ca-cert PATH` | Path to CA certificate PEM file |
| `--tls-client-cert PATH` | Client certificate for mutual TLS |
| `--tls-client-key PATH` | Client private key for mutual TLS |

## Common Options

| Option | Description | Default |
|--------|-------------|---------|
| `--port` | Server port | 10000 |
| `--jvm-args` | JVM arguments (embedded only) | `-Xmx4g` |
| `-v, --verbose` | Show startup/connection messages | off |
| `-c CODE` | Execute inline code (exec only) | — |
| `--timeout` | Max execution time in seconds (exec only) | none |
| `--no-show-tables` | Suppress table preview (exec only) | tables shown |
| `--no-table-meta` | Suppress column types/row count (exec only) | metadata shown |
| `--vi` | Vi key bindings (repl only) | Emacs |
| `--host` | Connect to remote server | embedded |

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Script error |
| 2 | Connection error |
| 3 | Timeout |
| 130 | Interrupted (Ctrl+C) |

## Special Characters and Shell Quoting

Deephaven query strings use backticks (`` ` ``) for string literals:

```python
# In a .py file - works correctly
stocks.where('Sym = `DOG`')
```

When passing code via the shell, backticks are interpreted as command substitution. The recommended solutions:

**1. Use a script file (most reliable):**
```bash
dh exec my_script.py
```

**2. Use `-c` with ANSI-C quoting (recommended for one-liners):**
```bash
dh -c $'from deephaven.plot import express as dx\nstocks = dx.data.stocks()\ndog = stocks.where("Sym = \`DOG\`")'
```

**3. Pipe with ANSI-C quoting:**
```bash
echo $'from deephaven.plot import express as dx\nstocks = dx.data.stocks()\ndog = stocks.where("Sym = \`DOG\`")' | dh exec -
```

## Examples

### Quick One-Liner

```bash
# Run a simple expression
dh -c $'print(2 + 2)'

# Create and display a table (tables shown by default)
dh -c $'from deephaven import empty_table\nt = empty_table(5).update("X = i")'
```

### Run a Script File

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

### Long-Running Dashboard

**dashboard.py:**
```python
from deephaven import time_table

# Ticking table that updates every second
ticking = time_table("PT1S").update([
    "Value = Math.random()",
    "RunningSum = cumsum(Value)"
])
print("Dashboard running. Open http://localhost:10000 in your browser.")
```

```bash
# Keep server alive for web UI access (opens browser automatically)
dh serve dashboard.py
```

### Automation with Timeout

```bash
# Run validation script with 60s timeout
dh exec validate_data.py --timeout 60

# Exit code 0 = success, 1 = script error, 3 = timeout
```

## Development

```bash
# Clone and install with dev dependencies
git clone <repo>
cd dh-cli
uv venv --python 3.13
uv pip install -e ".[dev]"

# Run tests
uv run pytest

# Run a specific test
uv run pytest tests/test_phase4_executor.py -v
```

## How It Works

The CLI embeds a Deephaven server in the same process:

1. **Server startup** - Launches an embedded Deephaven server with JVM
2. **Client connection** - Connects via gRPC using `pydeephaven`
3. **Code execution** - Wraps user code to capture stdout/stderr and expression results
4. **Result transfer** - Uses pickle + base64 encoding through Deephaven tables for safe string transfer
5. **Cleanup** - Shuts down server on exit
