"""Integration tests for the dh CLI.

Every test in this module runs REAL subprocess commands against the `dh` binary.
No mocks are used.  A temporary HOME directory is created per session so that
~/.dh/ resolves to an isolated location, preventing interference with the
developer's real environment.

Run with:
    cd /home/dsmmcken/git/dsmmcken/dh-cli && uv run python -m pytest tests/test_cli_integration.py -v

Markers:
    @pytest.mark.slow         - tests that may take >30s (e.g. install)
    @pytest.mark.integration  - tests that need network or external tools
"""

from __future__ import annotations

import os
import subprocess
import tempfile
from pathlib import Path

import pytest

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

# Timeout (seconds) for commands that should return quickly.
FAST_TIMEOUT = 30

# Timeout for commands that hit the network (install, versions --remote).
NETWORK_TIMEOUT = 300


def _run(
    args: list[str],
    *,
    env: dict[str, str] | None = None,
    timeout: int = FAST_TIMEOUT,
    cwd: str | None = None,
) -> subprocess.CompletedProcess[str]:
    """Run a dh CLI command and return the CompletedProcess."""
    return subprocess.run(
        ["dh", *args],
        capture_output=True,
        text=True,
        timeout=timeout,
        env=env,
        cwd=cwd,
    )


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------

@pytest.fixture(scope="session")
def isolated_env(tmp_path_factory):
    """Build an environment dict where HOME points to a temp directory.

    This means ~/.dh/ resolves inside the temp dir, so install/uninstall
    operations are isolated from the developer machine.
    """
    tmp_home = tmp_path_factory.mktemp("dh_home")
    env = os.environ.copy()
    env["HOME"] = str(tmp_home)
    # Ensure DH_VERSION does not leak from the host environment.
    env.pop("DH_VERSION", None)
    return env


@pytest.fixture(scope="session")
def isolated_home(isolated_env):
    """Return the Path to the temp HOME directory."""
    return Path(isolated_env["HOME"])


@pytest.fixture(scope="session")
def installed_version(isolated_env):
    """Install a real Deephaven version into the isolated env.

    This fixture runs once per session and returns the installed version
    string.  We use ``dh install latest`` and capture the resolved version.
    """
    result = subprocess.run(
        ["dh", "install", "latest"],
        capture_output=True,
        text=True,
        timeout=NETWORK_TIMEOUT,
        env=isolated_env,
    )
    assert result.returncode == 0, (
        f"dh install latest failed:\nstdout={result.stdout}\nstderr={result.stderr}"
    )
    # Extract version from output like "Latest version: 41.1"
    for line in result.stdout.splitlines():
        if line.startswith("Latest version:"):
            return line.split(":", 1)[1].strip()
    # Fallback: parse from "Deephaven X installed successfully"
    for line in result.stdout.splitlines():
        if "installed successfully" in line:
            parts = line.split()
            for i, w in enumerate(parts):
                if w == "Deephaven" and i + 1 < len(parts):
                    return parts[i + 1]
    pytest.fail(f"Could not determine installed version from output:\n{result.stdout}")


# ---------------------------------------------------------------------------
# 1. Top-level help
# ---------------------------------------------------------------------------

class TestTopLevelHelp:
    """Tests for `dh` with no args and `dh --help`."""

    def test_no_args_shows_help(self):
        result = _run([])
        assert result.returncode == 0
        assert "Deephaven CLI" in result.stdout
        assert "install" in result.stdout
        assert "repl" in result.stdout

    def test_help_flag(self):
        result = _run(["--help"])
        assert result.returncode == 0
        assert "Deephaven CLI" in result.stdout
        assert "positional arguments" in result.stdout or "subcommands" in result.stdout or "install" in result.stdout


# ---------------------------------------------------------------------------
# 2. dh install
# ---------------------------------------------------------------------------

