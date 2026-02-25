"""Configuration management for Deephaven CLI.

Manages ~/.dh/ directory structure, config.toml settings, .dhrc project files,
and version resolution logic.
"""

from __future__ import annotations

import os
import tomllib
from pathlib import Path

DH_HOME = "~/.dh/"


def get_dh_home() -> Path:
    """Return the path to ~/.dh/, creating it if needed."""
    p = Path(DH_HOME).expanduser()
    p.mkdir(parents=True, exist_ok=True)
    return p


def get_versions_dir() -> Path:
    """Return the path to ~/.dh/versions/, creating it if needed."""
    p = get_dh_home() / "versions"
    p.mkdir(parents=True, exist_ok=True)
    return p


def get_java_dir() -> Path:
    """Return the path to ~/.dh/java/, creating it if needed."""
    p = get_dh_home() / "java"
    p.mkdir(parents=True, exist_ok=True)
    return p


def get_cache_dir() -> Path:
    """Return the path to ~/.dh/cache/, creating it if needed."""
    p = get_dh_home() / "cache"
    p.mkdir(parents=True, exist_ok=True)
    return p


def _config_path() -> Path:
    """Return the path to ~/.dh/config.toml."""
    return get_dh_home() / "config.toml"


def load_config() -> dict:
    """Read ~/.dh/config.toml and return its contents as a dict.

    Returns an empty dict if the file does not exist.
    """
    path = _config_path()
    if not path.exists():
        return {}
    with open(path, "rb") as f:
        return tomllib.load(f)


def save_config(data: dict) -> None:
    """Write data to ~/.dh/config.toml using simple string formatting."""
    path = _config_path()
    lines: list[str] = []
    for key, value in data.items():
        if isinstance(value, str):
            lines.append(f'{key} = "{value}"')
        elif isinstance(value, bool):
            lines.append(f"{key} = {'true' if value else 'false'}")
        elif isinstance(value, int | float):
            lines.append(f"{key} = {value}")
        else:
            lines.append(f'{key} = "{value}"')
    path.write_text("\n".join(lines) + "\n" if lines else "")


def get_default_version() -> str | None:
    """Read default_version from config.toml, or return None if unset."""
    config = load_config()
    return config.get("default_version")


def set_default_version(version: str) -> None:
    """Set default_version in config.toml, preserving other keys."""
    config = load_config()
    config["default_version"] = version
    save_config(config)


def read_dhrc(path: Path) -> str | None:
    """Read a .dhrc TOML file and return the version string, or None."""
    if not path.exists():
        return None
    with open(path, "rb") as f:
        data = tomllib.load(f)
    return data.get("version")


def find_dhrc(start_dir: Path | None = None) -> str | None:
    """Walk up from start_dir (default cwd) looking for .dhrc.

    Returns the version string from the first .dhrc found, or None.
    """
    current = (start_dir or Path.cwd()).resolve()
    while True:
        dhrc = current / ".dhrc"
        if dhrc.is_file():
            return read_dhrc(dhrc)
        parent = current.parent
        if parent == current:
            break
        current = parent
    return None


def write_dhrc(directory: Path, version: str) -> None:
    """Write a .dhrc file with the given version."""
    path = directory / ".dhrc"
    path.write_text(f'version = "{version}"\n')


def get_installed_versions() -> list[str]:
    """List subdirectories of the versions dir that contain a .venv subdir."""
    versions_dir = get_dh_home() / "versions"
    if not versions_dir.exists():
        return []
    versions = []
    for entry in versions_dir.iterdir():
        if entry.is_dir() and (entry / ".venv").is_dir():
            versions.append(entry.name)
    return _sort_versions(versions)


def _sort_versions(versions: list[str]) -> list[str]:
    """Sort version strings, preferring packaging.version if available."""
    try:
        from packaging.version import Version

        return sorted(versions, key=Version)
    except ImportError:
        return sorted(versions)


def get_latest_installed_version() -> str | None:
    """Return the highest installed version, or None if none installed."""
    versions = get_installed_versions()
    if not versions:
        return None
    return versions[-1]


def resolve_version(cli_version: str | None = None, env_var: bool = True) -> str | None:
    """Resolve which Deephaven version to use.

    Precedence:
    1. cli_version arg (--version flag)
    2. DH_VERSION env var (if env_var=True)
    3. .dhrc file (walk up from cwd)
    4. config.toml default_version
    5. Latest installed version
    6. None
    """
    if cli_version:
        return cli_version

    if env_var:
        env_version = os.environ.get("DH_VERSION")
        if env_version:
            return env_version

    dhrc_version = find_dhrc()
    if dhrc_version:
        return dhrc_version

    default = get_default_version()
    if default:
        return default

    latest = get_latest_installed_version()
    if latest:
        return latest

    return None
