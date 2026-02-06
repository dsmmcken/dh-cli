"""Discover running Deephaven servers on the local machine."""
from __future__ import annotations

import os
import re
import signal
import subprocess
import sys
from dataclasses import dataclass


@dataclass
class ServerInfo:
    """Information about a discovered Deephaven server."""
    port: int
    pid: int
    source: str  # "dh serve", "dh repl", "dh exec", "docker", "java"
    script: str | None = None
    cwd: str | None = None
    container_id: str | None = None


@dataclass
class _ListeningSocket:
    """A listening TCP socket parsed from /proc/net/tcp."""
    port: int
    inode: int


def discover_servers() -> list[ServerInfo]:
    """Discover all running Deephaven servers on this machine."""
    servers = []
    if sys.platform == "linux":
        servers.extend(_discover_linux())
    elif sys.platform == "darwin":
        servers.extend(_discover_macos())

    # Docker containers run as root and their ports are owned by docker-proxy,
    # so /proc-based discovery can't see them. Use docker ps instead.
    docker_ports = {s.port for s in servers}
    for ds in _discover_docker():
        if ds.port not in docker_ports:
            servers.append(ds)

    return servers


def format_server_list(servers: list[ServerInfo]) -> str:
    """Format a list of servers for display."""
    if not servers:
        return "No Deephaven servers found."

    # Calculate column widths
    headers = ("PORT", "PID", "SOURCE", "LOCATION")
    rows = []
    for s in sorted(servers, key=lambda s: s.port):
        # Show script path if available, otherwise cwd, otherwise image name
        location = s.script or s.cwd or "-"
        rows.append((
            str(s.port),
            str(s.pid) if s.pid else "-",
            s.source,
            location,
        ))

    widths = [len(h) for h in headers]
    for row in rows:
        for i, val in enumerate(row):
            widths[i] = max(widths[i], len(val))

    fmt = "  ".join(f"{{:<{w}}}" for w in widths)
    lines = [fmt.format(*headers)]
    for row in rows:
        lines.append(fmt.format(*row))
    return "\n".join(lines)


def kill_server(port: int) -> tuple[bool, str]:
    """Kill the Deephaven server on the given port.

    Returns (success, message).
    """
    servers = discover_servers()
    match = [s for s in servers if s.port == port]
    if not match:
        return False, f"No Deephaven server found on port {port}"

    server = match[0]

    if server.source == "docker":
        if not server.container_id:
            return False, f"Docker container on port {port} has no container ID"
        try:
            result = subprocess.run(
                ["docker", "stop", server.container_id],
                capture_output=True, text=True, timeout=30,
            )
            if result.returncode == 0:
                return True, f"Stopped docker container on port {port}"
            return False, f"Failed to stop docker container: {result.stderr.strip()}"
        except FileNotFoundError:
            return False, "docker command not found"
        except subprocess.TimeoutExpired:
            return False, "Timed out waiting for docker stop"

    # For all other sources (dh serve, dh repl, java, etc.) — send SIGTERM
    try:
        os.kill(server.pid, signal.SIGTERM)
        return True, f"Stopped {server.source} (pid {server.pid}) on port {port}"
    except ProcessLookupError:
        return False, f"Process {server.pid} not found (already exited?)"
    except PermissionError:
        return False, f"Permission denied killing pid {server.pid}"


# ---------------------------------------------------------------------------
# /proc parsing helpers (pure functions, easily testable)
# ---------------------------------------------------------------------------

def _parse_proc_net_tcp(content: str) -> list[_ListeningSocket]:
    """Parse /proc/net/tcp or /proc/net/tcp6 content.

    Returns only LISTEN sockets (state 0A).
    """
    results = []
    for line in content.strip().splitlines()[1:]:  # skip header
        fields = line.split()
        if len(fields) < 10:
            continue
        state = fields[3]
        if state != "0A":  # 0A = LISTEN
            continue
        # local_address is "addr:port" in hex
        _, port_hex = fields[1].rsplit(":", 1)
        port = int(port_hex, 16)
        inode = int(fields[9])
        if inode == 0:
            continue
        results.append(_ListeningSocket(port=port, inode=inode))
    return results


