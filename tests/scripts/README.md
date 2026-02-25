# Test Scripts

Manual test scripts for the Deephaven CLI (`dh exec` and `dh app` commands).

## Usage

```bash
# Run a script
dh exec tests/scripts/01_hello.py --quiet

# Run with table preview
dh exec tests/scripts/05_create_table.py --quiet --show-tables

# Test timeout (will timeout after 5 seconds)
dh exec tests/scripts/09_slow_script.py --quiet --timeout=5
echo "Exit code: $?"

# Test error handling
dh exec tests/scripts/07_error_division.py --quiet
echo "Exit code: $?"

# Run as long-running app (Ctrl+C to stop)
dh app tests/scripts/11_time_table.py
```

## Scripts

| Script | Description | Expected Exit Code |
|--------|-------------|-------------------|
| 01_hello.py | Simple hello world | 0 |
| 02_expression.py | Expression result | 0 |
| 03_multiline.py | Function definition and loop | 0 |
| 04_stdout_stderr.py | Tests output stream separation | 0 |
| 05_create_table.py | Creates a single table | 0 |
| 06_multiple_tables.py | Creates multiple tables | 0 |
| 07_error_division.py | Division by zero error | 1 |
| 08_error_name.py | Undefined variable error | 1 |
| 09_slow_script.py | For timeout testing | 3 (with --timeout=5) |
| 10_data_analysis.py | Aggregation example | 0 |
| 11_time_table.py | Time-based table | 0 |
| 12_import_check.py | Shows available modules | 0 |

## Run All Tests

```bash
# Quick smoke test of all scripts
for f in tests/scripts/*.py; do
  echo "=== $f ==="
  dh exec "$f" --quiet --timeout=30
  echo "Exit: $?"
  echo
done
```
