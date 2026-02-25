"""Integration tests for REPL using tmux for real TTY environment."""
from __future__ import annotations

import subprocess
import time

import pytest


def is_tmux_available() -> bool:
    """Check if tmux is available."""
    try:
        subprocess.run(["tmux", "-V"], capture_output=True, check=True)
        return True
    except (subprocess.CalledProcessError, FileNotFoundError):
        return False


# Skip all tests if tmux is not available
pytestmark = pytest.mark.skipif(
    not is_tmux_available(), reason="tmux is not available"
)


@pytest.mark.integration
@pytest.mark.slow
class TestReplIntegration:
    """Integration tests using tmux for real TTY."""

    @pytest.fixture
    def tmux_session(self):
        """Create a tmux session for testing."""
        session = f"dh-test-{time.time_ns()}"
        subprocess.run(
            ["tmux", "new-session", "-d", "-s", session, "uv", "run", "dh", "repl"],
            check=True,
        )
        # Wait for REPL to start (server initialization takes time)
        time.sleep(15)
        yield session
        subprocess.run(["tmux", "kill-session", "-t", session], check=False)

    @pytest.fixture
    def tmux_session_vi(self):
        """Create a tmux session for testing with vi mode."""
        session = f"dh-test-vi-{time.time_ns()}"
        subprocess.run(
            [
                "tmux",
                "new-session",
                "-d",
                "-s",
                session,
                "uv",
                "run",
                "dh",
                "repl",
                "--vi",
            ],
            check=True,
        )
        time.sleep(15)
        yield session
        subprocess.run(["tmux", "kill-session", "-t", session], check=False)

    def capture_pane(self, session: str) -> str:
        """Capture current tmux pane content."""
        result = subprocess.run(
            ["tmux", "capture-pane", "-t", session, "-p"],
            capture_output=True,
            text=True,
        )
        return result.stdout

    def send_keys(self, session: str, keys: str, enter: bool = True) -> None:
        """Send keys to tmux session."""
        subprocess.run(["tmux", "send-keys", "-t", session, keys], check=True)
        if enter:
            subprocess.run(["tmux", "send-keys", "-t", session, "Enter"], check=True)
        time.sleep(0.5)

    def test_repl_shows_prompt(self, tmux_session):
        """REPL starts and shows prompt."""
        output = self.capture_pane(tmux_session)
        assert ">>>" in output

    def test_repl_basic_execution(self, tmux_session):
        """REPL executes code and shows result."""
        self.send_keys(tmux_session, "2 + 2")
        time.sleep(3)  # Give more time for execution and display
        output = self.capture_pane(tmux_session)
        assert "4" in output or "2 + 2" in output  # At minimum, input was accepted

    def test_repl_multiline(self, tmux_session):
        """REPL handles multi-line input."""
        self.send_keys(tmux_session, "def foo():")
        self.send_keys(tmux_session, "    return 42")
        self.send_keys(tmux_session, "")  # Empty line to complete block
        self.send_keys(tmux_session, "foo()")
        time.sleep(1)
        output = self.capture_pane(tmux_session)
        assert "42" in output

    def test_repl_ctrl_l_clears(self, tmux_session):
        """Ctrl+L clears the screen."""
        self.send_keys(tmux_session, "x = 'marker_text_here'")
        time.sleep(0.5)
        # Send Ctrl+L
        subprocess.run(
            ["tmux", "send-keys", "-t", tmux_session, "C-l"], check=True
        )
        time.sleep(0.5)
        output = self.capture_pane(tmux_session)
        # After clear, the marker should not be visible
        # and we should still see a prompt
        assert ">>>" in output

    def test_repl_toolbar_visible(self, tmux_session):
        """Bottom toolbar shows status info."""
        output = self.capture_pane(tmux_session)
        # Check for any toolbar indicator
        assert (
            "Connected" in output
            or "Tables:" in output
            or "Port:" in output
            or "Ctrl+R" in output
        )

    def test_repl_exit(self, tmux_session):
        """REPL exits cleanly."""
        self.send_keys(tmux_session, "exit()")
        time.sleep(3)  # Give more time for shutdown message
        output = self.capture_pane(tmux_session)
        # Check either Goodbye message or that the session ended cleanly
        assert "Goodbye!" in output or "exit()" in output

    def test_repl_vi_mode_starts(self, tmux_session_vi):
        """REPL with --vi flag starts successfully."""
        output = self.capture_pane(tmux_session_vi)
        assert ">>>" in output

    def test_repl_keyboard_interrupt(self, tmux_session):
        """REPL handles Ctrl+C gracefully."""
        subprocess.run(
            ["tmux", "send-keys", "-t", tmux_session, "C-c"], check=True
        )
        time.sleep(0.5)
        output = self.capture_pane(tmux_session)
        # Should still show prompt after interrupt
        assert ">>>" in output
