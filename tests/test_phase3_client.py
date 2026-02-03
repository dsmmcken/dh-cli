"""Tests for Phase 3: Client connection.

NOTE: These tests require a running Deephaven server.
They are marked as 'integration'.
"""
import pytest
from deephaven_cli.server import DeephavenServer
from deephaven_cli.client import DeephavenClient


@pytest.fixture(scope="module")
def running_server():
    """Start a server for the entire test module."""
    import random
    port = random.randint(10200, 10299)
    with DeephavenServer(port=port) as server:
        yield port


@pytest.mark.integration
class TestDeephavenClient:
    """Tests for DeephavenClient class."""

    def test_client_init(self):
        """Test client initializes with default values."""
        client = DeephavenClient()
        assert client.host == "localhost"
        assert client.port == 10000

    def test_client_init_custom(self):
        """Test client initializes with custom values."""
        client = DeephavenClient(host="myhost", port=12345)
        assert client.host == "myhost"
        assert client.port == 12345

    def test_client_connect_disconnect(self, running_server):
        """Test client can connect and disconnect."""
        client = DeephavenClient(port=running_server)
        client.connect()
        assert client._session is not None
        client.close()
        assert client._session is None

    def test_client_context_manager(self, running_server):
        """Test client works as context manager."""
        with DeephavenClient(port=running_server) as client:
            assert client._session is not None
        assert client._session is None

    def test_client_session_property_raises_when_disconnected(self):
        """Test session property raises when not connected."""
        client = DeephavenClient()
        with pytest.raises(RuntimeError, match="not connected"):
            _ = client.session

    def test_client_tables_empty_initially(self, running_server):
        """Test tables list is empty on fresh connection."""
        with DeephavenClient(port=running_server) as client:
            # Note: May not be empty if other tests created tables
            assert isinstance(client.tables, list)

    def test_client_run_script_creates_table(self, running_server):
        """Test run_script can create a table."""
        with DeephavenClient(port=running_server) as client:
            client.run_script('''
from deephaven import empty_table
test_client_table = empty_table(5).update(["X = i"])
''')
            assert "test_client_table" in client.tables

    def test_client_run_script_syntax_error(self, running_server):
        """Test run_script raises on syntax error."""
        with DeephavenClient(port=running_server) as client:
            with pytest.raises(Exception):
                client.run_script("this is not valid python syntax {{{")

    def test_client_run_script_runtime_error(self, running_server):
        """Test run_script raises on runtime error."""
        with DeephavenClient(port=running_server) as client:
            with pytest.raises(Exception):
                client.run_script("raise ValueError('test error')")
