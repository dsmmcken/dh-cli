"""Tests for deephaven_cli.manager.pypi -- behavioral tests based on spec."""
import io
import json
from unittest.mock import patch, MagicMock

import pytest

from deephaven_cli.manager.pypi import (
    fetch_available_versions,
    fetch_latest_version,
    is_valid_version,
)


# ---------------------------------------------------------------------------
# Mock data: simulates PyPI JSON API response for deephaven-server
# ---------------------------------------------------------------------------

MOCK_PYPI_JSON = {
    "releases": {
        "0.36.0": [{}],
        "0.37.0": [{}],
        "0.37.1": [{}],
        "0.38.0": [{}],
        "0.39.0": [{}],
        "0.40.0": [{}],
        "41.0": [{}],
        "41.1": [{}],
        "42.0a1": [{}],
        "42.0b2": [{}],
        "42.0rc1": [{}],
        "0.35.0.dev1": [{}],
    }
}


@pytest.fixture
def mock_urlopen():
    """Mock urllib.request.urlopen to return canned PyPI JSON."""
    response_data = json.dumps(MOCK_PYPI_JSON).encode("utf-8")
    mock_response = MagicMock()
    mock_response.read.return_value = response_data
    mock_response.status = 200
    mock_response.__enter__ = lambda s: s
    mock_response.__exit__ = MagicMock(return_value=False)
    with patch("urllib.request.urlopen", return_value=mock_response) as m:
        yield m


# ---------------------------------------------------------------------------
# fetch_available_versions
# ---------------------------------------------------------------------------

class TestFetchAvailableVersions:
    def test_returns_list(self, mock_urlopen):
        result = fetch_available_versions()
        assert isinstance(result, list)

    def test_sorted_newest_first(self, mock_urlopen):
        result = fetch_available_versions()
        assert result[0] == "41.1"
        assert result[1] == "41.0"

    def test_filters_out_prereleases(self, mock_urlopen):
        result = fetch_available_versions()
        for v in result:
            assert "a" not in v
            assert "b" not in v
            assert "rc" not in v
            assert "dev" not in v

    def test_includes_stable_versions(self, mock_urlopen):
        result = fetch_available_versions()
        assert "41.1" in result
        assert "41.0" in result
        assert "0.40.0" in result
        assert "0.37.0" in result

    def test_excludes_alpha(self, mock_urlopen):
        result = fetch_available_versions()
        assert "42.0a1" not in result

    def test_excludes_beta(self, mock_urlopen):
        result = fetch_available_versions()
        assert "42.0b2" not in result

    def test_excludes_rc(self, mock_urlopen):
        result = fetch_available_versions()
        assert "42.0rc1" not in result

    def test_excludes_dev(self, mock_urlopen):
        result = fetch_available_versions()
        assert "0.35.0.dev1" not in result

    def test_stable_count(self, mock_urlopen):
        """All stable versions from mock data should be included."""
        result = fetch_available_versions()
        # We have 8 stable versions: 0.36.0, 0.37.0, 0.37.1, 0.38.0, 0.39.0, 0.40.0, 41.0, 41.1
        assert len(result) == 8


# ---------------------------------------------------------------------------
# fetch_latest_version
# ---------------------------------------------------------------------------

class TestFetchLatestVersion:
    def test_returns_first_in_list(self, mock_urlopen):
        result = fetch_latest_version()
        assert result == "41.1"

    def test_returns_string(self, mock_urlopen):
        result = fetch_latest_version()
        assert isinstance(result, str)


# ---------------------------------------------------------------------------
# is_valid_version
# ---------------------------------------------------------------------------

class TestIsValidVersion:
    def test_valid_two_part(self, mock_urlopen):
        assert is_valid_version("41.1") is True

    def test_valid_three_part(self, mock_urlopen):
        assert is_valid_version("0.37.0") is True

    def test_invalid_not_in_pypi(self, mock_urlopen):
        assert is_valid_version("99.99.99") is False

    def test_prerelease_exists_in_pypi(self, mock_urlopen):
        # is_valid_version checks existence in PyPI, pre-releases exist there
        assert is_valid_version("42.0a1") is True


# ---------------------------------------------------------------------------
# Error handling
# ---------------------------------------------------------------------------

class TestNetworkErrors:
    def test_fetch_handles_network_error(self):
        with patch("urllib.request.urlopen", side_effect=Exception("Connection refused")):
            with pytest.raises(Exception):
                fetch_available_versions()

    def test_fetch_handles_timeout(self):
        from urllib.error import URLError
        with patch("urllib.request.urlopen", side_effect=URLError("timeout")):
            with pytest.raises(Exception):
                fetch_available_versions()
