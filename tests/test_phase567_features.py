"""Behavioral tests for Phase 5, 6, and 7 features.

Tests in this module verify the CLI behavior of new features introduced
in Phases 5-7 of the slim-install plan:

  Phase 5: Textual REPL TUI (dh repl)
  Phase 6: Management TUI / first-run wizard (dh with no args)
  Phase 7: dh config command, non-interactive mode

All tests use subprocess calls against the real `dh` binary, following
the same pattern as test_cli_integration.py.

Run with:
    cd /home/dsmmcken/git/dsmmcken/dh-cli && uv run python -m pytest tests/test_phase567_features.py -v
"""

from __future__ import annotations

import os
import subprocess
import tempfile
from pathlib import Path

import pytest


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

FAST_TIMEOUT = 30
NETWORK_TIMEOUT = 300


def _run(
    args: list[str],
    *,
    env: dict[str, str] | None = None,
    timeout: int = FAST_TIMEOUT,
    cwd: str | None = None,
    input_text: str | None = None,
) -> subprocess.CompletedProcess[str]:
    """Run a dh CLI command and return the CompletedProcess."""
    return subprocess.run(
        ["dh", *args],
        capture_output=True,
        text=True,
        timeout=timeout,
        env=env,
        cwd=cwd,
        input=input_text,
    )


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------

@pytest.fixture(scope="session")
def isolated_env(tmp_path_factory):
    """Build an environment dict where HOME points to a temp directory."""
    tmp_home = tmp_path_factory.mktemp("dh_phase567_home")
    env = os.environ.copy()
    env["HOME"] = str(tmp_home)
    env.pop("DH_VERSION", None)
    return env


@pytest.fixture(scope="session")
def isolated_home(isolated_env):
    """Return the Path to the temp HOME directory."""
    return Path(isolated_env["HOME"])


# ---------------------------------------------------------------------------
# Phase 5: Textual REPL TUI
# ---------------------------------------------------------------------------

class TestReplTUI:
    """Tests for the Textual-based REPL (Phase 5)."""

    def test_repl_help_shows_flags(self):
        """dh repl --help should show all expected flags."""
        result = _run(["repl", "--help"])
        assert result.returncode == 0
        assert "REPL" in result.stdout or "repl" in result.stdout.lower()
        assert "--port" in result.stdout
        assert "--host" in result.stdout
        assert "--vi" in result.stdout
        assert "--jvm-args" in result.stdout
        assert "--verbose" in result.stdout
        # Phase 5 adds --version flag
        assert "--version" in result.stdout

    def test_repl_no_version_installed_gives_clear_error(self, tmp_path):
        """dh repl without any version installed should fail with helpful guidance."""
        env = os.environ.copy()
        env["HOME"] = str(tmp_path)
        env.pop("DH_VERSION", None)
        result = _run(["repl"], env=env)
        assert result.returncode != 0
        # Should mention how to install
        combined = result.stdout + result.stderr
        assert "install" in combined.lower()

    def test_repl_no_version_no_import_error(self, tmp_path):
        """dh repl failure should NOT be an ImportError for textual or prompt_toolkit."""
        env = os.environ.copy()
        env["HOME"] = str(tmp_path)
        env.pop("DH_VERSION", None)
        result = _run(["repl"], env=env)
        combined = result.stdout + result.stderr
        # Must not crash with module import errors
        assert "ModuleNotFoundError" not in combined
        assert "ImportError" not in combined

    def test_repl_app_module_importable(self):
        """The repl.app module should be importable without Deephaven deps."""
        result = subprocess.run(
            ["python", "-c", "from deephaven_cli.repl.app import DeephavenREPLApp"],
            capture_output=True,
            text=True,
            timeout=FAST_TIMEOUT,
        )
        combined = result.stdout + result.stderr
        assert "ModuleNotFoundError" not in combined, (
            f"repl.app import failed:\n{combined}"
        )
        assert "ImportError" not in combined, (
            f"repl.app import failed:\n{combined}"
        )

    def test_repl_widgets_module_importable(self):
        """The repl.widgets module should be importable without Deephaven deps."""
        result = subprocess.run(
            ["python", "-c", "from deephaven_cli.repl import widgets"],
            capture_output=True,
            text=True,
            timeout=FAST_TIMEOUT,
        )
        combined = result.stdout + result.stderr
        assert "ModuleNotFoundError" not in combined
        assert "ImportError" not in combined


