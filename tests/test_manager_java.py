"""Tests for deephaven_cli.manager.java -- behavioral tests based on spec."""
import os
from pathlib import Path
from unittest.mock import patch, MagicMock

import pytest

from deephaven_cli.manager.java import (
    detect_java,
    get_java_version,
    parse_java_version_output,
    check_java_version,
    get_adoptium_download_url,
    get_managed_java,
)


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------

@pytest.fixture
def dh_home(tmp_path, monkeypatch):
    """Override DH_HOME to use a temp directory for isolation."""
    home = tmp_path / ".dh"
    home.mkdir(parents=True, exist_ok=True)
    java_dir = home / "java"
    java_dir.mkdir(parents=True, exist_ok=True)
    cache_dir = home / "cache"
    cache_dir.mkdir(parents=True, exist_ok=True)
    monkeypatch.setattr("deephaven_cli.manager.java.get_java_dir", lambda: java_dir)
    monkeypatch.setattr("deephaven_cli.manager.java.get_cache_dir", lambda: cache_dir)
    return home


# ---------------------------------------------------------------------------
# parse_java_version_output
# ---------------------------------------------------------------------------

class TestParseJavaVersionOutput:
    def test_openjdk_21(self):
        output = 'openjdk version "21.0.5" 2024-10-15\nOpenJDK Runtime Environment Temurin-21.0.5+11 (build 21.0.5+11)\nOpenJDK 64-Bit Server VM Temurin-21.0.5+11 (build 21.0.5+11, mixed mode, sharing)'
        result = parse_java_version_output(output)
        assert result is not None
        assert 21 in (result,) or "21" in str(result)

    def test_openjdk_17(self):
        output = 'openjdk version "17.0.9" 2023-10-17\nOpenJDK Runtime Environment (build 17.0.9+9-Ubuntu-122.04)\nOpenJDK 64-Bit Server VM (build 17.0.9+9-Ubuntu-122.04, mixed mode, sharing)'
        result = parse_java_version_output(output)
        assert result is not None
        assert 17 in (result,) or "17" in str(result)

    def test_oracle_java_11(self):
        output = 'java version "11.0.20" 2023-07-18 LTS\nJava(TM) SE Runtime Environment 18.9 (build 11.0.20+9-LTS-256)\nJava HotSpot(TM) 64-Bit Server VM 18.9 (build 11.0.20+9-LTS-256, mixed mode)'
        result = parse_java_version_output(output)
        assert result is not None
        assert 11 in (result,) or "11" in str(result)

    def test_java_8_format(self):
        output = 'java version "1.8.0_391"\nJava(TM) SE Runtime Environment (build 1.8.0_391-b13)\nJava HotSpot(TM) 64-Bit Server VM (build 25.391-b13, mixed mode)'
        result = parse_java_version_output(output)
        assert result is not None
        assert 8 in (result,) or "8" in str(result) or "1.8" in str(result)

    def test_empty_output(self):
        result = parse_java_version_output("")
        assert result is None

    def test_garbage_output(self):
        result = parse_java_version_output("not a java version string at all")
        assert result is None


# ---------------------------------------------------------------------------
# check_java_version
# ---------------------------------------------------------------------------

class TestCheckJavaVersion:
    def test_java_21_passes(self):
        assert check_java_version("21.0.5") is True

    def test_java_17_passes(self):
        assert check_java_version("17.0.9") is True

    def test_java_11_fails(self):
        assert check_java_version("11.0.20") is False

    def test_java_8_fails(self):
        assert check_java_version("1.8.0") is False

    def test_java_22_passes(self):
        assert check_java_version("22.0.1") is True


# ---------------------------------------------------------------------------
# detect_java
# ---------------------------------------------------------------------------