def _classify_process(comm: str, cmdline: str) -> dict | None:
    """Classify a process as Deephaven or not based on comm and cmdline.

    Args:
        comm: Process name (from /proc/<pid>/comm).
        cmdline: Raw cmdline string with null byte separators.

    Returns:
        Dict with 'source', 'script' keys, or None if not Deephaven.
    """
    parts = cmdline.split("\x00")
    # Remove empty trailing parts
    parts = [p for p in parts if p]

    if comm == "dh" and len(parts) >= 2:
        # Find the subcommand — skip any leading python/path entries
        # cmdline may be: "python /path/to/dh repl" or "dh serve script.py"
        sub_idx = None
        for i, p in enumerate(parts):
            if p in ("serve", "repl", "exec"):
                sub_idx = i
                break
        subcommand = parts[sub_idx] if sub_idx is not None else ""
        source = f"dh {subcommand}" if subcommand in ("serve", "repl", "exec") else "dh"
        script = None
        remaining = parts[sub_idx + 1:] if sub_idx is not None else []
        if subcommand in ("serve", "exec") and remaining:
            # Find the script argument (first positional, skip flags and their values)
            skip_next = False
            for arg in remaining:
                if skip_next:
                    skip_next = False
                    continue
                if arg.startswith("--"):
                    # Flags like --port, --jvm-args consume the next arg
                    if "=" not in arg:
                        skip_next = True
                    continue
                if arg.startswith("-") and not arg.endswith(".py"):
                    skip_next = True
                    continue
                script = os.path.basename(arg)
                break
        return {"source": source, "script": script}

    if comm == "java" and "io.deephaven" in cmdline:
        return {"source": "java", "script": None}

    # Also catch python processes running deephaven_server directly
    if "deephaven" in cmdline.lower() and comm != "dh":
        return {"source": "python", "script": None}

    return None


# ---------------------------------------------------------------------------
# Linux discovery via /proc
# ---------------------------------------------------------------------------

def _discover_linux() -> list[ServerInfo]:
    """Discover Deephaven servers using /proc filesystem."""
    # Step 1: Find all listening sockets
    listening = []
    for path in ("/proc/net/tcp", "/proc/net/tcp6"):
        try:
            with open(path, "r") as f:
                listening.extend(_parse_proc_net_tcp(f.read()))
        except OSError:
            continue

    if not listening:
        return []

    # Build inode -> socket mapping
    inode_to_socket = {s.inode: s for s in listening}

    # Step 2: Map inodes to PIDs by scanning /proc/<pid>/fd/
    inode_to_pid: dict[int, int] = {}
    try:
        pids = [d for d in os.listdir("/proc") if d.isdigit()]
    except OSError:
        return []

    target_inodes = set(inode_to_socket.keys())

    for pid_str in pids:
        if not target_inodes:
            break
        fd_dir = f"/proc/{pid_str}/fd"
        try:
            fds = os.listdir(fd_dir)
        except (OSError, PermissionError):
            continue
        for fd in fds:
            try:
                link = os.readlink(f"{fd_dir}/{fd}")
            except (OSError, PermissionError):
                continue
            if link.startswith("socket:["):
                inode = int(link[8:-1])
                if inode in target_inodes:
                    inode_to_pid[inode] = int(pid_str)
                    target_inodes.discard(inode)

    # Step 3: For each matched PID, classify the process
    servers = []
    seen_pids: dict[int, ServerInfo] = {}  # deduplicate by PID (may have multiple ports)

    for inode, pid in inode_to_pid.items():
        socket = inode_to_socket[inode]

        if pid in seen_pids:
            # Same PID listening on multiple ports — skip duplicates
            continue

        try:
            with open(f"/proc/{pid}/comm", "r") as f:
                comm = f.read().strip()
            with open(f"/proc/{pid}/cmdline", "r") as f:
                cmdline = f.read()
        except OSError:
            continue

        info = _classify_process(comm, cmdline)
        if info is None:
            continue

        cwd = None
        try:
            cwd = os.readlink(f"/proc/{pid}/cwd")
        except OSError:
            pass

        server = ServerInfo(
            port=socket.port,
            pid=pid,
            source=info["source"],
            script=info["script"],
            cwd=cwd,
        )
        servers.append(server)
        seen_pids[pid] = server

    return servers