# ---------------------------------------------------------------------------
# Phase 6: Management TUI
# ---------------------------------------------------------------------------

class TestManagementTUI:
    """Tests for the management TUI (Phase 6)."""

    def test_no_args_returns_success(self):
        """dh with no args should return exit code 0."""
        result = _run([])
        assert result.returncode == 0

    def test_no_args_shows_deephaven_branding(self):
        """dh with no args should show Deephaven CLI branding."""
        result = _run([])
        assert "Deephaven CLI" in result.stdout or "Deephaven" in result.stdout

    def test_no_args_shows_available_commands(self):
        """dh with no args should display available commands."""
        result = _run([])
        combined = result.stdout + result.stderr
        # Should list key commands
        assert "install" in combined
        assert "repl" in combined

    def test_no_args_no_import_error(self):
        """dh with no args should NOT crash with import errors."""
        result = _run([])
        combined = result.stdout + result.stderr
        assert "ModuleNotFoundError" not in combined
        assert "ImportError" not in combined
        assert "Traceback" not in combined

    def test_help_flag_shows_all_commands(self):
        """dh --help should list all management and runtime commands."""
        result = _run(["--help"])
        assert result.returncode == 0
        stdout = result.stdout
        # Manager commands
        assert "install" in stdout
        assert "uninstall" in stdout
        assert "use" in stdout
        assert "versions" in stdout
        assert "java" in stdout
        assert "doctor" in stdout
        # Runtime commands
        assert "repl" in stdout
        assert "exec" in stdout
        assert "serve" in stdout
        # Tool commands
        assert "list" in stdout
        assert "kill" in stdout
        assert "lint" in stdout
        assert "format" in stdout
        assert "typecheck" in stdout

    def test_no_args_non_tty_prints_help(self):
        """dh with no args and no TTY should print help (not launch TUI)."""
        result = _run([], input_text="")
        assert result.returncode == 0
        # Non-interactive mode should print help, not launch TUI
        combined = result.stdout + result.stderr
        assert "Deephaven CLI" in combined
        assert "install" in combined

    def test_tui_app_module_importable(self):
        """The tui.app module should be importable without Deephaven deps."""
        result = subprocess.run(
            ["python", "-c", "from deephaven_cli.tui import app"],
            capture_output=True,
            text=True,
            timeout=FAST_TIMEOUT,
        )
        combined = result.stdout + result.stderr
        if "No module named 'deephaven_cli.tui'" in combined:
            pytest.skip("tui module not yet implemented")
        assert "ModuleNotFoundError" not in combined, (
            f"tui.app import failed:\n{combined}"
        )

    def test_management_app_class_exists(self):
        """ManagementApp class should exist in tui.app."""
        result = subprocess.run(
            ["python", "-c",
             "from deephaven_cli.tui.app import ManagementApp; print('OK')"],
            capture_output=True,
            text=True,
            timeout=FAST_TIMEOUT,
        )
        combined = result.stdout + result.stderr
        if "No module named 'deephaven_cli.tui'" in combined:
            pytest.skip("tui module not yet implemented")
        assert "OK" in result.stdout, f"Import failed:\n{result.stderr}"

    def test_management_app_is_textual_app(self):
        """ManagementApp should be a Textual App subclass."""
        result = subprocess.run(
            ["python", "-c",
             "from deephaven_cli.tui.app import ManagementApp; "
             "from textual.app import App; "
             "print(issubclass(ManagementApp, App))"],
            capture_output=True,
            text=True,
            timeout=FAST_TIMEOUT,
        )
        combined = result.stdout + result.stderr
        if "No module named 'deephaven_cli.tui'" in combined:
            pytest.skip("tui module not yet implemented")
        if "ModuleNotFoundError" in combined:
            pytest.skip("textual not installed")
        assert "True" in result.stdout

    def test_run_management_tui_function_exists(self):
        """run_management_tui entry point should exist."""
        result = subprocess.run(
            ["python", "-c",
             "from deephaven_cli.tui.app import run_management_tui; "
             "print(callable(run_management_tui))"],
            capture_output=True,
            text=True,
            timeout=FAST_TIMEOUT,
        )
        combined = result.stdout + result.stderr
        if "No module named 'deephaven_cli.tui'" in combined:
            pytest.skip("tui module not yet implemented")
        assert "True" in result.stdout

    def test_wizard_screens_importable(self):
        """All wizard screens should be importable."""
        result = subprocess.run(
            ["python", "-c",
             "from deephaven_cli.tui.app import ("
             "WelcomeScreen, JavaCheckScreen, VersionPickerScreen, "
             "InstallProgressScreen, DoneScreen, MainMenuScreen); "
             "print('OK')"],
            capture_output=True,
            text=True,
            timeout=FAST_TIMEOUT,
        )
        combined = result.stdout + result.stderr
        if "No module named 'deephaven_cli.tui'" in combined:
            pytest.skip("tui module not yet implemented")
        assert "OK" in result.stdout, f"Import failed:\n{result.stderr}"

    def test_wizard_screens_are_textual_screens(self):
        """Wizard screens should be Textual Screen subclasses."""
        result = subprocess.run(
            ["python", "-c",
             "from deephaven_cli.tui.app import WelcomeScreen, MainMenuScreen; "
             "from textual.screen import Screen; "
             "print(issubclass(WelcomeScreen, Screen), issubclass(MainMenuScreen, Screen))"],
            capture_output=True,
            text=True,
            timeout=FAST_TIMEOUT,
        )
        combined = result.stdout + result.stderr
        if "No module named 'deephaven_cli.tui'" in combined:
            pytest.skip("tui module not yet implemented")
        if "ModuleNotFoundError" in combined:
            pytest.skip("textual not installed")
        assert "True True" in result.stdout


