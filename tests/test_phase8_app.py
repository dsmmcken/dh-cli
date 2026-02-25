"""Tests for Phase 8: App mode (long-running applications).

App mode tests are tricky because the process stays alive.
We use timeouts and signals to test behavior.
"""
import pytest
import subprocess
import sys
import signal
import tempfile
import os
import time


@pytest.mark.integration
class TestAppMode:
    """Integration tests for dh app command."""

    def test_app_file_not_found(self):
        """Test app with nonexistent file returns exit code 2."""
        result = subprocess.run(
            ["dh", "app", "/nonexistent/script.py"],
            capture_output=True,
            text=True,
            timeout=30,
        )
        assert result.returncode == 2
        assert "not found" in result.stderr.lower()

    def test_app_script_error(self):
        """Test app with script error returns exit code 1."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.py', delete=False) as f:
            f.write('raise ValueError("startup error")\n')
            f.flush()
            try:
                result = subprocess.run(
                    ["dh", "app", f.name],
                    capture_output=True,
                    text=True,
                    timeout=120,
                )
                assert result.returncode == 1
                # Error should be reported
                assert "error" in result.stderr.lower() or "ValueError" in result.stderr
            finally:
                os.unlink(f.name)

    def test_app_starts_and_runs_script(self):
        """Test app starts server and runs script, then stays alive."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.py', delete=False) as f:
            f.write('print("App started successfully")\n')
            f.flush()
            try:
                proc = subprocess.Popen(
                    ["dh", "app", f.name],
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    text=True,
                )
                # Wait for startup (give it time)
                time.sleep(30)

                # Process should still be running
                assert proc.poll() is None, "App should stay alive"

                # Send SIGTERM to shut down
                proc.terminate()
                stdout, stderr = proc.communicate(timeout=30)

                # Should have printed startup message
                combined = stdout + stderr
                assert "App started successfully" in combined or "Server" in combined
            finally:
                os.unlink(f.name)
                # Ensure cleanup
                if proc.poll() is None:
                    proc.kill()

    @pytest.mark.skipif(sys.platform == "win32", reason="SIGINT handling differs on Windows")
    def test_app_sigint_clean_shutdown(self):
        """Test Ctrl+C (SIGINT) causes clean shutdown."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.py', delete=False) as f:
            f.write('print("Running")\n')
            f.flush()
            try:
                proc = subprocess.Popen(
                    ["dh", "app", f.name],
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    text=True,
                )
                # Wait for startup
                time.sleep(30)

                # Send SIGINT (like Ctrl+C)
                proc.send_signal(signal.SIGINT)

                # Should exit cleanly
                stdout, stderr = proc.communicate(timeout=30)
                # Exit code 130 (128 + SIGINT=2) or 0 are both acceptable
                assert proc.returncode in [0, 130, -2], f"Unexpected exit code: {proc.returncode}"
            finally:
                os.unlink(f.name)
                # Ensure cleanup
                if proc.poll() is None:
                    proc.kill()
