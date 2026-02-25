# Plan: Remove Row Index from Table Preview Output

**Status: COMPLETED** (2026-02-04)

## Problem Summary

The current table preview displays pandas-style row indices that don't make sense for Deephaven tables:

**Current output:**
```
=== Table: t (5 rows, static) ===
Columns: X (int32), Y (int32)

   X  Y
0  0  0
1  1  2
2  2  4
```

**Desired output:**
```
=== Table: t (5 rows, static) ===
Columns: X (int32), Y (int32)

 X  Y
 0  0
 1  2
 2  4
```

## Root Cause

In `src/deephaven_cli/repl/executor.py`, the `get_table_preview()` method uses:

```python
preview_df = arrow_table.slice(0, rows).to_pandas()
lines.append(preview_df.to_string())
```

The `DataFrame.to_string()` method includes the row index by default (`index=True`).

## Solution

Change line 270 from:
```python
lines.append(preview_df.to_string())
```
to:
```python
lines.append(preview_df.to_string(index=False))
```

## Implementation Steps

### Step 1: Modify executor.py

**File:** `src/deephaven_cli/repl/executor.py`
**Line:** 270

```python
# Before
lines.append(preview_df.to_string())

# After
lines.append(preview_df.to_string(index=False))
```

### Step 2: Add Tests

**File:** `tests/test_phase4_executor.py`

```python
def test_get_table_preview_no_row_index(self, executor):
    """Test table preview does NOT include pandas row index."""
    executor.execute('''
from deephaven import empty_table
no_index_table = empty_table(3).update(["X = i", "Y = i * 2"])
''')
    preview, meta = executor.get_table_preview("no_index_table")
    lines = preview.strip().split('\n')

    # Find the data section (after "Columns:" and empty line)
    data_lines = []
    in_data = False
    for line in lines:
        if line.strip() == '':
            in_data = True
            continue
        if in_data:
            data_lines.append(line)

    # Data lines should NOT start with row indices (0, 1, 2)
    # The first data line should start with actual data values
    assert len(data_lines) >= 1
    # With index=False, lines start with spaces then values, not "0  "
    first_data = data_lines[0].lstrip()
    assert not first_data.startswith('0 ')

def test_get_table_preview_empty_table_no_index(self, executor):
    """Test preview of empty table displays correctly."""
    executor.execute('''
from deephaven import empty_table
empty_table_test = empty_table(0).update(["A = i"])
''')
    preview, meta = executor.get_table_preview("empty_table_test")
    assert "(empty table)" in preview
    assert meta.row_count == 0

def test_get_table_preview_single_row_no_index(self, executor):
    """Test preview of single-row table has no index."""
    executor.execute('''
from deephaven import empty_table
single_row_test = empty_table(1).update(["Val = 42"])
''')
    preview, meta = executor.get_table_preview("single_row_test")
    assert "42" in preview
    # Should NOT have "0" as a row index before the value
    lines = preview.split('\n')
    data_line = [l for l in lines if '42' in l][0]
    # The line should not start with "0" followed by spaces
    assert not data_line.lstrip().startswith('0 ')

def test_get_table_preview_alignment_preserved(self, executor):
    """Test column alignment is preserved without index."""
    executor.execute('''
from deephaven import empty_table
align_test = empty_table(3).update(["Short = i", "LongerColumnName = i * 100"])
''')
    preview, meta = executor.get_table_preview("align_test")
    # Both column names should appear
    assert "Short" in preview
    assert "LongerColumnName" in preview
    # Values should be present
    assert "0" in preview
    assert "200" in preview
```

## Edge Cases

| Edge Case | Expected Behavior | Handled By |
|-----------|-------------------|------------|
| Empty table (0 rows) | Shows "(empty table)" | Existing code at line 267-268 |
| Single row table | No index "0" shown | `index=False` |
| Wide values | Alignment maintained | pandas handles internally |
| Many columns | Column metadata in row format | Existing code |
| Mixed data types | Proper alignment per type | pandas handles |

## Verification

```bash
# Manual test
dh repl
>>> from deephaven import empty_table
>>> t = empty_table(5).update(["X = i", "Y = i * 2"])
# Should show data without 0,1,2,3,4 on left

# Run tests
uv run python -m pytest tests/test_phase4_executor.py -v -k "preview"
```

## Files to Modify

1. `src/deephaven_cli/repl/executor.py` - Line 270: add `index=False`
2. `tests/test_phase4_executor.py` - Add new test cases
