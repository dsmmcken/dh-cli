"""Embedded runner for dhg exec/serve. Executed via python -c, reads user code from stdin."""
from __future__ import annotations

import argparse
import ast
import base64
import json
import os
import pickle
import sys
import textwrap


# --- AST helpers (ported from executor.py) ---

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


# --- Wrapper script builder (ported from executor.py) ---

def build_wrapper(code: str, script_path: str | None = None, cwd: str | None = None) -> str:
    """Build the wrapper script that captures output and creates result table."""
    code_repr = repr(code)
    lines: list[str] = []

    if cwd is not None:
        lines.append("import os as __dh_os")
        lines.append("__dh_orig_cwd = __dh_os.getcwd()")
        lines.append(f"__dh_os.chdir({repr(cwd)})")
    if script_path is not None:
        lines.append(f"__file__ = {repr(script_path)}")

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
    if cwd is not None:
        lines.append("    __dh_os.chdir(__dh_orig_cwd)")
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
    if cwd is not None:
        lines.append("del __dh_os, __dh_orig_cwd")
    lines.append("del __dh_io, __dh_sys, __dh_pickle, __dh_base64")
    lines.append("del __dh_stdout_buf, __dh_stderr_buf, __dh_orig_stdout, __dh_orig_stderr")
    lines.append("del __dh_result, __dh_error, __dh_results_dict, __dh_pickled")

    return "\n".join(lines)


# --- Result reading (ported from executor.py) ---

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


# --- Table preview (ported from executor.py) ---

def get_table_preview(session, name: str, show_meta: bool = True) -> dict | None:
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


# --- Cleanup ---

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


# --- Execution modes ---

def _is_port_available(port: int) -> bool:
    """Check if a port is available for binding."""
    import socket
    if port == 0:
        return True
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        try:
            s.bind(('localhost', port))
            return True
        except OSError:
            return False


def run_embedded(args, code: str):
    """Start an embedded server, connect, execute, return results."""
    from deephaven_server import Server

    # Check port availability and fall back to auto-assign if needed
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
    return _execute_on_server("localhost", actual_port, args, code)


def run_remote(args, code: str):
    """Connect to a remote server, execute, return results."""
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

    return _execute_on_server(args.host, args.port, args, code, **kwargs)


def _execute_on_server(host: str, port: int, args, code: str, **session_kwargs):
    """Connect to server at host:port, execute code, return exit code."""
    from pydeephaven import Session

    try:
        session = Session(host=host, port=port, **session_kwargs)
    except Exception as e:
        _emit_error(args, f"Failed to connect to {host}:{port}: {e}", exit_code=2)
        return 2

    try:
        # Get assigned names from user code
        assigned_names = get_assigned_names(code)

        # Build and execute wrapper
        wrapper = build_wrapper(code, script_path=args.script_path, cwd=args.cwd)

        try:
            session.run_script(wrapper)
        except Exception as e:
            _emit_error(args, str(e), exit_code=1)
            return 1

        # Read results
        result = read_result_table(session)
        cleanup_result_table(session)

        # Find assigned tables
        server_tables = set(session.tables) - {"__dh_result_table"}
        assigned_tables = [name for name in assigned_names if name in server_tables]

        # Gather table info
        show_meta = args.show_table_meta
        tables_info = []
        if args.show_tables and assigned_tables:
            for tname in assigned_tables:
                info = get_table_preview(session, tname, show_meta=show_meta)
                if info:
                    tables_info.append(info)

        stdout_text = result.get("stdout", "")
        stderr_text = result.get("stderr", "")
        result_repr = result.get("result_repr")
        error_text = result.get("error")

        if args.output_json:
            # JSON output mode
            output = {
                "exit_code": 1 if error_text else 0,
                "stdout": stdout_text,
                "stderr": stderr_text,
                "result_repr": result_repr,
                "error": error_text,
                "tables": tables_info,
            }
            print(json.dumps(output))
        else:
            # Normal output mode
            if stdout_text:
                print(stdout_text, end="")
                if not stdout_text.endswith("\n"):
                    print()

            if stderr_text:
                print(stderr_text, file=sys.stderr, end="")
                if not stderr_text.endswith("\n"):
                    print(file=sys.stderr)

            if result_repr is not None and result_repr != "None":
                print(result_repr)

            if args.show_tables and assigned_tables:
                for info in tables_info:
                    if info is None:
                        continue
                    if show_meta:
                        status = "refreshing" if info["is_refreshing"] else "static"
                        print(f"\n=== Table: {info['name']} ({info['row_count']:,} rows, {status}) ===")
                    else:
                        print(f"\n=== Table: {info['name']} ===")
                    print(info["preview"])

            if error_text:
                print(error_text, file=sys.stderr)
                hint = _suggest_backtick_hint(code, error_text)
                if hint:
                    print(hint, file=sys.stderr)
                return 1

        return 1 if error_text else 0

    except KeyboardInterrupt:
        print("\nInterrupted.", file=sys.stderr)
        return 130
    except Exception as e:
        _emit_error(args, str(e), exit_code=2)
        return 2
    finally:
        try:
            session.close()
        except Exception:
            pass


