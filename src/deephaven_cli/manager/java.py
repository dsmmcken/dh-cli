"""Java detection, version checking, and Temurin JDK download.

Uses only stdlib: ``urllib.request``, ``tarfile``, ``zipfile``,
``platform``, ``shutil``, ``subprocess``, ``os``, ``re``.
"""

from __future__ import annotations

import os
import platform
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
import urllib.request
import zipfile
from pathlib import Path
from typing import Callable

from deephaven_cli.manager.config import get_cache_dir, get_java_dir

_MIN_JAVA_VERSION = 17
_ADOPTIUM_FEATURE_VERSION = 21


def detect_java() -> dict | None:
    """Detect a usable Java (>= 17) installation.

    Checks in order:
    1. ``JAVA_HOME`` environment variable
    2. ``java`` on ``PATH``
    3. Managed JDK in ``~/.dh/java/``

    Returns a dict ``{"path", "version", "home", "source"}`` or ``None``.
    """
    # 1. JAVA_HOME
    java_home = os.environ.get("JAVA_HOME")
    if java_home:
        java_path = os.path.join(java_home, "bin", "java")
        if os.path.isfile(java_path):
            version = get_java_version(java_path)
            if version and check_java_version(version):
                return {
                    "path": java_path,
                    "version": version,
                    "home": java_home,
                    "source": "JAVA_HOME",
                }

    # 2. java on PATH
    java_on_path = shutil.which("java")
    if java_on_path:
        version = get_java_version(java_on_path)
        if version and check_java_version(version):
            # Derive JAVA_HOME: <home>/bin/java -> <home>
            home = str(Path(java_on_path).resolve().parent.parent)
            return {
                "path": java_on_path,
                "version": version,
                "home": home,
                "source": "PATH",
            }

    # 3. Managed JDK in ~/.dh/java/
    managed = get_managed_java()
    if managed:
        java_path = str(managed / "bin" / "java")
        if os.path.isfile(java_path):
            version = get_java_version(java_path)
            if version and check_java_version(version):
                return {
                    "path": java_path,
                    "version": version,
                    "home": str(managed),
                    "source": "managed",
                }

    return None


def get_java_version(java_path: str) -> str | None:
    """Run ``java -version`` and return the parsed version string, or ``None``."""
    try:
        result = subprocess.run(
            [java_path, "-version"],
            capture_output=True,
            text=True,
            timeout=10,
        )
        # Java prints version info to stderr.
        return parse_java_version_output(result.stderr)
    except (subprocess.TimeoutExpired, OSError):
        return None


def parse_java_version_output(output: str) -> str | None:
    """Parse the stderr output of ``java -version``.

    Handles formats like::

        openjdk version "21.0.5" 2024-10-15
        java version "17.0.2" 2022-01-18

    Returns the version string (e.g. ``"21.0.5"``) or ``None``.
    """
    m = re.search(r'version "(\d+[\d._]*)"', output)
    if not m:
        return None
    # Strip trailing underscores/dots and underscore suffixes for clean version
    version = m.group(1).rstrip("._")
    return version


def check_java_version(version_str: str) -> bool:
    """Return ``True`` if *version_str* represents Java >= 17.

    Handles both modern (``"21.0.5"``) and legacy (``"1.8.0_292"``) formats.
    """
    major = int(version_str.split(".")[0])
    # Legacy: "1.8.0" means Java 8
    if major == 1:
        parts = version_str.split(".")
        if len(parts) >= 2:
            major = int(parts[1])
    return major >= _MIN_JAVA_VERSION


def get_adoptium_download_url() -> str:
    """Return the Adoptium API URL for downloading Temurin JDK on this platform."""
    system = platform.system()
    machine = platform.machine()

    os_map = {
        "Linux": "linux",
        "Darwin": "mac",
        "Windows": "windows",
    }
    arch_map = {
        "x86_64": "x64",
        "AMD64": "x64",
        "aarch64": "aarch64",
        "arm64": "aarch64",
    }

    os_name = os_map.get(system)
    arch_name = arch_map.get(machine)

    if not os_name or not arch_name:
        raise RuntimeError(
            f"Unsupported platform: {system} {machine}"
        )

    return (
        f"https://api.adoptium.net/v3/binary/latest/"
        f"{_ADOPTIUM_FEATURE_VERSION}/ga/{os_name}/{arch_name}/"
        f"jdk/hotspot/normal/eclipse"
    )


def download_java(
    on_progress: Callable[[int, int], None] | None = None,
) -> Path:
    """Download Eclipse Temurin JDK to ``~/.dh/java/``.

    Returns the ``JAVA_HOME`` path (the extracted ``jdk-*`` directory).
    """
    url = get_adoptium_download_url()
    cache_dir = get_cache_dir()
    cache_dir.mkdir(parents=True, exist_ok=True)
    java_dir = get_java_dir()
    java_dir.mkdir(parents=True, exist_ok=True)

    system = platform.system()
    suffix = ".zip" if system == "Windows" else ".tar.gz"

    # Download to a temp file in the cache directory.
    fd, tmp_path = tempfile.mkstemp(suffix=suffix, dir=str(cache_dir))
    try:
        with urllib.request.urlopen(url) as response:
            total = int(response.headers.get("Content-Length", 0))
            downloaded = 0
            with os.fdopen(fd, "wb") as out:
                while True:
                    chunk = response.read(65536)
                    if not chunk:
                        break
                    out.write(chunk)
                    downloaded += len(chunk)
                    if on_progress:
                        on_progress(downloaded, total)

        # Extract
        if suffix == ".zip":
            with zipfile.ZipFile(tmp_path) as zf:
                zf.extractall(str(java_dir))
        else:
            with tarfile.open(tmp_path, "r:gz") as tf:
                tf.extractall(str(java_dir))
    finally:
        # Clean up the temp archive.
        try:
            os.unlink(tmp_path)
        except OSError:
            pass

    # Find the extracted jdk-* directory.
    jdk_home = get_managed_java()
    if jdk_home is None:
        raise RuntimeError(
            f"Extraction succeeded but no jdk-* directory found in {java_dir}"
        )
    return jdk_home


def install_java() -> Path:
    """Download and install Eclipse Temurin JDK, printing progress to stderr.

    Returns the ``JAVA_HOME`` path.
    """

    def _progress(downloaded: int, total: int) -> None:
        if total > 0:
            pct = downloaded * 100 // total
            mb_done = downloaded / (1024 * 1024)
            mb_total = total / (1024 * 1024)
            print(
                f"\rDownloading Eclipse Temurin {_ADOPTIUM_FEATURE_VERSION}... "
                f"{mb_done:.0f}/{mb_total:.0f} MB ({pct}%)",
                end="",
                file=sys.stderr,
                flush=True,
            )

    print(
        f"Installing Eclipse Temurin JDK {_ADOPTIUM_FEATURE_VERSION}...",
        file=sys.stderr,
    )
    jdk_home = download_java(on_progress=_progress)
    print(file=sys.stderr)  # newline after progress
    print(f"Java installed to {jdk_home}", file=sys.stderr)
    return jdk_home


def get_managed_java() -> Path | None:
    """Return the ``JAVA_HOME`` path of a managed JDK in ``~/.dh/java/``, or ``None``."""
    java_dir = get_java_dir()
    if not java_dir.is_dir():
        return None
    for entry in sorted(java_dir.iterdir(), reverse=True):
        if entry.is_dir() and entry.name.startswith("jdk-"):
            # On macOS the actual home is inside Contents/Home
            contents_home = entry / "Contents" / "Home"
            if contents_home.is_dir():
                return contents_home
            return entry
    return None
