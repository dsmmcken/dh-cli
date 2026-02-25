# Plan: Intercept "clear" command and remove toolbar hint

## Summary
1. Intercept `clear` as a special command in the REPL loop (like `exit()`/`quit()`)
2. Remove "Ctrl+L: clear" from the bottom toolbar (keep Ctrl+L working)

## Files to Modify

### 1. `src/deephaven_cli/repl/console.py`
Add `clear` handling in `interact()` method around line 40:

```python
# Handle special commands
if text.strip() in ("exit()", "quit()"):
    break

if text.strip() == "clear":
    print("\033[2J\033[H", end="", flush=True)  # ANSI clear screen + move cursor home
    continue
```

### 2. `src/deephaven_cli/repl/toolbar.py`
Change line 24 from:
```python
f"Mem: {mem_mb:.0f}MB | <i>Ctrl+R: search | Ctrl+L: clear</i>"
```
to:
```python
f"Mem: {mem_mb:.0f}MB | <i>Ctrl+R: search | Alt+Enter: newline</i>"
```

## Testing

### Unit test in `tests/test_repl_prompt.py`
Add test for clear command handling in console:

```python
def test_clear_command_clears_screen(capsys):
    """Typing 'clear' clears the screen."""
    # Mock session.prompt to return "clear" then "exit()"
    # Verify ANSI escape sequence was printed
```

### Manual verification
```bash
uv run dh repl
>>> clear
# Screen should clear
# Ctrl+L should also still work
```

### Run existing tests
```bash
uv run python -m pytest tests/test_repl_prompt.py -v
```
