# Plan: Improve CLI Help System

## Goal
Make `uv run dh` (no command) helpful instead of showing an error, and improve help documentation across all commands.

## Current State
- Running `dh` with no args shows: `dh: error: the following arguments are required: command`
- Help descriptions are minimal (e.g., "Execute a script file")
- No usage examples in help text

## Changes

### 1. Make no-args show help (cli.py:24, cli.py:105)

**Change subparsers from required to optional:**
```python
# Line 24: Remove required=True
subparsers = parser.add_subparsers(dest="command")  # was: required=True
```

**Add handler after parse_args (line 105):**
```python
args = parser.parse_args()

if args.command is None:
    parser.print_help()
    return EXIT_SUCCESS
```

### 2. Add description and examples to main parser (cli.py:20-24)

Add constants and update parser:
```python
DESCRIPTION = """\
Deephaven CLI - Command-line tool for Deephaven servers

Launch embedded Deephaven servers and execute Python scripts with
real-time data capabilities.
"""

EPILOG = """\
Examples:
  dh repl                              Start interactive session
  dh exec script.py                    Run script and exit
  dh exec script.py -q --timeout 30    Quiet mode with timeout
  cat script.py | dh exec -            Read from stdin
  dh app dashboard.py                  Long-running server

Use 'dh <command> --help' for more details.
"""

parser = argparse.ArgumentParser(
    prog="dh",
    description=DESCRIPTION,
    epilog=EPILOG,
    formatter_class=argparse.RawDescriptionHelpFormatter,
)
```

### 3. Improve subcommand help (cli.py:27-103)

**repl (line 27-42):**
```python
repl_parser = subparsers.add_parser(
    "repl",
    help="Start an interactive REPL session",
    description="Interactive Python REPL with Deephaven server context.\n\n"
                "Provides full Python environment with Deephaven imports,\n"
                "tab completion, and direct table manipulation.",
    epilog="Examples:\n"
           "  dh repl\n"
           "  dh repl --port 8080\n"
           "  dh repl --jvm-args -Xmx8g",
    formatter_class=argparse.RawDescriptionHelpFormatter,
)
```

**exec (line 45-81):**
```python
exec_parser = subparsers.add_parser(
    "exec",
    help="Execute a script and exit (ideal for automation)",
    description="Execute a Python script in batch mode.\n\n"
                "Best for automation and AI agents:\n"
                "  - Clean stdout/stderr separation\n"
                "  - Structured exit codes\n"
                "  - Optional timeout",
    epilog="Exit codes:\n"
           "  0   Success\n"
           "  1   Script error\n"
           "  2   Connection error\n"
           "  3   Timeout\n"
           "  130 Interrupted\n\n"
           "Examples:\n"
           "  dh exec script.py\n"
           "  dh exec script.py -q --timeout 60\n"
           "  echo \"print('hi')\" | dh exec -\n"
           "  dh exec script.py --show-tables",
    formatter_class=argparse.RawDescriptionHelpFormatter,
)
```

**app (line 84-103):**
```python
app_parser = subparsers.add_parser(
    "app",
    help="Run script and keep server alive (dashboards/services)",
    description="Run a script and keep the Deephaven server running.\n\n"
                "Use for:\n"
                "  - Dashboards and visualizations\n"
                "  - Long-running data pipelines\n"
                "  - Services that need persistent server\n\n"
                "Server runs until Ctrl+C.",
    epilog="Examples:\n"
           "  dh app dashboard.py\n"
           "  dh app dashboard.py --port 8080",
    formatter_class=argparse.RawDescriptionHelpFormatter,
)
```

### 4. Improve argument help strings

```python
# --port (all subcommands)
"--port", help="Server port (default: %(default)s)"

# --jvm-args (all subcommands)
"--jvm-args", help="JVM arguments (default: %(default)s). Example: -Xmx8g"

# --timeout (exec)
"--timeout", help="Max execution time in seconds (exit code 3 on timeout)"

# script (exec)
"script", help="Python script to execute (use '-' for stdin)"
```

### 5. Update test (test_phase6_cli.py:13-22)

```python
def test_cli_no_args_shows_help(self):
    """CLI with no args shows help and exits successfully."""
    result = subprocess.run(
        [sys.executable, "-m", "deephaven_cli.cli"],
        capture_output=True,
        text=True,
    )
    assert result.returncode == 0
    assert "repl" in result.stdout
    assert "exec" in result.stdout
    assert "Examples:" in result.stdout
```

## Files to Modify
- `src/deephaven_cli/cli.py` - Main help improvements
- `tests/test_phase6_cli.py` - Update test expectation

## Expected Output

**`dh` (no args) or `dh --help`:**
```
usage: dh [-h] {repl,exec,app} ...

Deephaven CLI - Command-line tool for Deephaven servers

Launch embedded Deephaven servers and execute Python scripts with
real-time data capabilities.

positional arguments:
  {repl,exec,app}
    repl           Start an interactive REPL session
    exec           Execute a script and exit (ideal for automation)
    app            Run script and keep server alive (dashboards/services)

options:
  -h, --help       show this help message and exit

Examples:
  dh repl                              Start interactive session
  dh exec script.py                    Run script and exit
  dh exec script.py -q --timeout 30    Quiet mode with timeout
  cat script.py | dh exec -            Read from stdin
  dh app dashboard.py                  Long-running server

Use 'dh <command> --help' for more details.
```

## Verification
1. Run `uv run dh` - should show help with examples, exit 0
2. Run `uv run dh --help` - same output
3. Run `uv run dh exec --help` - should show exec details with exit codes
4. Run `uv run dh repl --help` - should show repl details
5. Run `uv run pytest tests/test_phase6_cli.py -v` - all tests pass
