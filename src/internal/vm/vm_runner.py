"""In-VM runner daemon for dh exec --vm.

Runs inside the Firecracker VM. Connects to the local Deephaven server via
pydeephaven, then listens on a vsock port for JSON execution requests from the host.
This daemon + its warm Session are captured in the VM snapshot.
"""
import ast
import json
import os
import socket
import sys
import threading
import traceback

VMADDR_CID_ANY = 0xFFFFFFFF
VSOCK_PORT = 10000
DH_SERVER_PORT = 10000  # Deephaven HTTP server inside the VM
HTTP_PROXY_PORT = 10002


# --- AST helpers ---

def get_assigned_names(code):
    """Extract variable names being assigned in the code."""
    try:
        tree = ast.parse(code)
    except SyntaxError:
        return set()

    names = set()
    for node in ast.walk(tree):
        if isinstance(node, ast.Assign):
            for target in node.targets:
                names.update(_extract_names(target))
        elif isinstance(node, ast.AnnAssign) and node.target:
            names.update(_extract_names(node.target))
        elif isinstance(node, ast.AugAssign):
            names.update(_extract_names(node.target))
        elif isinstance(node, ast.NamedExpr):
            names.add(node.target.id)
    return names


def _extract_names(target):
    names = set()
    if isinstance(target, ast.Name):
        names.add(target.id)
    elif isinstance(target, (ast.Tuple, ast.List)):
        for elt in target.elts:
            names.update(_extract_names(elt))
    return names


# --- Wrapper script builder ---

def build_wrapper(code):
    """Build the wrapper script that captures output and writes result to file."""
    code_repr = repr(code)
    lines = []

    # Set CWD to /workspace so relative paths in user code resolve to
    # /workspace/* which triggers the LD_PRELOAD interceptor to fetch
    # files from the host transparently. This runs inside the Deephaven
    # server process (not the runner), which is where file I/O happens.
    lines.append("import os as __dh_os")
    lines.append("try:")
    lines.append("    __dh_os.chdir('/workspace')")
    lines.append("except OSError:")
    lines.append("    pass")
    lines.append("del __dh_os")
    lines.append("")
    lines.append("import io as __dh_io")
    lines.append("import sys as __dh_sys")
    lines.append("import json as __dh_json")
    lines.append("")
    lines.append("__dh_stdout_buf = __dh_io.StringIO()")
    lines.append("__dh_stderr_buf = __dh_io.StringIO()")
    lines.append("__dh_orig_stdout = __dh_sys.stdout")
    lines.append("__dh_orig_stderr = __dh_sys.stderr")
    lines.append("__dh_sys.stdout = __dh_stdout_buf")
    lines.append("__dh_sys.stderr = __dh_stderr_buf")
    lines.append("__dh_result = None")
    lines.append("__dh_error = None")
    lines.append("")
    lines.append("try:")
    lines.append("    try:")
    lines.append(f"        __dh_result = eval({code_repr})")
    lines.append("    except SyntaxError:")
    lines.append(f"        exec({code_repr})")
    lines.append("except Exception as __dh_e:")
    lines.append("    import traceback as __dh_tb")
    lines.append("    __dh_error = __dh_tb.format_exc()")
    lines.append("finally:")
    lines.append("    __dh_sys.stdout = __dh_orig_stdout")
    lines.append("    __dh_sys.stderr = __dh_orig_stderr")
    lines.append("")
    lines.append("__dh_results_dict = {")
    lines.append('    "stdout": __dh_stdout_buf.getvalue(),')
    lines.append('    "stderr": __dh_stderr_buf.getvalue(),')
    lines.append('    "result_repr": repr(__dh_result) if __dh_result is not None else None,')
    lines.append('    "error": __dh_error,')
    lines.append("}")
    lines.append("")
    lines.append("with open('/tmp/__dh_result.json', 'w') as __dh_f:")
    lines.append("    __dh_json.dump(__dh_results_dict, __dh_f)")
    lines.append("")
    lines.append("del __dh_io, __dh_sys, __dh_json")
    lines.append("del __dh_stdout_buf, __dh_stderr_buf, __dh_orig_stdout, __dh_orig_stderr")
    lines.append("del __dh_result, __dh_error, __dh_results_dict, __dh_f")

    return "\n".join(lines)


