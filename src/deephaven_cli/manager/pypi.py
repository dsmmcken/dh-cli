"""PyPI version discovery for deephaven-server."""
from __future__ import annotations

import json
import re
import urllib.request

_PYPI_URL = "https://pypi.org/pypi/deephaven-server/json"
_TIMEOUT = 10
_PRE_RELEASE_RE = re.compile(r"(a|b|rc|dev|alpha|beta|preview|post)\d*", re.IGNORECASE)


def _parse_version(version: str) -> tuple[int, ...]:
    """Parse a version string into a tuple of integers for sorting.

    Handles both 2-part (41.1) and 3-part (0.37.0) versions.
    """
    parts = version.split(".")
    return tuple(int(p) for p in parts)


def _is_stable(version: str) -> bool:
    """Return True if the version string has no pre-release markers."""
    return _PRE_RELEASE_RE.search(version) is None


def _fetch_pypi_json() -> dict:
    """Fetch the PyPI JSON metadata for deephaven-server."""
    try:
        req = urllib.request.Request(_PYPI_URL, headers={"Accept": "application/json"})
        with urllib.request.urlopen(req, timeout=_TIMEOUT) as resp:
            if resp.status != 200:
                raise RuntimeError(f"PyPI returned HTTP {resp.status}")
            try:
                return json.loads(resp.read())
            except (json.JSONDecodeError, ValueError) as exc:
                raise RuntimeError(f"Malformed JSON from PyPI: {exc}") from exc
    except urllib.error.URLError as exc:
        raise ConnectionError(f"Failed to reach PyPI: {exc}") from exc
    except TimeoutError as exc:
        raise ConnectionError(f"PyPI request timed out after {_TIMEOUT}s") from exc


def fetch_available_versions() -> list[str]:
    """Fetch all stable versions of deephaven-server from PyPI, newest first."""
    data = _fetch_pypi_json()
    releases = data.get("releases", {})
    stable = [v for v in releases if _is_stable(v)]
    stable.sort(key=_parse_version, reverse=True)
    return stable


def fetch_latest_version() -> str:
    """Return the latest stable version from PyPI."""
    versions = fetch_available_versions()
    if not versions:
        raise RuntimeError("No stable versions found on PyPI")
    return versions[0]


def is_valid_version(version: str) -> bool:
    """Check if a version exists on PyPI.

    Accepts "latest" as a special alias (always valid if PyPI is reachable).
    """
    if version == "latest":
        return True
    data = _fetch_pypi_json()
    return version in data.get("releases", {})
