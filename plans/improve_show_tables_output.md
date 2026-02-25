# Plan: Improve --show-tables Output with Column Types and Table Size

## Overview

Enhance the `--show-tables` output to display column types and table size by default, with CLI options to suppress these.

## Current Behavior

```
=== Table: my_table ===
   X  Y
0  1  a
1  2  b
2  3  c
```

Only shows column names and data rows (first 10 by default).

## Proposed Behavior

```
=== Table: my_table (3 rows, static) ===
Columns: X (int64), Y (string)

   X  Y
0  1  a
1  2  b
2  3  c
```

For refreshing tables:
```
=== Table: live_data (1,000 rows, refreshing) ===
Columns: timestamp (timestamp[ns]), value (double)
...
```

Shows:
- Table size (row count) in header
- Static/refreshing indicator in header
- Column names with types
- Data preview

## CLI Options

Add to `exec` subcommand:

| Flag | Description |
|------|-------------|
| `--no-table-meta` | Suppress column types and row count |

## Implementation Steps

### 1. Update `get_table_preview()` in `executor.py`

Modify the function signature and implementation:

```python
@dataclass
class TableMeta:
    """Metadata about a table."""
    row_count: int
    is_refreshing: bool
    columns: list[tuple[str, str]]  # (name, type) pairs

def get_table_preview(
    self,
    table_name: str,
    rows: int = 10,
    show_meta: bool = True
) -> tuple[str, TableMeta | None]:
    """Get a string preview of a table.

    Args:
        table_name: Name of the table to preview
        rows: Number of rows to show (default: 10)
        show_meta: Include column types and row count (default: True)

    Returns:
        Tuple of (preview_string, TableMeta or None on error)
    """
    session = self.client.session
    try:
        table = session.open_table(table_name)
        arrow_table = table.to_arrow()

        # Get metadata
        total_rows = arrow_table.num_rows
        is_refreshing = table.is_refreshing
        schema = arrow_table.schema
        columns = [(field.name, str(field.type)) for field in schema]

        meta = TableMeta(total_rows, is_refreshing, columns)

        # Build output
        lines = []

        if show_meta:
            # Format column types
            col_info = ", ".join(f"{name} ({typ})" for name, typ in columns)
            if len(f"Columns: {col_info}") > 80:
                # Use row format for many columns
                lines.append("Columns:")
                for name, typ in columns:
                    lines.append(f"  {name} ({typ})")
            else:
                lines.append(f"Columns: {col_info}")
            lines.append("")

        # Data preview
        if total_rows == 0:
            lines.append("(empty table)")
        else:
            preview_df = arrow_table.slice(0, rows).to_pandas()
            lines.append(preview_df.to_string())

        return "\n".join(lines), meta

    except Exception as e:
        return f"(error previewing table: {e})", None
```

### 2. Update CLI argument parser in `cli.py`

Add new argument after `--show-tables`:

```python
exec_parser.add_argument(
    "--no-table-meta",
    action="store_true",
    help="Suppress column types and row count in table output",
)
```

### 3. Update `run_exec()` in `cli.py`

Pass the new flag through to `get_table_preview()`:

```python
if show_tables and result.assigned_tables:
    show_meta = not args.no_table_meta
    for table_name in result.assigned_tables:
        preview, meta = executor.get_table_preview(
            table_name,
            show_meta=show_meta
        )
        if meta is not None:
            status = "refreshing" if meta.is_refreshing else "static"
            print(f"\n=== Table: {table_name} ({meta.row_count:,} rows, {status}) ===")
        else:
            print(f"\n=== Table: {table_name} ===")
        print(preview)
```

### 4. Update REPL table display

The REPL in `console.py` also displays tables after commands. Apply the same formatting for consistency.

Location: `src/deephaven_cli/repl/console.py` lines 85-92

The REPL should show column types and row count by default, matching the `exec` behavior.

### 5. Add tests

Add test cases in `tests/test_phase7_exec.py`:

- Test default output includes column types and row count
- Test `--no-table-meta` suppresses metadata
- Test various column types display correctly

Update REPL tests to verify consistent behavior:

- Test REPL table display includes column types and row count

## Files to Modify

1. `src/deephaven_cli/repl/executor.py` - Update `get_table_preview()`
2. `src/deephaven_cli/cli.py` - Add `--no-table-meta` flag, update display logic
3. `src/deephaven_cli/repl/console.py` - Update REPL to use same table display format
4. `tests/test_phase7_exec.py` - Add tests

## Example Output Formats

### Default (with metadata, static table):
```
=== Table: trades (1,000,000 rows, static) ===
Columns: timestamp (timestamp[ns]), symbol (string), price (double), volume (int64)

                     timestamp symbol   price  volume
0  2024-01-01 09:30:00.000000   AAPL  185.50    1000
1  2024-01-01 09:30:00.100000   GOOG  140.25     500
...
```

### Refreshing table:
```
=== Table: live_feed (5,000 rows, refreshing) ===
Columns: timestamp (timestamp[ns]), value (double)

                     timestamp   value
0  2024-01-01 09:30:00.000000  123.45
1  2024-01-01 09:30:00.100000  123.67
...
```

### With `--no-table-meta`:
```
=== Table: trades ===
                     timestamp symbol   price  volume
0  2024-01-01 09:30:00.000000   AAPL  185.50    1000
1  2024-01-01 09:30:00.100000   GOOG  140.25     500
...
```

## Edge Cases

### Empty tables
Show "0 rows" and column info, with message instead of empty data:
```
=== Table: empty_table (0 rows) ===
Columns: X (int64), Y (string)

(empty table)
```

### Tables with many columns
If single-line format exceeds 80 characters, display one column per row:
```
=== Table: wide_table (100 rows) ===
Columns:
  id (int64)
  name (string)
  timestamp (timestamp[ns])
  price (double)
  volume (int64)
  exchange (string)

   id  name  ...
```

Implementation: Build single-line string first, if `len > 80`, switch to row format with 2-space indent.

### Complex types (arrays, structs)
Use Arrow's string representation, which is human-readable:
- Arrays: `list<int64>`
- Structs: `struct<x: int64, y: string>`
- Maps: `map<string, int64>`

No special handling needed - Arrow's `field.type` already formats these well.

### Very large row counts
Format with commas for readability:
```
=== Table: big_table (1,234,567 rows) ===
```

Implementation: Use `f"{row_count:,}"` formatting.