# --- Result reading ---

def read_result_file():
    """Read results from the JSON file written by the wrapper script."""
    try:
        with open('/tmp/__dh_result.json', 'r') as f:
            return json.load(f)
    except Exception as e:
        return {"error": f"Failed to read results: {e}"}


# --- Table preview ---

def get_table_preview(session, name, show_meta=True):
    """Get table metadata and preview string. Returns dict or None on error."""
    try:
        table = session.open_table(name)
        arrow_table = table.to_arrow()
        total_rows = arrow_table.num_rows
        is_refreshing = table.is_refreshing
        schema = arrow_table.schema
        columns = [{"name": field.name, "type": str(field.type)} for field in schema]

        lines = []
        if show_meta:
            col_info = ", ".join(f"{c['name']} ({c['type']})" for c in columns)
            if len(f"Columns: {col_info}") > 80:
                lines.append("Columns:")
                for c in columns:
                    lines.append(f"  {c['name']} ({c['type']})")
            else:
                lines.append(f"Columns: {col_info}")
            lines.append("")

        if total_rows == 0:
            lines.append("(empty table)")
        else:
            preview_df = arrow_table.slice(0, 10).to_pandas()
            lines.append(preview_df.to_string(index=False))

        return {
            "name": name,
            "row_count": total_rows,
            "is_refreshing": is_refreshing,
            "columns": columns,
            "preview": "\n".join(lines),
        }
    except Exception:
        return None


# --- Request handling ---

def handle_request(session, request):
    """Process a single execution request. Returns response dict."""
    import time as _t
    _t0 = _t.time()

    code = request.get("code", "")
    show_tables = request.get("show_tables", False)
    show_table_meta = request.get("show_table_meta", False)

    if not code.strip():
        return {
            "exit_code": 0,
            "stdout": "",
            "stderr": "",
            "result_repr": None,
            "error": None,
            "tables": [],
        }

    if show_tables:
        assigned_names = get_assigned_names(code)
    else:
        assigned_names = set()
    wrapper = build_wrapper(code)
    _t1 = _t.time()

    try:
        session.run_script(wrapper)
    except Exception as e:
        return {
            "exit_code": 1,
            "stdout": "",
            "stderr": "",
            "result_repr": None,
            "error": str(e),
            "tables": [],
        }

    _t2 = _t.time()
    result = read_result_file()
    _t3 = _t.time()

    stdout_text = result.get("stdout", "")
    stderr_text = result.get("stderr", "")
    result_repr = result.get("result_repr")
    error_text = result.get("error")

    tables_info = []
    if show_tables and assigned_names:
        # Use assigned_names directly to avoid a session.tables gRPC call.
        # Each get_table_preview opens the table individually; if it doesn't
        # exist on the server, it returns None.
        for tname in assigned_names:
            info = get_table_preview(session, tname, show_meta=show_table_meta)
            if info:
                tables_info.append(info)

    return {
        "exit_code": 1 if error_text else 0,
        "stdout": stdout_text,
        "stderr": stderr_text,
        "result_repr": result_repr,
        "error": error_text,
        "tables": tables_info,
        "_timing": {
            "build_wrapper_ms": int((_t1-_t0)*1000),
            "run_script_ms": int((_t2-_t1)*1000),
            "read_result_ms": int((_t3-_t2)*1000),
        },
    }


RENDER_DAEMON_SOCKET = "/tmp/render-daemon.sock"