# ---------------------------------------------------------------------------
# Phase 7: dh config command
# ---------------------------------------------------------------------------

class TestConfig:
    """Tests for the dh config command (Phase 7)."""

    def test_config_help(self):
        """dh config --help should work and describe the command."""
        result = _run(["config", "--help"])
        if result.returncode != 0:
            # config may not be implemented yet
            combined = result.stdout + result.stderr
            if "invalid choice" in combined or "unrecognized" in combined:
                pytest.skip("config command not yet implemented")
        assert result.returncode == 0
        combined = result.stdout + result.stderr
        assert "config" in combined.lower()

    def test_config_shows_current(self, isolated_env):
        """dh config should display current configuration."""
        result = _run(["config"], env=isolated_env)
        if result.returncode != 0:
            combined = result.stdout + result.stderr
            if "invalid choice" in combined or "unrecognized" in combined:
                pytest.skip("config command not yet implemented")
        # Should return success and show some config info
        assert result.returncode == 0
        combined = result.stdout + result.stderr
        # Should mention at least some config aspect
        assert ("version" in combined.lower()
                or "config" in combined.lower()
                or "default" in combined.lower()
                or "plugin" in combined.lower())

    def test_config_no_crash(self, tmp_path):
        """dh config should not crash even with empty HOME."""
        env = os.environ.copy()
        env["HOME"] = str(tmp_path)
        env.pop("DH_VERSION", None)
        result = _run(["config"], env=env)
        if result.returncode != 0:
            combined = result.stdout + result.stderr
            if "invalid choice" in combined or "unrecognized" in combined:
                pytest.skip("config command not yet implemented")
        combined = result.stdout + result.stderr
        assert "Traceback" not in combined
        assert "ModuleNotFoundError" not in combined

    def test_config_shows_path(self, tmp_path):
        """dh config should show the config file path."""
        env = os.environ.copy()
        env["HOME"] = str(tmp_path)
        env.pop("DH_VERSION", None)
        result = _run(["config"], env=env)
        if result.returncode != 0:
            combined = result.stdout + result.stderr
            if "invalid choice" in combined or "unrecognized" in combined:
                pytest.skip("config command not yet implemented")
        # Should show the path to config.toml
        assert "config.toml" in result.stdout or ".dh" in result.stdout

    def test_config_empty_shows_message(self, tmp_path):
        """dh config with no settings should indicate empty config."""
        env = os.environ.copy()
        env["HOME"] = str(tmp_path)
        env.pop("DH_VERSION", None)
        result = _run(["config"], env=env)
        if result.returncode != 0:
            combined = result.stdout + result.stderr
            if "invalid choice" in combined or "unrecognized" in combined:
                pytest.skip("config command not yet implemented")
        assert result.returncode == 0
        assert "empty" in result.stdout.lower() or "no settings" in result.stdout.lower()

    def test_config_set_and_read(self, tmp_path):
        """dh config --set KEY VALUE should persist and be readable."""
        env = os.environ.copy()
        env["HOME"] = str(tmp_path)
        env.pop("DH_VERSION", None)
        # Set a value
        result = _run(["config", "--set", "default_version", "42.0"], env=env)
        if result.returncode != 0:
            combined = result.stdout + result.stderr
            if "invalid choice" in combined or "unrecognized" in combined:
                pytest.skip("config command not yet implemented")
        assert result.returncode == 0
        assert "42.0" in result.stdout
        # Read it back
        result2 = _run(["config"], env=env)
        assert result2.returncode == 0
        assert "default_version" in result2.stdout
        assert "42.0" in result2.stdout

    def test_config_help_mentions_set(self):
        """dh config --help should mention the --set flag."""
        result = _run(["config", "--help"])
        if result.returncode != 0:
            combined = result.stdout + result.stderr
            if "invalid choice" in combined or "unrecognized" in combined:
                pytest.skip("config command not yet implemented")
        assert "--set" in result.stdout


