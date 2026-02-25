"""Shared pytest fixtures for deephaven-cli tests."""
import pytest
import random


@pytest.fixture(scope="session")
def test_port_range():
    """Return a function to get unique ports for testing."""
    used_ports = set()

    def get_port():
        while True:
            port = random.randint(10100, 10999)
            if port not in used_ports:
                used_ports.add(port)
                return port
    return get_port
