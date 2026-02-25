"""Deephaven client wrapper."""
from __future__ import annotations

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from pydeephaven import Session


class DeephavenClient:
    """Client for communicating with Deephaven server."""

    def __init__(
        self,
        host: str = "localhost",
        port: int = 10000,
        auth_type: str = "Anonymous",
        auth_token: str = "",
        use_tls: bool = False,
        tls_root_certs: bytes | None = None,
        client_cert_chain: bytes | None = None,
        client_private_key: bytes | None = None,
    ):
        self.host = host
        self.port = port
        self.auth_type = auth_type
        self.auth_token = auth_token
        self.use_tls = use_tls
        self.tls_root_certs = tls_root_certs
        self.client_cert_chain = client_cert_chain
        self.client_private_key = client_private_key
        self._session: Session | None = None

    def connect(self) -> DeephavenClient:
        """Connect to the Deephaven server."""
        from pydeephaven import Session

        self._session = Session(
            host=self.host,
            port=self.port,
            auth_type=self.auth_type,
            auth_token=self.auth_token,
            use_tls=self.use_tls,
            tls_root_certs=self.tls_root_certs,
            client_cert_chain=self.client_cert_chain,
            client_private_key=self.client_private_key,
        )
        return self

    @property
    def is_connected(self) -> bool:
        """Check if the client has an active session."""
        return self._session is not None and self._session.is_connected

    def close(self) -> None:
        """Close the client connection."""
        if self._session:
            try:
                self._session.close()
            except Exception:
                self._force_cleanup_session()
            self._session = None

    def force_disconnect(self) -> None:
        """Force-disconnect without sending any RPCs.

        Immediately stops the keep-alive daemon thread and closes the gRPC
        channel. Use this when the server is known to be dead.
        """
        if self._session:
            self._force_cleanup_session()
            self._session = None

    def _force_cleanup_session(self) -> None:
        """Stop keep-alive timer and close transport on the current session."""
        s = self._session
        if s is None:
            return
        s.is_connected = False
        timer = getattr(s, "_keep_alive_timer", None)
        if timer is not None:
            timer.cancel()
            s._keep_alive_timer = None
        try:
            ch = getattr(s, "grpc_channel", None)
            if ch:
                ch.close()
        except Exception:
            pass
        try:
            fc = getattr(s, "_flight_client", None)
            if fc:
                fc.close()
        except Exception:
            pass

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