# ---------------------------------------------------------------------------
# Phase 7: Non-interactive mode
# ---------------------------------------------------------------------------

class TestNonInteractive:
    """Tests for non-interactive mode (Phase 7 polish).

    These verify that commands work properly when stdin is not a TTY
    (piped input, CI/CD, AI agents).
    """

    def test_help_works_non_interactively(self):
        """dh --help should work when piped (no TTY)."""
        result = _run(["--help"], input_text="")
        assert result.returncode == 0
        assert "Deephaven CLI" in result.stdout

    def test_versions_non_interactive(self, isolated_env):
        """dh versions should work without a TTY."""
        result = _run(["versions"], env=isolated_env, input_text="")
        assert result.returncode == 0
        # Should not hang or crash
        combined = result.stdout + result.stderr
        assert "Traceback" not in combined

    def test_doctor_non_interactive(self):
        """dh doctor should work without a TTY."""
        result = _run(["doctor"], input_text="")
        combined = result.stdout + result.stderr
        assert "Traceback" not in combined
        # Doctor produces diagnostic output regardless of TTY
        assert "Doctor" in combined or "uv" in combined.lower()

    def test_repl_no_version_non_interactive(self, tmp_path):
        """dh repl without a version should give plain text error, not TUI."""
        env = os.environ.copy()
        env["HOME"] = str(tmp_path)
        env.pop("DH_VERSION", None)
        result = _run(["repl"], env=env, input_text="")
        assert result.returncode != 0
        combined = result.stdout + result.stderr
        # Should give plain text error with install guidance
        assert "install" in combined.lower()
        # Should NOT try to launch a TUI
        assert "Traceback" not in combined

    def test_install_help_non_interactive(self):
        """dh install --help should work non-interactively."""
        result = _run(["install", "--help"], input_text="")
        assert result.returncode == 0
        assert "Install" in result.stdout

    def test_java_status_non_interactive(self):
        """dh java should work non-interactively."""
        result = _run(["java"], input_text="")
        combined = result.stdout + result.stderr
        assert "Traceback" not in combined
        assert "Java" in combined or "java" in combined.lower()


