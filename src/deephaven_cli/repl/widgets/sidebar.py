"""Sidebar widget for the Deephaven REPL.

Shows all global variables in the server session, with type information.
Clicking a variable emits a message to display it in the output panel.
"""
from __future__ import annotations

import json
from typing import TYPE_CHECKING

from textual.containers import Vertical
from textual.message import Message
from textual.widgets import Static

if TYPE_CHECKING:
    from deephaven_cli.client import DeephavenClient


class VariableInfo:
    """Holds information about a single server-side variable."""

    __slots__ = ("name", "type_name")

    def __init__(self, name: str, type_name: str) -> None:
        self.name = name
        self.type_name = type_name

    def display_type(self) -> str:
        """Short display string for the type."""
        short = {
            "Table": "Table",
            "DataFrame": "DataFrame",
            "int": "int",
            "float": "float",
            "str": "str",
            "list": "list",
            "dict": "dict",
            "bool": "bool",
        }
        return short.get(self.type_name, self.type_name)


class Sidebar(Vertical):
    """Displays session variables from the Deephaven server."""

    DEFAULT_CSS = """
    Sidebar {
        width: 28;
        border: solid $secondary;
        overflow-y: auto;
        padding: 0 1;
    }
    """

    class VariableClicked(Message):
        """Emitted when a variable is clicked in the sidebar."""

        def __init__(self, name: str, type_name: str) -> None:
            super().__init__()
            self.name = name
            self.type_name = type_name

    def __init__(self, **kwargs) -> None:
        super().__init__(**kwargs)
        self._variables: list[VariableInfo] = []
        self._refresh_counter: int = 0

    def compose(self):
        yield Static("[bold]Variables[/bold]", id="sidebar-title")
        yield Static("[dim]No variables yet.[/dim]", id="sidebar-vars")

    def on_mount(self) -> None:
        pass

    def refresh_variables(self, client: DeephavenClient) -> None:
        """Query the server for current global variables and update display."""
        variables = _fetch_variables(client)
        self._variables = variables
        self._render_variables()

    def _render_variables(self) -> None:
        """Render the current variable list as a Static widget (avoids ListView ID conflicts)."""
        content = self.query_one("#sidebar-vars", Static)
        if not self._variables:
            content.update("[dim]No variables yet.[/dim]")
            return

        lines = []
        for var in self._variables:
            type_str = var.display_type()
            lines.append(f"  [@click=var_click('{var.name}')]{var.name:<14}[/] [dim]{type_str}[/dim]")
        content.update("\n".join(lines))

    def on_click(self, event) -> None:
        """Handle clicks — check if a variable name was clicked."""
        pass

    def action_var_click(self, name: str) -> None:
        """Action handler for clicking a variable name."""
        for v in self._variables:
            if v.name == name:
                self.post_message(self.VariableClicked(name, v.type_name))
                break


def _fetch_variables(client: DeephavenClient) -> list[VariableInfo]:
    """Fetch the list of user-defined variables from the Deephaven server.

    Runs a small script server-side that collects global variable names and
    types, encodes them as JSON, and stores the result in a temporary table
    that we read back.
    """
    fetch_script = """
import json as __dh_json
import base64 as __dh_base64
import pickle as __dh_pickle
__dh_var_info = {k: type(v).__name__ for k, v in globals().items() if not k.startswith('_')}
__dh_var_payload = __dh_base64.b64encode(__dh_pickle.dumps(__dh_var_info)).decode("ascii")
from deephaven import empty_table
__dh_var_table = empty_table(1).update([f"data = `{__dh_var_payload}`"])
"""
    cleanup_script = """
try:
    del __dh_var_table, __dh_json, __dh_base64, __dh_pickle, __dh_var_info, __dh_var_payload
except NameError:
    pass
"""

    try:
        client.run_script(fetch_script)

        import base64
        import pickle

        session = client.session
        table = session.open_table("__dh_var_table")
        arrow_table = table.to_arrow()
        df = arrow_table.to_pandas()
        if len(df) > 0:
            encoded = df.iloc[0]["data"]
            var_dict = pickle.loads(base64.b64decode(encoded.encode("ascii")))
        else:
            var_dict = {}

        client.run_script(cleanup_script)

        return [VariableInfo(name, type_name) for name, type_name in sorted(var_dict.items())]

    except Exception:
        return []
