"""Tests for Deephaven server discovery."""
import os
import subprocess
import signal
import tempfile
import time
from unittest.mock import patch, MagicMock

import pytest

from deephaven_cli.discovery import (
    ServerInfo,
    _ListeningSocket,
    _classify_process,
    _parse_docker_ps_output,
    _parse_proc_net_tcp,
    format_server_list,
    kill_server,
)


# ---------------------------------------------------------------------------
# Unit tests: /proc/net/tcp parsing
# ---------------------------------------------------------------------------

PROC_NET_TCP_HEADER = "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode"


class TestParseProcNetTcp:
    def test_listen_entry(self):
        content = (
            f"{PROC_NET_TCP_HEADER}\n"
            "   0: 00000000:2710 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 1163600 1 00000000 100 0 0 10 0"
        )
        result = _parse_proc_net_tcp(content)
        assert len(result) == 1
        assert result[0].port == 10000
        assert result[0].inode == 1163600

    def test_skips_established(self):
        content = (
            f"{PROC_NET_TCP_HEADER}\n"
            "   0: 0100007F:2710 0100007F:9C40 01 00000000:00000000 00:00000000 00000000  1000        0 123456 1 00000000 100 0 0 10 0"
        )
        result = _parse_proc_net_tcp(content)
        assert len(result) == 0

    def test_skips_zero_inode(self):
        content = (
            f"{PROC_NET_TCP_HEADER}\n"
            "   0: 00000000:2710 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 0 1 00000000 100 0 0 10 0"
        )
        result = _parse_proc_net_tcp(content)
        assert len(result) == 0

    def test_multiple_entries(self):
        content = (
            f"{PROC_NET_TCP_HEADER}\n"
            "   0: 00000000:2710 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 100 1 00000000 100 0 0 10 0\n"
            "   1: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 200 1 00000000 100 0 0 10 0\n"
            "   2: 0100007F:ABCD 0100007F:2710 01 00000000:00000000 00:00000000 00000000  1000        0 300 1 00000000 100 0 0 10 0"
        )
        result = _parse_proc_net_tcp(content)
        assert len(result) == 2
        assert result[0].port == 10000
        assert result[1].port == 8080

    def test_hex_port_decoding(self):
        content = (
            f"{PROC_NET_TCP_HEADER}\n"
            "   0: 00000000:6EC1 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 999 1 00000000 100 0 0 10 0"
        )
        result = _parse_proc_net_tcp(content)
        assert result[0].port == 28353

    def test_empty_content(self):
        content = PROC_NET_TCP_HEADER
        result = _parse_proc_net_tcp(content)
        assert len(result) == 0

    def test_tcp6_format(self):
        # IPv6 addresses are 32 hex chars
        content = (
            f"{PROC_NET_TCP_HEADER}\n"
            "   0: 00000000000000000000000000000000:2710 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 555 1 00000000 100 0 0 10 0"
        )
        result = _parse_proc_net_tcp(content)
        assert len(result) == 1
        assert result[0].port == 10000


# ---------------------------------------------------------------------------
# Unit tests: process classification
# ---------------------------------------------------------------------------

class TestClassifyProcess:
    def test_dh_serve(self):
        info = _classify_process("dh", "dh\x00serve\x00dashboard.py")
        assert info is not None
        assert info["source"] == "dh serve"
        assert info["script"] == "dashboard.py"

    def test_dh_serve_with_flags(self):
        info = _classify_process("dh", "dh\x00serve\x00--port\x0010001\x00dashboard.py")
        assert info is not None
        assert info["source"] == "dh serve"
        assert info["script"] == "dashboard.py"

    def test_dh_serve_with_path(self):
        info = _classify_process("dh", "dh\x00serve\x00/home/user/scripts/dashboard.py")
        assert info is not None
        assert info["script"] == "dashboard.py"

    def test_dh_repl(self):
        info = _classify_process("dh", "dh\x00repl")
        assert info is not None
        assert info["source"] == "dh repl"
        assert info["script"] is None

    def test_dh_repl_via_python_path(self):
        """When installed via uv, cmdline is: python /path/to/dh repl"""
        info = _classify_process("dh", "/usr/bin/python\x00/home/user/.local/bin/dh\x00repl")
        assert info is not None
        assert info["source"] == "dh repl"

    def test_dh_serve_via_python_path(self):
        info = _classify_process("dh", "/usr/bin/python\x00/home/user/.local/bin/dh\x00serve\x00app.py")
        assert info is not None
        assert info["source"] == "dh serve"
        assert info["script"] == "app.py"

    def test_dh_exec(self):
        info = _classify_process("dh", "dh\x00exec\x00script.py")
        assert info is not None
        assert info["source"] == "dh exec"
        assert info["script"] == "script.py"

    def test_java_deephaven(self):
        info = _classify_process("java", "java\x00-cp\x00deephaven.jar\x00io.deephaven.server.jetty.JettyMain")
        assert info is not None
        assert info["source"] == "java"
        assert info["script"] is None

    def test_unrelated_java(self):
        info = _classify_process("java", "java\x00-cp\x00app.jar\x00com.example.Main")
        assert info is None

    def test_unrelated_node(self):
        info = _classify_process("node", "node\x00server.js")
        assert info is None

    def test_unrelated_python(self):
        info = _classify_process("python3", "python3\x00app.py")
        assert info is None


