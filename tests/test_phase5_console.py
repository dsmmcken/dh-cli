"""Tests for Phase 5: REPL console.

Some tests can be unit tests (no server needed), others need integration.
"""
import pytest
from unittest.mock import MagicMock, patch
from deephaven_cli.repl.console import DeephavenConsole


class TestNeedsMoreInputUnit:
    """Unit tests for _needs_more_input method (no server needed)."""

    @pytest.fixture
    def console(self):
        """Create console with mocked client."""
        mock_client = MagicMock()
        return DeephavenConsole(mock_client)

    def test_complete_expression(self, console):
        """Complete expression doesn't need more input."""
        assert console._needs_more_input("2 + 2") is False

    def test_complete_print_statement(self, console):
        """Complete print statement doesn't need more input."""
        assert console._needs_more_input('print("hello")') is False

    def test_incomplete_function_def(self, console):
        """Incomplete function definition needs more input."""
        assert console._needs_more_input("def foo():") is True

    def test_incomplete_class_def(self, console):
        """Incomplete class definition needs more input."""
        assert console._needs_more_input("class Foo:") is True

    def test_incomplete_if_statement(self, console):
        """Incomplete if statement needs more input."""
        assert console._needs_more_input("if True:") is True

    def test_incomplete_for_loop(self, console):
        """Incomplete for loop needs more input."""
        assert console._needs_more_input("for i in range(10):") is True

    def test_incomplete_while_loop(self, console):
        """Incomplete while loop needs more input."""
        assert console._needs_more_input("while True:") is True

    def test_complete_multiline_function(self, console):
        """Complete multiline function doesn't need more input."""
        code = '''def foo():
    return 42
'''
        assert console._needs_more_input(code) is False

    def test_multiline_function_without_trailing_newline(self, console):
        """Function without trailing newline is complete (Python considers it valid)."""
        code = '''def foo():
    x = 1'''
        # compile_command considers this complete - the body is present
        assert console._needs_more_input(code) is False

    def test_open_paren_in_def_needs_more(self, console):
        """Open parenthesis in def needs more input (not a syntax error yet)."""
        # Unclosed parenthesis is incomplete, not a syntax error
        assert console._needs_more_input("def broken(") is True

    def test_syntax_error_doesnt_need_more(self, console):
        """Actual syntax errors don't request more input (let them fail)."""
        # Invalid syntax that can't be completed
        assert console._needs_more_input("def 123invalid") is False

    def test_open_parenthesis(self, console):
        """Open parenthesis needs more input."""
        assert console._needs_more_input("print(") is True

    def test_open_bracket(self, console):
        """Open bracket needs more input."""
        assert console._needs_more_input("x = [1, 2,") is True

    def test_triple_quote_string(self, console):
        """Unclosed triple-quote string needs more input."""
        assert console._needs_more_input('x = """hello') is True


class TestSpecialCommands:
    """Unit tests for special command handling (no server needed)."""

    @pytest.fixture
    def console(self):
        """Create console with mocked client."""
        mock_client = MagicMock()
        return DeephavenConsole(mock_client)

    def test_clear_command_clears_screen(self, console):
        """Typing 'clear' calls os.system('clear')."""
        with patch.object(console._session, "prompt", side_effect=["clear", "exit()"]):
            with patch("os.system") as mock_system:
                console.interact()
                mock_system.assert_called_with("clear")

    def test_clear_command_with_whitespace(self, console):
        """'clear' with surrounding whitespace still works."""
        with patch.object(console._session, "prompt", side_effect=["  clear  ", "exit()"]):
            with patch("os.system") as mock_system:
                console.interact()
                mock_system.assert_called_with("clear")


@pytest.mark.integration
class TestConsoleExecuteAndDisplay:
    """Integration tests for console execution (requires server)."""

    @pytest.fixture(scope="class")
    def console(self):
        """Provide console with real server connection."""
        import random
        from deephaven_cli.server import DeephavenServer
        from deephaven_cli.client import DeephavenClient

        port = random.randint(10400, 10499)
        server = DeephavenServer(port=port)
        server.start()
        client = DeephavenClient(port=port)
        client.connect()
        console = DeephavenConsole(client)
        yield console
        client.close()
        server.stop()

    def test_execute_displays_result(self, console, capsys):
        """Test expression result is printed."""
        console._execute_and_display("2 + 2")
        captured = capsys.readouterr()
        assert "4" in captured.out

    def test_execute_displays_print(self, console, capsys):
        """Test print output is displayed."""
        console._execute_and_display('print("test output")')
        captured = capsys.readouterr()
        assert "test output" in captured.out

    def test_execute_displays_error_to_stderr(self, console, capsys):
        """Test errors go to stderr."""
        console._execute_and_display("1/0")
        captured = capsys.readouterr()
        assert "ZeroDivisionError" in captured.err

    def test_execute_displays_new_table(self, console, capsys):
        """Test new tables are shown."""
        console._execute_and_display('''
from deephaven import empty_table
console_display_table = empty_table(2).update(["X = i"])
''')
        captured = capsys.readouterr()
        assert "console_display_table" in captured.out