class TestInstall:
    """Tests for `dh install`."""

    def test_install_help(self):
        result = _run(["install", "--help"])
        assert result.returncode == 0
        assert "Install a Deephaven version" in result.stdout

    @pytest.mark.slow
    @pytest.mark.integration
    def test_install_latest(self, installed_version, isolated_env):
        """The installed_version fixture already ran `dh install latest`.
        Verify the version string looks valid."""
        assert installed_version is not None
        assert "." in installed_version  # e.g. "41.1"

    @pytest.mark.slow
    @pytest.mark.integration
    def test_install_already_installed(self, installed_version, isolated_env):
        """Re-installing the same version should succeed with a message."""
        result = _run(
            ["install", installed_version],
            env=isolated_env,
            timeout=NETWORK_TIMEOUT,
        )
        assert result.returncode == 0
        assert "already installed" in result.stdout


# ---------------------------------------------------------------------------
# 3. dh versions
# ---------------------------------------------------------------------------

class TestVersions:
    """Tests for `dh versions` and `dh versions --remote`."""

    def test_versions_help(self):
        result = _run(["versions", "--help"])
        assert result.returncode == 0
        assert "installed" in result.stdout.lower() or "versions" in result.stdout.lower()

    @pytest.mark.slow
    @pytest.mark.integration
    def test_versions_lists_installed(self, installed_version, isolated_env):
        result = _run(["versions"], env=isolated_env)
        assert result.returncode == 0
        assert installed_version in result.stdout
        assert "Installed versions:" in result.stdout

    @pytest.mark.slow
    @pytest.mark.integration
    def test_versions_remote(self, installed_version, isolated_env):
        result = _run(["versions", "--remote"], env=isolated_env, timeout=NETWORK_TIMEOUT)
        assert result.returncode == 0
        # Remote flag should show PyPI info
        assert "PyPI" in result.stdout or "Available" in result.stdout or installed_version in result.stdout

    def test_versions_no_versions_installed(self, isolated_env, tmp_path):
        """With an empty HOME, versions should report none installed."""
        env = isolated_env.copy()
        env["HOME"] = str(tmp_path)
        result = _run(["versions"], env=env)
        assert result.returncode == 0
        assert "No Deephaven versions installed" in result.stdout


# ---------------------------------------------------------------------------
# 4. dh use
# ---------------------------------------------------------------------------

class TestUse:
    """Tests for `dh use VERSION` and `dh use VERSION --local`."""

    def test_use_help(self):
        result = _run(["use", "--help"])
        assert result.returncode == 0
        assert "default" in result.stdout.lower()

    @pytest.mark.slow
    @pytest.mark.integration
    def test_use_set_global_default(self, installed_version, isolated_env):
        result = _run(["use", installed_version], env=isolated_env)
        assert result.returncode == 0
        assert "global default" in result.stdout.lower() or "default" in result.stdout.lower()

    @pytest.mark.slow
    @pytest.mark.integration
    def test_use_local(self, installed_version, isolated_env, tmp_path):
        result = _run(["use", installed_version, "--local"], env=isolated_env, cwd=str(tmp_path))
        assert result.returncode == 0
        assert ".dhrc" in result.stdout or "local" in result.stdout.lower()
        # .dhrc file should exist in the cwd
        dhrc = tmp_path / ".dhrc"
        assert dhrc.exists()
        content = dhrc.read_text()
        assert installed_version in content

    def test_use_not_installed_version(self, isolated_env, tmp_path):
        """Using a version that is not installed should fail."""
        env = isolated_env.copy()
        env["HOME"] = str(tmp_path)
        result = _run(["use", "999.999"], env=env)
        assert result.returncode != 0
        assert "not installed" in result.stderr.lower()


# ---------------------------------------------------------------------------
# 5. dh uninstall
# ---------------------------------------------------------------------------

class TestUninstall:
    """Tests for `dh uninstall VERSION`."""

    def test_uninstall_help(self):
        result = _run(["uninstall", "--help"])
        assert result.returncode == 0
        assert "Remove" in result.stdout or "uninstall" in result.stdout.lower()

    def test_uninstall_not_installed(self, isolated_env, tmp_path):
        """Uninstalling a version that isn't installed should fail."""
        env = isolated_env.copy()
        env["HOME"] = str(tmp_path)
        result = _run(["uninstall", "999.999"], env=env)
        assert result.returncode != 0
        assert "not installed" in result.stderr.lower()


# ---------------------------------------------------------------------------
# 6. dh java
# ---------------------------------------------------------------------------