def _render_via_daemon(widget, actions, render_timeout, max_rows,
                       render_json, stderr_lines, _t_start, verbose=False):
    """Try to render via the persistent Node.js daemon. Returns None if unavailable."""
    import time as _t

    if not os.path.exists(RENDER_DAEMON_SOCKET):
        return None

    try:
        sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        sock.settimeout(render_timeout / 1000 + 30)
        sock.connect(RENDER_DAEMON_SOCKET)
    except Exception:
        return None

    _t_daemon = _t.time()
    request = json.dumps({
        "widget": widget,
        "actions": actions,
        "timeout": render_timeout,
        "rows": max_rows,
        "json": render_json,
        "verbose": verbose,
    }) + "\n"

    try:
        sock.sendall(request.encode("utf-8"))
        buf = b""
        while b"\n" not in buf:
            chunk = sock.recv(65536)
            if not chunk:
                break
            buf += chunk
        sock.close()
    except Exception as e:
        try:
            sock.close()
        except Exception:
            pass
        return None

    line = buf.split(b"\n", 1)[0]
    try:
        resp = json.loads(line)
    except Exception:
        return None

    _t_done = _t.time()
    if verbose:
        stderr_lines.append(f"[timing] render daemon: {int((_t_done-_t_daemon)*1000)}ms")
        stderr_lines.append(f"[timing] total render: {int((_t_done-_t_start)*1000)}ms")

    timing_str = "\n".join(stderr_lines) + "\n" if stderr_lines else ""
    daemon_stderr = resp.get("stderr", "")

    return {
        "exit_code": resp.get("exit_code", 1),
        "stdout": "",
        "stderr": timing_str + (daemon_stderr if verbose else ""),
        "error": resp.get("error"),
        "render_output": resp.get("render_output", ""),
    }


def _render_via_subprocess(widget, actions, render_timeout, max_rows,
                           render_json, stderr_lines, _t_start, verbose=False):
    """Spawn Node.js oneshot renderer to render a widget."""
    import subprocess
    import time as _t
    import urllib.request

    _dh_url = f"http://127.0.0.1:{DH_SERVER_PORT}"

    # Wait for HTTP endpoint before spawning Node.js.
    _t3 = _t.time()
    for _attempt in range(50):  # up to 5 seconds
        try:
            urllib.request.urlopen(
                f"{_dh_url}/jsapi/dh-internal.js",
                timeout=1,
            )
            break
        except Exception:
            import time as _delay
            _delay.sleep(0.1)
    _t4 = _t.time()
    if verbose:
        stderr_lines.append(f"[timing] http ready wait: {int((_t4-_t3)*1000)}ms")

    node_args = [
        "node",
        "--no-warnings",
        "--import", "/opt/render/src/css-loader.mjs",
        "/opt/render/bin/oneshot.mjs",
        "--url", _dh_url,
        "--widget", widget,
        "--timeout", str(render_timeout),
        "--rows", str(max_rows),
    ]
    if render_json:
        node_args.append("--json")
    if verbose:
        node_args.append("--verbose")
    node_args.extend(actions)

    # Use V8 compile cache to speed up module loading on repeat runs.
    node_env = dict(os.environ)
    node_env["NODE_COMPILE_CACHE"] = "/opt/render/.compile-cache"

    try:
        proc = subprocess.run(
            node_args,
            capture_output=True,
            text=True,
            timeout=render_timeout / 1000 + 30,
            env=node_env,
        )
        _t5 = _t.time()
        if verbose:
            stderr_lines.append(f"[timing] node.js renderer: {int((_t5-_t4)*1000)}ms")
            stderr_lines.append(f"[timing] total render: {int((_t5-_t_start)*1000)}ms")
        timing_str = "\n".join(stderr_lines) + "\n" if stderr_lines else ""
        # Filter oneshot.mjs timing from stderr unless verbose
        node_stderr = proc.stderr if verbose else ""
        return {
            "exit_code": proc.returncode,
            "stdout": "",
            "stderr": timing_str + node_stderr,
            "error": None if proc.returncode == 0 else proc.stderr,
            "render_output": proc.stdout,
        }
    except subprocess.TimeoutExpired:
        return {
            "exit_code": 1,
            "stdout": "",
            "stderr": "",
            "error": "Node.js renderer timed out",
            "render_output": "",
        }
    except Exception as e:
        return {
            "exit_code": 1,
            "stdout": "",
            "stderr": "",
            "error": f"Failed to run Node.js renderer: {e}",
            "render_output": "",
        }


