# Plan: Integrate prompt_toolkit into Deephaven CLI REPL

## Goal

Enhance the Deephaven CLI REPL with prompt_toolkit featuring:

**Input & Editing:**
- Python syntax highlighting (via Pygments)
- Multi-line editing
- Vi/Emacs key binding modes

**History:**
- Persistent history (~/.dh_history)
- History search (Ctrl+R)
- Up/Down arrow navigation
- Auto-suggestions (fish-style grayed-out suggestions from history)

**Additional:**
- Bottom toolbar (connection status, table count, memory usage, server port)
- Custom key bindings (Ctrl+L to clear, etc.)
- Mouse support (click to position, scroll)

## Current State

The REPL in `src/deephaven_cli/repl/console.py` uses:
- Python's built-in `input()` function
- `code.compile_command()` for multi-line detection
- No syntax highlighting or history

## File Structure

```
src/deephaven_cli/repl/
    __init__.py              # Existing
    console.py               # MODIFY - main console entry point
    executor.py              # Existing - unchanged
    prompt/                  # NEW directory
        __init__.py          # Package exports
        session.py           # PromptSession configuration
        toolbar.py           # Bottom toolbar rendering
        keybindings.py       # Custom key bindings
```

## Implementation Steps

### Step 1: Add Required Dependencies

**File:** `pyproject.toml`

```toml
dependencies = [
    "deephaven-server>=0.37.0",
    "pydeephaven>=0.37.0",
    "prompt_toolkit>=3.0.0",
    "pygments>=2.0.0",
]
```

### Step 2: Create prompt/ Package

**File:** `src/deephaven_cli/repl/prompt/__init__.py`

```python
"""Enhanced prompt components for Deephaven REPL."""
from deephaven_cli.repl.prompt.session import create_prompt_session

__all__ = ["create_prompt_session"]
```

### Step 3: Create Bottom Toolbar

**File:** `src/deephaven_cli/repl/prompt/toolbar.py`

```python
from prompt_toolkit.formatted_text import HTML
import os

def create_toolbar(client, port):
    """Create bottom toolbar showing rich status info."""
    def get_toolbar():
        try:
            table_count = len(client.tables)
            # Get process memory (RSS) in MB
            import resource
            mem_mb = resource.getrusage(resource.RUSAGE_SELF).ru_maxrss / 1024
            return HTML(
                f'<b>Connected</b> | Port: {port} | Tables: {table_count} | '
                f'Mem: {mem_mb:.0f}MB | <i>Ctrl+R: search | Ctrl+L: clear</i>'
            )
        except Exception:
            return HTML('<ansired><b>Disconnected</b></ansired>')
    return get_toolbar
```

### Step 4: Create Custom Key Bindings

**File:** `src/deephaven_cli/repl/prompt/keybindings.py`

```python
from prompt_toolkit.key_binding import KeyBindings
from prompt_toolkit.keys import Keys

def create_key_bindings():
    bindings = KeyBindings()

    @bindings.add(Keys.ControlL)
    def clear_screen(event):
        event.app.renderer.clear()

    return bindings
```

### Step 5: Create Session Factory

**File:** `src/deephaven_cli/repl/prompt/session.py`

```python
from prompt_toolkit import PromptSession
from prompt_toolkit.history import FileHistory
from prompt_toolkit.lexers import PygmentsLexer
from prompt_toolkit.auto_suggest import AutoSuggestFromHistory
from prompt_toolkit.enums import EditingMode
from pygments.lexers.python import PythonLexer
from pathlib import Path

from .toolbar import create_toolbar
from .keybindings import create_key_bindings

def create_prompt_session(client, port, vi_mode=False):
    """Create configured PromptSession."""
    return PromptSession(
        history=FileHistory(str(Path.home() / ".dh_history")),
        lexer=PygmentsLexer(PythonLexer),
        auto_suggest=AutoSuggestFromHistory(),
        bottom_toolbar=create_toolbar(client, port),
        key_bindings=create_key_bindings(),
        enable_history_search=True,
        mouse_support=True,
        editing_mode=EditingMode.VI if vi_mode else EditingMode.EMACS,
        multiline=True,
    )
```

### Step 6: Modify DeephavenConsole

**File:** `src/deephaven_cli/repl/console.py`

Key changes:
1. Replace `input()` loop with `PromptSession.prompt()`
2. Use prompt_toolkit's built-in multi-line handling
3. Accept port for toolbar display

```python
from deephaven_cli.repl.prompt import create_prompt_session

class DeephavenConsole:
    def __init__(self, client, port, *, vi_mode=False):
        self.client = client
        self.executor = CodeExecutor(client)
        self._session = create_prompt_session(client, port, vi_mode=vi_mode)

    def interact(self, banner=None):
        if banner:
            print(banner)

        while True:
            try:
                # prompt_toolkit handles multi-line, history, suggestions
                text = self._session.prompt(">>> ")

                if text.strip() in ("exit()", "quit()"):
                    break

                if text.strip():
                    self._execute_and_display(text)

            except EOFError:
                print()
                break
            except KeyboardInterrupt:
                print("\nKeyboardInterrupt")
                continue

        print("Goodbye!")
```