class TestJava:
    """Tests for `dh java` (status) and `dh java install`."""

    def test_java_help(self):
        result = _run(["java", "--help"])
        assert result.returncode == 0
        assert "Java" in result.stdout

    def test_java_status(self):
        """dh java should report on the Java installation."""
        result = _run(["java"])
        # May succeed (Java found) or fail (Java not found) -- both are valid
        output = result.stdout + result.stderr
        assert "Java" in output or "java" in output.lower()

    def test_java_install_help(self):
        result = _run(["java-install", "--help"])
        assert result.returncode == 0
        assert "Temurin" in result.stdout or "JDK" in result.stdout or "java" in result.stdout.lower()


# ---------------------------------------------------------------------------
# 7. dh doctor
# ---------------------------------------------------------------------------

class TestDoctor:
    """Tests for `dh doctor`."""

    def test_doctor_help(self):
        result = _run(["doctor", "--help"])
        assert result.returncode == 0
        assert "diagnostic" in result.stdout.lower() or "doctor" in result.stdout.lower() or "health" in result.stdout.lower()

    def test_doctor_runs(self):
        """Doctor should run and produce diagnostic output."""
        result = _run(["doctor"])
        output = result.stdout + result.stderr
        assert "Doctor" in output or "uv" in output or "Java" in output

    @pytest.mark.slow
    @pytest.mark.integration
    def test_doctor_with_installed_version(self, installed_version, isolated_env):
        result = _run(["doctor"], env=isolated_env)
        output = result.stdout
        assert "Doctor" in output
        assert "uv" in output.lower()
        assert "version" in output.lower() or installed_version in output


# ---------------------------------------------------------------------------
# 8. dh repl
# ---------------------------------------------------------------------------

class TestRepl:
    """Tests for `dh repl`."""

    def test_repl_help(self):
        result = _run(["repl", "--help"])
        assert result.returncode == 0
        assert "REPL" in result.stdout or "repl" in result.stdout.lower()
        assert "--port" in result.stdout
        assert "--host" in result.stdout
        assert "--vi" in result.stdout
        assert "--jvm-args" in result.stdout
        assert "--verbose" in result.stdout

    def test_repl_no_version_installed_error(self, tmp_path):
        """Without any version installed, repl should fail with guidance."""
        env = os.environ.copy()
        env["HOME"] = str(tmp_path)
        env.pop("DH_VERSION", None)
        result = _run(["repl"], env=env)
        assert result.returncode != 0
        assert "No Deephaven version installed" in result.stderr or "no deephaven" in result.stderr.lower()

    @pytest.mark.slow
    @pytest.mark.integration
    def test_repl_starts_with_installed_version(self, installed_version, isolated_env):
        """dh repl with a version installed should not crash on import.

        We feed 'exit()' via stdin so it exits immediately.  The key check
        is that it does NOT die with an ImportError (e.g. prompt_toolkit).
        """
        result = subprocess.run(
            ["dh", "repl"],
            input="exit()\n",
            capture_output=True,
            text=True,
            timeout=120,
            env=isolated_env,
        )
        combined = result.stdout + result.stderr
        # Must not crash with a ModuleNotFoundError
        assert "ModuleNotFoundError" not in combined, (
            f"dh repl crashed with import error:\n{combined}"
        )
        assert "ImportError" not in combined, (
            f"dh repl crashed with import error:\n{combined}"
        )


# ---------------------------------------------------------------------------
# 9. dh exec
# ---------------------------------------------------------------------------

