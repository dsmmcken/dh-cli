"""Interactive REPL console for Deephaven."""
from __future__ import annotations

import code
import sys
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from deephaven_cli.client import DeephavenClient
    from deephaven_cli.repl.executor import CodeExecutor


class DeephavenConsole:
    """Interactive console that executes code on Deephaven server."""

    def __init__(self, client: DeephavenClient):
        from deephaven_cli.repl.executor import CodeExecutor

        self.client = client
        self.executor = CodeExecutor(client)
        self._buffer: list[str] = []

    def interact(self, banner: str | None = None) -> None:
        """Start the interactive REPL loop."""
        if banner:
            print(banner)

        while True:
            try:
                # Get prompt based on buffer state
                prompt = "... " if self._buffer else ">>> "
                line = input(prompt)

                # Handle special commands
                if not self._buffer and line.strip() in ("exit()", "quit()"):
                    break

                self._buffer.append(line)
                source = "\n".join(self._buffer)

                # Check if we need more input
                if self._needs_more_input(source):
                    continue

                # Execute the complete source
                self._execute_and_display(source)
                self._buffer.clear()

            except EOFError:
                print()
                break
            except KeyboardInterrupt:
                print("\nKeyboardInterrupt")
                self._buffer.clear()

        print("Goodbye!")

    def _needs_more_input(self, source: str) -> bool:
        """Check if the source code is incomplete (needs more lines)."""
        # compile_command returns:
        # - Code object if complete and valid
        # - None if incomplete (needs more input)
        # - Raises exception if invalid syntax
        try:
            result = code.compile_command(source, "<input>", "exec")
            # None means incomplete, needs more input
            return result is None
        except (OverflowError, SyntaxError, ValueError):
            # Syntax error - don't ask for more input, let it fail on execute
            return False

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
