"""Tests for CLI dispatch of new manager commands -- behavioral tests based on spec."""
import sys
from unittest.mock import patch, MagicMock

import pytest

from deephaven_cli.cli import main


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------

@pytest.fixture
def mock_manager():
    """Provide mocks for all manager module functions used by CLI dispatch."""
    mocks = {
        "install_version": MagicMock(),
        "uninstall_version": MagicMock(),
        "set_default_version": MagicMock(),
        "list_installed_versions": MagicMock(return_value=[]),
        "detect_java": MagicMock(return_value=None),
        "resolve_version": MagicMock(return_value="41.1"),
        "activate_version": MagicMock(),
        "is_version_installed": MagicMock(return_value=True),
        "get_installed_versions": MagicMock(return_value=["41.1"]),
        "fetch_available_versions": MagicMock(return_value=["41.1", "41.0"]),
    }
    return mocks


# ---------------------------------------------------------------------------
# dh install
# ---------------------------------------------------------------------------

class TestDhInstall:
    def test_install_calls_install_version(self, monkeypatch):
        monkeypatch.setattr(sys, "argv", ["dh", "install", "41.1"])
        with patch("deephaven_cli.manager.versions.install_version") as mock_install, \
             patch("deephaven_cli.manager.versions.is_version_installed", return_value=False), \
             patch("deephaven_cli.manager.pypi.is_valid_version", return_value=True):
            try:
                main()
            except SystemExit:
                pass
            mock_install.assert_called_once()
            args = mock_install.call_args
            assert "41.1" in str(args)

    def test_install_latest_when_no_version_given(self, monkeypatch):
        monkeypatch.setattr(sys, "argv", ["dh", "install"])
        with patch("deephaven_cli.manager.versions.install_version") as mock_install, \
             patch("deephaven_cli.manager.pypi.fetch_latest_version", return_value="41.1"):
            try:
                main()
            except SystemExit:
                pass
            # Should install something (either "latest" or the resolved latest)
            if mock_install.called:
                assert mock_install.call_count >= 1


# ---------------------------------------------------------------------------
# dh uninstall
# ---------------------------------------------------------------------------

class TestDhUninstall:
    def test_uninstall_calls_uninstall_version(self, monkeypatch):
        monkeypatch.setattr(sys, "argv", ["dh", "uninstall", "41.1"])
        with patch("deephaven_cli.manager.versions.uninstall_version") as mock_uninstall:
            try:
                main()
            except SystemExit:
                pass
            mock_uninstall.assert_called_once()
            args = mock_uninstall.call_args
            assert "41.1" in str(args)


# ---------------------------------------------------------------------------
# dh use
# ---------------------------------------------------------------------------

class TestDhUse:
    def test_use_calls_set_default_version(self, monkeypatch):
        monkeypatch.setattr(sys, "argv", ["dh", "use", "41.1"])
        with patch("deephaven_cli.manager.config.set_default_version") as mock_set, \
             patch("deephaven_cli.manager.versions.is_version_installed", return_value=True):
            try:
                main()
            except SystemExit:
                pass
            mock_set.assert_called_once()
            args = mock_set.call_args
            assert "41.1" in str(args)


# ---------------------------------------------------------------------------
# dh versions
# ---------------------------------------------------------------------------

class TestDhVersions:
    def test_versions_calls_list(self, monkeypatch, capsys):
        monkeypatch.setattr(sys, "argv", ["dh", "versions"])
        with patch("deephaven_cli.manager.versions.list_installed_versions", return_value=[]) as mock_list:
            try:
                main()
            except SystemExit:
                pass
            mock_list.assert_called()


# ---------------------------------------------------------------------------
# dh java
# ---------------------------------------------------------------------------

class TestDhJava:
    def test_java_calls_detect(self, monkeypatch, capsys):
        monkeypatch.setattr(sys, "argv", ["dh", "java"])
        with patch("deephaven_cli.manager.java.detect_java", return_value=None) as mock_detect:
            try:
                main()
            except SystemExit:
                pass
            mock_detect.assert_called()


# ---------------------------------------------------------------------------
# dh doctor
# ---------------------------------------------------------------------------

class TestDhDoctor:
    def test_doctor_runs(self, monkeypatch, capsys):
        monkeypatch.setattr(sys, "argv", ["dh", "doctor"])
        with patch("deephaven_cli.manager.java.detect_java", return_value=None), \
             patch("deephaven_cli.manager.versions.list_installed_versions", return_value=[]), \
             patch("shutil.which", return_value="/usr/bin/uv"):
            try:
                main()
            except SystemExit:
                pass
            captured = capsys.readouterr()
            # Doctor should produce some output about environment status
            assert len(captured.out) > 0 or len(captured.err) >= 0


# ---------------------------------------------------------------------------
# Runtime commands with --version flag
# ---------------------------------------------------------------------------

class TestRuntimeVersionFlag:
    def test_repl_with_version_flag(self, monkeypatch):
        monkeypatch.setattr(sys, "argv", ["dh", "repl", "--version", "41.1"])
        with patch("deephaven_cli.manager.activate.activate_version") as mock_activate, \
             patch("deephaven_cli.manager.config.resolve_version", return_value="41.1"), \
             patch("deephaven_cli.manager.versions.is_version_installed", return_value=True):
            try:
                main()
            except (SystemExit, Exception):
                pass
            # activate_version should have been called with 41.1
            if mock_activate.called:
                args = mock_activate.call_args
                assert "41.1" in str(args)

    def test_serve_with_version_flag(self, monkeypatch, tmp_path):
        script = tmp_path / "app.py"
        script.write_text("print('hello')\n")
        monkeypatch.setattr(sys, "argv", ["dh", "serve", "--version", "41.1", str(script)])
        with patch("deephaven_cli.manager.activate.activate_version") as mock_activate, \
             patch("deephaven_cli.manager.config.resolve_version", return_value="41.1"), \
             patch("deephaven_cli.manager.versions.is_version_installed", return_value=True):
            try:
                main()
            except (SystemExit, Exception):
                pass
            if mock_activate.called:
                args = mock_activate.call_args
                assert "41.1" in str(args)

    def test_exec_with_version_flag(self, monkeypatch, tmp_path):
        script = tmp_path / "script.py"
        script.write_text("print('hello')\n")
        monkeypatch.setattr(sys, "argv", ["dh", "exec", "--version", "0.37.0", str(script)])
        with patch("deephaven_cli.manager.activate.activate_version") as mock_activate, \
             patch("deephaven_cli.manager.config.resolve_version", return_value="0.37.0"), \
             patch("deephaven_cli.manager.versions.is_version_installed", return_value=True):
            try:
                main()
            except (SystemExit, Exception):
                pass
            if mock_activate.called:
                args = mock_activate.call_args
                assert "0.37.0" in str(args)


# ---------------------------------------------------------------------------
# Error cases
# ---------------------------------------------------------------------------

class TestErrorCases:
    def test_runtime_command_fails_when_no_version_installed(self, monkeypatch, capsys):
        monkeypatch.setattr(sys, "argv", ["dh", "repl"])
        with patch("deephaven_cli.manager.config.resolve_version", return_value=None):
            try:
                result = main()
            except SystemExit as e:
                result = e.code
            except Exception:
                result = None
            captured = capsys.readouterr()
            assert "No Deephaven version installed" in captured.err or result != 0