def handle_render_request(session, request):
    """Process a render request: run script, then render via daemon or subprocess.

    Pool VMs have a pre-started render daemon (render-daemon.mjs) that keeps
    Node.js modules and JSAPI loaded in memory. Rendering through the daemon
    takes ~1.2s vs ~6s for a fresh subprocess. Falls back to the overlapped
    subprocess approach if the daemon isn't available (cold renders, older
    snapshots).
    """
    import time as _t

    _t_start = _t.time()
    code = request.get("code", "")
    widget = request.get("widget", "")
    actions = request.get("actions", [])
    render_timeout = request.get("render_timeout", 15000)
    max_rows = request.get("max_rows", 10)
    render_json = request.get("render_json", False)
    verbose = request.get("verbose", False)

    stderr_lines = []

    # Step 1: Run the user's Python script — must complete before rendering
    # so the widget exists on the server.
    if code.strip():
        wrapper = build_wrapper(code)
        _t1 = _t.time()
        try:
            session.run_script(wrapper)
        except Exception as e:
            return {
                "exit_code": 1,
                "stdout": "",
                "stderr": "",
                "error": str(e),
                "render_output": "",
            }
        _t2 = _t.time()
        if verbose:
            stderr_lines.append(f"[timing] script execution: {int((_t2-_t1)*1000)}ms")

        result = read_result_file()
        if result.get("error"):
            return {
                "exit_code": 1,
                "stdout": result.get("stdout", ""),
                "stderr": result.get("stderr", ""),
                "error": result["error"],
                "render_output": "",
            }

    # Step 2: Try the render daemon (fast path, ~1.2s).
    # Pool VMs start render-daemon.mjs during fillOne. The daemon has all
    # modules pre-loaded so createTestClient is ~200ms, not ~3600ms.
    daemon_result = _render_via_daemon(
        widget, actions, render_timeout, max_rows,
        render_json, stderr_lines, _t_start, verbose,
    )
    if daemon_result is not None:
        return daemon_result

    # Step 3: Fall back to subprocess (cold renders, daemon not ready).
    # Uses overlapped execution: start Node.js before waiting, so session.open
    # overlaps with any remaining setup time.
    return _render_via_subprocess(
        widget, actions, render_timeout, max_rows, render_json,
        stderr_lines, _t_start, verbose,
    )


# --- HTTP proxy (vsock → TCP bridge for web UI) ---

def _bridge(src, dst):
    """Copy bytes from src to dst until EOF or error."""
    try:
        while True:
            data = src.recv(65536)
            if not data:
                break
            dst.sendall(data)
    except Exception:
        pass
    finally:
        try:
            src.close()
        except Exception:
            pass
        try:
            dst.close()
        except Exception:
            pass


def _handle_proxy_conn(vsock_conn, dh_port):
    """Bridge a single vsock connection to the local Deephaven server."""
    tcp = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    try:
        tcp.connect(("127.0.0.1", dh_port))
    except Exception:
        vsock_conn.close()
        return
    t1 = threading.Thread(target=_bridge, args=(vsock_conn, tcp), daemon=True)
    t2 = threading.Thread(target=_bridge, args=(tcp, vsock_conn), daemon=True)
    t1.start()
    t2.start()
    # Wait for either direction to finish (connection closed)
    t1.join()
    t2.join()


