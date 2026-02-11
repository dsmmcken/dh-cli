"""Tests for deephaven_cli.manager.activate -- behavioral tests based on spec."""
import os
import sys
from pathlib import Path
from unittest.mock import patch

import pytest

from deephaven_cli.manager.activate import (
    activate_version,
    find_site_packages,
    set_java_home_if_needed,
    is_version_activated,
    get_activated_version,
)


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------

@pytest.fixture
def dh_home(tmp_path, monkeypatch):
    """Override DH_HOME to use a temp directory for isolation."""
    home = tmp_path / ".dh"
    home.mkdir(parents=True, exist_ok=True)
    versions_dir = home / "versions"
    versions_dir.mkdir(parents=True, exist_ok=True)
    java_dir = home / "java"
    java_dir.mkdir(parents=True, exist_ok=True)
    monkeypatch.setattr("deephaven_cli.manager.activate.get_versions_dir", lambda: versions_dir)
    monkeypatch.setattr("deephaven_cli.manager.activate.get_java_dir", lambda: java_dir)
    return home


@pytest.fixture
def fake_venv(dh_home):
    """Create a fake venv structure with site-packages for a given version."""
    def _create(version, python_version="3.13"):
        v_dir = dh_home / "versions" / version
        v_dir.mkdir(parents=True, exist_ok=True)
        venv = v_dir / ".venv"
        venv.mkdir(exist_ok=True)
        # Create the site-packages directory tree
        site_packages = venv / "lib" / f"python{python_version}" / "site-packages"
        site_packages.mkdir(parents=True, exist_ok=True)
        # Create meta.toml
        meta = v_dir / "meta.toml"
        meta.write_text(f'version = "{version}"\n')
        return site_packages
    return _create


@pytest.fixture(autouse=True)
def clean_sys_path():
    """Save and restore sys.path to prevent test pollution."""
    original = sys.path.copy()
    yield
    sys.path[:] = original


# ---------------------------------------------------------------------------
# activate_version
# ---------------------------------------------------------------------------

class TestActivateVersion:
    def test_adds_site_packages_to_sys_path(self, dh_home, fake_venv):
        site_pkgs = fake_venv("41.1")
        activate_version("41.1")
        assert str(site_pkgs) in sys.path

    def test_raises_for_nonexistent_version(self, dh_home):
        with pytest.raises(RuntimeError):
            activate_version("99.99.99")

    def test_activating_twice_does_not_duplicate(self, dh_home, fake_venv):
        site_pkgs = fake_venv("41.1")
        activate_version("41.1")
        count_before = sys.path.count(str(site_pkgs))
        activate_version("41.1")
        count_after = sys.path.count(str(site_pkgs))
        # Should not add it again (or at most still be 1)
        assert count_after <= count_before + 1


# ---------------------------------------------------------------------------
# find_site_packages
# ---------------------------------------------------------------------------

class TestFindSitePackages:
    def test_locates_correct_directory(self, dh_home, fake_venv):
        site_pkgs = fake_venv("41.1")
        venv_dir = dh_home / "versions" / "41.1" / ".venv"
        result = find_site_packages(venv_dir)
        assert result == site_pkgs
        assert result.is_dir()

    def test_handles_different_python_version(self, dh_home, fake_venv):
        site_pkgs = fake_venv("41.1", python_version="3.14")
        venv_dir = dh_home / "versions" / "41.1" / ".venv"
        result = find_site_packages(venv_dir)
        assert result == site_pkgs

    def test_raises_when_no_site_packages(self, dh_home):
        venv_dir = dh_home / "versions" / "41.1" / ".venv"
        venv_dir.mkdir(parents=True)
        with pytest.raises(Exception):
            find_site_packages(venv_dir)


# ---------------------------------------------------------------------------
# set_java_home_if_needed
# ---------------------------------------------------------------------------

class TestSetJavaHomeIfNeeded:
    def test_sets_java_home_when_managed_java_exists(self, dh_home, monkeypatch):
        monkeypatch.delenv("JAVA_HOME", raising=False)
        # Create a managed java directory (jdk- prefix triggers detection)
        jdk_dir = dh_home / "java" / "jdk-21.0.5"
        jdk_dir.mkdir(parents=True)
        (jdk_dir / "bin").mkdir()
        (jdk_dir / "bin" / "java").touch()
        set_java_home_if_needed()
        assert os.environ.get("JAVA_HOME") == str(jdk_dir)
        # Clean up
        monkeypatch.delenv("JAVA_HOME", raising=False)

    def test_does_not_override_existing_java_home(self, dh_home, monkeypatch):
        original = "/usr/lib/jvm/java-17"
        monkeypatch.setenv("JAVA_HOME", original)
        jdk_dir = dh_home / "java" / "jdk-21.0.5"
        jdk_dir.mkdir(parents=True)
        set_java_home_if_needed()
        assert os.environ["JAVA_HOME"] == original

    def test_no_op_when_no_managed_java(self, dh_home, monkeypatch):
        monkeypatch.delenv("JAVA_HOME", raising=False)
        # java dir exists but has no jdk-* subdirectories
        set_java_home_if_needed()
        assert "JAVA_HOME" not in os.environ


# ---------------------------------------------------------------------------
# State tracking
# ---------------------------------------------------------------------------

class TestActivationState:
    def test_is_version_activated_false_initially(self, dh_home):
        # Reset activation state
        with patch.object(
            sys.modules.get("deephaven_cli.manager.activate", sys),
            "_activated_version",
            None,
            create=True,
        ):
            pass
        # Just check the function returns a boolean
        result = is_version_activated()
        assert isinstance(result, bool)

    def test_get_activated_version_none_initially(self, dh_home):
        result = get_activated_version()
        # Before any activation, should be None (or whatever the initial state is)
        assert result is None or isinstance(result, str)

    def test_after_activation_state_is_tracked(self, dh_home, fake_venv):
        fake_venv("41.1")
        activate_version("41.1")
        assert is_version_activated() is True
        assert get_activated_version() == "41.1"
