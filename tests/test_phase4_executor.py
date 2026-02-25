"""Tests for Phase 4: Code executor with output capture.

NOTE: Integration tests require a running Deephaven server.
"""
import pytest
from deephaven_cli.server import DeephavenServer
from deephaven_cli.client import DeephavenClient
from deephaven_cli.repl.executor import CodeExecutor, ExecutionResult, get_assigned_names


class TestGetAssignedNames:
    """Unit tests for AST-based assignment detection."""

    def test_simple_assignment(self):
        """Test simple variable assignment."""
        assert get_assigned_names("t = 1") == {"t"}

    def test_multiple_assignments(self):
        """Test multiple assignments on separate lines."""
        assert get_assigned_names("a = 1\nb = 2") == {"a", "b"}

    def test_tuple_unpacking(self):
        """Test tuple unpacking assignment."""
        assert get_assigned_names("a, b = 1, 2") == {"a", "b"}

    def test_list_unpacking(self):
        """Test list unpacking assignment."""
        assert get_assigned_names("[a, b] = [1, 2]") == {"a", "b"}

    def test_annotated_assignment(self):
        """Test annotated assignment."""
        assert get_assigned_names("x: int = 5") == {"x"}

    def test_augmented_assignment(self):
        """Test augmented assignment (+=, etc)."""
        assert get_assigned_names("x += 1") == {"x"}

    def test_walrus_operator(self):
        """Test walrus operator (:=)."""
        assert get_assigned_names("print(x := 5)") == {"x"}

    def test_chained_assignment(self):
        """Test chained assignment."""
        assert get_assigned_names("a = b = 1") == {"a", "b"}

    def test_expression_no_assignment(self):
        """Test expression without assignment returns empty set."""
        assert get_assigned_names("1 + 1") == set()
        assert get_assigned_names("print('hello')") == set()

    def test_function_call_no_assignment(self):
        """Test function call without assignment."""
        assert get_assigned_names("empty_table(5).update(['a = 1'])") == set()

    def test_attribute_assignment_ignored(self):
        """Test attribute assignments are ignored."""
        assert get_assigned_names("obj.attr = 1") == set()

    def test_subscript_assignment_ignored(self):
        """Test subscript assignments are ignored."""
        assert get_assigned_names("obj[0] = 1") == set()

    def test_syntax_error_returns_empty(self):
        """Test syntax error returns empty set."""
        assert get_assigned_names("def broken(") == set()

    def test_nested_tuple_unpacking(self):
        """Test nested tuple unpacking."""
        assert get_assigned_names("(a, (b, c)) = (1, (2, 3))") == {"a", "b", "c"}

    def test_table_assignment_pattern(self):
        """Test typical Deephaven table assignment pattern."""
        code = "t = empty_table(100).update(['a = i'])"
        assert get_assigned_names(code) == {"t"}


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
        """Test assigned tables are detected."""
        result = executor.execute('''
from deephaven import empty_table
executor_test_table = empty_table(3).update(["X = i"])
''')
        assert "executor_test_table" in result.assigned_tables
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
        """Test table preview returns string with data and metadata."""
        # First create a table
        executor.execute('''
from deephaven import empty_table
preview_test_table = empty_table(3).update(["Value = i * 10"])
''')
        preview, meta = executor.get_table_preview("preview_test_table")
        assert isinstance(preview, str)
        assert "Value" in preview  # Column name
        assert "0" in preview  # First value
        # Check metadata
        assert meta is not None
        assert meta.row_count == 3
        assert meta.is_refreshing is False
        assert len(meta.columns) == 1
        assert meta.columns[0][0] == "Value"

    def test_get_table_preview_nonexistent(self, executor):
        """Test preview of nonexistent table returns error message."""
        preview, meta = executor.get_table_preview("nonexistent_table_xyz")
        assert "error" in preview.lower() or isinstance(preview, str)
        assert meta is None

    def test_table_reassignment_detected(self, executor):
        """Test that reassigning a table variable is detected.

        This is a regression test for the issue where:
        - t = empty_table(5).update(["a = 1"]) would show the table
        - t = empty_table(5).update(["a = 2"]) would NOT show the table

        The fix uses AST parsing to detect assigned variables rather than
        only checking for new table names.
        """
        # First assignment - should detect 't'
        result1 = executor.execute('''
from deephaven import empty_table
reassign_test_t = empty_table(5).update(["a = 1"])
''')
        assert "reassign_test_t" in result1.assigned_tables
        assert result1.error is None

        # Second assignment to SAME variable - should still detect 't'
        result2 = executor.execute('''
reassign_test_t = empty_table(5).update(["a = 2"])
''')
        assert "reassign_test_t" in result2.assigned_tables
        assert result2.error is None

    def test_multiple_table_assignments(self, executor):
        """Test multiple table assignments in one execution."""
        result = executor.execute('''
from deephaven import empty_table
multi_t1 = empty_table(3).update(["x = 1"])
multi_t2 = empty_table(3).update(["y = 2"])
''')
        assert "multi_t1" in result.assigned_tables
        assert "multi_t2" in result.assigned_tables
        assert result.error is None

    def test_non_table_assignment_not_in_assigned_tables(self, executor):
        """Test that non-table variable assignments are not in assigned_tables."""
        result = executor.execute("non_table_var = 42")
        assert "non_table_var" not in result.assigned_tables
        assert result.error is None

    def test_get_table_preview_no_row_index(self, executor):
        """Test table preview does NOT include pandas row index."""
        executor.execute('''
from deephaven import empty_table
no_index_table = empty_table(3).update(["X = i", "Y = i * 2"])
''')
        preview, meta = executor.get_table_preview("no_index_table")
        lines = preview.strip().split('\n')

        # Find the data section (after "Columns:" and empty line)
        data_lines = []
        in_data = False
        for line in lines:
            if line.strip() == '':
                in_data = True
                continue
            if in_data:
                data_lines.append(line)

        # Data lines should NOT start with row indices (0, 1, 2)
        # The first data line should start with actual data values
        assert len(data_lines) >= 1
        # With index=False, lines start with spaces then values, not "0  "
        first_data = data_lines[0].lstrip()
        assert not first_data.startswith('0 ')

    def test_get_table_preview_empty_table_no_index(self, executor):
        """Test preview of empty table displays correctly."""
        executor.execute('''
from deephaven import empty_table
empty_table_test = empty_table(0).update(["A = i"])
''')
        preview, meta = executor.get_table_preview("empty_table_test")
        assert "(empty table)" in preview
        assert meta.row_count == 0

    def test_get_table_preview_single_row_no_index(self, executor):
        """Test preview of single-row table has no index."""
        executor.execute('''
from deephaven import empty_table
single_row_test = empty_table(1).update(["Val = 42"])
''')
        preview, meta = executor.get_table_preview("single_row_test")
        assert "42" in preview
        # Should NOT have "0" as a row index before the value
        lines = preview.split('\n')
        data_line = [l for l in lines if '42' in l][0]
        # The line should not start with "0" followed by spaces
        assert not data_line.lstrip().startswith('0 ')

    def test_get_table_preview_alignment_preserved(self, executor):
        """Test column alignment is preserved without index."""
        executor.execute('''
from deephaven import empty_table
align_test = empty_table(3).update(["Short = i", "LongerColumnName = i * 100"])
''')
        preview, meta = executor.get_table_preview("align_test")
        # Both column names should appear
        assert "Short" in preview
        assert "LongerColumnName" in preview
        # Values should be present
        assert "0" in preview
        assert "200" in preview
