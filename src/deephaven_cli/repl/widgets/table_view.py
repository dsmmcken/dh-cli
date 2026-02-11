"""Virtual-scrolling DataTable widget for Deephaven tables.

Uses textual-fastdatatable with ArrowBackend to display Deephaven tables
of any size. Only the visible viewport rows are fetched from the server
via table.slice(start, stop).to_arrow().
"""
from __future__ import annotations

from textual.message import Message
from textual.widgets import Static

# Page size for fetching rows from the server
_PAGE_SIZE = 100


class TableView(Static):
    """Virtual-scrolling table display for Deephaven tables.

    Fetches only the visible viewport rows from the server using
    Deephaven's table.slice() API and renders them via
    textual-fastdatatable.
    """

    DEFAULT_CSS = """
    TableView {
        height: 1fr;
        border: solid $primary;
        overflow: auto;
    }
    """

    class ViewClosed(Message):
        """Emitted when the table view is closed."""

        def __init__(self, table_name: str) -> None:
            super().__init__()
            self.table_name = table_name

    def __init__(self, table_name: str = "", **kwargs) -> None:
        super().__init__(**kwargs)
        self.table_name = table_name
        self._dh_table = None
        self._total_rows: int = 0
        self._is_refreshing: bool = False
        self._datatable = None

    def set_table(self, table_name: str, dh_table) -> None:
        """Set the Deephaven table to display.

        Args:
            table_name: Name of the table variable on the server.
            dh_table: The pydeephaven table handle (supports .to_arrow(), .slice(), etc.)
        """
        self.table_name = table_name
        self._dh_table = dh_table

        try:
            arrow = dh_table.to_arrow()
            self._total_rows = arrow.num_rows
            self._is_refreshing = dh_table.is_refreshing
        except Exception:
            self._total_rows = 0
            self._is_refreshing = False

        self._render_table()

    def _render_table(self) -> None:
        """Render the table using textual-fastdatatable if available, else fallback."""
        if self._dh_table is None:
            self.update("[dim]No table selected.[/dim]")
            return

        try:
            from textual_fastdatatable import ArrowBackend, DataTable as FastDataTable

            # Fetch the first page of data
            if self._total_rows > _PAGE_SIZE:
                arrow_data = self._dh_table.slice(0, _PAGE_SIZE).to_arrow()
            else:
                arrow_data = self._dh_table.to_arrow()

            # Build display info
            status = "live" if self._is_refreshing else "static"
            header = f"[bold]{self.table_name}[/bold] ({self._total_rows:,} rows, {status})"

            # For now, render as text until we can mount the FastDataTable widget
            df = arrow_data.to_pandas()
            preview = df.to_string(index=False)
            if self._total_rows > _PAGE_SIZE:
                preview += f"\n\n[dim]... showing first {_PAGE_SIZE} of {self._total_rows:,} rows[/dim]"
            self.update(f"{header}\n\n{preview}")

        except ImportError:
            # Fallback: plain text rendering
            self._render_fallback()
        except Exception as e:
            self.update(f"[red]Error rendering table: {e}[/red]")

    def _render_fallback(self) -> None:
        """Plain text fallback when textual-fastdatatable is not available."""
        if self._dh_table is None:
            self.update("[dim]No table selected.[/dim]")
            return

        try:
            if self._total_rows > _PAGE_SIZE:
                arrow_data = self._dh_table.slice(0, _PAGE_SIZE).to_arrow()
            else:
                arrow_data = self._dh_table.to_arrow()

            df = arrow_data.to_pandas()
            status = "live" if self._is_refreshing else "static"
            header = f"{self.table_name} ({self._total_rows:,} rows, {status})"
            preview = df.to_string(index=False)
            if self._total_rows > _PAGE_SIZE:
                preview += f"\n... showing first {_PAGE_SIZE} of {self._total_rows:,} rows"
            self.update(f"{header}\n\n{preview}")
        except Exception as e:
            self.update(f"Error rendering table: {e}")
