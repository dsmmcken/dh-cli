"""Main CLI entry point for deephaven-cli."""
from __future__ import annotations

import argparse
import os
import sys
import threading
import time

# Exit codes
EXIT_SUCCESS = 0
EXIT_SCRIPT_ERROR = 1
EXIT_CONNECTION_ERROR = 2
EXIT_TIMEOUT = 3
EXIT_INTERRUPTED = 130


def _suggest_backtick_hint(script_content: str, error: str) -> str | None:
    """Check if error might be caused by shell backtick interpretation.

    When piping scripts via shell, backticks get interpreted as command
    substitution before reaching dh-cli. This heuristic detects likely cases.
    """
    # Patterns that suggest Deephaven query operations
    query_patterns = ['.where(', '.update(', '.select(', '.view(', '.update_view(']
    has_query_ops = any(p in script_content for p in query_patterns)
    has_backticks = '`' in script_content
    is_syntax_error = 'SyntaxError' in error or 'NameError' in error

    if has_query_ops and not has_backticks and is_syntax_error:
        return (
            "\nHint: If your script contains backticks (`) for Deephaven strings,\n"
            "they may have been interpreted by the shell. Use a script file\n"
            "or $'...' quoting. See 'dh exec --help' for details."
        )
    return None


DESCRIPTION = """\
Deephaven CLI - Command-line tool for Deephaven servers

Launch embedded Deephaven servers and execute Python scripts with
real-time data capabilities.
"""

EPILOG = """\
Examples:
  dh repl                              Start interactive session
  dh exec script.py                    Run script and exit
  dh exec -c $'print("hello")'         Execute inline code
  dh -c $'from deephaven import *'     Shorthand for exec -c
  dh exec script.py -v --timeout 30    Verbose mode with timeout
  cat script.py | dh exec -            Read from stdin
  dh app dashboard.py                  Long-running server

Use 'dh <command> --help' for more details.
"""


