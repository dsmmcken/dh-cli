"""Deephaven client wrapper."""
from __future__ import annotations

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from pydeephaven import Session


class DeephavenClient:
    """Client for communicating with Deephaven server."""

    def __init__(self, host: str = "localhost", port: int = 10000):
        self.host = host
        self.port = port
        self._session: Session | None = None

    def connect(self) -> DeephavenClient:
        """Connect to the Deephaven server."""
        from pydeephaven import Session

        self._session = Session(host=self.host, port=self.port)
        return self

    def close(self) -> None:
        """Close the client connection."""
        if self._session:
            self._session.close()
            self._session = None

    @property
    def session(self) -> Session:
        """Get the underlying session."""
        if not self._session:
            raise RuntimeError("Client not connected")
        return self._session

    @property
    def tables(self) -> list[str]:
        """Get list of available table names."""
        return list(self.session.tables)

    def run_script(self, script: str) -> None:
        """Run a script on the server."""
        self.session.run_script(script)

    def __enter__(self) -> DeephavenClient:
        return self.connect()

    def __exit__(self, exc_type, exc_val, exc_tb) -> None:
        self.close()