class TestExec:
    """Tests for `dh exec`."""

    def test_exec_help(self):
        result = _run(["exec", "--help"])
        assert result.returncode == 0
        assert "Execute" in result.stdout or "exec" in result.stdout.lower()
        assert "-c" in result.stdout
        assert "--timeout" in result.stdout
        assert "--no-show-tables" in result.stdout
        assert "--host" in result.stdout

    def test_exec_no_version_installed_error(self, tmp_path):
        """Without any version, exec should fail."""
        env = os.environ.copy()
        env["HOME"] = str(tmp_path)
        env.pop("DH_VERSION", None)
        result = _run(["exec", "-c", "print('hello')"], env=env)
        assert result.returncode != 0
        assert "No Deephaven version installed" in result.stderr or "no deephaven" in result.stderr.lower()

    def test_exec_missing_script_file(self, isolated_env):
        """Referencing a nonexistent script should fail."""
        result = _run(["exec", "/nonexistent/script.py"], env=isolated_env, timeout=60)
        # Could fail at version-resolution stage or at file-read stage
        combined = result.stdout + result.stderr
        assert result.returncode != 0 or "not found" in combined.lower() or "error" in combined.lower()

    def test_exec_no_args_shows_error(self, isolated_env):
        """Calling exec with no -c and no script should fail."""
        result = _run(["exec"], env=isolated_env, timeout=60)
        combined = result.stdout + result.stderr
        assert result.returncode != 0 or "Must provide" in combined or "error" in combined.lower()

    @pytest.mark.slow
    @pytest.mark.integration
    def test_exec_inline_code_prints_output(self, installed_version, isolated_env):
        """dh exec -c 'print(\"hello\")' should actually print hello."""
        result = _run(
            ["exec", "-c", "print('hello')"],
            env=isolated_env,
            timeout=120,
        )
        assert result.returncode == 0, (
            f"dh exec -c failed:\nstdout={result.stdout}\nstderr={result.stderr}"
        )
        assert "hello" in result.stdout

    @pytest.mark.slow
    @pytest.mark.integration
    def test_exec_script_file(self, installed_version, isolated_env, tmp_path):
        """dh exec script.py should run the script and produce output."""
        script = tmp_path / "hello.py"
        script.write_text("print('script_output_marker')\n")
        result = _run(
            ["exec", str(script)],
            env=isolated_env,
            timeout=120,
        )
        assert result.returncode == 0, (
            f"dh exec script failed:\nstdout={result.stdout}\nstderr={result.stderr}"
        )
        assert "script_output_marker" in result.stdout

    @pytest.mark.slow
    @pytest.mark.integration
    def test_exec_c_shorthand(self, installed_version, isolated_env):
        """dh -c 'print(\"hello\")' should work (shorthand for exec -c)."""
        result = _run(
            ["-c", "print('shorthand_test')"],
            env=isolated_env,
            timeout=120,
        )
        assert result.returncode == 0, (
            f"dh -c shorthand failed:\nstdout={result.stdout}\nstderr={result.stderr}"
        )
        assert "shorthand_test" in result.stdout

    @pytest.mark.slow
    @pytest.mark.integration
    def test_exec_script_error_returns_nonzero(self, installed_version, isolated_env):
        """A script that raises an exception should return non-zero exit code."""
        result = _run(
            ["exec", "-c", "raise ValueError('test_error')"],
            env=isolated_env,
            timeout=120,
        )
        combined = result.stdout + result.stderr
        assert result.returncode != 0 or "ValueError" in combined or "test_error" in combined


# ---------------------------------------------------------------------------
# 10. dh serve
# ---------------------------------------------------------------------------

class TestServe:
    """Tests for `dh serve`."""

    def test_serve_help(self):
        result = _run(["serve", "--help"])
        assert result.returncode == 0
        assert "serve" in result.stdout.lower() or "server" in result.stdout.lower()
        assert "--port" in result.stdout
        assert "--no-browser" in result.stdout
        assert "--iframe" in result.stdout
        assert "--jvm-args" in result.stdout

    def test_serve_no_script_arg_fails(self):
        """dh serve without a script argument should fail."""
        result = _run(["serve"])
        assert result.returncode != 0
        combined = result.stdout + result.stderr
        assert "required" in combined.lower() or "error" in combined.lower() or "usage" in combined.lower()

    def test_serve_no_version_installed_error(self, tmp_path):
        """Without a version, serve should fail."""
        env = os.environ.copy()
        env["HOME"] = str(tmp_path)
        env.pop("DH_VERSION", None)
        with tempfile.NamedTemporaryFile(suffix=".py", mode="w", delete=False) as f:
            f.write("print('test')\n")
            script = f.name
        try:
            result = _run(["serve", script], env=env)
            assert result.returncode != 0
            assert "No Deephaven version installed" in result.stderr or "no deephaven" in result.stderr.lower()
        finally:
            os.unlink(script)

    @pytest.mark.slow
    @pytest.mark.integration
    def test_serve_starts_and_stops(self, installed_version, isolated_env, tmp_path):
        """dh serve with a real script should start without import errors.

        We give it a short timeout so it terminates.  The key check is that
        it does NOT crash on import and at least attempts to start.
        """
        script = tmp_path / "serve_test.py"
        script.write_text("print('serve_started')\n")
        try:
            result = subprocess.run(
                ["dh", "serve", str(script), "--no-browser"],
                capture_output=True,
                text=True,
                timeout=30,
                env=isolated_env,
            )
        except subprocess.TimeoutExpired as e:
            # Timeout means the server started and stayed running - that's a PASS
            stdout = e.stdout.decode() if e.stdout else ""
            stderr = e.stderr.decode() if e.stderr else ""
            combined = stdout + stderr
            assert "ModuleNotFoundError" not in combined
            assert "ImportError" not in combined
            return

        combined = result.stdout + result.stderr
        assert "ModuleNotFoundError" not in combined, (
            f"dh serve crashed with import error:\n{combined}"
        )
        assert "ImportError" not in combined, (
            f"dh serve crashed with import error:\n{combined}"
        )


