---
name: dh-cli
description: Run and test Deephaven Python code using the dh CLI tool. Use when working with Deephaven tables, executing Deephaven scripts, or testing real-time data code. Triggers on mentions of "deephaven", "dh exec", "dh repl", or testing table operations.
---

# Deephaven CLI (dh)

CLI tool for running Python code with Deephaven real-time data capabilities.

## Quick Reference

### Execute a script (preferred for testing)
```bash
dh exec script.py --quiet
```

### Execute inline code
```bash
dh exec - --quiet <<< "from deephaven import empty_table; t = empty_table(3).update(['X = i'])"
```

### Execute with table output preview
```bash
dh exec script.py --quiet --show-tables
```

### Execute with timeout
```bash
dh exec script.py --quiet --timeout 30
```

## Commands

| Command | Purpose |
|---------|---------|
| `dh exec <script>` | Run script and exit (use `-` for stdin) |
| `dh repl` | Interactive REPL session |
| `dh app <script>` | Long-running server mode |

## Common Options

- `--quiet` / `-q` - Suppress startup messages (recommended for automation)
- `--port PORT` - Server port (default: 10000)
- `--timeout SECONDS` - Max execution time
- `--show-tables` - Display preview of created tables (**first 10 rows only**)
- `--jvm-args ARGS` - JVM arguments (default: `-Xmx4g`)

## Important Limitations

**Table previews show only the first 10 rows.** The `--show-tables` flag uses `.head(10)` internally, so large tables are truncated. To verify full table contents, print row counts or specific slices in your script:
```python
print(f"Total rows: {result.size}")  # Check actual size
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Script error (exception) |
| 2 | Connection/file error |
| 3 | Timeout exceeded |
| 130 | Interrupted (Ctrl+C) |

## Testing Patterns

### Test a simple expression
```bash
dh exec - --quiet <<< "print(6 * 7)"
```

### Test table creation
```bash
dh exec - --quiet --show-tables << 'EOF'
from deephaven import empty_table
result = empty_table(5).update(["X = i", "Y = X * 2"])
EOF
```

### Run pytest-style tests
```bash
dh exec tests/test_example.py --quiet --timeout 60
```

## Deephaven Basics

Within `dh exec` or `dh repl`, common imports:
```python
from deephaven import empty_table, new_table, time_table
from deephaven.column import int_col, string_col, double_col
```

Create tables:
```python
# Empty table with computed columns
t = empty_table(10).update(["X = i", "Y = X * 2"])

# Time table (streaming)
t = time_table("PT1S")  # tick every second

# From columns
t = new_table([
    int_col("A", [1, 2, 3]),
    string_col("B", ["x", "y", "z"])
])
```
