"""Interactive REPL console for Deephaven."""
from __future__ import annotations

import sys
from typing import TYPE_CHECKING

from deephaven_cli.repl.prompt import create_prompt_session

if TYPE_CHECKING:
    from deephaven_cli.client import DeephavenClient
    from deephaven_cli.repl.executor import CodeExecutor
    from prompt_toolkit import PromptSession


class DeephavenConsole:
    """Interactive console that executes code on Deephaven server."""

    def __init__(
        self, client: DeephavenClient, port: int = 10000, *, vi_mode: bool = False
    ):
        from deephaven_cli.repl.executor import CodeExecutor

        self.client = client
        self.executor = CodeExecutor(client)
        self._session: PromptSession = create_prompt_session(
            client, port, vi_mode=vi_mode
        )

    def interact(self, banner: str | None = None) -> None:
        """Start the interactive REPL loop."""
        if banner:
            print(banner)

        while True:
            try:
                # prompt_toolkit handles multi-line, history, suggestions
                text = self._session.prompt(">>> ")

                # Handle special commands
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

    def _execute_and_display(self, source: str) -> None:
        """Execute code and display results."""
        result = self.executor.execute(source)

        # Display error if any
        if result.error:
            print(result.error, file=sys.stderr)
            return

        # Display stdout
        if result.stdout:
            print(result.stdout, end="")
            if not result.stdout.endswith("\n"):
                print()

        # Display stderr
        if result.stderr:
            print(result.stderr, file=sys.stderr, end="")
            if not result.stderr.endswith("\n"):
                print(file=sys.stderr)

        # Display expression result
        if result.result_repr is not None and result.result_repr != "None":
            print(result.result_repr)

        # Display new tables
        for table_name in result.new_tables:
            print(f"\nTable '{table_name}':")
            try:
                preview = self.executor.get_table_preview(table_name)
                print(preview)
            except Exception as e:
                print(f"  (could not preview: {e})")
