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
- **Application mode** - Run scripts with a persistent server for dashboards and long-running services
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
dh repl                       # Start interactive session (quiet by default)
dh repl -v                    # Verbose mode with startup messages
dh repl --port 8080           # Custom port
dh repl --jvm-args -Xmx8g     # Custom JVM memory
```

The REPL provides:
- Full Python environment with Deephaven imports available
- Tab completion
- Automatic display of expression results
- Automatic preview of newly created tables

### Execute Scripts (Batch Mode)

Best for automation, CI/CD pipelines, and AI agents:

```bash
dh exec script.py                    # Run script and exit (quiet by default)
dh exec script.py -v                 # Verbose mode (show startup messages)
dh exec script.py --timeout 30       # Timeout after 30 seconds
dh exec script.py --show-tables      # Preview any tables created by the script
echo "print(2+2)" | dh exec -        # Read script from stdin
```

### Application Mode

For dashboards, visualizations, and long-running services:

```bash
dh app dashboard.py            # Run script, keep server alive until Ctrl+C
dh app dashboard.py --port 8080
```

The server stays running after script execution, allowing you to:
- Connect to the web UI at `http://localhost:<port>`
- Keep data pipelines running
- Serve real-time dashboards

## Common Options

| Option | Description | Default |
|--------|-------------|---------|
| `--port` | Server port | 10000 |
| `--jvm-args` | JVM arguments | `-Xmx4g` |
| `-v, --verbose` | Show startup messages | off (quiet by default) |
| `--timeout` | Max execution time (exec only) | none |
| `--show-tables` | Preview created tables (exec only) | off |

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Script error |
| 2 | Connection error |
| 3 | Timeout |
| 130 | Interrupted (Ctrl+C) |

## Special Characters in Piped Input

Deephaven query strings use backticks (`` ` ``) for string literals:

```python
# In a .py file - works correctly
stocks.where('Sym = `DOG`')
```

When piping scripts to `dh exec -`, the shell interprets backticks as command substitution **before** the data reaches dh-cli.

### Recommended Solutions

**1. Use a script file (most reliable):**
```bash
dh exec my_script.py
```

**2. Use ANSI-C quoting with `\n` for multiline:**
```bash
echo $'from deephaven.plot import express as dx\nstocks = dx.data.stocks()\ndog = stocks.where(\'Sym = `DOG`\')' | dh exec --show-tables -
```

**3. Escape backticks in double quotes:**
```bash
echo "from deephaven import empty_table; t = empty_table(1).update([\"S = \`hello\`\"])" | dh exec --show-tables -
```

**4. Use printf for complex multiline:**
```bash
printf '%s\n' \
  'from deephaven.plot import express as dx' \
  'stocks = dx.data.stocks()' \
  'dog = stocks.where('\''Sym = `DOG`'\'')' \
  | dh exec --show-tables -
```

## Examples

### Quick One-Liner

```bash
# Run a simple expression
echo "print(2 + 2)" | dh exec -

# Create and display a table
echo "from deephaven import empty_table; t = empty_table(5).update('X = i'); print(t.to_string())" | dh exec -
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
# Keep server alive for web UI access
dh app dashboard.py
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
