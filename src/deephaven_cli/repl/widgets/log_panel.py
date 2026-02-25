"""Log panel widget for the Deephaven REPL.

Shows timestamped operational messages: server events, table creation,
query timing, errors. Separate from command output.
"""
from __future__ import annotations

from datetime import datetime

from textual.widgets import RichLog


class LogPanel(RichLog):
    """Scrollable log panel with timestamped entries."""

    DEFAULT_CSS = """
    LogPanel {
        height: 4;
        border: solid $surface;
    }
    """

    def __init__(self, **kwargs) -> None:
        super().__init__(highlight=True, markup=True, wrap=True, auto_scroll=True, **kwargs)

    def on_mount(self) -> None:
        self.log_info("Ready.")

    def log_info(self, message: str) -> None:
        """Write a timestamped info message."""
        ts = datetime.now().strftime("%H:%M:%S")
        self.write(f"[dim]{ts}[/dim] {message}")

    def log_error(self, message: str) -> None:
        """Write a timestamped error message."""
        ts = datetime.now().strftime("%H:%M:%S")
        self.write(f"[dim]{ts}[/dim] [red]{message}[/red]")

    def log_success(self, message: str) -> None:
        """Write a timestamped success message."""
        ts = datetime.now().strftime("%H:%M:%S")
        self.write(f"[dim]{ts}[/dim] [green]{message}[/green]")
