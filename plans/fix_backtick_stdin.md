# Plan: Fix Backtick Handling in Piped Input

**Status: COMPLETED**

## Problem Summary

When users pipe scripts to `dh exec -`, backticks in Deephaven query syntax fail silently:

```bash
echo "stocks.where('Sym = `DOG`')" | dh exec -
# Fails silently - backticks interpreted as command substitution
```

## Root Cause Analysis

**This is a shell interpretation issue, NOT a dh-cli bug.**

When using double quotes in bash:
- Backticks trigger **command substitution**
- Shell tries to execute `DOG` as a command
- Fails silently or replaces with empty string
- By the time data reaches `dh exec`, it's already corrupted

**Deephaven requires backticks** for string literals in query expressions - there is no alternative syntax.

## Solution: Documentation + Warning Detection

Since the corruption happens before data reaches Python, we cannot fix it in code. Instead:

1. **Document the issue and workarounds** in CLI help and README
2. **Add warning detection** when errors suggest backtick issues
3. **Provide clear examples** of working approaches

## Implementation Steps

### Step 1: Update CLI Help

**File:** `src/deephaven_cli/cli.py`

Update the exec parser epilog:

```python
exec_parser = subparsers.add_parser(
    "exec",
    ...
    epilog="Exit codes:\n"
           "  0   Success\n"
           "  1   Script error\n"
           "  2   Connection error\n"
           "  3   Timeout\n"
           "  130 Interrupted\n\n"
           "Examples:\n"
           "  dh exec script.py\n"
           "  dh exec script.py -v --timeout 60\n"
           "  echo \"print('hi')\" | dh exec -\n"
           "  dh exec script.py --show-tables\n\n"
           "Backticks in piped input:\n"
           "  Deephaven uses backticks for string literals in queries.\n"
           "  Shell interprets backticks as command substitution.\n"
           "  Solutions:\n"
           "    1. Use a script file: dh exec script.py\n"
           "    2. Use $'...' syntax: echo $'t.where(\"X = \\`val\\`\")' | dh exec -\n"
           "    3. Escape in double quotes: echo \"t.where('X = \\`val\\`')\" | dh exec -",
    ...
)
```

### Step 2: Update README.md

Add section after the CLI usage examples:

```markdown
## Special Characters in Piped Input

Deephaven query strings use backticks (`) for string literals:

```python
# In a .py file - works correctly
stocks.where('Sym = `DOG`')
```

When piping scripts to `dh exec -`, the shell interprets backticks as command
substitution **before** the data reaches dh-cli.

### Recommended Solutions

**1. Use a script file (most reliable):**

**2. Use ANSI-C quoting with `\n` for multiline:**
```bash
echo $'from deephaven.plot import express as dx\nstocks = dx.data.stocks()\ndog = stocks.where(\'Sym = `DOG`\')' | dh exec --show-tables -
```

**3. Escape backticks in double quotes:**
```bash
# Use \` to escape each backtick
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

### Step 3: Add Warning Detection (Optional)

**File:** `src/deephaven_cli/cli.py` in `run_exec()`

```python
def _suggest_backtick_hint(script_content: str, error: str) -> str | None:
    """Check if error might be caused by shell backtick interpretation."""
    # Heuristics for potential backtick issues:
    # - Script has query operations but no backticks
    # - Error is syntax-related
    patterns = ['.where(', '.update(', '.select(']
    has_query_ops = any(p in script_content for p in patterns)
    has_backticks = '`' in script_content
    is_syntax_error = 'SyntaxError' in error or 'NameError' in error

    if has_query_ops and not has_backticks and is_syntax_error:
        return (
            "\nHint: If your script contains backticks (`) for Deephaven strings,\n"
            "they may have been interpreted by the shell. Use a script file\n"
            "or $'...' quoting. See 'dh exec --help' for details."
        )
    return None
```

Then in the error handling:

```python
if result.error:
    print(result.error, file=sys.stderr)
    hint = _suggest_backtick_hint(script_content, result.error)
    if hint:
        print(hint, file=sys.stderr)
    return EXIT_SCRIPT_ERROR
```

### Step 4: Add Tests

**File:** `tests/test_phase7_exec.py`

```python
import tempfile
import os