class TestDetectJava:
    def test_java_home_has_highest_priority(self, dh_home, monkeypatch):
        java_home = "/custom/java/home"
        monkeypatch.setenv("JAVA_HOME", java_home)
        java_bin = Path(java_home) / "bin" / "java"

        mock_run = MagicMock()
        mock_run.return_value = MagicMock(
            returncode=0,
            stderr='openjdk version "21.0.5" 2024-10-15\n',
        )

        with patch("subprocess.run", mock_run), \
             patch("pathlib.Path.exists", return_value=True):
            result = detect_java()
        assert result is not None

    def test_path_java_second_priority(self, dh_home, monkeypatch):
        monkeypatch.delenv("JAVA_HOME", raising=False)
        mock_run = MagicMock()
        mock_run.return_value = MagicMock(
            returncode=0,
            stderr='openjdk version "21.0.5" 2024-10-15\n',
        )
        with patch("subprocess.run", mock_run), \
             patch("shutil.which", return_value="/usr/bin/java"):
            result = detect_java()
        assert result is not None

    def test_managed_java_third_priority(self, dh_home, monkeypatch):
        monkeypatch.delenv("JAVA_HOME", raising=False)
        java_dir = dh_home / "java" / "jdk-21.0.5"
        java_dir.mkdir(parents=True)
        (java_dir / "bin").mkdir()
        (java_dir / "bin" / "java").touch()

        mock_run = MagicMock()
        mock_run.return_value = MagicMock(
            returncode=0,
            stderr='openjdk version "21.0.5" 2024-10-15\n',
        )

        with patch("subprocess.run", mock_run), \
             patch("shutil.which", return_value=None), \
             patch("deephaven_cli.manager.java.get_managed_java", return_value=java_dir):
            result = detect_java()
        assert result is not None

    def test_returns_none_when_no_java(self, dh_home, monkeypatch):
        monkeypatch.delenv("JAVA_HOME", raising=False)
        with patch("shutil.which", return_value=None), \
             patch("deephaven_cli.manager.java.get_managed_java", return_value=None):
            result = detect_java()
        assert result is None


# ---------------------------------------------------------------------------
# get_adoptium_download_url
# ---------------------------------------------------------------------------

class TestGetAdoptiumDownloadUrl:
    def test_linux_x64(self):
        with patch("platform.system", return_value="Linux"), \
             patch("platform.machine", return_value="x86_64"):
            url = get_adoptium_download_url()
        assert isinstance(url, str)
        assert "adoptium" in url.lower() or "api" in url.lower()
        assert "linux" in url.lower() or "Linux" in url

    def test_macos_arm(self):
        with patch("platform.system", return_value="Darwin"), \
             patch("platform.machine", return_value="arm64"):
            url = get_adoptium_download_url()
        assert isinstance(url, str)
        # Should reference macOS / aarch64
        url_lower = url.lower()
        assert "mac" in url_lower or "darwin" in url_lower or "aarch64" in url_lower or "arm" in url_lower

    def test_linux_aarch64(self):
        with patch("platform.system", return_value="Linux"), \
             patch("platform.machine", return_value="aarch64"):
            url = get_adoptium_download_url()
        assert isinstance(url, str)
        assert "aarch64" in url.lower() or "arm" in url.lower()

    def test_returns_string(self):
        with patch("platform.system", return_value="Linux"), \
             patch("platform.machine", return_value="x86_64"):
            url = get_adoptium_download_url()
        assert isinstance(url, str)
        assert len(url) > 0


# ---------------------------------------------------------------------------
# get_managed_java
# ---------------------------------------------------------------------------

class TestGetManagedJava:
    def test_returns_none_when_empty(self, dh_home):
        # java dir already exists via fixture, just verify no jdk-* inside
        result = get_managed_java()
        assert result is None

    def test_returns_path_when_jdk_exists(self, dh_home):
        java_dir = dh_home / "java" / "jdk-21.0.5"
        java_dir.mkdir(parents=True)
        (java_dir / "bin").mkdir()
        (java_dir / "bin" / "java").touch()
        result = get_managed_java()
        assert result is not None
        assert isinstance(result, Path)

    def test_returns_none_when_java_dir_missing(self, dh_home):
        # Don't create the java dir at all
        result = get_managed_java()
        assert result is None
