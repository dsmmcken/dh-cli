"""Long-running REPL runner for dhg repl. Executed via python -c, communicates via JSON on stdin/stdout."""
from __future__ import annotations

import argparse
import ast
import base64
import json
import os
import pickle
import sys
import textwrap
import threading
import time


# --- JSON protocol ---

_emit_lock = threading.Lock()

def emit(obj):
    """Write JSON line to stdout (the protocol channel). Thread-safe."""
    line = json.dumps(obj)
    with _emit_lock:
        print(line, flush=True)


# --- Subscription state ---

_subscription_lock = threading.Lock()
_active_subscription = None  # dict with keys: name, offset, limit, stop_event, thread


def _stop_subscription():
    """Stop the active subscription if any."""
    global _active_subscription
    with _subscription_lock:
        if _active_subscription is not None:
            _active_subscription["stop_event"].set()
            _active_subscription = None


# --- AST helpers (from runner.py) ---

def get_assigned_names(code: str) -> set[str]:
    """Extract variable names being assigned in the code."""
    try:
        tree = ast.parse(code)
    except SyntaxError:
        return set()

    names: set[str] = set()
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


def _extract_names(target) -> set[str]:
    names: set[str] = set()
    if isinstance(target, ast.Name):
        names.add(target.id)
    elif isinstance(target, (ast.Tuple, ast.List)):
        for elt in target.elts:
            names.update(_extract_names(elt))
    return names


# --- Wrapper script builder (from runner.py) ---

def build_wrapper(code: str) -> str:
    """Build the wrapper script that captures output and creates result table."""
    code_repr = repr(code)
    lines: list[str] = []

    lines.append("import io as __dh_io")
    lines.append("import sys as __dh_sys")
    lines.append("import pickle as __dh_pickle")
    lines.append("import base64 as __dh_base64")
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
    lines.append(
        '__dh_pickled = __dh_base64.b64encode('
        '__dh_pickle.dumps(__dh_results_dict)).decode("ascii")'
    )
    lines.append("")
    lines.append("from deephaven import empty_table as __dh_empty_table")
    lines.append('__dh_result_table = __dh_empty_table(1).update('
                  '[f"data = `{__dh_pickled}`"])')
    lines.append("")
    lines.append("del __dh_io, __dh_sys, __dh_pickle, __dh_base64")
    lines.append("del __dh_stdout_buf, __dh_stderr_buf, __dh_orig_stdout, __dh_orig_stderr")
    lines.append("del __dh_result, __dh_error, __dh_results_dict, __dh_pickled, __dh_empty_table")

    return "\n".join(lines)


# --- Result reading (from runner.py) ---

def read_result_table(session) -> dict:
    """Read and decode the pickled results from the result table."""
    table = session.open_table("__dh_result_table")
    try:
        arrow_table = table.to_arrow()
        df = arrow_table.to_pandas()
        if len(df) > 0:
            encoded_data = df.iloc[0]["data"]
            pickled_bytes = base64.b64decode(encoded_data.encode("ascii"))
            return pickle.loads(pickled_bytes)
    except Exception as e:
        return {"error": f"Failed to read results: {e}"}
    return {}


def cleanup_result_table(session):
    """Delete __dh_result_table from server namespace."""
    try:
        session.run_script(textwrap.dedent("""\
            try:
                del __dh_result_table
            except NameError:
                pass
        """))
    except Exception:
        pass


# --- Version helper ---

def get_version():
    try:
        import deephaven_server
        return deephaven_server.__version__
    except Exception:
        return "unknown"


# --- Server startup ---

def start_embedded(args):
    """Start an embedded DH server, return (session, port)."""
    from deephaven_server import Server

    # Check port availability
    port_to_use = args.port
    if not _is_port_available(port_to_use):
        print(f"Port {port_to_use} is in use, finding available port...", file=sys.stderr)
        port_to_use = 0

    # Suppress JVM/server output
    original_stdout_fd = os.dup(1)
    original_stderr_fd = os.dup(2)
    original_stdout = sys.stdout
    original_stderr = sys.stderr
    devnull_fd = os.open(os.devnull, os.O_WRONLY)
    devnull_file = open(os.devnull, "w")
    os.dup2(devnull_fd, 1)
    os.dup2(devnull_fd, 2)
    os.close(devnull_fd)
    sys.stdout = devnull_file
    sys.stderr = devnull_file

    try:
        server = Server(port=port_to_use, jvm_args=args.jvm_args.split() if args.jvm_args else ["-Xmx4g"])
        server.start()
    finally:
        os.dup2(original_stdout_fd, 1)
        os.dup2(original_stderr_fd, 2)
        os.close(original_stdout_fd)
        os.close(original_stderr_fd)
        sys.stdout = original_stdout
        sys.stderr = original_stderr
        devnull_file.close()

    actual_port = server.port

    from pydeephaven import Session
    session = Session(host="localhost", port=actual_port)
    return session, actual_port