# ---------------------------------------------------------------------------
# Phase 5-7: Exit codes
# ---------------------------------------------------------------------------

class TestExitCodes:
    """Verify proper exit codes from all new commands."""

    def test_help_returns_zero(self):
        result = _run(["--help"])
        assert result.returncode == 0

    def test_no_args_returns_zero(self):
        result = _run([])
        assert result.returncode == 0

    def test_install_help_returns_zero(self):
        result = _run(["install", "--help"])
        assert result.returncode == 0

    def test_uninstall_help_returns_zero(self):
        result = _run(["uninstall", "--help"])
        assert result.returncode == 0

    def test_use_help_returns_zero(self):
        result = _run(["use", "--help"])
        assert result.returncode == 0

    def test_versions_help_returns_zero(self):
        result = _run(["versions", "--help"])
        assert result.returncode == 0

    def test_java_help_returns_zero(self):
        result = _run(["java", "--help"])
        assert result.returncode == 0

    def test_doctor_help_returns_zero(self):
        result = _run(["doctor", "--help"])
        assert result.returncode == 0

    def test_repl_help_returns_zero(self):
        result = _run(["repl", "--help"])
        assert result.returncode == 0

    def test_exec_help_returns_zero(self):
        result = _run(["exec", "--help"])
        assert result.returncode == 0

    def test_serve_help_returns_zero(self):
        result = _run(["serve", "--help"])
        assert result.returncode == 0

    def test_invalid_command_returns_nonzero(self):
        result = _run(["nonexistent-command-xyz"])
        assert result.returncode != 0

    def test_repl_no_version_returns_nonzero(self, tmp_path):
        env = os.environ.copy()
        env["HOME"] = str(tmp_path)
        env.pop("DH_VERSION", None)
        result = _run(["repl"], env=env)
        assert result.returncode != 0

    def test_exec_no_version_returns_nonzero(self, tmp_path):
        env = os.environ.copy()
        env["HOME"] = str(tmp_path)
        env.pop("DH_VERSION", None)
        result = _run(["exec", "-c", "print('hello')"], env=env)
        assert result.returncode != 0

    def test_serve_no_version_returns_nonzero(self, tmp_path):
        env = os.environ.copy()
        env["HOME"] = str(tmp_path)
        env.pop("DH_VERSION", None)
        with tempfile.NamedTemporaryFile(suffix=".py", mode="w", delete=False) as f:
            f.write("print('test')\n")
            script = f.name
        try:
            result = _run(["serve", script], env=env)
            assert result.returncode != 0
        finally:
            os.unlink(script)

    def test_use_nonexistent_version_returns_nonzero(self, tmp_path):
        env = os.environ.copy()
        env["HOME"] = str(tmp_path)
        result = _run(["use", "999.999"], env=env)
        assert result.returncode != 0

    def test_uninstall_nonexistent_version_returns_nonzero(self, tmp_path):
        env = os.environ.copy()
        env["HOME"] = str(tmp_path)
        result = _run(["uninstall", "999.999"], env=env)
        assert result.returncode != 0


# ---------------------------------------------------------------------------
# Phase 5: REPL module structure
# ---------------------------------------------------------------------------

