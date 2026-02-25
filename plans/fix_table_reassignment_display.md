# Plan: Fix Table Reassignment Display

## Problem Statement

When a variable is reassigned to a new table, the REPL doesn't display it:

```python
>>> t = empty_table(200).update(["a = 1"])
# Table 't' is displayed ✓

>>> t = empty_table(200).update(["a = 2"])
# Nothing displayed ✗
```

## Root Cause Analysis

The issue is in `executor.py` lines 31-72. The table detection logic only finds **NEW** table names:

```python
tables_before = set(self.client.tables)
# ... execute code ...
tables_after = set(self.client.tables) - {"__dh_result_table"}
new_tables = list(tables_after - tables_before)  # ← Only detects NEW names
```

When `t` is reassigned:
1. `tables_before` already contains `t` (from the first assignment)
2. After execution, `t` still exists (with new data)
3. `tables_after - tables_before = {}` (empty set)
4. No table is displayed

The system conflates "table name exists" with "table hasn't changed".

## Possible Solutions

### Option A: AST-based Assignment Detection (Recommended)

Parse the user's code to identify which variables are being assigned, then display those if they're tables.

**Approach:**
1. Use Python's `ast` module to parse the code
2. Extract target variable names from assignment statements (`Assign`, `AnnAssign`, `AugAssign`)
3. After execution, check if those variables are tables on the server
4. Display them regardless of whether they're "new"

**Pros:**
- Handles both new assignments and reassignments uniformly
- Relatively simple to implement
- Works for all assignment patterns: `t = ...`, `a, b = ...`, `t: Table = ...`

**Cons:**
- Requires parsing code twice (once for AST, once for execution)
- May not catch dynamically-named assignments (e.g., `globals()['t'] = ...`)

**Implementation sketch:**
```python
import ast

def get_assigned_names(code: str) -> set[str]:
    """Extract variable names being assigned in the code."""
    try:
        tree = ast.parse(code)
    except SyntaxError:
        return set()

    names = set()
    for node in ast.walk(tree):
        if isinstance(node, ast.Assign):
            for target in node.targets:
                if isinstance(target, ast.Name):
                    names.add(target.id)
                elif isinstance(target, ast.Tuple):
                    for elt in target.elts:
                        if isinstance(elt, ast.Name):
                            names.add(elt.id)
        elif isinstance(node, ast.AnnAssign) and isinstance(node.target, ast.Name):
            names.add(node.target.id)
        elif isinstance(node, ast.AugAssign) and isinstance(node.target, ast.Name):
            names.add(node.target.id)
    return names
```

Then in `execute()`:
```python
assigned_names = get_assigned_names(code)
# ... execute code ...
# Filter to only table names that exist on server
assigned_tables = [name for name in assigned_names if name in self.client.tables]
```

### Option B: Track Table Identity/Version

Query some kind of table version or identity before and after execution.

**Pros:**
- Would catch any table modification, not just reassignment

**Cons:**
- Deephaven may not expose version/identity info easily
- More complex to implement
- May have performance implications

### Option C: Always Display Assigned Tables via Wrapper

Modify the wrapper script to capture the assigned value and return it.

**Approach:**
Parse AST, identify assignments, and inject code to capture the repr of assigned table values in the wrapper script itself.

**Pros:**
- Single execution
- Could show the exact value that was assigned

**Cons:**
- More complex wrapper logic
- Would need to detect Table types server-side

### Option D: Keep "new tables" behavior, add explicit display

Keep current behavior but give users a way to explicitly display:
```python
>>> t = empty_table(200).update(["a = 2"])
>>> t  # Just reference the variable
<table preview>
```

**Pros:**
- No code changes needed
- Clear user intent

**Cons:**
- Inconsistent with initial assignment behavior
- Worse user experience

## Recommendation

**Option A (AST-based Assignment Detection)** is the best approach because:
1. It's simple and reliable
2. It handles the common case correctly
3. It maintains consistent behavior: "if you assign a table, you see it"
4. Minor edge cases (dynamic assignments) are rare and can be worked around

## Implementation Plan

1. Add `get_assigned_names()` function to `executor.py`
2. Modify `execute()` to:
   - Call `get_assigned_names(code)` before execution
   - After execution, filter to names that are tables on the server
   - Return these in a new field `assigned_tables` (or repurpose `updated_tables`)
3. Modify `console.py` `_execute_and_display()` to:
   - Display `assigned_tables` instead of (or in addition to) `new_tables`
   - Avoid displaying the same table twice

4. Update `ExecutionResult` dataclass if needed

## Questions to Consider

1. **Should we show ALL assigned tables, or only tables that were reassigned (i.e., already existed)?**
   - Recommendation: Show all assigned tables. This gives consistent behavior and makes the intent clear.

2. **What about chained assignments like `a = b = empty_table(...)`?**
   - The AST parsing will catch both `a` and `b`.

3. **What about walrus operator `:=`?**
   - Should handle `ast.NamedExpr` as well for completeness.

4. **Performance concern?**
   - AST parsing is very fast for typical REPL inputs (single lines or small blocks). Not a concern.