def connect_remote(args):
    """Connect to a remote DH server, return (session, port)."""
    from pydeephaven import Session

    kwargs = {}
    if args.auth_type:
        kwargs["auth_type"] = args.auth_type
    if args.auth_token:
        kwargs["auth_token"] = args.auth_token
    if args.tls:
        kwargs["use_tls"] = True
    if args.tls_ca_cert:
        with open(args.tls_ca_cert, "rb") as f:
            kwargs["tls_root_certs"] = f.read()
    if args.tls_client_cert:
        with open(args.tls_client_cert, "rb") as f:
            kwargs["client_cert_chain"] = f.read()
    if args.tls_client_key:
        with open(args.tls_client_key, "rb") as f:
            kwargs["client_private_key"] = f.read()

    session = Session(host=args.host, port=args.port, **kwargs)
    return session, args.port


def _is_port_available(port: int) -> bool:
    import socket
    if port == 0:
        return True
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        try:
            s.bind(('localhost', port))
            return True
        except OSError:
            return False


# --- Command handlers ---

def handle_execute(session, cmd_id, code):
    start = time.monotonic()
    assigned_names = get_assigned_names(code)

    wrapper = build_wrapper(code)
    try:
        session.run_script(wrapper)
    except Exception as e:
        elapsed = int((time.monotonic() - start) * 1000)
        emit({
            "type": "result",
            "id": cmd_id,
            "stdout": "",
            "stderr": "",
            "error": str(e),
            "result_repr": None,
            "assigned_tables": [],
            "all_tables": sorted(set(session.tables) - {"__dh_result_table"}),
            "elapsed_ms": elapsed,
        })
        return

    result = read_result_table(session)
    cleanup_result_table(session)

    server_tables = set(session.tables) - {"__dh_result_table"}
    assigned_tables = [n for n in assigned_names if n in server_tables]
    all_tables = sorted(server_tables)

    elapsed = int((time.monotonic() - start) * 1000)

    emit({
        "type": "result",
        "id": cmd_id,
        "stdout": result.get("stdout", ""),
        "stderr": result.get("stderr", ""),
        "error": result.get("error"),
        "result_repr": result.get("result_repr"),
        "assigned_tables": assigned_tables,
        "all_tables": all_tables,
        "elapsed_ms": elapsed,
    })


def handle_list_tables(session, cmd_id):
    tables = []
    for name in sorted(set(session.tables) - {"__dh_result_table"}):
        try:
            t = session.open_table(name)
            arrow = t.to_arrow()
            tables.append({
                "name": name,
                "row_count": arrow.num_rows,
                "is_refreshing": t.is_refreshing,
                "columns": [{"name": f.name, "type": str(f.type)} for f in arrow.schema],
            })
        except Exception:
            tables.append({"name": name, "row_count": -1, "is_refreshing": False, "columns": []})
    emit({"type": "tables", "id": cmd_id, "tables": tables})


def handle_fetch_table(session, cmd_id, cmd):
    name = cmd.get("name", "")
    offset = cmd.get("offset", 0)
    limit = cmd.get("limit", 50)

    try:
        t = session.open_table(name)
        arrow = t.to_arrow()
        total = arrow.num_rows
        sliced = arrow.slice(offset, limit)

        columns = [f.name for f in sliced.schema]
        types = [str(f.type) for f in sliced.schema]
        rows = []
        for i in range(sliced.num_rows):
            row = []
            for col in columns:
                val = sliced.column(col)[i].as_py()
                if isinstance(val, (bytes, bytearray)):
                    val = base64.b64encode(val).decode("ascii")
                elif hasattr(val, 'isoformat'):
                    val = val.isoformat()
                row.append(val)
            rows.append(row)

        emit({
            "type": "table_data",
            "id": cmd_id,
            "name": name,
            "columns": columns,
            "types": types,
            "rows": rows,
            "total_rows": total,
            "offset": offset,
            "is_refreshing": t.is_refreshing,
        })
    except Exception as e:
        emit({"type": "error", "id": cmd_id, "message": f"Failed to fetch table {name}: {e}"})