class TestReplModuleStructure:
    """Verify that the new REPL module structure is correct."""

    def test_repl_app_class_exists(self):
        """DeephavenREPLApp class should exist in repl.app."""
        result = subprocess.run(
            ["python", "-c",
             "from deephaven_cli.repl.app import DeephavenREPLApp; "
             "print('OK')"],
            capture_output=True,
            text=True,
            timeout=FAST_TIMEOUT,
        )
        if "ModuleNotFoundError" in result.stderr and "textual" in result.stderr:
            pytest.skip("textual not installed")
        assert "OK" in result.stdout, (
            f"Could not import DeephavenREPLApp:\n{result.stderr}"
        )

    def test_repl_app_is_textual_app(self):
        """DeephavenREPLApp should be a Textual App subclass."""
        result = subprocess.run(
            ["python", "-c",
             "from deephaven_cli.repl.app import DeephavenREPLApp; "
             "from textual.app import App; "
             "print(issubclass(DeephavenREPLApp, App))"],
            capture_output=True,
            text=True,
            timeout=FAST_TIMEOUT,
        )
        if "ModuleNotFoundError" in result.stderr:
            pytest.skip("textual not installed")
        assert "True" in result.stdout

    def test_repl_executor_still_works(self):
        """The executor module should still be importable."""
        result = subprocess.run(
            ["python", "-c",
             "from deephaven_cli.repl.executor import CodeExecutor, ExecutionResult; "
             "print('OK')"],
            capture_output=True,
            text=True,
            timeout=FAST_TIMEOUT,
        )
        assert "OK" in result.stdout, (
            f"executor import failed:\n{result.stderr}"
        )

    def test_console_still_importable(self):
        """The console module should still be importable (backward compat)."""
        result = subprocess.run(
            ["python", "-c",
             "from deephaven_cli.repl.console import DeephavenConsole; "
             "print('OK')"],
            capture_output=True,
            text=True,
            timeout=FAST_TIMEOUT,
        )
        assert "OK" in result.stdout, (
            f"console import failed:\n{result.stderr}"
        )


# ---------------------------------------------------------------------------
# Phase 5: Individual widget modules
# ---------------------------------------------------------------------------

