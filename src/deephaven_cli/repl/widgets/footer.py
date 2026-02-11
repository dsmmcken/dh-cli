"""Custom footer widget for the Deephaven REPL."""
from __future__ import annotations

from textual.widgets import Static


class REPLFooter(Static):
    """Status bar showing connection info, version, and table counts."""

    DEFAULT_CSS = """
    REPLFooter {
        height: 1;
        background: $accent;
        color: $text;
        padding: 0 1;
    }
    """

    def __init__(self, host: str = "localhost", port: int = 10000, **kwargs) -> None:
        super().__init__(**kwargs)
        self._host = host
        self._port = port
        self._version: str = ""
        self._table_count: int = 0
        self._row_count: int | None = None
        self._is_remote: bool = host != "localhost"

    def on_mount(self) -> None:
        self._refresh_display()

    def set_version(self, version: str) -> None:
        self._version = version
        self._refresh_display()

    def set_table_count(self, count: int) -> None:
        self._table_count = count
        self._refresh_display()

    def set_row_count(self, count: int | None) -> None:
        self._row_count = count
        self._refresh_display()

    def _refresh_display(self) -> None:
        mode = "[remote]" if self._is_remote else "[local]"
        parts = [
            f"{self._host}:{self._port}",
        ]
        if self._version:
            parts.append(f"v{self._version}")
        parts.append(f"{self._table_count} tables")
        if self._row_count is not None:
            parts.append(f"{self._row_count:,} rows")
        parts.append(mode)
        self.update("  |  ".join(parts))