def handle_subscribe(session, cmd_id, cmd):
    """Start polling a refreshing table and emitting updates."""
    global _active_subscription
    name = cmd.get("name", "")
    offset = cmd.get("offset", 0)
    limit = cmd.get("limit", 200)

    # Stop any existing subscription first
    _stop_subscription()

    stop_event = threading.Event()

    def poll_loop():
        """Background thread that periodically snapshots the table."""
        last_hash = None
        while not stop_event.is_set():
            stop_event.wait(2.0)
            if stop_event.is_set():
                break
            try:
                t = session.open_table(name)
                arrow = t.to_arrow()
                total = arrow.num_rows
                slice_len = min(limit, total - offset) if total > offset else 0
                sliced = arrow.slice(offset, slice_len)

                # Simple change detection
                current_hash = (total, sliced.num_rows)
                if sliced.num_rows > 0:
                    first_vals = tuple(sliced.column(col)[0].as_py() for col in sliced.column_names)
                    last_vals = tuple(sliced.column(col)[-1].as_py() for col in sliced.column_names)
                    current_hash = (total, sliced.num_rows, first_vals, last_vals)

                if current_hash == last_hash:
                    continue
                last_hash = current_hash

                columns = [f.name for f in sliced.schema]
                types = [str(f.type) for f in sliced.schema]
                rows = []
                for i in range(sliced.num_rows):
                    row = []
                    for col in columns:
                        val = sliced.column(col)[i].as_py()
                        if isinstance(val, (bytes, bytearray)):
                            val = base64.b64encode(val).decode("ascii")
                        elif hasattr(val, 'isoformat'):
                            val = val.isoformat()
                        row.append(val)
                    rows.append(row)

                emit({
                    "type": "table_update",
                    "name": name,
                    "columns": columns,
                    "types": types,
                    "rows": rows,
                    "total_rows": total,
                    "offset": offset,
                })
            except Exception as e:
                print(f"Subscription error for {name}: {e}", file=sys.stderr)
                break

    thread = threading.Thread(target=poll_loop, daemon=True)
    thread.start()

    with _subscription_lock:
        _active_subscription = {
            "name": name,
            "offset": offset,
            "limit": limit,
            "stop_event": stop_event,
            "thread": thread,
        }

    emit({"type": "subscribe_ack", "id": cmd_id, "name": name})


def handle_unsubscribe(session, cmd_id, cmd):
    """Stop polling the currently subscribed table."""
    name = cmd.get("name", "")
    _stop_subscription()
    emit({"type": "unsubscribe_ack", "id": cmd_id, "name": name})


def handle_server_info(session, cmd_id, args):
    tables = set(session.tables) - {"__dh_result_table"}
    emit({
        "type": "server_info",
        "id": cmd_id,
        "host": args.host if args.mode == "remote" else "localhost",
        "port": args.port,
        "version": get_version(),
        "mode": args.mode,
        "table_count": len(tables),
    })


# --- Main loop ---

def run_loop(session, args):
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            cmd = json.loads(line)
        except json.JSONDecodeError as e:
            emit({"type": "error", "message": f"Invalid JSON: {e}"})
            continue

        cmd_type = cmd.get("type")
        cmd_id = cmd.get("id")

        if cmd_type == "execute":
            handle_execute(session, cmd_id, cmd.get("code", ""))
        elif cmd_type == "list_tables":
            handle_list_tables(session, cmd_id)
        elif cmd_type == "fetch_table":
            handle_fetch_table(session, cmd_id, cmd)
        elif cmd_type == "subscribe":
            handle_subscribe(session, cmd_id, cmd)
        elif cmd_type == "unsubscribe":
            handle_unsubscribe(session, cmd_id, cmd)
        elif cmd_type == "server_info":
            handle_server_info(session, cmd_id, args)
        elif cmd_type == "shutdown":
            _stop_subscription()
            emit({"type": "shutdown_ack"})
            try:
                session.close()
            except Exception:
                pass
            sys.exit(0)
        else:
            emit({"type": "error", "id": cmd_id, "message": f"Unknown command type: {cmd_type}"})


# --- Entry point ---

def main():
    parser = argparse.ArgumentParser(description="dhg repl runner")
    parser.add_argument("--mode", choices=["embedded", "remote"], required=True)
    parser.add_argument("--port", type=int, default=10000)
    parser.add_argument("--host", default="localhost")
    parser.add_argument("--jvm-args", default="-Xmx4g")
    parser.add_argument("--auth-type", default=None)
    parser.add_argument("--auth-token", default=None)
    parser.add_argument("--tls", action="store_true")
    parser.add_argument("--tls-ca-cert", default=None)
    parser.add_argument("--tls-client-cert", default=None)
    parser.add_argument("--tls-client-key", default=None)

    args = parser.parse_args()

    try:
        if args.mode == "embedded":
            session, port = start_embedded(args)
        else:
            session, port = connect_remote(args)
    except Exception as e:
        print(f"Error: Failed to start: {e}", file=sys.stderr)
        sys.exit(2)

    # Update port to actual port (may differ if auto-assigned)
    args.port = port

    emit({"type": "ready", "port": port, "version": get_version(), "mode": args.mode})

    try:
        run_loop(session, args)
    except KeyboardInterrupt:
        pass
    finally:
        try:
            session.close()
        except Exception:
            pass


if __name__ == "__main__":
    main()