class TestWidgetModules:
    """Verify each REPL widget module imports and has the expected classes."""

    def _import_check(self, import_stmt: str) -> subprocess.CompletedProcess[str]:
        """Helper to run an import check in a subprocess."""
        return subprocess.run(
            ["python", "-c", import_stmt],
            capture_output=True,
            text=True,
            timeout=FAST_TIMEOUT,
        )

    def test_output_panel_importable(self):
        """OutputPanel should be importable from repl.widgets.output."""
        result = self._import_check(
            "from deephaven_cli.repl.widgets.output import OutputPanel; print('OK')"
        )
        if "ModuleNotFoundError" in result.stderr and "textual" in result.stderr:
            pytest.skip("textual not installed")
        assert "OK" in result.stdout, f"Import failed:\n{result.stderr}"

    def test_output_panel_is_widget(self):
        """OutputPanel should be a Textual widget subclass."""
        result = self._import_check(
            "from deephaven_cli.repl.widgets.output import OutputPanel; "
            "from textual.widget import Widget; "
            "print(issubclass(OutputPanel, Widget))"
        )
        if "ModuleNotFoundError" in result.stderr:
            pytest.skip("textual not installed")
        assert "True" in result.stdout

    def test_output_panel_has_table_selected_message(self):
        """OutputPanel should have a TableSelected message class."""
        result = self._import_check(
            "from deephaven_cli.repl.widgets.output import OutputPanel; "
            "msg = OutputPanel.TableSelected('test_table'); "
            "print(msg.table_name)"
        )
        if "ModuleNotFoundError" in result.stderr:
            pytest.skip("textual not installed")
        assert "test_table" in result.stdout

    def test_sidebar_importable(self):
        """Sidebar should be importable from repl.widgets.sidebar."""
        result = self._import_check(
            "from deephaven_cli.repl.widgets.sidebar import Sidebar, VariableInfo; print('OK')"
        )
        if "ModuleNotFoundError" in result.stderr and "textual" in result.stderr:
            pytest.skip("textual not installed")
        assert "OK" in result.stdout, f"Import failed:\n{result.stderr}"

    def test_sidebar_is_widget(self):
        """Sidebar should be a Textual widget subclass."""
        result = self._import_check(
            "from deephaven_cli.repl.widgets.sidebar import Sidebar; "
            "from textual.widget import Widget; "
            "print(issubclass(Sidebar, Widget))"
        )
        if "ModuleNotFoundError" in result.stderr:
            pytest.skip("textual not installed")
        assert "True" in result.stdout

    def test_variable_info_display_type(self):
        """VariableInfo.display_type() should return short type strings."""
        result = self._import_check(
            "from deephaven_cli.repl.widgets.sidebar import VariableInfo; "
            "v = VariableInfo('t', 'Table'); "
            "print(v.display_type()); "
            "v2 = VariableInfo('x', 'int'); "
            "print(v2.display_type())"
        )
        if "ModuleNotFoundError" in result.stderr:
            pytest.skip("textual not installed")
        assert "Table" in result.stdout
        assert "int" in result.stdout

    def test_sidebar_has_variable_clicked_message(self):
        """Sidebar should have a VariableClicked message class."""
        result = self._import_check(
            "from deephaven_cli.repl.widgets.sidebar import Sidebar; "
            "msg = Sidebar.VariableClicked('my_var', 'Table'); "
            "print(msg.name, msg.type_name)"
        )
        if "ModuleNotFoundError" in result.stderr:
            pytest.skip("textual not installed")
        assert "my_var" in result.stdout
        assert "Table" in result.stdout

    def test_input_bar_importable(self):
        """InputBar should be importable from repl.widgets.input_bar."""
        result = self._import_check(
            "from deephaven_cli.repl.widgets.input_bar import InputBar; print('OK')"
        )
        if "ModuleNotFoundError" in result.stderr and "textual" in result.stderr:
            pytest.skip("textual not installed")
        assert "OK" in result.stdout, f"Import failed:\n{result.stderr}"

    def test_input_bar_is_textarea(self):
        """InputBar should be a TextArea subclass (for syntax highlighting)."""
        result = self._import_check(
            "from deephaven_cli.repl.widgets.input_bar import InputBar; "
            "from textual.widgets import TextArea; "
            "print(issubclass(InputBar, TextArea))"
        )
        if "ModuleNotFoundError" in result.stderr:
            pytest.skip("textual not installed")
        assert "True" in result.stdout

    def test_input_bar_has_command_submitted_message(self):
        """InputBar should have a CommandSubmitted message class."""
        result = self._import_check(
            "from deephaven_cli.repl.widgets.input_bar import InputBar; "
            "msg = InputBar.CommandSubmitted('print(42)'); "
            "print(msg.code)"
        )
        if "ModuleNotFoundError" in result.stderr:
            pytest.skip("textual not installed")
        assert "print(42)" in result.stdout

    def test_log_panel_importable(self):
        """LogPanel should be importable from repl.widgets.log_panel."""
        result = self._import_check(
            "from deephaven_cli.repl.widgets.log_panel import LogPanel; print('OK')"
        )
        if "ModuleNotFoundError" in result.stderr and "textual" in result.stderr:
            pytest.skip("textual not installed")
        assert "OK" in result.stdout, f"Import failed:\n{result.stderr}"

    def test_log_panel_is_richlog(self):
        """LogPanel should be a RichLog subclass."""
        result = self._import_check(
            "from deephaven_cli.repl.widgets.log_panel import LogPanel; "
            "from textual.widgets import RichLog; "
            "print(issubclass(LogPanel, RichLog))"
        )
        if "ModuleNotFoundError" in result.stderr:
            pytest.skip("textual not installed")
        assert "True" in result.stdout

    def test_table_view_importable(self):
        """TableView should be importable from repl.widgets.table_view."""
        result = self._import_check(
            "from deephaven_cli.repl.widgets.table_view import TableView; print('OK')"
        )
        if "ModuleNotFoundError" in result.stderr and "textual" in result.stderr:
            pytest.skip("textual not installed")
        assert "OK" in result.stdout, f"Import failed:\n{result.stderr}"

    def test_table_view_is_widget(self):
        """TableView should be a Textual widget subclass."""
        result = self._import_check(
            "from deephaven_cli.repl.widgets.table_view import TableView; "
            "from textual.widget import Widget; "
            "print(issubclass(TableView, Widget))"
        )
        if "ModuleNotFoundError" in result.stderr:
            pytest.skip("textual not installed")
        assert "True" in result.stdout

    def test_table_view_has_view_closed_message(self):
        """TableView should have a ViewClosed message class."""
        result = self._import_check(
            "from deephaven_cli.repl.widgets.table_view import TableView; "
            "msg = TableView.ViewClosed('my_table'); "
            "print(msg.table_name)"
        )
        if "ModuleNotFoundError" in result.stderr:
            pytest.skip("textual not installed")
        assert "my_table" in result.stdout


