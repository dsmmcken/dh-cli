"""Tests for deephaven_cli.manager.versions -- behavioral tests based on spec."""
import os
from pathlib import Path
from unittest.mock import patch, MagicMock, call

import pytest

from deephaven_cli.manager.versions import (
    install_version,
    uninstall_version,
    list_installed_versions,
    get_version_info,
    is_version_installed,
)


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------

@pytest.fixture
def dh_home(tmp_path, monkeypatch):
    """Override DH_HOME to use a temp directory for isolation."""
    home = tmp_path / ".dh"
    monkeypatch.setattr("deephaven_cli.manager.config.DH_HOME", home)
    return home


@pytest.fixture
def fake_installed(dh_home):
    """Create a fake installed version directory structure."""
    def _create(version):
        v_dir = dh_home / "versions" / version
        v_dir.mkdir(parents=True, exist_ok=True)
        venv = v_dir / ".venv"
        venv.mkdir(exist_ok=True)
        # Create a minimal meta.toml
        meta = v_dir / "meta.toml"
        meta.write_text(f'version = "{version}"\n')
        return v_dir
    return _create


# ---------------------------------------------------------------------------
# install_version
# ---------------------------------------------------------------------------

class TestInstallVersion:
    def test_install_creates_version_directory(self, dh_home):
        mock_run = MagicMock(return_value=MagicMock(returncode=0))
        with patch("subprocess.run", mock_run):
            install_version("41.1")
        v_dir = dh_home / "versions" / "41.1"
        assert v_dir.exists()

    def test_install_runs_uv_venv_command(self, dh_home):
        mock_run = MagicMock(return_value=MagicMock(returncode=0))
        with patch("subprocess.run", mock_run):
            install_version("41.1")
        # Check uv venv was called
        calls = mock_run.call_args_list
        uv_venv_call = None
        for c in calls:
            args = c[0][0] if c[0] else c[1].get("args", [])
            if isinstance(args, list) and "uv" in args and "venv" in args:
                uv_venv_call = c
                break
        assert uv_venv_call is not None, "Expected 'uv venv' subprocess call"

    def test_install_runs_uv_pip_install(self, dh_home):
        mock_run = MagicMock(return_value=MagicMock(returncode=0))
        with patch("subprocess.run", mock_run):
            install_version("41.1")
        calls = mock_run.call_args_list
        pip_install_call = None
        for c in calls:
            args = c[0][0] if c[0] else c[1].get("args", [])
            if isinstance(args, list) and "uv" in args and "install" in args:
                pip_install_call = c
                break
        assert pip_install_call is not None, "Expected 'uv pip install' subprocess call"

    def test_install_includes_deephaven_server(self, dh_home):
        mock_run = MagicMock(return_value=MagicMock(returncode=0))
        with patch("subprocess.run", mock_run):
            install_version("41.1")
        calls = mock_run.call_args_list
        found = False
        for c in calls:
            args = c[0][0] if c[0] else c[1].get("args", [])
            if isinstance(args, list):
                args_str = " ".join(str(a) for a in args)
                if "deephaven-server" in args_str:
                    found = True
                    break
        assert found, "Expected deephaven-server in pip install args"

    def test_install_creates_meta_toml(self, dh_home):
        mock_run = MagicMock(return_value=MagicMock(returncode=0))
        with patch("subprocess.run", mock_run):
            install_version("41.1")
        meta = dh_home / "versions" / "41.1" / "meta.toml"
        assert meta.exists()

    def test_install_uses_python_313(self, dh_home):
        mock_run = MagicMock(return_value=MagicMock(returncode=0))
        with patch("subprocess.run", mock_run):
            install_version("41.1")
        calls = mock_run.call_args_list
        found = False
        for c in calls:
            args = c[0][0] if c[0] else c[1].get("args", [])
            if isinstance(args, list):
                args_str = " ".join(str(a) for a in args)
                if "3.13" in args_str:
                    found = True
                    break
        assert found, "Expected --python 3.13 in uv venv args"


# ---------------------------------------------------------------------------
# uninstall_version
# ---------------------------------------------------------------------------

class TestUninstallVersion:
    def test_uninstall_removes_directory(self, dh_home, fake_installed):
        fake_installed("41.1")
        v_dir = dh_home / "versions" / "41.1"
        assert v_dir.exists()
        uninstall_version("41.1")
        assert not v_dir.exists()

    def test_uninstall_nonexistent_is_no_op(self, dh_home):
        # Should not raise for nonexistent version (idempotent)
        uninstall_version("99.99.99")  # No error expected


# ---------------------------------------------------------------------------
# list_installed_versions
# ---------------------------------------------------------------------------

class TestListInstalledVersions:
    def test_empty_when_none_installed(self, dh_home):
        result = list_installed_versions()
        assert result == [] or result == ()

    def test_lists_installed_versions(self, dh_home, fake_installed):
        fake_installed("41.0")
        fake_installed("41.1")
        result = list_installed_versions()
        version_names = [v if isinstance(v, str) else v.get("version", v) for v in result]
        assert "41.0" in version_names or any("41.0" in str(v) for v in result)
        assert "41.1" in version_names or any("41.1" in str(v) for v in result)

    def test_ignores_dirs_without_venv(self, dh_home):
        versions_dir = dh_home / "versions"
        versions_dir.mkdir(parents=True)
        incomplete = versions_dir / "bad_version"
        incomplete.mkdir()
        result = list_installed_versions()
        result_strs = [str(v) for v in result]
        assert not any("bad_version" in s for s in result_strs)


# ---------------------------------------------------------------------------
# get_version_info
# ---------------------------------------------------------------------------

class TestGetVersionInfo:
    def test_returns_info_for_installed_version(self, dh_home, fake_installed):
        fake_installed("41.1")
        result = get_version_info("41.1")
        assert result is not None

    def test_returns_none_for_missing_version(self, dh_home):
        result = get_version_info("99.99.99")
        assert result is None


# ---------------------------------------------------------------------------
# is_version_installed
# ---------------------------------------------------------------------------

class TestIsVersionInstalled:
    def test_true_when_installed(self, dh_home, fake_installed):
        fake_installed("41.1")
        assert is_version_installed("41.1") is True

    def test_false_when_not_installed(self, dh_home):
        assert is_version_installed("99.99.99") is False

    def test_false_when_no_venv(self, dh_home):
        """A version dir without .venv is not considered installed."""
        versions_dir = dh_home / "versions"
        versions_dir.mkdir(parents=True)
        v_dir = versions_dir / "41.0"
        v_dir.mkdir()
        assert is_version_installed("41.0") is False


# ---------------------------------------------------------------------------
# Edge cases
# ---------------------------------------------------------------------------

class TestEdgeCases:
    def test_install_handles_uv_not_found(self, dh_home):
        with patch("subprocess.run", side_effect=FileNotFoundError("uv not found")):
            with pytest.raises(Exception):
                install_version("41.1")
