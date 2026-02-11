"""Tests for deephaven_cli.manager.config -- behavioral tests based on spec."""
import os
from pathlib import Path
from unittest.mock import patch

import pytest

from deephaven_cli.manager.config import (
    get_dh_home,
    get_versions_dir,
    get_java_dir,
    get_cache_dir,
    load_config,
    save_config,
    get_default_version,
    set_default_version,
    find_dhrc,
    read_dhrc,
    write_dhrc,
    resolve_version,
    get_installed_versions,
    get_latest_installed_version,
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


# ---------------------------------------------------------------------------
# Path functions
# ---------------------------------------------------------------------------

class TestPathFunctions:
    def test_get_dh_home_returns_path(self):
        result = get_dh_home()
        assert isinstance(result, Path)

    def test_get_dh_home_under_user_home(self):
        result = get_dh_home()
        assert result.name == ".dh"

    def test_get_versions_dir(self, dh_home):
        result = get_versions_dir()
        assert result == dh_home / "versions"

    def test_get_versions_dir_creates_directory(self, dh_home):
        result = get_versions_dir()
        assert result.is_dir()

    def test_get_java_dir(self, dh_home):
        result = get_java_dir()
        assert result == dh_home / "java"

    def test_get_java_dir_creates_directory(self, dh_home):
        result = get_java_dir()
        assert result.is_dir()

    def test_get_cache_dir(self, dh_home):
        result = get_cache_dir()
        assert result == dh_home / "cache"

    def test_get_cache_dir_creates_directory(self, dh_home):
        result = get_cache_dir()
        assert result.is_dir()


# ---------------------------------------------------------------------------
# Config load/save
# ---------------------------------------------------------------------------

class TestConfigLoadSave:
    def test_load_config_returns_empty_dict_when_no_file(self, dh_home):
        result = load_config()
        assert result == {}

    def test_save_and_load_roundtrip(self, dh_home):
        data = {"default_version": "41.1"}
        save_config(data)
        loaded = load_config()
        assert loaded["default_version"] == "41.1"

    def test_save_config_creates_file(self, dh_home):
        save_config({"key": "value"})
        config_file = dh_home / "config.toml"
        assert config_file.exists()

    def test_save_config_overwrites_existing(self, dh_home):
        save_config({"old": "data"})
        save_config({"new": "data"})
        loaded = load_config()
        assert "old" not in loaded
        assert loaded["new"] == "data"


# ---------------------------------------------------------------------------
# Default version get/set
# ---------------------------------------------------------------------------

class TestDefaultVersion:
    def test_get_default_version_none_when_not_set(self, dh_home):
        result = get_default_version()
        assert result is None

    def test_set_and_get_default_version(self, dh_home):
        set_default_version("41.1")
        result = get_default_version()
        assert result == "41.1"

    def test_set_default_version_overwrites(self, dh_home):
        set_default_version("41.0")
        set_default_version("41.1")
        result = get_default_version()
        assert result == "41.1"

    def test_set_default_version_preserves_other_config(self, dh_home):
        save_config({"some_other_key": "some_value"})
        set_default_version("41.1")
        config = load_config()
        assert config["some_other_key"] == "some_value"
        assert config["default_version"] == "41.1"


# ---------------------------------------------------------------------------
# .dhrc discovery and read/write
# ---------------------------------------------------------------------------

class TestDhrc:
    def test_find_dhrc_in_current_dir(self, tmp_path):
        dhrc = tmp_path / ".dhrc"
        dhrc.write_text('version = "41.1"\n')
        result = find_dhrc(tmp_path)
        # find_dhrc may return the path or the version string
        assert result is not None

    def test_find_dhrc_walks_up(self, tmp_path):
        dhrc = tmp_path / ".dhrc"
        dhrc.write_text('version = "41.1"\n')
        child = tmp_path / "sub" / "deep"
        child.mkdir(parents=True)
        result = find_dhrc(child)
        assert result is not None

    def test_find_dhrc_returns_none_when_missing(self, tmp_path):
        result = find_dhrc(tmp_path)
        assert result is None

    def test_write_dhrc_creates_file(self, tmp_path):
        write_dhrc(tmp_path, "41.1")
        dhrc = tmp_path / ".dhrc"
        assert dhrc.exists()

    def test_read_write_dhrc_roundtrip(self, tmp_path):
        write_dhrc(tmp_path, "0.37.0")
        result = read_dhrc(tmp_path / ".dhrc")
        assert result == "0.37.0"

    def test_read_dhrc_returns_version_string(self, tmp_path):
        dhrc = tmp_path / ".dhrc"
        dhrc.write_text('version = "41.1"\n')
        result = read_dhrc(dhrc)
        assert result == "41.1"


# ---------------------------------------------------------------------------
# Version resolution precedence
# ---------------------------------------------------------------------------

class TestResolveVersion:
    def test_cli_flag_takes_highest_priority(self, dh_home, tmp_path, monkeypatch):
        monkeypatch.setenv("DH_VERSION", "0.37.0")
        monkeypatch.chdir(tmp_path)
        write_dhrc(tmp_path, "0.38.0")
        set_default_version("0.39.0")
        result = resolve_version(cli_version="41.1")
        assert result == "41.1"

    def test_env_var_second_priority(self, dh_home, tmp_path, monkeypatch):
        monkeypatch.setenv("DH_VERSION", "0.37.0")
        monkeypatch.chdir(tmp_path)
        write_dhrc(tmp_path, "0.38.0")
        set_default_version("0.39.0")
        result = resolve_version()
        assert result == "0.37.0"

    def test_dhrc_third_priority(self, dh_home, tmp_path, monkeypatch):
        monkeypatch.delenv("DH_VERSION", raising=False)
        monkeypatch.chdir(tmp_path)
        write_dhrc(tmp_path, "0.38.0")
        set_default_version("0.39.0")
        result = resolve_version()
        assert result == "0.38.0"

    def test_config_default_fourth_priority(self, dh_home, tmp_path, monkeypatch):
        monkeypatch.delenv("DH_VERSION", raising=False)
        # Use a clean dir with no .dhrc
        clean_dir = tmp_path / "no_dhrc"
        clean_dir.mkdir()
        monkeypatch.chdir(clean_dir)
        set_default_version("0.39.0")
        result = resolve_version()
        assert result == "0.39.0"

    def test_latest_installed_fifth_priority(self, dh_home, tmp_path, monkeypatch):
        monkeypatch.delenv("DH_VERSION", raising=False)
        clean_dir = tmp_path / "no_dhrc2"
        clean_dir.mkdir()
        monkeypatch.chdir(clean_dir)
        # Create a fake installed version
        v_dir = dh_home / "versions" / "41.1"
        v_dir.mkdir(parents=True)
        (v_dir / ".venv").mkdir()
        result = resolve_version()
        assert result == "41.1"

    def test_returns_none_when_nothing_available(self, dh_home, tmp_path, monkeypatch):
        monkeypatch.delenv("DH_VERSION", raising=False)
        clean_dir = tmp_path / "no_dhrc3"
        clean_dir.mkdir()
        monkeypatch.chdir(clean_dir)
        result = resolve_version()
        assert result is None


# ---------------------------------------------------------------------------
# Installed versions
# ---------------------------------------------------------------------------

class TestInstalledVersions:
    def test_get_installed_versions_empty(self, dh_home):
        result = get_installed_versions()
        assert result == []

    def test_get_installed_versions_only_dirs_with_venv(self, dh_home):
        versions_dir = dh_home / "versions"
        versions_dir.mkdir(parents=True)
        # Valid version (has .venv)
        v1 = versions_dir / "41.1"
        v1.mkdir()
        (v1 / ".venv").mkdir()
        # Invalid version (no .venv)
        v2 = versions_dir / "41.0"
        v2.mkdir()
        result = get_installed_versions()
        assert "41.1" in result
        assert "41.0" not in result

    def test_get_installed_versions_multiple(self, dh_home):
        versions_dir = dh_home / "versions"
        versions_dir.mkdir(parents=True)
        for v in ["0.37.0", "0.38.0", "41.1"]:
            d = versions_dir / v
            d.mkdir()
            (d / ".venv").mkdir()
        result = get_installed_versions()
        assert len(result) == 3

    def test_get_latest_installed_version_returns_highest(self, dh_home):
        versions_dir = dh_home / "versions"
        versions_dir.mkdir(parents=True)
        for v in ["0.37.0", "0.38.0", "41.1"]:
            d = versions_dir / v
            d.mkdir()
            (d / ".venv").mkdir()
        result = get_latest_installed_version()
        assert result == "41.1"

    def test_get_latest_installed_version_none_when_empty(self, dh_home):
        result = get_latest_installed_version()
        assert result is None