# ---------------------------------------------------------------------------
# macOS discovery via lsof
# ---------------------------------------------------------------------------

def _discover_macos() -> list[ServerInfo]:
    """Discover Deephaven servers using lsof on macOS."""
    try:
        result = subprocess.run(
            ["lsof", "-iTCP", "-sTCP:LISTEN", "-P", "-n", "-F", "pcn"],
            capture_output=True, text=True, timeout=10,
        )
    except (FileNotFoundError, subprocess.TimeoutExpired):
        return []

    if result.returncode != 0:
        return []

    return _parse_lsof_output(result.stdout)


# ---------------------------------------------------------------------------
# Docker discovery via docker ps
# ---------------------------------------------------------------------------

def _discover_docker() -> list[ServerInfo]:
    """Discover Deephaven servers running in Docker containers."""
    try:
        result = subprocess.run(
            ["docker", "ps", "--format", "{{.ID}}\t{{.Image}}\t{{.Ports}}"],
            capture_output=True, text=True, timeout=10,
        )
    except (FileNotFoundError, subprocess.TimeoutExpired):
        return []

    if result.returncode != 0:
        return []

    return _parse_docker_ps_output(result.stdout)


def _parse_docker_ps_output(output: str) -> list[ServerInfo]:
    """Parse docker ps output to find Deephaven containers."""
    servers = []
    for line in output.strip().splitlines():
        if not line:
            continue
        parts = line.split("\t", 2)
        if len(parts) < 3:
            continue
        container_id, image, ports = parts

        # Only match deephaven images
        if "deephaven" not in image.lower():
            continue

        # Parse port mappings like "0.0.0.0:10000->10000/tcp, :::10000->10000/tcp"
        for match in re.finditer(r"(?:[\d.]+|::):(\d+)->(\d+)/tcp", ports):
            host_port = int(match.group(1))
            servers.append(ServerInfo(
                port=host_port,
                pid=0,  # container PID not meaningful to the user
                source="docker",
                script=image,
                cwd=None,
                container_id=container_id,
            ))
            break  # one entry per container (avoid duplicates from ipv4+ipv6)

    return servers


def _parse_lsof_output(output: str) -> list[ServerInfo]:
    """Parse lsof -F pcn output to find Deephaven servers."""
    servers = []
    current_pid = None
    current_comm = None

    for line in output.splitlines():
        if line.startswith("p"):
            current_pid = int(line[1:])
        elif line.startswith("c"):
            current_comm = line[1:]
        elif line.startswith("n"):
            if current_pid is None or current_comm is None:
                continue
            # n field is like "*:10000" or "127.0.0.1:10000"
            addr = line[1:]
            match = re.search(r":(\d+)$", addr)
            if not match:
                continue
            port = int(match.group(1))

            # Read full cmdline on macOS via ps
            cmdline = ""
            try:
                ps_result = subprocess.run(
                    ["ps", "-p", str(current_pid), "-o", "command="],
                    capture_output=True, text=True, timeout=5,
                )
                if ps_result.returncode == 0:
                    cmdline = ps_result.stdout.strip().replace(" ", "\x00")
            except (FileNotFoundError, subprocess.TimeoutExpired):
                pass

            info = _classify_process(current_comm, cmdline)
            if info is None:
                continue

            cwd = None
            try:
                cwd_result = subprocess.run(
                    ["lsof", "-p", str(current_pid), "-Fn", "-d", "cwd"],
                    capture_output=True, text=True, timeout=5,
                )
                for cwd_line in cwd_result.stdout.splitlines():
                    if cwd_line.startswith("n/"):
                        cwd = cwd_line[1:]
                        break
            except (FileNotFoundError, subprocess.TimeoutExpired):
                pass

            servers.append(ServerInfo(
                port=port, pid=current_pid,
                source=info["source"], script=info["script"], cwd=cwd,
            ))

    return servers