class TestExecBackticks:
    """Tests for backtick handling in piped input."""

    def test_exec_backticks_from_file(self):
        """Test backticks work correctly from file."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.py', delete=False) as f:
            f.write('''
from deephaven import empty_table
t = empty_table(1).update(["S = `hello`"])
''')
            f.flush()
            try:
                result = subprocess.run(
                    ["dh", "exec", f.name, "--show-tables"],
                    capture_output=True,
                    text=True,
                    timeout=120,
                )
                assert result.returncode == 0
                assert "hello" in result.stdout
            finally:
                os.unlink(f.name)

    def test_exec_backticks_escaped_double_quotes(self):
        """Test escaped backticks in double quotes work."""
        result = subprocess.run(
            ["bash", "-c", r'echo "from deephaven import empty_table; t = empty_table(1).update([\"S = \`hi\`\"])" | dh exec --show-tables -'],
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 0
        assert "hi" in result.stdout

    def test_exec_backticks_ansi_c_quoting(self):
        """Test $'...' ANSI-C quoting preserves backticks."""
        result = subprocess.run(
            ["bash", "-c", r"echo $'from deephaven import empty_table\nt = empty_table(1).update([\"S = `test`\"])' | dh exec --show-tables -"],
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 0
        assert "test" in result.stdout

    def test_exec_empty_backtick_string(self):
        """Test empty string literal with backticks."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.py', delete=False) as f:
            f.write('from deephaven import empty_table\nt = empty_table(1).update(["S = ``"])')
            f.flush()
            try:
                result = subprocess.run(
                    ["dh", "exec", f.name, "--show-tables"],
                    capture_output=True,
                    text=True,
                    timeout=120,
                )
                assert result.returncode == 0
            finally:
                os.unlink(f.name)

    def test_exec_multiple_backtick_pairs(self):
        """Test multiple backtick pairs in same script."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.py', delete=False) as f:
            f.write('''
from deephaven import empty_table
t = empty_table(1).update(["A = `one`", "B = `two`", "C = `three`"])
''')
            f.flush()
            try:
                result = subprocess.run(
                    ["dh", "exec", f.name, "--show-tables"],
                    capture_output=True,
                    text=True,
                    timeout=120,
                )
                assert result.returncode == 0
                assert "one" in result.stdout
                assert "two" in result.stdout
                assert "three" in result.stdout
            finally:
                os.unlink(f.name)

    def test_exec_backticks_in_output(self):
        """Test output containing backticks is preserved."""
        result = subprocess.run(
            ["dh", "exec", "-"],
            input="print('has `backticks` inside')",
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 0
        assert "`backticks`" in result.stdout

    def test_exec_backticks_with_special_chars(self):
        """Test backticks with other special characters."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.py', delete=False) as f:
            f.write('''
from deephaven import empty_table
# Mix of quotes and backticks
t = empty_table(1).update(["S = `hello world`"])
print("Testing 'single' and \\"double\\" quotes")
''')
            f.flush()
            try:
                result = subprocess.run(
                    ["dh", "exec", f.name],
                    capture_output=True,
                    text=True,
                    timeout=120,
                )
                assert result.returncode == 0
                assert "single" in result.stdout
                assert "double" in result.stdout
            finally:
                os.unlink(f.name)
```

## Edge Cases

| Case | Expected | Test |
|------|----------|------|
| Backticks in .py file | Works | `test_exec_backticks_from_file` |
| Escaped in double quotes | Works | `test_exec_backticks_escaped_double_quotes` |
| ANSI-C `$'...'` quoting | Works | `test_exec_backticks_ansi_c_quoting` |
| Empty backticks `` | Works | `test_exec_empty_backtick_string` |
| Multiple backtick pairs | Works | `test_exec_multiple_backtick_pairs` |
| Backticks in print output | Preserved | `test_exec_backticks_in_output` |
| Mixed special chars | Works | `test_exec_backticks_with_special_chars` |

## Files to Modify

1. `src/deephaven_cli/cli.py` - Update epilog, add warning detection
2. `README.md` - Add documentation section
3. `tests/test_phase7_exec.py` - Add test cases

## Why We Can't Fix This in Code

The shell interprets the command **before** Python runs:

```
User types: echo "t.where('Sym = `DOG`')" | dh exec -
                                  ^^^^^
                                  Shell sees this as $(DOG)

Shell sends: t.where('Sym = ')   <-- backticks already gone!

dh exec receives corrupted input - nothing we can do
```

The only solution is to help users avoid the problem through documentation and helpful warnings.