# ---------------------------------------------------------------------------
# Unit tests: docker ps parsing
# ---------------------------------------------------------------------------

class TestParseDockerPsOutput:
    def test_deephaven_container(self):
        output = "addb8c15017f\tdeephaven/server-jetty:local-build\t0.0.0.0:10000->10000/tcp, :::10000->10000/tcp"
        result = _parse_docker_ps_output(output)
        assert len(result) == 1
        assert result[0].port == 10000
        assert result[0].source == "docker"
        assert result[0].script == "deephaven/server-jetty:local-build"
        assert result[0].container_id == "addb8c15017f"

    def test_non_deephaven_container(self):
        output = "abc123\tnginx:latest\t0.0.0.0:8080->80/tcp"
        result = _parse_docker_ps_output(output)
        assert len(result) == 0

    def test_custom_port_mapping(self):
        output = "abc123\tdeephaven/server:latest\t0.0.0.0:8080->10000/tcp"
        result = _parse_docker_ps_output(output)
        assert len(result) == 1
        assert result[0].port == 8080

    def test_no_port_mapping(self):
        output = "abc123\tdeephaven/server:latest\t"
        result = _parse_docker_ps_output(output)
        assert len(result) == 0

    def test_multiple_containers(self):
        output = (
            "aaa\tdeephaven/server:latest\t0.0.0.0:10000->10000/tcp\n"
            "bbb\tnginx:latest\t0.0.0.0:80->80/tcp\n"
            "ccc\tdeephaven/server:v2\t0.0.0.0:10001->10000/tcp"
        )
        result = _parse_docker_ps_output(output)
        assert len(result) == 2
        assert result[0].port == 10000
        assert result[1].port == 10001

    def test_empty_output(self):
        result = _parse_docker_ps_output("")
        assert len(result) == 0

    def test_dedup_ipv4_ipv6(self):
        """Should only report one entry even when both IPv4 and IPv6 mappings exist."""
        output = "addb8c15017f\tdeephaven/server:latest\t0.0.0.0:10000->10000/tcp, :::10000->10000/tcp"
        result = _parse_docker_ps_output(output)
        assert len(result) == 1


# ---------------------------------------------------------------------------
# Unit tests: output formatting
# ---------------------------------------------------------------------------

class TestFormatServerList:
    def test_empty(self):
        result = format_server_list([])
        assert "No Deephaven servers found" in result

    def test_single_server_with_script(self):
        """Script path is preferred over cwd for LOCATION."""
        servers = [ServerInfo(port=10000, pid=123, source="dh serve", script="app.py", cwd="/home/user")]
        result = format_server_list(servers)
        assert "10000" in result
        assert "dh serve" in result
        assert "app.py" in result

    def test_falls_back_to_cwd(self):
        """When no script, LOCATION shows cwd."""
        servers = [ServerInfo(port=10000, pid=1, source="dh repl", cwd="/home/user")]
        result = format_server_list(servers)
        assert "/home/user" in result

    def test_multiple_servers_sorted_by_port(self):
        servers = [
            ServerInfo(port=10002, pid=3, source="dh repl"),
            ServerInfo(port=10000, pid=1, source="dh serve", script="a.py"),
            ServerInfo(port=10001, pid=2, source="java"),
        ]
        result = format_server_list(servers)
        lines = result.strip().splitlines()
        assert len(lines) == 4  # header + 3 rows
        assert "10000" in lines[1]
        assert "10002" in lines[3]

    def test_missing_fields_show_dash(self):
        servers = [ServerInfo(port=10000, pid=1, source="java")]
        result = format_server_list(servers)
        assert "-" in result

    def test_has_header(self):
        servers = [ServerInfo(port=10000, pid=1, source="dh serve")]
        result = format_server_list(servers)
        assert "PORT" in result
        assert "PID" in result
        assert "SOURCE" in result
        assert "LOCATION" in result


