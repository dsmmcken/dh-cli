"""Deephaven server lifecycle management."""
from __future__ import annotations

import atexit
import os
import sys
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from deephaven_server import Server


class DeephavenServer:
    """Manages an embedded Deephaven server lifecycle."""

    def __init__(self, port: int = 10000, jvm_args: list[str] | None = None, quiet: bool = False):
        self.port = port
        self.jvm_args = jvm_args or ["-Xmx4g"]
        self.quiet = quiet
        self._server: Server | None = None
        self._started = False

    def start(self) -> DeephavenServer:
        """Start the Deephaven server."""
        if self._started:
            raise RuntimeError("Server already started")

        # Import here to avoid JVM initialization on import
        from deephaven_server import Server

        # Suppress JVM/server output when quiet mode is enabled
        # Must redirect at both file descriptor level (JVM writes to fd 1/2)
        # and Python level (Server.start() sets up TeeStreams)
        if self.quiet:
            # Save original file descriptors
            original_stdout_fd = os.dup(1)
            original_stderr_fd = os.dup(2)
            # Save original Python streams
            original_stdout = sys.stdout
            original_stderr = sys.stderr
            # Open /dev/null
            devnull_fd = os.open(os.devnull, os.O_WRONLY)
            devnull_file = open(os.devnull, 'w')
            # Redirect at fd level (for JVM)
            os.dup2(devnull_fd, 1)
            os.dup2(devnull_fd, 2)
            os.close(devnull_fd)
            # Redirect Python streams
            sys.stdout = devnull_file
            sys.stderr = devnull_file

        try:
            self._server = Server(port=self.port, jvm_args=self.jvm_args)
            self._server.start()
        finally:
            if self.quiet:
                # Restore original file descriptors
                os.dup2(original_stdout_fd, 1)
                os.dup2(original_stderr_fd, 2)
                os.close(original_stdout_fd)
                os.close(original_stderr_fd)
                # Restore Python streams (Server.start() may have changed these to TeeStreams)
                sys.stdout = original_stdout
                sys.stderr = original_stderr
                devnull_file.close()

        self._started = True

        # Register cleanup on exit
        atexit.register(self._cleanup)

        return self

    def stop(self) -> None:
        """Stop the Deephaven server."""
        if not self._started:
            return

        self._started = False
        # Note: deephaven_server.Server doesn't have explicit stop
        # The JVM will be cleaned up on process exit
        # Unregister atexit handler since we're stopping explicitly
        try:
            atexit.unregister(self._cleanup)
        except Exception:
            pass

    def _cleanup(self) -> None:
        """Cleanup handler for atexit."""
        self.stop()

    def __enter__(self) -> DeephavenServer:
        return self.start()

    def __exit__(self, exc_type, exc_val, exc_tb) -> None:
        self.stop()

    @property
    def is_running(self) -> bool:
        """Check if server is running."""
        return self._started
