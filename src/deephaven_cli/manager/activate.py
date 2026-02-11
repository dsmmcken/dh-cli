"""Version activation via site.addsitedir().

Adds a specific Deephaven version's site-packages to sys.path so that
lazy imports (e.g. ``from deephaven_server import Server``) resolve to
the packages installed in that version's venv.
"""

from __future__ import annotations

import os
import re
import site
import sys
from pathlib import Path

from deephaven_cli.manager.config import get_java_dir, get_versions_dir


def activate_version(version: str) -> None:
    """Add *version*'s site-packages to ``sys.path``.

    Raises ``RuntimeError`` if the version is not installed.
    """
    venv_dir = get_versions_dir() / version / ".venv"
    if not venv_dir.is_dir():
        raise RuntimeError(f"Version {version} is not installed")

    site_packages = find_site_packages(venv_dir)
    site.addsitedir(str(site_packages))
    set_java_home_if_needed()


def find_site_packages(venv_dir: Path) -> Path:
    """Locate the ``site-packages`` directory inside *venv_dir*.

    Raises ``FileNotFoundError`` if the directory cannot be found.
    """
    matches = list(venv_dir.glob("lib/python3.*/site-packages"))
    if not matches:
        raise FileNotFoundError(
            f"No site-packages directory found in {venv_dir}"
        )
    return matches[0]


def set_java_home_if_needed() -> None:
    """Set ``JAVA_HOME`` if it is not already set.

    Looks for directories starting with ``jdk-`` under ``~/.dh/java/``.
    """
    if os.environ.get("JAVA_HOME"):
        return

    java_dir = get_java_dir()
    if not java_dir.is_dir():
        return

    for entry in sorted(java_dir.iterdir(), reverse=True):
        if entry.is_dir() and entry.name.startswith("jdk-"):
            os.environ["JAVA_HOME"] = str(entry)
            return


def is_version_activated() -> bool:
    """Return ``True`` if any ``~/.dh/versions/*/`` path is on ``sys.path``."""
    versions_dir = str(get_versions_dir())
    return any(p.startswith(versions_dir) for p in sys.path)


def get_activated_version() -> str | None:
    """Return the currently activated version string, or ``None``.

    Checks ``sys.path`` for entries matching ``~/.dh/versions/{version}/``.
    """
    versions_dir = str(get_versions_dir())
    pattern = re.compile(
        re.escape(versions_dir) + r"/([^/]+)/"
    )
    for p in sys.path:
        m = pattern.search(p)
        if m:
            return m.group(1)
    return None