# ---------------------------------------------------------------------------
# Unit tests: kill_server
# ---------------------------------------------------------------------------

class TestKillServer:
    def test_port_not_found(self):
        with patch("deephaven_cli.discovery.discover_servers", return_value=[]):
            success, msg = kill_server(99999)
            assert not success
            assert "No Deephaven server found on port 99999" in msg

    def test_kill_dh_serve_sends_sigterm(self):
        server = ServerInfo(port=10000, pid=12345, source="dh serve")
        with patch("deephaven_cli.discovery.discover_servers", return_value=[server]), \
             patch("os.kill") as mock_kill:
            success, msg = kill_server(10000)
            assert success
            assert "dh serve" in msg
            mock_kill.assert_called_once_with(12345, signal.SIGTERM)

    def test_kill_dh_repl_sends_sigterm(self):
        """REPL handles SIGTERM via handler registered in console.interact()."""
        server = ServerInfo(port=10000, pid=12345, source="dh repl")
        with patch("deephaven_cli.discovery.discover_servers", return_value=[server]), \
             patch("os.kill") as mock_kill:
            success, msg = kill_server(10000)
            assert success
            assert "dh repl" in msg
            mock_kill.assert_called_once_with(12345, signal.SIGTERM)

    def test_kill_docker_container(self):
        server = ServerInfo(port=10000, pid=0, source="docker", container_id="abc123")
        mock_result = MagicMock(returncode=0)
        with patch("deephaven_cli.discovery.discover_servers", return_value=[server]), \
             patch("subprocess.run", return_value=mock_result) as mock_run:
            success, msg = kill_server(10000)
            assert success
            assert "docker" in msg
            mock_run.assert_called_once_with(
                ["docker", "stop", "abc123"],
                capture_output=True, text=True, timeout=30,
            )

    def test_kill_docker_failure(self):
        server = ServerInfo(port=10000, pid=0, source="docker", container_id="abc123")
        mock_result = MagicMock(returncode=1, stderr="error msg")
        with patch("deephaven_cli.discovery.discover_servers", return_value=[server]), \
             patch("subprocess.run", return_value=mock_result):
            success, msg = kill_server(10000)
            assert not success
            assert "Failed" in msg

    def test_kill_process_already_exited(self):
        server = ServerInfo(port=10000, pid=12345, source="dh repl")
        with patch("deephaven_cli.discovery.discover_servers", return_value=[server]), \
             patch("os.kill", side_effect=ProcessLookupError):
            success, msg = kill_server(10000)
            assert not success
            assert "not found" in msg

    def test_kill_permission_denied(self):
        server = ServerInfo(port=10000, pid=12345, source="java")
        with patch("deephaven_cli.discovery.discover_servers", return_value=[server]), \
             patch("os.kill", side_effect=PermissionError):
            success, msg = kill_server(10000)
            assert not success
            assert "Permission denied" in msg


# ---------------------------------------------------------------------------
# Integration tests (require running server)
# ---------------------------------------------------------------------------

@pytest.mark.integration
class TestListIntegration:
    def test_list_finds_running_serve(self):
        """Start dh serve, then verify dh list finds it."""
        with tempfile.NamedTemporaryFile(mode="w", suffix=".py", delete=False) as f:
            f.write('print("running")\n')
            f.flush()
            script_path = f.name

        try:
            proc = subprocess.Popen(
                ["dh", "serve", script_path, "--no-browser"],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
            )
            # Wait for server startup
            time.sleep(30)

            # Server should still be running
            assert proc.poll() is None, "Server should still be running"

            result = subprocess.run(
                ["dh", "list"],
                capture_output=True,
                text=True,
                timeout=10,
            )
            assert result.returncode == 0
            assert "dh serve" in result.stdout
        finally:
            os.unlink(script_path)
            if proc.poll() is None:
                proc.send_signal(signal.SIGINT)
                try:
                    proc.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    proc.kill()

    def test_list_empty_when_no_servers(self):
        """dh list should succeed even with no servers running."""
        result = subprocess.run(
            ["dh", "list"],
            capture_output=True,
            text=True,
            timeout=10,
        )
        assert result.returncode == 0