def run_serve(args, code: str):
    """Start an embedded server, run script, keep server alive until interrupted."""
    import time
    from deephaven_server import Server

    # Check port availability and fall back to auto-assign if needed
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

    # Connect and run script directly (no output capture wrapper)
    from pydeephaven import Session

    try:
        session = Session(host="localhost", port=actual_port)
    except Exception as e:
        print(f"Error: Failed to connect to server: {e}", file=sys.stderr)
        return 2

    try:
        session.run_script(code)
    except Exception as e:
        print(f"Error: Script execution failed: {e}", file=sys.stderr)
        try:
            session.close()
        except Exception:
            pass
        return 1

    # Build URL
    url = f"http://localhost:{actual_port}"
    if args.iframe:
        url = f"{url}/iframe/widget/?name={args.iframe}"

    # Signal to Go that we're ready (consumed by Go, not shown to user)
    print(f"__DHG_READY__:{url}", flush=True)

    # User-visible output
    print(f"Server running at {url}", flush=True)
    print("Press Ctrl+C to stop.", flush=True)

    # Keep alive until interrupted
    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        print("\nShutting down...", flush=True)
    finally:
        try:
            session.close()
        except Exception:
            pass

    return 0


def _suggest_backtick_hint(script_content: str, error: str) -> str | None:
    """Check if error might be caused by shell backtick interpretation."""
    query_patterns = ['.where(', '.update(', '.select(', '.view(', '.update_view(']
    has_query_ops = any(p in script_content for p in query_patterns)
    has_backticks = '`' in script_content
    is_syntax_error = 'SyntaxError' in error or 'NameError' in error

    if has_query_ops and not has_backticks and is_syntax_error:
        return (
            "\nHint: If your script contains backticks (`) for Deephaven strings,\n"
            "they may have been interpreted by the shell. Use a script file\n"
            "or $'...' quoting. See 'dhg exec --help' for details."
        )
    return None


def _emit_error(args, message: str, exit_code: int):
    """Emit an error in the appropriate format."""
    if args.output_json:
        output = {
            "exit_code": exit_code,
            "stdout": "",
            "stderr": "",
            "result_repr": None,
            "error": message,
            "tables": [],
        }
        print(json.dumps(output))
    else:
        print(f"Error: {message}", file=sys.stderr)


# --- Main ---

def main():
    parser = argparse.ArgumentParser(description="dhg exec/serve runner")
    parser.add_argument("--mode", choices=["embedded", "remote", "serve"], required=True)
    parser.add_argument("--port", type=int, default=10000)
    parser.add_argument("--host", default="localhost")
    parser.add_argument("--jvm-args", default="-Xmx4g")
    parser.add_argument("--show-tables", action="store_true")
    parser.add_argument("--show-table-meta", action="store_true")
    parser.add_argument("--script-path", default=None)
    parser.add_argument("--cwd", default=None)
    parser.add_argument("--output-json", action="store_true")
    parser.add_argument("--auth-type", default=None)
    parser.add_argument("--auth-token", default=None)
    parser.add_argument("--tls", action="store_true")
    parser.add_argument("--tls-ca-cert", default=None)
    parser.add_argument("--tls-client-cert", default=None)
    parser.add_argument("--tls-client-key", default=None)
    parser.add_argument("--iframe", default=None)

    args = parser.parse_args()

    # Read user code from stdin
    code = sys.stdin.read()
    if not code.strip():
        # Empty code is a no-op success
        if args.output_json:
            print(json.dumps({
                "exit_code": 0,
                "stdout": "",
                "stderr": "",
                "result_repr": None,
                "error": None,
                "tables": [],
            }))
        sys.exit(0)

    try:
        if args.mode == "serve":
            exit_code = run_serve(args, code)
        elif args.mode == "embedded":
            exit_code = run_embedded(args, code)
        else:
            exit_code = run_remote(args, code)
    except KeyboardInterrupt:
        print("\nInterrupted.", file=sys.stderr)
        sys.exit(130)
    except Exception as e:
        _emit_error(args, str(e), exit_code=2)
        sys.exit(2)

    sys.exit(exit_code)


if __name__ == "__main__":
    main()
