"""Tests for Phase 6: CLI entry point.

Most CLI tests can be unit tests using argument parsing.
"""
import pytest
import subprocess
import sys


class TestCLIArgumentParsing:
    """Unit tests for CLI argument parsing."""

    def test_cli_no_args_shows_help(self):
        """CLI with no args shows help and exits successfully."""
        result = subprocess.run(
            [sys.executable, "-m", "deephaven_cli.cli"],
            capture_output=True,
            text=True,
        )
        assert result.returncode == 0
        assert "repl" in result.stdout
        assert "exec" in result.stdout

    def test_cli_help(self):
        """CLI --help shows usage information."""
        result = subprocess.run(
            [sys.executable, "-m", "deephaven_cli.cli", "--help"],
            capture_output=True,
            text=True,
        )
        assert result.returncode == 0
        assert "repl" in result.stdout
        assert "exec" in result.stdout

    def test_cli_repl_help(self):
        """CLI repl --help shows options."""
        result = subprocess.run(
            [sys.executable, "-m", "deephaven_cli.cli", "repl", "--help"],
            capture_output=True,
            text=True,
        )
        assert result.returncode == 0
        assert "--port" in result.stdout
        assert "--jvm-args" in result.stdout

    def test_cli_exec_help(self):
        """CLI exec --help shows options."""
        result = subprocess.run(
            [sys.executable, "-m", "deephaven_cli.cli", "exec", "--help"],
            capture_output=True,
            text=True,
        )
        assert result.returncode == 0
        assert "--port" in result.stdout
        assert "--verbose" in result.stdout
        assert "--timeout" in result.stdout

    def test_cli_config_help(self):
        """CLI config --help shows options."""
        result = subprocess.run(
            [sys.executable, "-m", "deephaven_cli.cli", "config", "--help"],
            capture_output=True,
            text=True,
        )
        assert result.returncode == 0
        assert "config" in result.stdout.lower()

    def test_cli_unknown_command(self):
        """CLI unknown command shows error."""
        result = subprocess.run(
            [sys.executable, "-m", "deephaven_cli.cli", "unknown_command"],
            capture_output=True,
            text=True,
        )
        assert result.returncode != 0


class TestCLIExitCodes:
    """Test CLI exit codes (unit tests where possible)."""

    def test_exit_codes_defined(self):
        """Verify exit codes are defined in cli module."""
        from deephaven_cli.cli import (
            EXIT_SUCCESS,
            EXIT_SCRIPT_ERROR,
            EXIT_CONNECTION_ERROR,
            EXIT_TIMEOUT,
            EXIT_INTERRUPTED,
        )
        assert EXIT_SUCCESS == 0
        assert EXIT_SCRIPT_ERROR == 1
        assert EXIT_CONNECTION_ERROR == 2
        assert EXIT_TIMEOUT == 3
        assert EXIT_INTERRUPTED == 130


class TestCLIMainFunction:
    """Test CLI main function argument handling."""

    def test_main_function_exists(self):
        """Verify main function exists and is callable."""
        from deephaven_cli.cli import main
        assert callable(main)

    def test_run_repl_function_exists(self):
        """Verify run_repl function exists."""
        from deephaven_cli.cli import run_repl
        assert callable(run_repl)

    def test_run_exec_function_exists(self):
        """Verify run_exec function exists (placeholder for Phase 7)."""
        from deephaven_cli.cli import run_exec
        assert callable(run_exec)

    def test_run_management_tui_function_exists(self):
        """Verify run_management_tui function exists."""
        from deephaven_cli.cli import run_management_tui
        assert callable(run_management_tui)


@pytest.mark.integration
class TestCLIIntegration:
    """Integration tests for CLI (require Java/server).

    These tests require a working Deephaven environment with Java.
    They are skipped by default - run with: pytest -m integration
    """

    def test_cli_repl_starts_server(self):
        """Test that repl command starts server (interactive - manual test only).

        This is marked as integration because it requires Java and interactive input.
        Manual verification: run `dh repl` and check server starts.
        """
        pytest.skip("Interactive test - run manually with: dh repl")