def serve_http_proxy(dh_port):
    """Listen on vsock for HTTP proxy connections and bridge to DH server."""
    vs = socket.socket(socket.AF_VSOCK, socket.SOCK_STREAM)
    vs.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    vs.bind((VMADDR_CID_ANY, HTTP_PROXY_PORT))
    vs.listen(32)
    while True:
        try:
            conn, _ = vs.accept()
            threading.Thread(
                target=_handle_proxy_conn, args=(conn, dh_port), daemon=True
            ).start()
        except Exception:
            continue


# --- JVM cache writability ---

def _ensure_writable_jvm_cache():
    """Mount tmpfs over the JVM's compilation cache if the root fs is read-only.

    Pool VMs share the ext4 disk image as read-only, so the JVM's query
    compiler can't write compiled class files. This mounts tmpfs over the
    cache directory so the JVM can compile queries. The original cache files
    (from boot/warmup) are hidden, but the JVM recompiles on demand.

    New snapshots built with the updated init.sh already mount this tmpfs
    at boot time, so this function becomes a no-op for them.
    """
    cache_dir = "/root/.cache/deephaven"
    if not os.path.isdir(cache_dir):
        return

    import subprocess
    # Check if already on tmpfs (e.g., init.sh already mounted it)
    result = subprocess.run(
        ["findmnt", "-n", "-o", "FSTYPE", "-T", cache_dir],
        capture_output=True, text=True)
    if "tmpfs" in result.stdout:
        return

    # Check if root fs is read-only
    result = subprocess.run(
        ["findmnt", "-n", "-o", "OPTIONS", "-T", cache_dir],
        capture_output=True, text=True)
    if "ro" not in result.stdout.split(","):
        return  # Root is read-write, no action needed

    subprocess.run(
        ["mount", "-t", "tmpfs", "tmpfs", cache_dir],
        capture_output=True)


# --- Vsock server ---

def serve_forever(session):
    """Listen on vsock, handle one request per connection."""
    _ensure_writable_jvm_cache()

    vs = socket.socket(socket.AF_VSOCK, socket.SOCK_STREAM)
    vs.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    vs.bind((VMADDR_CID_ANY, VSOCK_PORT))
    vs.listen(5)

    while True:
        conn, _ = vs.accept()
        try:
            data = b""
            while True:
                chunk = conn.recv(65536)
                if not chunk:
                    break
                data += chunk
                if b"\n" in data:
                    break

            line = data.split(b"\n", 1)[0]
            if not line.strip():
                # Probe connection from waitForVsock -- just close
                continue

            request = json.loads(line)
            if request.get("render", False):
                response = handle_render_request(session, request)
            else:
                response = handle_request(session, request)
            conn.sendall(json.dumps(response).encode("utf-8") + b"\n")
        except Exception:
            try:
                err_resp = json.dumps({
                    "exit_code": 2,
                    "stdout": "",
                    "stderr": "",
                    "result_repr": None,
                    "error": f"Runner error: {traceback.format_exc()}",
                    "tables": [],
                }).encode("utf-8") + b"\n"
                conn.sendall(err_resp)
            except Exception:
                pass
        finally:
            try:
                conn.close()
            except Exception:
                pass


def main():
    os.environ.setdefault("JAVA_HOME", "/usr/lib/jvm/java-17-openjdk-amd64")

    # Wait for DH readiness
    import time
    for _ in range(6000):  # 10 minutes max
        if os.path.exists("/tmp/dh_ready"):
            break
        time.sleep(0.1)
    else:
        print("RUNNER: Timed out waiting for DH", file=sys.stderr, flush=True)
        sys.exit(1)

    from pydeephaven import Session
    session = Session(host="localhost", port=DH_SERVER_PORT)

    # Start HTTP proxy thread (bridges vsock connections to DH web UI).
    # This runs before snapshot capture so it's already active on restore.
    threading.Thread(target=serve_http_proxy, args=(DH_SERVER_PORT,), daemon=True).start()

    # Signal readiness via marker file
    import pathlib
    pathlib.Path("/tmp/runner_ready").touch()

    serve_forever(session)


if __name__ == "__main__":
    main()
