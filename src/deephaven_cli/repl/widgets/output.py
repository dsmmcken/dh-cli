"""Output panel widget for the Deephaven REPL.

Displays command results in a scrollable panel: text output via RichLog,
table data via textual-fastdatatable DataTable widgets, and
syntax-highlighted error tracebacks.
"""
from __future__ import annotations

from typing import TYPE_CHECKING

from textual.app import ComposeResult
from textual.containers import VerticalScroll
from textual.message import Message
from textual.widgets import RichLog, Static

if TYPE_CHECKING:
    import pyarrow as pa

    from deephaven_cli.repl.executor import TableMeta


class OutputPanel(VerticalScroll):
    """Scrollable panel showing interleaved commands, text, and DataTables."""

    DEFAULT_CSS = """
    OutputPanel {
        height: 1fr;
        border: solid $primary;
        padding: 0 1;
    }

    OutputPanel RichLog {
        height: auto;
        max-height: 100%;
    }

    OutputPanel .table-header {
        color: $accent;
        text-style: bold;
        margin-top: 1;
    }

    OutputPanel .table-truncated {
        color: $text-muted;
    }
    """

    _MAX_TABLE_HEIGHT = 22  # rows visible in DataTable widget (20 data + header + border)

    class TableSelected(Message):
        """Emitted when a table name is clicked in the output."""

        def __init__(self, table_name: str) -> None:
            super().__init__()
            self.table_name = table_name

    def __init__(self, **kwargs) -> None:
        super().__init__(**kwargs)
        self._current_log: RichLog | None = None

    def compose(self) -> ComposeResult:
        self._current_log = RichLog(highlight=True, markup=True, wrap=True, auto_scroll=False)
        yield self._current_log

    def on_mount(self) -> None:
        self._current_log.write("[dim]Output will appear here.[/dim]")

    def _ensure_log(self) -> RichLog:
        """Return the current RichLog, creating a new one after a table mount."""
        if self._current_log is None:
            self._current_log = RichLog(highlight=True, markup=True, wrap=True, auto_scroll=False)
            self.mount(self._current_log)
        return self._current_log

    def _scroll_down(self) -> None:
        """Scroll to the bottom of the output."""
        self.call_later(lambda: self.scroll_end(animate=False))

    def append_command(self, code: str) -> None:
        """Append a command prompt line."""
        self._ensure_log().write(f"[bold]>>> {code}[/bold]")
        self._scroll_down()

    def append_text(self, text: str) -> None:
        """Append plain text output (stdout)."""
        if text:
            self._ensure_log().write(text.rstrip("\n"))
            self._scroll_down()

    def append_error(self, error: str) -> None:
        """Append an error traceback."""
        self._ensure_log().write(f"[red]{error.rstrip(chr(10))}[/red]")
        self._scroll_down()

    def append_stderr(self, text: str) -> None:
        """Append stderr output."""
        if text:
            self._ensure_log().write(f"[yellow]{text.rstrip(chr(10))}[/yellow]")
            self._scroll_down()

    def append_result(self, result_repr: str) -> None:
        """Append an expression result."""
        self._ensure_log().write(result_repr)
        self._scroll_down()

    def append_table(self, table_name: str, arrow_data: pa.Table, meta: TableMeta) -> None:
        """Mount a real DataTable widget backed by Arrow data.

        Args:
            table_name: Name of the table variable.
            arrow_data: Arrow table with the data to display.
            meta: Table metadata (total row count, is_refreshing, columns).
        """
        from textual_fastdatatable import ArrowBackend
        from textual_fastdatatable import DataTable as FastDataTable

        # Write table header to current log
        status = "live" if meta.is_refreshing else "static"
        self._ensure_log().write(
            f"[bold cyan]=== {table_name} ({meta.row_count:,} rows, {status}) ===[/bold cyan]"
        )

        # Mount the DataTable widget
        backend = ArrowBackend(arrow_data)
        num_display = min(arrow_data.num_rows, self._MAX_TABLE_HEIGHT - 2)
        dt = FastDataTable(
            backend=backend,
            zebra_stripes=True,
            show_cursor=True,
        )
        dt.styles.height = num_display + 2  # data rows + header + border
        dt.styles.max_height = self._MAX_TABLE_HEIGHT
        self.mount(dt)

        # Truncation note if not all rows shown
        if meta.row_count > arrow_data.num_rows:
            self.mount(
                Static(
                    f"[dim]  Showing {arrow_data.num_rows:,} of {meta.row_count:,} rows[/dim]",
                    markup=True,
                    classes="table-truncated",
                )
            )

        # Start a new log segment for any text that follows
        self._current_log = None
        self._scroll_down()

    def append_table_preview(self, table_name: str, preview: str, meta=None) -> None:
        """Append a text-based table preview (fallback when Arrow data unavailable)."""
        if meta is not None:
            status = "refreshing" if meta.is_refreshing else "static"
            header = f"[bold cyan]=== Table: {table_name} ({meta.row_count:,} rows, {status}) ===[/bold cyan]"
        else:
            header = f"[bold cyan]=== Table: {table_name} ===[/bold cyan]"
        self._ensure_log().write(header)
        self._ensure_log().write(preview)
        self._scroll_down()