### Step 7: Add CLI Flag for Vi Mode

**File:** `src/deephaven_cli/cli.py`

Add `--vi` flag to repl subcommand:
```python
repl_parser.add_argument("--vi", action="store_true", help="Use Vi key bindings")
```

Pass port and vi_mode to console:
```python
console = DeephavenConsole(client, port=args.port, vi_mode=args.vi)
```

### Step 8: Add Unit Tests

**File:** `tests/test_repl_prompt.py`

Unit test cases:
- Toolbar returns expected HTML with connection status, table count, port
- Key bindings are registered (Ctrl+L)
- Session is configured with correct options (history, lexer, etc.)
- History file path is ~/.dh_history

### Step 9: Add Integration Tests (tmux-based)

**File:** `tests/test_repl_integration.py`

Integration tests using tmux for real TTY environment:

```python
import subprocess
import time
import pytest

class TestReplIntegration:
    """Integration tests using tmux for real TTY."""

    @pytest.fixture
    def tmux_session(self):
        """Create a tmux session for testing."""
        session = f"dh-test-{time.time_ns()}"
        subprocess.run(
            ["tmux", "new-session", "-d", "-s", session, "uv", "run", "dh", "repl"],
            check=True
        )
        time.sleep(3)  # Wait for REPL to start
        yield session
        subprocess.run(["tmux", "kill-session", "-t", session], check=False)

    def capture_pane(self, session):
        """Capture current tmux pane content."""
        result = subprocess.run(
            ["tmux", "capture-pane", "-t", session, "-p"],
            capture_output=True, text=True
        )
        return result.stdout

    def send_keys(self, session, keys, enter=True):
        """Send keys to tmux session."""
        subprocess.run(["tmux", "send-keys", "-t", session, keys], check=True)
        if enter:
            subprocess.run(["tmux", "send-keys", "-t", session, "Enter"], check=True)
        time.sleep(0.5)

    def test_repl_basic_execution(self, tmux_session):
        """REPL executes code and shows result."""
        self.send_keys(tmux_session, "2 + 2")
        time.sleep(1)
        output = self.capture_pane(tmux_session)
        assert "4" in output

    def test_repl_multiline(self, tmux_session):
        """REPL handles multi-line input."""
        self.send_keys(tmux_session, "def foo():")
        self.send_keys(tmux_session, "    return 42")
        self.send_keys(tmux_session, "")
        self.send_keys(tmux_session, "foo()")
        time.sleep(1)
        output = self.capture_pane(tmux_session)
        assert "42" in output

    def test_repl_ctrl_l_clears(self, tmux_session):
        """Ctrl+L clears the screen."""
        self.send_keys(tmux_session, "x = 'marker'")
        subprocess.run(["tmux", "send-keys", "-t", tmux_session, "C-l"], check=True)
        time.sleep(0.5)
        output = self.capture_pane(tmux_session)
        assert output.strip().endswith(">>>")

    def test_repl_toolbar_visible(self, tmux_session):
        """Bottom toolbar shows status info."""
        output = self.capture_pane(tmux_session)
        assert "Connected" in output or "Tables:" in output or "Port:" in output

    def test_repl_exit(self, tmux_session):
        """REPL exits cleanly."""
        self.send_keys(tmux_session, "exit()")
        time.sleep(1)
        output = self.capture_pane(tmux_session)
        assert "Goodbye!" in output
```

Requires tmux installed (add to CI if needed).

## Files to Modify/Create

| File | Action |
|------|--------|
| `pyproject.toml` | MODIFY - add required deps |
| `src/deephaven_cli/repl/prompt/__init__.py` | CREATE |
| `src/deephaven_cli/repl/prompt/toolbar.py` | CREATE |
| `src/deephaven_cli/repl/prompt/keybindings.py` | CREATE |
| `src/deephaven_cli/repl/prompt/session.py` | CREATE |
| `src/deephaven_cli/repl/console.py` | MODIFY |
| `src/deephaven_cli/cli.py` | MODIFY - add --vi flag |
| `tests/test_repl_prompt.py` | CREATE - unit tests |
| `tests/test_repl_integration.py` | CREATE - tmux integration tests |

## Verification (Automated)

All verification is automated via tests:

```bash
# Run all REPL tests
uv run pytest tests/test_repl_prompt.py tests/test_repl_integration.py -v
```

**Unit tests** (`test_repl_prompt.py`) verify:
- Toolbar returns correct HTML with status info
- Key bindings are registered
- Session configured with history, lexer, mouse support
- History path is ~/.dh_history

**Integration tests** (`test_repl_integration.py`) verify via tmux:
- REPL starts and shows prompt
- Basic execution works (2 + 2 = 4)
- Multi-line input works
- Ctrl+L clears screen
- Bottom toolbar is visible
- exit() shows "Goodbye!"
- --vi flag accepted

**Prerequisites:**
- tmux must be installed for integration tests
