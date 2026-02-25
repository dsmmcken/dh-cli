"""In-VM runner daemon for dhg exec --vm.

Runs inside the Firecracker VM. Connects to the local Deephaven server via
pydeephaven, then listens on a vsock port for JSON execution requests from the host.
This daemon + its warm Session are captured in the VM snapshot.
"""
import ast
import base64
import json
import os
import pickle
import socket
import sys
import textwrap
import traceback

VMADDR_CID_ANY = 0xFFFFFFFF
VSOCK_PORT = 10000


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
    """Build the wrapper script that captures output and creates result table."""
    code_repr = repr(code)
    lines = []

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
    lines.append("from deephaven import empty_table")
    lines.append('__dh_result_table = empty_table(1).update('
                  '[f"data = `{__dh_pickled}`"])')
    lines.append("")
    lines.append("del __dh_io, __dh_sys, __dh_pickle, __dh_base64")
    lines.append("del __dh_stdout_buf, __dh_stderr_buf, __dh_orig_stdout, __dh_orig_stderr")
    lines.append("del __dh_result, __dh_error, __dh_results_dict, __dh_pickled")

    return "\n".join(lines)


# --- Result reading ---

def read_result_table(session):
    """Read and decode the pickled results from the result table."""
    table = session.open_table("__dh_result_table")
    try:
        arrow_table = table.to_arrow()
        if arrow_table.num_rows > 0:
            encoded_data = arrow_table.column("data")[0].as_py()
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

    assigned_names = get_assigned_names(code)
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
    result = read_result_table(session)
    _t3 = _t.time()
    cleanup_result_table(session)
    _t4 = _t.time()

    stdout_text = result.get("stdout", "")
    stderr_text = result.get("stderr", "")
    result_repr = result.get("result_repr")
    error_text = result.get("error")

    tables_info = []
    if show_tables:
        server_tables = set(session.tables) - {"__dh_result_table"}
        assigned_tables = [n for n in assigned_names if n in server_tables]
        for tname in assigned_tables:
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
            "cleanup_ms": int((_t4-_t3)*1000),
        },
    }


# --- Vsock server ---

def serve_forever(session):
    """Listen on vsock, handle one request per connection."""
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
    session = Session(host="localhost", port=10000)

    # Signal readiness via marker file
    import pathlib
    pathlib.Path("/tmp/runner_ready").touch()

    serve_forever(session)


if __name__ == "__main__":
    main()