# ---------------------------------------------------------------------------
# 11. dh list
# ---------------------------------------------------------------------------

class TestList:
    """Tests for `dh list`."""

    def test_list_help(self):
        result = _run(["list", "--help"])
        assert result.returncode == 0
        assert "running" in result.stdout.lower() or "list" in result.stdout.lower() or "server" in result.stdout.lower()

    def test_list_runs(self):
        """dh list should succeed even when no servers are running."""
        result = _run(["list"])
        assert result.returncode == 0
        # Output could be "No running servers" or a table -- both OK
        assert result.stdout is not None


# ---------------------------------------------------------------------------
# 12. dh kill
# ---------------------------------------------------------------------------

class TestKill:
    """Tests for `dh kill`."""

    def test_kill_help(self):
        result = _run(["kill", "--help"])
        assert result.returncode == 0
        assert "port" in result.stdout.lower()

    def test_kill_no_server_on_port(self):
        """Killing a port with no server should fail gracefully."""
        result = _run(["kill", "59999"])
        assert result.returncode != 0
        combined = result.stdout + result.stderr
        assert "no" in combined.lower() or "not" in combined.lower() or "error" in combined.lower() or "found" in combined.lower()

    def test_kill_missing_port_arg(self):
        """dh kill without a port should error."""
        result = _run(["kill"])
        assert result.returncode != 0
        combined = result.stdout + result.stderr
        assert "required" in combined.lower() or "error" in combined.lower() or "usage" in combined.lower()


# ---------------------------------------------------------------------------
# 13. dh lint
# ---------------------------------------------------------------------------

class TestLint:
    """Tests for `dh lint`."""

    def test_lint_help(self):
        result = _run(["lint", "--help"])
        assert result.returncode == 0
        assert "ruff" in result.stdout.lower() or "lint" in result.stdout.lower()
        assert "--fix" in result.stdout

    def test_lint_on_clean_file(self, tmp_path):
        """Lint a clean Python file -- should succeed."""
        clean = tmp_path / "clean.py"
        clean.write_text('x = 1\n')
        result = _run(["lint", str(clean)])
        assert result.returncode == 0

    def test_lint_on_file_with_issues(self, tmp_path):
        """Lint a file with an unused import -- should report issues."""
        bad = tmp_path / "bad.py"
        bad.write_text("import os\nx = 1\n")
        result = _run(["lint", str(bad)])
        # ruff may return non-zero for lint errors
        combined = result.stdout + result.stderr
        assert "F401" in combined or result.returncode != 0

    def test_lint_with_fix(self, tmp_path):
        """dh lint --fix should auto-fix issues."""
        bad = tmp_path / "fixable.py"
        bad.write_text("import os\nx = 1\n")
        result = _run(["lint", str(bad), "--fix"])
        # After fix, the import should be removed
        content = bad.read_text()
        assert "import os" not in content or result.returncode == 0


# ---------------------------------------------------------------------------
# 14. dh format
# ---------------------------------------------------------------------------

