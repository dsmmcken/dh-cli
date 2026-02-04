"""Bottom toolbar for Deephaven REPL."""
from __future__ import annotations

from typing import TYPE_CHECKING, Callable

from prompt_toolkit.formatted_text import HTML

if TYPE_CHECKING:
    from deephaven_cli.client import DeephavenClient


def create_toolbar(client: DeephavenClient, port: int) -> Callable[[], HTML]:
    """Create bottom toolbar showing rich status info."""

    def get_toolbar() -> HTML:
        try:
            table_count = len(client.tables)
            # Get process memory (RSS) in MB
            import resource

            mem_mb = resource.getrusage(resource.RUSAGE_SELF).ru_maxrss / 1024
            return HTML(
                f"<b>Connected</b> | Port: {port} | Tables: {table_count} | "
                f"Mem: {mem_mb:.0f}MB | <i>Ctrl+R: search</i>"
            )
        except Exception:
            return HTML("<ansired><b>Disconnected</b></ansired>")

    return get_toolbar