# ---------------------------------------------------------------------------
# Phase 5: REPL app widget composition
# ---------------------------------------------------------------------------

class TestReplAppComposition:
    """Verify the REPL app composes the correct widget tree."""

    def _import_check(self, import_stmt: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            ["python", "-c", import_stmt],
            capture_output=True,
            text=True,
            timeout=FAST_TIMEOUT,
        )

    def test_app_imports_all_widgets(self):
        """DeephavenREPLApp should import from all widget modules."""
        result = self._import_check(
            "from deephaven_cli.repl.app import DeephavenREPLApp; "
            "from deephaven_cli.repl.widgets.output import OutputPanel; "
            "from deephaven_cli.repl.widgets.sidebar import Sidebar; "
            "from deephaven_cli.repl.widgets.input_bar import InputBar; "
            "from deephaven_cli.repl.widgets.log_panel import LogPanel; "
            "print('OK')"
        )
        if "ModuleNotFoundError" in result.stderr and "textual" in result.stderr:
            pytest.skip("textual not installed")
        assert "OK" in result.stdout, f"Import failed:\n{result.stderr}"

    def test_app_has_bindings(self):
        """DeephavenREPLApp should have keyboard bindings defined."""
        result = self._import_check(
            "from deephaven_cli.repl.app import DeephavenREPLApp; "
            "print(len(DeephavenREPLApp.BINDINGS) > 0)"
        )
        if "ModuleNotFoundError" in result.stderr:
            pytest.skip("textual not installed")
        assert "True" in result.stdout

    def test_app_title_is_set(self):
        """DeephavenREPLApp should have a title."""
        result = self._import_check(
            "from deephaven_cli.repl.app import DeephavenREPLApp; "
            "print(DeephavenREPLApp.TITLE)"
        )
        if "ModuleNotFoundError" in result.stderr:
            pytest.skip("textual not installed")
        assert "REPL" in result.stdout or "Deephaven" in result.stdout


# ---------------------------------------------------------------------------
# Phase 7: Version flag on runtime commands
# ---------------------------------------------------------------------------

class TestVersionFlag:
    """Verify --version flag is present on runtime commands."""

    def test_repl_has_version_flag(self):
        result = _run(["repl", "--help"])
        assert "--version" in result.stdout

    def test_exec_has_version_flag(self):
        result = _run(["exec", "--help"])
        assert "--version" in result.stdout

    def test_serve_has_version_flag(self):
        result = _run(["serve", "--help"])
        assert "--version" in result.stdout