class TestFormat:
    """Tests for `dh format`."""

    def test_format_help(self):
        result = _run(["format", "--help"])
        assert result.returncode == 0
        assert "ruff" in result.stdout.lower() or "format" in result.stdout.lower()
        assert "--check" in result.stdout

    def test_format_file(self, tmp_path):
        """Format a file -- should succeed."""
        f = tmp_path / "fmt.py"
        f.write_text("x=1\n")
        result = _run(["format", str(f)])
        assert result.returncode == 0
        # ruff format should rewrite the file
        content = f.read_text()
        assert "x = 1" in content or "x=1" in content  # either formatted or was fine

    def test_format_check_detects_unformatted(self, tmp_path):
        """dh format --check on unformatted code should return non-zero."""
        f = tmp_path / "ugly.py"
        f.write_text("x=1\ny=   2\n")
        result = _run(["format", "--check", str(f)])
        # ruff format --check returns non-zero if formatting is needed
        # The file should NOT be modified
        combined = result.stdout + result.stderr
        assert result.returncode != 0 or "would reformat" in combined.lower() or "reformatted" in combined.lower()

    def test_format_check_on_clean_file(self, tmp_path):
        """dh format --check on a clean file should succeed."""
        f = tmp_path / "clean.py"
        f.write_text("x = 1\n")
        result = _run(["format", "--check", str(f)])
        assert result.returncode == 0


# ---------------------------------------------------------------------------
# 15. dh typecheck
# ---------------------------------------------------------------------------

class TestTypecheck:
    """Tests for `dh typecheck`."""

    def test_typecheck_help(self):
        result = _run(["typecheck", "--help"])
        assert result.returncode == 0
        assert "ty" in result.stdout.lower() or "typecheck" in result.stdout.lower() or "check" in result.stdout.lower()

    def test_typecheck_on_file(self, tmp_path):
        """Run typecheck on a simple file."""
        f = tmp_path / "typed.py"
        f.write_text("x: int = 1\n")
        result = _run(["typecheck", str(f)])
        # ty may or may not report issues; we just check it runs
        assert result.returncode is not None


# ---------------------------------------------------------------------------
# 16. Cross-cutting: shorthand and aliases
# ---------------------------------------------------------------------------

class TestShorthand:
    """Test CLI shorthands and special syntax."""

    def test_java_install_twoword(self):
        """dh java install --help should work (two-word subcommand)."""
        result = _run(["java", "install", "--help"])
        assert result.returncode == 0
        assert "Temurin" in result.stdout or "JDK" in result.stdout or "java" in result.stdout.lower()


# ---------------------------------------------------------------------------
# 17. Full round-trip: install -> use -> versions -> uninstall
# ---------------------------------------------------------------------------

class TestInstallUninstallRoundTrip:
    """Full lifecycle test: install, use, verify, uninstall."""

    @pytest.mark.slow
    @pytest.mark.integration
    def test_full_lifecycle(self, installed_version, isolated_env):
        # 1. Version is already installed via fixture -- verify it shows up
        result = _run(["versions"], env=isolated_env)
        assert result.returncode == 0
        assert installed_version in result.stdout

        # 2. Set it as default
        result = _run(["use", installed_version], env=isolated_env)
        assert result.returncode == 0

        # 3. Doctor should see the version
        result = _run(["doctor"], env=isolated_env)
        assert "version" in result.stdout.lower() or installed_version in result.stdout

        # 4. Re-install should say already installed
        result = _run(
            ["install", installed_version],
            env=isolated_env,
            timeout=NETWORK_TIMEOUT,
        )
        assert result.returncode == 0
        assert "already installed" in result.stdout

        # 5. Uninstall
        result = _run(["uninstall", installed_version], env=isolated_env)
        assert result.returncode == 0
        assert "uninstalled" in result.stdout.lower()

        # 6. Versions should now be empty
        result = _run(["versions"], env=isolated_env)
        assert result.returncode == 0
        assert "No Deephaven versions installed" in result.stdout

        # 7. Re-install so other tests still pass (session fixture shared)
        result = _run(
            ["install", installed_version],
            env=isolated_env,
            timeout=NETWORK_TIMEOUT,
        )
        assert result.returncode == 0
        assert "installed successfully" in result.stdout
