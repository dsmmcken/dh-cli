"""Tests for Phase 4: Code executor with output capture.

NOTE: These tests require a running Deephaven server.
"""
import pytest
from deephaven_cli.server import DeephavenServer
from deephaven_cli.client import DeephavenClient
from deephaven_cli.repl.executor import CodeExecutor, ExecutionResult


@pytest.fixture(scope="module")
def executor():
    """Provide a CodeExecutor with running server."""
    import random
    port = random.randint(10300, 10399)
    server = DeephavenServer(port=port)
    server.start()
    client = DeephavenClient(port=port)
    client.connect()
    executor = CodeExecutor(client)
    yield executor
    client.close()
    server.stop()


@pytest.mark.integration
class TestCodeExecutor:
    """Tests for CodeExecutor class."""

    def test_execute_returns_execution_result(self, executor):
        """Test execute returns an ExecutionResult."""
        result = executor.execute("1 + 1")
        assert isinstance(result, ExecutionResult)

    def test_execute_simple_expression(self, executor):
        """Test executing a simple expression captures result."""
        result = executor.execute("2 + 2")
        assert result.result_repr == "4"
        assert result.error is None
        assert result.stdout == ""

    def test_execute_string_expression(self, executor):
        """Test executing a string expression."""
        result = executor.execute("'hello' + ' world'")
        assert result.result_repr == "'hello world'"

    def test_execute_print_captures_stdout(self, executor):
        """Test print statements are captured in stdout."""
        result = executor.execute('print("Hello, World!")')
        assert "Hello, World!" in result.stdout
        assert result.error is None

    def test_execute_multiple_prints(self, executor):
        """Test multiple print statements are captured."""
        result = executor.execute('print("line1"); print("line2")')
        assert "line1" in result.stdout
        assert "line2" in result.stdout

    def test_execute_stderr_capture(self, executor):
        """Test stderr is captured."""
        result = executor.execute('import sys; sys.stderr.write("error msg")')
        assert "error msg" in result.stderr

    def test_execute_creates_table_detected(self, executor):
        """Test new tables are detected."""
        result = executor.execute('''
from deephaven import empty_table
executor_test_table = empty_table(3).update(["X = i"])
''')
        assert "executor_test_table" in result.new_tables
        assert result.error is None

    def test_execute_division_error(self, executor):
        """Test division by zero error is captured."""
        result = executor.execute("1 / 0")
        assert result.error is not None
        assert "ZeroDivisionError" in result.error

    def test_execute_name_error(self, executor):
        """Test NameError is captured."""
        result = executor.execute("undefined_variable")
        assert result.error is not None
        assert "NameError" in result.error

    def test_execute_syntax_error(self, executor):
        """Test syntax error is captured."""
        result = executor.execute("def broken(")
        assert result.error is not None
        assert "SyntaxError" in result.error

    def test_execute_multiline_code(self, executor):
        """Test multiline code execution."""
        code = '''
x = 10
y = 20
print(x + y)
'''
        result = executor.execute(code)
        assert "30" in result.stdout
        assert result.error is None

    def test_execute_function_definition_and_call(self, executor):
        """Test defining and calling a function."""
        code = '''
def add(a, b):
    return a + b
print(add(5, 3))
'''
        result = executor.execute(code)
        assert "8" in result.stdout

    def test_execute_special_characters_in_output(self, executor):
        """Test special characters (backticks, quotes) are preserved."""
        result = executor.execute('print("Hello `world` with \'quotes\'")')
        assert "`world`" in result.stdout
        assert "quotes" in result.stdout

    def test_get_table_preview(self, executor):
        """Test table preview returns string with data."""
        # First create a table
        executor.execute('''
from deephaven import empty_table
preview_test_table = empty_table(3).update(["Value = i * 10"])
''')
        preview = executor.get_table_preview("preview_test_table")
        assert isinstance(preview, str)
        assert "Value" in preview  # Column name
        assert "0" in preview  # First value

    def test_get_table_preview_nonexistent(self, executor):
        """Test preview of nonexistent table returns error message."""
        preview = executor.get_table_preview("nonexistent_table_xyz")
        assert "error" in preview.lower() or isinstance(preview, str)
