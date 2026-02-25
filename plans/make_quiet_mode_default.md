# Plan: Make Quiet Mode Default for All Commands

## Goal
Make quiet mode the default for all commands (`repl`, `exec`, `app`) and only offer a `--verbose` / `-v` flag. Remove the `--quiet` / `-q` flag entirely.

## Current State

| Command | Flag | Default Behavior |
|---------|------|------------------|
| `repl` | `--verbose` / `-v` | Quiet by default ✓ |
| `exec` | `--quiet` / `-q` | Verbose by default ✗ |
| `app` | None | Always verbose ✗ |

## Target State

| Command | Flag | Default Behavior |
|---------|------|------------------|
| `repl` | `--verbose` / `-v` | Quiet by default |
| `exec` | `--verbose` / `-v` | Quiet by default |
| `app` | `--verbose` / `-v` | Quiet by default |

## Files to Modify

### 1. `src/deephaven_cli/cli.py`

**Remove from `exec` subparser (lines 116-121):**
```python
exec_parser.add_argument(
    "--quiet",
    "-q",
    action="store_true",
    help="Suppress startup messages",
)
```

**Add to `exec` subparser (after line 115):**
```python
exec_parser.add_argument(
    "--verbose",
    "-v",
    action="store_true",
    help="Show startup messages (default: quiet)",
)
```

**Add to `app` subparser (after line 167):**
```python
app_parser.add_argument(
    "--verbose",
    "-v",
    action="store_true",
    help="Show startup messages (default: quiet)",
)
```

**Update `run_exec()` call (line 178):**
```python
# Change from:
args.quiet,
# To:
args.verbose,
```

**Update `run_app()` call (line 187):**
```python
# Change from:
return run_app(args.script, args.port, args.jvm_args)
# To:
return run_app(args.script, args.port, args.jvm_args, args.verbose)
```

**Update `run_exec()` function signature (line 223):**
```python
# Change from:
def run_exec(script_path, port, jvm_args, quiet, timeout, show_tables):
# To:
def run_exec(script_path, port, jvm_args, verbose, timeout, show_tables):
```

**Update `run_exec()` function body:**
- Change all `quiet` references to `not verbose`
- Lines affected: ~270, 273, 278, 279

**Update `run_app()` function signature (line 333):**
```python
# Change from:
def run_app(script_path: str, port: int, jvm_args: list[str]) -> int:
# To:
def run_app(script_path: str, port: int, jvm_args: list[str], verbose: bool = False) -> int:
```

**Update `run_app()` function body:**
- Add conditional printing based on `verbose`
- Pass `quiet=not verbose` to DeephavenServer
- Lines affected: ~349, 352-356, 365

### 2. `README.md`

**Update Common Options table:**
- Remove `-q, --quiet` row
- Update `-v, --verbose` description to apply to all commands

**Update Execute Scripts section:**
- Remove `dh exec script.py -q` example
- Add `dh exec script.py -v` example if needed

### 3. `tests/test_phase6_cli.py`

- Line 56: Change `assert "--quiet" in result.stdout` to `assert "--verbose" in result.stdout`

### 4. `tests/test_phase7_exec.py`

This file has 15 occurrences of `--quiet`. Since quiet is now the default, most tests should:
- Remove `--quiet` flag entirely (it's now the default behavior)
- Update `test_exec_quiet_suppresses_startup` → rename to `test_exec_default_quiet`
- Update `test_exec_without_quiet_shows_startup` → use `--verbose` flag instead

Lines to update: 19, 35, 48, 61, 81, 97, 109, 122, 133, 150, 171, 183, 195

## Implementation Order

1. Update CLI argument parsing in `cli.py`
2. Update `run_exec()` function
3. Update `run_app()` function
4. Update README.md
5. Update tests

## Verification

```bash
# Test exec defaults to quiet
echo "print('hello')" | dh exec -
# Should output only: hello

# Test exec verbose mode
echo "print('hello')" | dh exec -v -
# Should output startup messages + hello

# Test app defaults to quiet
dh app dashboard.py
# Should only show essential output

# Test app verbose mode
dh app dashboard.py -v
# Should show startup messages

# Test repl still works (already quiet by default)
dh repl
dh repl -v

# Run tests
uv run pytest tests/ -v
```