def main() -> int:
    """Main entry point."""
    # Handle shorthand: dh -c "code" -> dh exec -c "code"
    if len(sys.argv) >= 2 and sys.argv[1] == "-c":
        sys.argv.insert(1, "exec")

    parser = argparse.ArgumentParser(
        prog="dh",
        description=DESCRIPTION,
        epilog=EPILOG,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    subparsers = parser.add_subparsers(dest="command")

    # repl subcommand
    repl_parser = subparsers.add_parser(
        "repl",
        help="Start an interactive REPL session",
        description="Interactive Python REPL with Deephaven server context.\n\n"
                    "Provides full Python environment with Deephaven imports,\n"
                    "tab completion, and direct table manipulation.",
        epilog="Examples:\n"
               "  dh repl\n"
               "  dh repl --port 8080\n"
               "  dh repl --jvm-args -Xmx8g",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    repl_parser.add_argument(
        "--port",
        type=int,
        default=10000,
        help="Server port (default: %(default)s)",
    )
    repl_parser.add_argument(
        "--jvm-args",
        nargs="*",
        default=["-Xmx4g"],
        help="JVM arguments (default: %(default)s). Example: -Xmx8g",
    )
    repl_parser.add_argument(
        "--verbose",
        "-v",
        action="store_true",
        help="Show startup messages (default: quiet)",
    )
    repl_parser.add_argument(
        "--vi",
        action="store_true",
        help="Use Vi key bindings (default: Emacs)",
    )

    # exec subcommand (agent-friendly batch mode)
    exec_parser = subparsers.add_parser(
        "exec",
        help="Execute a script and exit (ideal for automation)",
        description="Execute a Python script in batch mode.\n\n"
                    "Best for automation and AI agents:\n"
                    "  - Clean stdout/stderr separation\n"
                    "  - Structured exit codes\n"
                    "  - Optional timeout",
        epilog="Exit codes:\n"
               "  0   Success\n"
               "  1   Script error\n"
               "  2   Connection error\n"
               "  3   Timeout\n"
               "  130 Interrupted\n\n"
               "Examples:\n"
               "  dh exec script.py\n"
               "  dh exec -c $'print(\"hello\")'\n"
               "  dh -c $'from deephaven import empty_table\\nt = empty_table(5)'\n"
               "  dh exec script.py -v --timeout 60\n"
               "  dh exec script.py --no-show-tables\n\n"
               "Using -c with ANSI-C quoting ($'...'):\n"
               "  Always use $'...' quoting with -c to avoid shell issues:\n"
               "  - Backticks work: dh -c $'t.where(\"X = `val`\")'\n"
               "  - Newlines work:  dh -c $'x = 1\\nprint(x)'\n"
               "  - Quotes work:    dh -c $'print(\"hello\")'\n\n"
               "Stdin input:\n"
               "  echo $'print(\"hi\")' | dh exec -",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    exec_parser.add_argument(
        "-c",
        dest="code",
        metavar="CODE",
        help="Execute CODE as a Python string (use $'...' quoting)",
    )
    exec_parser.add_argument(
        "script",
        nargs="?",
        help="Python script to execute (use '-' for stdin)",
    )
    exec_parser.add_argument(
        "--port",
        type=int,
        default=10000,
        help="Server port (default: %(default)s)",
    )
    exec_parser.add_argument(
        "--jvm-args",
        nargs="*",
        default=["-Xmx4g"],
        help="JVM arguments (default: %(default)s). Example: -Xmx8g",
    )
    exec_parser.add_argument(
        "--verbose",
        "-v",
        action="store_true",
        help="Show startup messages (default: quiet)",
    )
    exec_parser.add_argument(
        "--timeout",
        type=int,
        default=None,
        help="Max execution time in seconds (exit code 3 on timeout)",
    )
    exec_parser.add_argument(
        "--no-show-tables",
        action="store_true",
        help="Suppress table preview output after execution",
    )
    exec_parser.add_argument(
        "--no-table-meta",
        action="store_true",
        help="Suppress column types and row count in table output",
    )

    # app subcommand (long-running application mode)
    app_parser = subparsers.add_parser(
        "app",
        help="Run script and keep server alive (dashboards/services)",
        description="Run a script and keep the Deephaven server running.\n\n"
                    "Use for:\n"
                    "  - Dashboards and visualizations\n"
                    "  - Long-running data pipelines\n"
                    "  - Services that need persistent server\n\n"
                    "Server runs until Ctrl+C.",
        epilog="Examples:\n"
               "  dh app dashboard.py\n"
               "  dh app dashboard.py --port 8080",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    app_parser.add_argument(
        "script",
        help="Python script to execute",
    )
    app_parser.add_argument(
        "--port",
        type=int,
        default=10000,
        help="Server port (default: %(default)s)",
    )
    app_parser.add_argument(
        "--jvm-args",
        nargs="*",
        default=["-Xmx4g"],
        help="JVM arguments (default: %(default)s). Example: -Xmx8g",
    )
    app_parser.add_argument(
        "--verbose",
        "-v",
        action="store_true",
        help="Show startup messages (default: quiet)",
    )

    args = parser.parse_args()

    if args.command is None:
        parser.print_help()
        return EXIT_SUCCESS

    if args.command == "repl":
        return run_repl(args.port, args.jvm_args, args.verbose, args.vi)
    elif args.command == "exec":
        return run_exec(
            script_path=args.script,
            code=args.code,
            port=args.port,
            jvm_args=args.jvm_args,
            verbose=args.verbose,
            timeout=args.timeout,
            show_tables=not args.no_show_tables,
            no_table_meta=args.no_table_meta,
        )
    elif args.command == "app":
        return run_app(args.script, args.port, args.jvm_args, args.verbose)

    return EXIT_SUCCESS


# Inline script for animation subprocess (avoids import overhead)
_ANIMATION_SCRIPT = '''
import sys
import time
frame = 0
while True:
    num_dots = (frame % 3) + 1
    sys.stdout.write(f"\\rStarting Deephaven{'.' * num_dots:<3}")
    sys.stdout.flush()
    frame += 1
    time.sleep(0.25)
'''


def run_repl(
    port: int, jvm_args: list[str], verbose: bool = False, vi_mode: bool = False
) -> int:
    """Run the interactive REPL."""
    import subprocess
    from deephaven_cli.server import DeephavenServer
    from deephaven_cli.client import DeephavenClient
    from deephaven_cli.repl.console import DeephavenConsole

    animation_proc = None

    if verbose:
        print(f"Starting Deephaven server on port {port}...")
        print("(this may take a moment for JVM initialization)")
    else:
        # Start animation in a separate process (avoids GIL)
        animation_proc = subprocess.Popen(
            [sys.executable, "-c", _ANIMATION_SCRIPT],
            stdout=sys.stdout,
            stderr=subprocess.DEVNULL,
        )

    try:
        with DeephavenServer(port=port, jvm_args=jvm_args, quiet=not verbose) as server:
            actual_port = server.actual_port
            if verbose:
                print(f"Server started on port {actual_port}. Connecting client...")

            with DeephavenClient(port=actual_port) as client:
                # Stop animation now that we're connected
                if animation_proc:
                    animation_proc.terminate()
                    animation_proc.wait(timeout=1.0)
                    # Clear the connecting message
                    sys.stdout.write("\r" + " " * 25 + "\r")
                    sys.stdout.flush()

                if verbose:
                    print("Connected!\n")
                    print("Deephaven REPL")
                    print("Type 'exit()' or press Ctrl+D to quit.\n")

                console = DeephavenConsole(client, port=actual_port, vi_mode=vi_mode)
                console.interact()

    except KeyboardInterrupt:
        if animation_proc:
            animation_proc.terminate()
            sys.stdout.write("\r" + " " * 25 + "\r")
            sys.stdout.flush()
        print("\nInterrupted.")
        return EXIT_INTERRUPTED
    except Exception as e:
        if animation_proc:
            animation_proc.terminate()
            sys.stdout.write("\r" + " " * 25 + "\r")
            sys.stdout.flush()
        print(f"Error: {e}", file=sys.stderr)
        return EXIT_CONNECTION_ERROR

    return EXIT_SUCCESS


def run_exec(
    script_path: str | None,
    code: str | None,
    port: int,
    jvm_args: list[str],
    verbose: bool,
    timeout: int | None,
    show_tables: bool,
    no_table_meta: bool = False,
) -> int:
    """Execute a script in batch mode (agent-friendly)."""
    from deephaven_cli.server import DeephavenServer
    from deephaven_cli.client import DeephavenClient
    from deephaven_cli.repl.executor import CodeExecutor

    # Determine code source: -c argument, file, or stdin
    if code is not None and script_path is not None:
        print("Error: Cannot use both -c and a script file", file=sys.stderr)
        return EXIT_CONNECTION_ERROR
    elif code is not None:
        script_content = code
    elif script_path is None:
        print("Error: Must provide either -c CODE or a script file", file=sys.stderr)
        return EXIT_CONNECTION_ERROR
    else:
        # Read from file or stdin
        try:
            if script_path == "-":
                script_content = sys.stdin.read()
            else:
                with open(script_path, "r") as f:
                    script_content = f.read()
        except FileNotFoundError:
            print(f"Error: Script file not found: {script_path}", file=sys.stderr)
            return EXIT_CONNECTION_ERROR
        except Exception as e:
            print(f"Error reading script: {e}", file=sys.stderr)
            return EXIT_CONNECTION_ERROR

    if not script_content.strip():
        # Empty script is a no-op success
        return EXIT_SUCCESS

    # Set up timeout using threading (SIGALRM doesn't work reliably with JNI/gRPC)
    timer = None
    if timeout:
        def timeout_killer():
            print(
                f"Error: Execution timed out after {timeout} seconds",
                file=sys.stderr,
                flush=True,
            )
            os._exit(EXIT_TIMEOUT)

        timer = threading.Timer(timeout, timeout_killer)
        timer.daemon = True
        timer.start()

    try:
        if verbose:
            print(f"Starting Deephaven server on port {port}...", file=sys.stderr)

        with DeephavenServer(port=port, jvm_args=jvm_args, quiet=not verbose) as server:
            actual_port = server.actual_port
            if verbose:
                print(f"Server on port {actual_port}. Connecting client...", file=sys.stderr)

            with DeephavenClient(port=actual_port) as client:
                if verbose:
                    print("Executing script...", file=sys.stderr)

                executor = CodeExecutor(client)
                result = executor.execute(script_content)

                # Cancel timeout now that execution is done
                if timer:
                    timer.cancel()

                # Output stdout (to stdout)
                if result.stdout:
                    print(result.stdout, end="")
                    if not result.stdout.endswith("\n"):
                        print()

                # Output stderr (to stderr)
                if result.stderr:
                    print(result.stderr, file=sys.stderr, end="")
                    if not result.stderr.endswith("\n"):
                        print(file=sys.stderr)

                # Output expression result (to stdout)
                if result.result_repr is not None and result.result_repr != "None":
                    print(result.result_repr)

                # Show assigned tables if requested (covers new and reassigned)
                if show_tables and result.assigned_tables:
                    show_meta = not no_table_meta
                    for table_name in result.assigned_tables:
                        preview, meta = executor.get_table_preview(
                            table_name,
                            show_meta=show_meta,
                        )
                        if meta is not None and not no_table_meta:
                            status = "refreshing" if meta.is_refreshing else "static"
                            print(f"\n=== Table: {table_name} ({meta.row_count:,} rows, {status}) ===")
                        else:
                            print(f"\n=== Table: {table_name} ===")
                        print(preview)

                # Check for errors
                if result.error:
                    print(result.error, file=sys.stderr)
                    # Check if error might be due to shell backtick interpretation
                    hint = _suggest_backtick_hint(script_content, result.error)
                    if hint:
                        print(hint, file=sys.stderr)
                    return EXIT_SCRIPT_ERROR

                return EXIT_SUCCESS

    except KeyboardInterrupt:
        print("\nInterrupted.", file=sys.stderr)
        return EXIT_INTERRUPTED
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        return EXIT_CONNECTION_ERROR
    finally:
        # Always cancel the timer
        if timer:
            timer.cancel()


def run_app(script_path: str, port: int, jvm_args: list[str], verbose: bool = False) -> int:
    """Run a script and keep the server alive until interrupted."""
    from deephaven_cli.server import DeephavenServer
    from deephaven_cli.client import DeephavenClient

    # Read the script
    try:
        with open(script_path, "r") as f:
            script_content = f.read()
    except FileNotFoundError:
        print(f"Error: Script file not found: {script_path}", file=sys.stderr)
        return EXIT_CONNECTION_ERROR
    except Exception as e:
        print(f"Error reading script: {e}", file=sys.stderr)
        return EXIT_CONNECTION_ERROR

    if verbose:
        print(f"Starting Deephaven server on port {port}...", flush=True)

    try:
        with DeephavenServer(port=port, jvm_args=jvm_args, quiet=not verbose) as server:
            actual_port = server.actual_port
            if verbose:
                print(f"Server started on port {actual_port}", flush=True)

            with DeephavenClient(port=actual_port) as client:
                if verbose:
                    print(f"Running {script_path}...", flush=True)

                # Run the script directly (no output capture wrapper)
                try:
                    client.run_script(script_content)
                except Exception as e:
                    print(f"Script error: {e}", file=sys.stderr, flush=True)
                    return EXIT_SCRIPT_ERROR

                if verbose:
                    print("Script executed. Server running. Press Ctrl+C to stop.", flush=True)

                # Keep alive until interrupted
                try:
                    while True:
                        time.sleep(1)
                except KeyboardInterrupt:
                    print("\nShutting down...", flush=True)

    except KeyboardInterrupt:
        print("\nInterrupted.", flush=True)
        return EXIT_INTERRUPTED
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr, flush=True)
        return EXIT_CONNECTION_ERROR

    return EXIT_SUCCESS


if __name__ == "__main__":
    sys.exit(main())
