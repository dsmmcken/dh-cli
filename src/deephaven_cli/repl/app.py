"""Main Textual App for the Deephaven REPL."""
from __future__ import annotations

from typing import TYPE_CHECKING

from textual.app import App, ComposeResult
from textual.containers import Horizontal
from textual.widgets import Header

from deephaven_cli.repl.widgets.footer import REPLFooter
from deephaven_cli.repl.widgets.input_bar import InputBar
from deephaven_cli.repl.widgets.log_panel import LogPanel
from deephaven_cli.repl.widgets.output import OutputPanel
from deephaven_cli.repl.widgets.sidebar import Sidebar

if TYPE_CHECKING:
    from deephaven_cli.client import DeephavenClient


class DeephavenREPLApp(App):
    """Textual application for the Deephaven interactive REPL."""

    TITLE = "Deephaven REPL"

    CSS = """
    Screen {
        layout: vertical;
    }

    #main-area {
        height: 1fr;
    }

    #output-panel {
        width: 1fr;
    }

    #sidebar {
        width: 28;
    }

    #log-panel {
        height: 4;
    }

    """

    BINDINGS = [
        ("ctrl+c", "quit", "Quit"),
        ("ctrl+d", "quit", "Quit"),
    ]

    def __init__(
        self,
        client: DeephavenClient,
        port: int = 10000,
        vi_mode: bool = False,
        host: str | None = None,
    ) -> None:
        super().__init__()
        self.client = client
        self.port = port
        self.vi_mode = vi_mode
        self.host = host or "localhost"

        from deephaven_cli.repl.executor import CodeExecutor

        self.executor = CodeExecutor(client)

    def compose(self) -> ComposeResult:
        yield Header(show_clock=True)
        with Horizontal(id="main-area"):
            yield OutputPanel(id="output-panel")
            yield Sidebar(id="sidebar")
        yield LogPanel(id="log-panel")
        yield InputBar(id="repl-input")
        yield REPLFooter(host=self.host, port=self.port, id="repl-footer")

    def on_mount(self) -> None:
        log_panel = self.query_one("#log-panel", LogPanel)
        log_panel.log_info(f"Connected to {self.host}:{self.port}")
        self.query_one("#repl-input", InputBar).focus()

    def on_input_bar_command_submitted(self, event: InputBar.CommandSubmitted) -> None:
        """Handle command submission from the InputBar."""
        code = event.code

        if code in ("exit()", "quit()"):
            self.exit()
            return

        output = self.query_one("#output-panel", OutputPanel)
        output.append_command(code)

        # Run execution in a worker thread to avoid blocking the event loop
        self.run_worker(lambda: self._execute_code(code), thread=True)

    def _execute_code(self, code: str) -> None:
        """Execute code on the server and update the UI (runs in worker thread)."""
        result = self.executor.execute(code)

        # Pre-fetch Arrow data for any assigned tables (still in worker thread)
        table_data = {}
        for table_name in result.assigned_tables:
            arrow_result = self.executor.get_table_arrow(table_name)
            if arrow_result is not None:
                table_data[table_name] = arrow_result

        # Pre-fetch sidebar variables (still in worker thread)
        from deephaven_cli.repl.widgets.sidebar import _fetch_variables

        variables = _fetch_variables(self.client)

        def _update_ui():
            output = self.query_one("#output-panel", OutputPanel)
            sidebar = self.query_one("#sidebar", Sidebar)
            log_panel = self.query_one("#log-panel", LogPanel)

            if result.error:
                output.append_error(result.error)
                log_panel.log_error("Execution error")
            else:
                if result.stdout:
                    output.append_text(result.stdout)
                if result.stderr:
                    output.append_stderr(result.stderr)
                if result.result_repr is not None and result.result_repr != "None":
                    output.append_result(result.result_repr)

                for table_name in result.assigned_tables:
                    if table_name in table_data:
                        arrow_table, meta = table_data[table_name]
                        output.append_table(table_name, arrow_table, meta)
                        if meta is not None:
                            log_panel.log_info(f"Table: {table_name} ({meta.row_count:,} rows)")

            # Update sidebar
            sidebar._variables = variables
            sidebar._render_variables()

            # Update footer table count
            footer = self.query_one("#repl-footer", REPLFooter)
            footer.set_table_count(len([v for v in variables if v.type_name == "Table"]))

            # Update tab completions
            input_bar = self.query_one("#repl-input", InputBar)
            input_bar.set_completions([v.name for v in variables])

        self.call_from_thread(_update_ui)

    def on_sidebar_variable_clicked(self, event: Sidebar.VariableClicked) -> None:
        """Handle variable click from the sidebar."""
        # Run in worker thread since it makes server calls
        name = event.name
        type_name = event.type_name
        self.run_worker(lambda: self._show_variable(name, type_name), thread=True)

    def _show_variable(self, name: str, type_name: str) -> None:
        """Fetch and display a variable (runs in worker thread)."""
        if type_name == "Table":
            arrow_result = self.executor.get_table_arrow(name)

            def _update():
                output = self.query_one("#output-panel", OutputPanel)
                log_panel = self.query_one("#log-panel", LogPanel)
                if arrow_result is not None:
                    arrow_table, meta = arrow_result
                    output.append_table(name, arrow_table, meta)
                    if meta is not None:
                        log_panel.log_info(f"Table: {name} ({meta.row_count:,} rows)")
                else:
                    output.append_error(f"Could not load table: {name}")

            self.call_from_thread(_update)
        else:
            result = self.executor.execute(f"repr({name})")

            def _update():
                output = self.query_one("#output-panel", OutputPanel)
                if result.result_repr:
                    output.append_text(f"{name} = {result.result_repr}")

            self.call_from_thread(_update)
