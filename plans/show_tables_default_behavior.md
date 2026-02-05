# Plan: Make Show Tables the Default Behavior

**Status: IMPLEMENTED** ✓

## Current State

| Command | Table Display | Flag |
|---------|--------------|------|
| `dh repl` | **Always shows** tables after each command | None needed |
| `dh exec` | **Opt-in** via `--show-tables` flag | `--show-tables`, `--no-table-meta` |
| `dh app` | Never shows (web UI mode) | N/A |

## Goal

Make `--show-tables` the **default** behavior for `dh exec`, so users see table output without needing to specify the flag.

## Proposed Changes

### 1. Change `--show-tables` to `--no-show-tables` (cli.py)

**File:** `src/deephaven_cli/cli.py` (lines 177-180)

**Before:**
```python
exec_parser.add_argument(
    "--show-tables",
    action="store_true",
    help="After execution, show assigned tables (name, schema, preview)",
)
```

**After:**
```python
exec_parser.add_argument(
    "--no-show-tables",
    action="store_true",
    help="Suppress table preview output after execution",
)
```

### 2. Update `run_exec()` Logic (cli.py)

**File:** `src/deephaven_cli/cli.py` (lines 421-434)

**Before:**
```python
if args.show_tables:
    # show table previews
```

**After:**
```python
if not args.no_show_tables:
    # show table previews (default behavior)
```

### 3. Update Help Text (cli.py)

Update the exec subcommand help/description to reflect that table display is now the default.

### 4. Update Tests (test_phase7_exec.py)

**File:** `tests/test_phase7_exec.py`

- Rename/update `test_exec_show_tables()` to test default behavior
- Add new test `test_exec_no_show_tables()` to verify suppression works
- Update any tests that rely on the old flag behavior

### 5. Update Documentation

- Update README if it mentions `--show-tables`
- Update any inline help strings

## Alternative Considered

**Keep both flags:** `--show-tables` (default true) and `--no-show-tables`

This was rejected because:
- More complexity for no real benefit
- Confusing to have a flag that's already the default
- `--no-show-tables` is clear and follows common CLI patterns (e.g., `--no-color`)

## Impact Analysis

- **Breaking change:** Scripts that relied on the absence of output will now get table output
- **Mitigation:** Users can add `--no-show-tables` to restore old behavior
- **Benefit:** More useful default for interactive use and debugging

## Implementation Order

1. Update argparse flag definition
2. Update `run_exec()` logic
3. Update tests
4. Update documentation
5. Test manually with various scenarios

## Test Scenarios

1. `dh exec script.py` → Should show tables (new default)
2. `dh exec --no-show-tables script.py` → Should NOT show tables
3. `dh exec --no-table-meta script.py` → Should show tables without metadata
4. `dh exec --no-show-tables --no-table-meta script.py` → Should NOT show tables
5. `echo "t = empty_table(5)" | dh exec -` → Should show tables
6. `dh exec -c "t = empty_table(5)"` → Should show tables
