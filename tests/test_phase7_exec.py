"""Tests for Phase 7: Agent-friendly batch execution mode.

Tests focus on the exec subcommand behavior.
"""
import pytest
import subprocess
import sys
import tempfile
import os


@pytest.mark.integration
class TestExecMode:
    """Integration tests for dh exec command."""

    def test_exec_stdin_simple(self):
        """Test exec reads from stdin with '-'."""
        result = subprocess.run(
            ["dh", "exec", "-"],
            input="print('from stdin')",
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 0
        assert "from stdin" in result.stdout

    def test_exec_file(self):
        """Test exec reads from file."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.py', delete=False) as f:
            f.write('print("from file")\n')
            f.flush()
            try:
                result = subprocess.run(
                    ["dh", "exec", f.name],
                    capture_output=True,
                    text=True,
                    timeout=120,
                )
                assert result.returncode == 0
                assert "from file" in result.stdout
            finally:
                os.unlink(f.name)

    def test_exec_default_is_quiet(self):
        """Test default is quiet (no startup messages)."""
        result = subprocess.run(
            ["dh", "exec", "-"],
            input="print('only this')",
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 0
        # Should NOT contain startup messages
        assert "Starting" not in result.stderr
        assert "Connecting" not in result.stderr
        assert "only this" in result.stdout

    def test_exec_verbose_shows_startup(self):
        """Test --verbose shows startup messages."""
        result = subprocess.run(
            ["dh", "exec", "-", "--verbose"],
            input="print('test')",
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 0
        # Should contain startup messages in stderr
        assert "Starting" in result.stderr or "Server" in result.stderr

    def test_exec_stdout_stderr_separation(self):
        """Test stdout and stderr are correctly separated."""
        code = '''
import sys
print("to stdout")
sys.stderr.write("to stderr\\n")
'''
        result = subprocess.run(
            ["dh", "exec", "-"],
            input=code,
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 0
        assert "to stdout" in result.stdout
        assert "to stderr" in result.stderr
        # Verify they're not mixed
        assert "to stderr" not in result.stdout
        assert "to stdout" not in result.stderr

    def test_exec_expression_result(self):
        """Test expression result is printed."""
        result = subprocess.run(
            ["dh", "exec", "-"],
            input="42 * 2",
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 0
        assert "84" in result.stdout

    def test_exec_error_exit_code_1(self):
        """Test script error returns exit code 1."""
        result = subprocess.run(
            ["dh", "exec", "-"],
            input="raise ValueError('test error')",
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 1
        assert "ValueError" in result.stderr
        assert "test error" in result.stderr

    def test_exec_file_not_found_exit_code_2(self):
        """Test file not found returns exit code 2."""
        result = subprocess.run(
            ["dh", "exec", "/nonexistent/path/to/script.py"],
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 2

    @pytest.mark.skipif(sys.platform == "win32", reason="Threading timeout not on Windows")
    def test_exec_timeout_exit_code_3(self):
        """Test timeout returns exit code 3."""
        result = subprocess.run(
            ["dh", "exec", "-", "--timeout=5"],
            input="import time; time.sleep(30)",
            capture_output=True,
            text=True,
            timeout=60,
        )
        assert result.returncode == 3
        # Note: stderr message may not be captured due to os._exit in thread
        # The important thing is the correct exit code

    def test_exec_show_tables(self):
        """Test --show-tables displays table preview."""
        code = '''
from deephaven import empty_table
show_tables_test = empty_table(3).update(["X = i"])
'''
        result = subprocess.run(
            ["dh", "exec", "-", "--show-tables"],
            input=code,
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 0
        assert "show_tables_test" in result.stdout
        assert "X" in result.stdout  # Column name

    def test_exec_multiline_script(self):
        """Test multiline script execution."""
        code = '''
def factorial(n):
    if n <= 1:
        return 1
    return n * factorial(n - 1)

print(f"5! = {factorial(5)}")
'''
        result = subprocess.run(
            ["dh", "exec", "-"],
            input=code,
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 0
        assert "5! = 120" in result.stdout

    def test_exec_empty_script_success(self):
        """Test empty script is a successful no-op."""
        result = subprocess.run(
            ["dh", "exec", "-"],
            input="",
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 0
        assert result.stdout == ""

    def test_exec_whitespace_only_script(self):
        """Test whitespace-only script is a successful no-op."""
        result = subprocess.run(
            ["dh", "exec", "-"],
            input="   \n\n   \n",
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 0


@pytest.mark.integration
class TestExecBackticks:
    """Tests for backtick handling in piped input."""

    def test_exec_backticks_from_file(self):
        """Test backticks work correctly from file (no shell interpretation)."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.py', delete=False) as f:
            f.write('from deephaven import empty_table\n')
            f.write('t = empty_table(1).update(["S = `hello`"])\n')
            f.flush()
            try:
                result = subprocess.run(
                    ["dh", "exec", f.name, "--show-tables"],
                    capture_output=True,
                    text=True,
                    timeout=120,
                )
                assert result.returncode == 0
                assert "hello" in result.stdout
            finally:
                os.unlink(f.name)

    def test_exec_backticks_escaped_double_quotes(self):
        """Test escaped backticks in double quotes work."""
        result = subprocess.run(
            ["bash", "-c", r'echo "from deephaven import empty_table; t = empty_table(1).update([\"S = \`hi\`\"])" | dh exec --show-tables -'],
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 0
        assert "hi" in result.stdout

    def test_exec_backticks_ansi_c_quoting(self):
        """Test $'...' ANSI-C quoting preserves backticks."""
        result = subprocess.run(
            ["bash", "-c", r"echo $'from deephaven import empty_table\nt = empty_table(1).update([\"S = `test`\"])' | dh exec --show-tables -"],
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 0
        assert "test" in result.stdout

    def test_exec_empty_backtick_string(self):
        """Test empty string literal with backticks."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.py', delete=False) as f:
            f.write('from deephaven import empty_table\n')
            f.write('t = empty_table(1).update(["S = ``"])\n')
            f.flush()
            try:
                result = subprocess.run(
                    ["dh", "exec", f.name, "--show-tables"],
                    capture_output=True,
                    text=True,
                    timeout=120,
                )
                assert result.returncode == 0
            finally:
                os.unlink(f.name)

    def test_exec_multiple_backtick_pairs(self):
        """Test multiple backtick pairs in same script."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.py', delete=False) as f:
            f.write('from deephaven import empty_table\n')
            f.write('t = empty_table(1).update(["A = `one`", "B = `two`", "C = `three`"])\n')
            f.flush()
            try:
                result = subprocess.run(
                    ["dh", "exec", f.name, "--show-tables"],
                    capture_output=True,
                    text=True,
                    timeout=120,
                )
                assert result.returncode == 0
                assert "one" in result.stdout
                assert "two" in result.stdout
                assert "three" in result.stdout
            finally:
                os.unlink(f.name)

    def test_exec_backticks_in_output(self):
        """Test output containing backticks is preserved."""
        result = subprocess.run(
            ["dh", "exec", "-"],
            input="print('has `backticks` inside')",
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 0
        assert "`backticks`" in result.stdout

    def test_exec_backticks_with_special_chars(self):
        """Test backticks with other special characters."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.py', delete=False) as f:
            f.write('from deephaven import empty_table\n')
            f.write('t = empty_table(1).update(["S = `hello world`"])\n')
            f.write('print("Testing single and double quotes")\n')
            f.flush()
            try:
                result = subprocess.run(
                    ["dh", "exec", f.name],
                    capture_output=True,
                    text=True,
                    timeout=120,
                )
                assert result.returncode == 0
                assert "single" in result.stdout
                assert "double" in result.stdout
            finally:
                os.unlink(f.name)
