"""Command input bar widget for the Deephaven REPL.

Provides a TextArea with Python syntax highlighting, command history
(Up/Down arrows), and multi-line support (Shift+Enter).
"""
from __future__ import annotations

from pathlib import Path

from textual.message import Message
from textual.widgets import TextArea

# History file location
_HISTORY_FILE = Path.home() / ".dh" / "history"
_MAX_HISTORY = 500


class InputBar(TextArea):
    """Command input with syntax highlighting, history, and multi-line support."""

    DEFAULT_CSS = """
    InputBar {
        height: auto;
        min-height: 1;
        max-height: 8;
        border: solid $accent;
    }
    """

    class CommandSubmitted(Message):
        """Emitted when the user submits a command (Enter key)."""

        def __init__(self, code: str) -> None:
            super().__init__()
            self.code = code

    def __init__(self, **kwargs) -> None:
        super().__init__(
            language="python",
            theme="monokai",
            soft_wrap=True,
            **kwargs,
        )
        self._history: list[str] = []
        self._history_index: int = -1
        self._draft: str = ""
        self._completions: list[str] = []
        self._completion_matches: list[str] = []
        self._completion_index: int = -1
        self._completion_prefix: str = ""
        self._load_history()

    def _load_history(self) -> None:
        """Load command history from disk."""
        if _HISTORY_FILE.exists():
            try:
                lines = _HISTORY_FILE.read_text().splitlines()
                self._history = lines[-_MAX_HISTORY:]
            except Exception:
                self._history = []

    def _save_history(self) -> None:
        """Persist command history to disk."""
        try:
            _HISTORY_FILE.parent.mkdir(parents=True, exist_ok=True)
            _HISTORY_FILE.write_text("\n".join(self._history[-_MAX_HISTORY:]) + "\n")
        except Exception:
            pass

    def _clear_input(self) -> None:
        """Clear the input bar (called via call_later to ensure DOM is updated)."""
        self.load_text("")
        self.call_later(self.focus)

    def _set_text(self, text: str) -> None:
        """Replace all text in the TextArea and move cursor to end."""
        self.load_text(text)
        end = self.document.end
        self.move_cursor(end)
        self.call_later(self.focus)

    def set_completions(self, names: list[str]) -> None:
        """Update the list of available completion names (variable names, etc.)."""
        self._completions = sorted(names)

    def _add_to_history(self, code: str) -> None:
        """Add a command to history (deduplicating consecutive duplicates)."""
        if code and (not self._history or self._history[-1] != code):
            self._history.append(code)
            self._save_history()

    async def _on_key(self, event) -> None:
        """Handle key events for history and submission."""
        if event.key != "tab":
            self._completion_index = -1
            self._completion_prefix = ""

        if event.key == "enter":
            # Submit on Enter (Shift+Enter comes as "shift+enter", not "enter")
            event.prevent_default()
            event.stop()
            code = self.text.strip()
            if code:
                self._add_to_history(code)
                self._history_index = -1
                self._draft = ""
                # Clear text and re-focus (call_later since load_text shifts focus async)
                self.load_text("")
                self.call_later(self.focus)
                self.post_message(self.CommandSubmitted(code))
            return

        if event.key == "up":
            # Navigate history backwards
            if self._history:
                event.prevent_default()
                if self._history_index == -1:
                    self._draft = self.text
                    self._history_index = len(self._history) - 1
                elif self._history_index > 0:
                    self._history_index -= 1
                self._set_text(self._history[self._history_index])
            return

        if event.key == "down":
            # Navigate history forwards
            if self._history_index >= 0:
                event.prevent_default()
                if self._history_index < len(self._history) - 1:
                    self._history_index += 1
                    self._set_text(self._history[self._history_index])
                else:
                    self._history_index = -1
                    self._set_text(self._draft)
            return

        if event.key == "tab":
            event.prevent_default()
            text = self.text
            # Find the word being typed (last token after space or start)
            # Get text up to cursor
            cursor_row, cursor_col = self.cursor_location
            line = self.document.get_line(cursor_row)
            prefix_text = line[:cursor_col]
            # Extract the last word fragment
            import re
            match = re.search(r'(\w+)$', prefix_text)
            if not match:
                return
            prefix = match.group(1)

            if prefix != self._completion_prefix or self._completion_index == -1:
                # New completion session
                self._completion_prefix = prefix
                self._completion_matches = [
                    name for name in self._completions
                    if name.startswith(prefix) and name != prefix
                ]
                self._completion_index = 0
            else:
                # Cycle to next match
                self._completion_index = (self._completion_index + 1) % max(len(self._completion_matches), 1)

            if self._completion_matches:
                completion = self._completion_matches[self._completion_index]
                # Replace the prefix with the completion
                start_col = cursor_col - len(prefix)
                # Build new line content
                new_line = line[:start_col] + completion + line[cursor_col:]
                # Replace entire text
                lines = self.text.split('\n')
                lines[cursor_row] = new_line
                new_text = '\n'.join(lines)
                new_col = start_col + len(completion)
                self.load_text(new_text)
                self.move_cursor((cursor_row, new_col))
                # Re-focus after load_text completes (it shifts focus async)
                self.call_later(self.focus)
            return

        # Shift+Enter handled natively by TextArea (inserts newline)
