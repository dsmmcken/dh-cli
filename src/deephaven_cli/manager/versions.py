"""Version install, uninstall, and listing via uv."""
from __future__ import annotations

import shutil
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path

from deephaven_cli.manager.config import get_default_version, get_versions_dir

_DEFAULT_PLUGINS = [
    "deephaven-plugin-ui",
    "deephaven-plugin-plotly-express",
    "deephaven-plugin-theme-pack",
]


def _version_dir(version: str) -> Path:
    return get_versions_dir() / version


def _venv_path(version: str) -> Path:
    return _version_dir(version) / ".venv"


def _meta_path(version: str) -> Path:
    return _version_dir(version) / "meta.toml"


def _read_meta(version: str) -> dict | None:
    """Read meta.toml for a version, returning parsed dict or None."""
    path = _meta_path(version)
    if not path.exists():
        return None
    try:
        import tomllib

        return tomllib.loads(path.read_text())
    except Exception:
        return None


def _write_meta(version: str, packages: dict[str, str]) -> None:
    """Write meta.toml with install timestamp and package versions."""
    now = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S")
    lines = [f'installed = "{now}"', "", "[packages]"]
    for pkg, ver in sorted(packages.items()):
        lines.append(f'{pkg} = "{ver}"')
    lines.append("")
    _meta_path(version).write_text("\n".join(lines))


def install_version(
    version: str,
    plugins: list[str] | None = None,
    on_progress=None,
) -> bool:
    """Install a Deephaven version into ~/.dh/versions/{version}/.venv.

    Returns True on success, False on failure.
    """
    if shutil.which("uv") is None:
        print("Error: uv is not installed. Install it from https://docs.astral.sh/uv/", file=sys.stderr)
        return False

    if plugins is None:
        plugins = list(_DEFAULT_PLUGINS)

    venv = _venv_path(version)
    ver_dir = _version_dir(version)
    ver_dir.mkdir(parents=True, exist_ok=True)

    # Step 1: create venv
    if on_progress:
        on_progress("Creating virtual environment...")
    result = subprocess.run(
        ["uv", "venv", str(venv), "--python", "3.13"],
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        print(f"Error creating venv: {result.stderr}", file=sys.stderr)
        return False

    # Step 2: install packages
    python = venv / "bin" / "python"
    install_pkgs = [
        f"deephaven-server=={version}",
        f"pydeephaven=={version}",
        *plugins,
    ]

    if on_progress:
        on_progress("Installing packages...")
    result = subprocess.run(
        ["uv", "pip", "install", "--python", str(python), *install_pkgs],
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        print(f"Error installing packages: {result.stderr}", file=sys.stderr)
        # Clean up on failure
        shutil.rmtree(ver_dir, ignore_errors=True)
        return False

    # Step 3: write meta.toml
    packages = {
        "deephaven-server": version,
        "pydeephaven": version,
    }
    for plugin in plugins:
        packages[plugin] = "latest"
    _write_meta(version, packages)

    return True


def uninstall_version(version: str) -> bool:
    """Remove an installed version. Returns True if removed, False if not found."""
    ver_dir = _version_dir(version)
    if not ver_dir.exists():
        return False
    shutil.rmtree(ver_dir)
    return True


def list_installed_versions() -> list[dict]:
    """List all installed versions with metadata.

    Returns list of dicts: {version, installed_date, is_default}.
    """
    versions_dir = get_versions_dir()
    if not versions_dir.exists():
        return []

    default = get_default_version()
    result = []

    for entry in sorted(versions_dir.iterdir()):
        if not entry.is_dir():
            continue
        if not (entry / ".venv").exists():
            continue
        version = entry.name
        meta = _read_meta(version)
        installed_date = meta.get("installed", "unknown") if meta else "unknown"
        result.append({
            "version": version,
            "installed_date": installed_date,
            "is_default": version == default,
        })

    return result


def get_version_info(version: str) -> dict | None:
    """Return detailed info for one installed version, or None if not found."""
    if not is_version_installed(version):
        return None
    meta = _read_meta(version)
    if meta is None:
        return {"version": version, "installed": "unknown", "packages": {}}
    return {
        "version": version,
        "installed": meta.get("installed", "unknown"),
        "packages": meta.get("packages", {}),
    }


def is_version_installed(version: str) -> bool:
    """Check if a version is installed (has a .venv directory)."""
    return _venv_path(version).exists()
