# Plan: Add `-c` Flag to `dh exec` Command

## Overview

Add support for `dh exec -c <code>` (and `dh -c <code>` shorthand) to execute inline Python code directly from the command line, similar to `python -c`.

## Current State

- `dh exec` takes a positional `script` argument (file path or `-` for stdin)
- Code is read from file/stdin, then passed to `CodeExecutor.execute()`
- CLI parsing uses argparse in `cli.py`

## Goal

Enable these usage patterns:
```bash
# Execute inline code
dh exec -c "from deephaven import empty_table; t = empty_table(10)"
dh exec -c 'print("hello")'

# Combine with other flags
dh exec -c "t = empty_table(5)" --show-tables
dh exec -c "import time; time.sleep(10)" --timeout 5

# Shorthand (skip 'exec' subcommand)
dh -c "print('hello')"
```

## Implementation Steps

### 1. Modify argparse configuration in `cli.py`

**File:** `src/deephaven_cli/cli.py`

**Changes to `setup_exec_parser()` (around line 69):**
- Make `script` positional argument optional (use `nargs='?'`)
- Add `-c` / `--command` argument that accepts a string
- Add validation: either `-c` or `script` must be provided, but not both

```python
exec_parser.add_argument(
    "-c", "--command",
    metavar="CODE",
    help="Execute CODE as a Python string (like python -c)"
)
exec_parser.add_argument(
    "script",
    nargs="?",  # Make optional
    help="Python script path, or '-' for stdin"
)
```

### 2. Modify `run_exec()` function

**File:** `src/deephaven_cli/cli.py`

**Changes in `run_exec()` (around line 312):**
- Add logic to determine code source (file, stdin, or `-c` argument)
- Validate mutual exclusivity
- Set appropriate source label for error messages

```python
def run_exec(args):
    # Determine code source
    if args.command:
        if args.script:
            print("Error: Cannot use both -c and a script file", file=sys.stderr)
            return 2
        code = args.command
        source = "<string>"
    elif args.script == "-":
        code = sys.stdin.read()
        source = "<stdin>"
    elif args.script:
        # existing file reading logic
        ...
    else:
        print("Error: Must provide either -c CODE or a script file", file=sys.stderr)
        return 2
```

### 3. Add top-level `-c` shorthand

**File:** `src/deephaven_cli/cli.py`

**Changes to `main()` or argument preprocessing:**
- Detect when `-c` is used without a subcommand
- Route to exec with appropriate args

Two approaches:

**Option A: Pre-process sys.argv** (simpler)
```python
def main():
    # Check for shorthand: dh -c "code"
    if len(sys.argv) >= 2 and sys.argv[1] == "-c":
        # Insert 'exec' subcommand
        sys.argv.insert(1, "exec")

    # Continue with normal parsing
    ...
```

**Option B: Add -c to main parser** (cleaner but more complex)
Add `-c` to the main parser and handle it specially.

**Recommendation:** Option A is simpler and matches `python -c` behavior exactly.

### 4. Update help text

Update help strings to document the new `-c` option clearly:
- In exec subcommand help
- In main command description

### 5. Add tests

**File:** `tests/test_phase7_exec.py` (or new test file)

Test cases:
1. Basic `-c` execution with simple print
2. `-c` with Deephaven table creation
3. `-c` combined with `--show-tables`
4. `-c` combined with `--timeout`
5. Error case: both `-c` and script file provided
6. Error case: neither `-c` nor script provided
7. Shorthand `dh -c` without `exec` subcommand
8. `-c` with multiline code (using `;` or actual newlines in quotes)

### 6. Update documentation

**File:** `README.md` or relevant docs

Add examples showing `-c` usage.

## Edge Cases to Handle

1. **Empty code string:** `dh exec -c ""` - should probably error or do nothing gracefully
2. **Code with quotes:** Shell escaping is user's responsibility, but document common patterns
3. **Code with newlines:** Works with `$'...\n...'` or semicolons
4. **Interactive hints:** The backtick warning system shouldn't trigger for `-c`

## Testing Commands

```bash
# After implementation, verify:
dh exec -c "print('hello')"
dh exec -c "from deephaven import empty_table; t = empty_table(5)" --show-tables
dh -c "print('shorthand works')"
dh exec -c "code" script.py  # Should error
dh exec  # Should error (no input)
```

## Files to Modify

1. `src/deephaven_cli/cli.py` - Main changes
2. `tests/test_phase7_exec.py` - Add tests
3. `README.md` - Update documentation (optional)

## Estimated Scope

- ~30-50 lines of code changes in `cli.py`
- ~50-100 lines of new tests
- Minimal risk of breaking existing functionality (all changes are additive)
