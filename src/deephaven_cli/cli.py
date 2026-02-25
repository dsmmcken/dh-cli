"""Main CLI entry point for deephaven-cli."""
from __future__ import annotations

import argparse
import os
import shutil
import subprocess as _subprocess
import sys
import threading
import time

# Exit codes
EXIT_SUCCESS = 0
EXIT_SCRIPT_ERROR = 1
EXIT_CONNECTION_ERROR = 2
EXIT_TIMEOUT = 3
EXIT_INTERRUPTED = 130

# Commands that require a resolved + activated Deephaven version
_RUNTIME_COMMANDS = {"repl", "exec", "serve"}


def _is_wsl() -> bool:
    """Check if running inside WSL."""
    try:
        with open("/proc/version", "r") as f:
            return "microsoft" in f.read().lower()
    except OSError:
        return False


def _open_browser(url: str) -> None:
    """Open URL in the default browser, handling WSL."""
    try:
        if _is_wsl():
            _subprocess.Popen(
                ["cmd.exe", "/c", "start", url],
                stdout=_subprocess.DEVNULL,
                stderr=_subprocess.DEVNULL,
            )
        else:
            import webbrowser
            webbrowser.open(url)
    except Exception:
        pass


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


def _add_connection_args(parser: argparse.ArgumentParser) -> None:
    """Add common connection arguments to a parser."""
    conn_group = parser.add_argument_group("connection options")
    conn_group.add_argument(
        "--host",
        help="Connect to remote server (skips embedded server)",
    )
    conn_group.add_argument(
        "--auth-type",
        default=os.environ.get("DH_AUTH_TYPE", "Anonymous"),
        help="Authentication type: Anonymous, Basic, or custom (default: Anonymous)",
    )
    conn_group.add_argument(
        "--auth-token",
        default=os.environ.get("DH_AUTH_TOKEN", ""),
        help="Auth token (for Basic: 'user:password'). Can use DH_AUTH_TOKEN env var",
    )

    tls_group = parser.add_argument_group("TLS options")
    tls_group.add_argument(
        "--tls",
        action="store_true",
        help="Enable TLS/SSL encryption",
    )
    tls_group.add_argument(
        "--tls-ca-cert",
        help="Path to CA certificate PEM file",
    )
    tls_group.add_argument(
        "--tls-client-cert",
        help="Path to client certificate PEM file (mutual TLS)",
    )
    tls_group.add_argument(
        "--tls-client-key",
        help="Path to client private key PEM file (mutual TLS)",
    )


def _add_version_flag(parser: argparse.ArgumentParser) -> None:
    """Add --version flag for selecting Deephaven version."""
    parser.add_argument(
        "--version",
        dest="dh_version",
        metavar="VERSION",
        help="Deephaven version to use (default: auto-resolved)",
    )


def _read_cert_file(path: str | None) -> bytes | None:
    """Read a certificate file and return its contents as bytes."""
    if path is None:
        return None
    try:
        with open(path, "rb") as f:
            return f.read()
    except FileNotFoundError:
        print(f"Error: Certificate file not found: {path}", file=sys.stderr)
        sys.exit(EXIT_CONNECTION_ERROR)
    except Exception as e:
        print(f"Error reading certificate file {path}: {e}", file=sys.stderr)
        sys.exit(EXIT_CONNECTION_ERROR)


def _get_client_kwargs(args: argparse.Namespace) -> dict:
    """Build client kwargs from parsed arguments."""
    return {
        "host": args.host or "localhost",
        "port": args.port,
        "auth_type": args.auth_type,
        "auth_token": args.auth_token,
        "use_tls": args.tls,
        "tls_root_certs": _read_cert_file(args.tls_ca_cert),
        "client_cert_chain": _read_cert_file(args.tls_client_cert),
        "client_private_key": _read_cert_file(args.tls_client_key),
    }


def _resolve_and_activate(args: argparse.Namespace) -> int | None:
    """Resolve and activate the Deephaven version for a runtime command.

    Returns None on success, or an exit code on failure.
    """
    from deephaven_cli.manager.activate import activate_version
    from deephaven_cli.manager.config import resolve_version

    cli_version = getattr(args, "dh_version", None)
    version = resolve_version(cli_version=cli_version)
    if version is None:
        print("Error: No Deephaven version installed.", file=sys.stderr)
        print("\n  dh install latest    Install the latest version", file=sys.stderr)
        print("  dh install 41.1      Install a specific version", file=sys.stderr)
        return EXIT_CONNECTION_ERROR
    try:
        activate_version(version)
    except RuntimeError as e:
        print(f"Error: {e}", file=sys.stderr)
        return EXIT_CONNECTION_ERROR
    return None


DESCRIPTION = """\
Deephaven CLI - Command-line tool for Deephaven servers

Manage Deephaven versions and launch servers with real-time
data capabilities.
"""

EPILOG = """\
Version management:
  dh install                           Install latest Deephaven
  dh install 41.1                      Install a specific version
  dh uninstall 41.1                    Remove a version
  dh use 41.1                          Set global default version
  dh versions                          List installed versions

Runtime:
  dh repl                              Start interactive session
  dh exec script.py                    Run script and exit
  dh serve dashboard.py                Long-running server

Tools:
  dh list                              Show running servers
  dh kill 10000                        Stop server on port
  dh doctor                            Check environment health

Use 'dh <command> --help' for more details.
"""


def main() -> int:
    """Main entry point."""
    # Handle shorthand: dh -c "code" -> dh exec -c "code"
    if len(sys.argv) >= 2 and sys.argv[1] == "-c":
        sys.argv.insert(1, "exec")

    # Handle "dh java install" as a two-word subcommand
    if len(sys.argv) >= 3 and sys.argv[1] == "java" and sys.argv[2] == "install":
        sys.argv[1:3] = ["java-install"]

    parser = argparse.ArgumentParser(
        prog="dh",
        description=DESCRIPTION,
        epilog=EPILOG,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    subparsers = parser.add_subparsers(dest="command")

    # --- Manager commands (no Deephaven deps needed) ---

    # install subcommand
    install_parser = subparsers.add_parser(
        "install",
        help="Install a Deephaven version",
        description="Install a Deephaven version into ~/.dh/versions/.\n\n"
                    "Downloads deephaven-server, pydeephaven, and default plugins\n"
                    "into an isolated venv managed by uv.",
        epilog="Examples:\n"
               "  dh install              Install the latest version\n"
               "  dh install 41.1         Install a specific version\n"
               "  dh install latest       Same as 'dh install'",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    install_parser.add_argument(
        "install_version",
        nargs="?",
        default="latest",
        metavar="VERSION",
        help="Version to install (default: latest)",
    )

    # uninstall subcommand
    uninstall_parser = subparsers.add_parser(
        "uninstall",
        help="Remove an installed Deephaven version",
        description="Remove a previously installed Deephaven version.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    uninstall_parser.add_argument(
        "uninstall_version",
        metavar="VERSION",
        help="Version to remove",
    )

    # use subcommand
    use_parser = subparsers.add_parser(
        "use",
        help="Set the default Deephaven version",
        description="Set the global default version, or write a .dhrc file for the current directory.",
        epilog="Examples:\n"
               "  dh use 41.1             Set global default\n"
               "  dh use 41.1 --local     Write .dhrc in current directory",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    use_parser.add_argument(
        "use_version",
        metavar="VERSION",
        help="Version to set as default",
    )
    use_parser.add_argument(
        "--local",
        action="store_true",
        help="Write .dhrc in current directory instead of global config",
    )

    # versions subcommand
    versions_parser = subparsers.add_parser(
        "versions",
        help="List installed Deephaven versions",
        description="Show all locally installed Deephaven versions.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    versions_parser.add_argument(
        "--remote",
        action="store_true",
        help="Also show versions available from PyPI",
    )

    # java subcommand (status)
    subparsers.add_parser(
        "java",
        help="Show Java status",
        description="Detect and display information about the current Java installation.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )

    # java install (handled via argv rewrite as "java-install")
    subparsers.add_parser(
        "java-install",
        help="Download Eclipse Temurin JDK 21",
        description="Download and install Eclipse Temurin JDK 21 into ~/.dh/java/.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )

    # doctor subcommand
    subparsers.add_parser(
        "doctor",
        help="Check environment health",
        description="Run diagnostic checks on the Deephaven CLI environment.\n\n"
                    "Checks Java installation, installed versions, and uv availability.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )

    # config subcommand
    config_parser = subparsers.add_parser(
        "config",
        help="Show or edit Deephaven CLI configuration",
        description="Show the current configuration from ~/.dh/config.toml.\n\n"
                    "Use --set to change a configuration value.",
        epilog="Examples:\n"
               "  dh config                        Show all config\n"
               "  dh config --set default_version 41.1\n",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    config_parser.add_argument(
        "--set",
        nargs=2,
        metavar=("KEY", "VALUE"),
        help="Set a configuration key to a value",
    )

    # --- Runtime commands (need activated Deephaven version) ---

    # repl subcommand
    repl_parser = subparsers.add_parser(
        "repl",
        help="Start an interactive REPL session",
        description="Interactive Python REPL with Deephaven server context.\n\n"
                    "Provides full Python environment with Deephaven imports,\n"
                    "tab completion, and direct table manipulation.\n\n"
                    "By default starts an embedded server. Use --host to connect\n"
                    "to an existing remote server instead.",
        epilog="Examples:\n"
               "  dh repl                              Embedded server\n"
               "  dh repl --host myserver.com          Remote server\n"
               "  dh repl --host localhost --port 8080 Remote on localhost\n"
               "  dh repl --jvm-args -Xmx8g            Custom JVM memory",
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
        help="JVM arguments for embedded server (default: %(default)s)",
    )
    repl_parser.add_argument(
        "--verbose",
        "-v",
        action="store_true",
        help="Show startup/connection messages",
    )
    repl_parser.add_argument(
        "--vi",
        action="store_true",
        help="Use Vi key bindings (default: Emacs)",
    )
    _add_version_flag(repl_parser)
    _add_connection_args(repl_parser)

    # exec subcommand (agent-friendly batch mode)
    exec_parser = subparsers.add_parser(
        "exec",
        help="Execute a script and exit (ideal for automation)",
        description="Execute a Python script in batch mode.\n\n"
                    "Best for automation and AI agents:\n"
                    "  - Clean stdout/stderr separation\n"
                    "  - Structured exit codes\n"
                    "  - Optional timeout\n\n"
                    "By default starts an embedded server. Use --host to connect\n"
                    "to an existing remote server instead.",
        epilog="Exit codes:\n"
               "  0   Success\n"
               "  1   Script error\n"
               "  2   Connection error\n"
               "  3   Timeout\n"
               "  130 Interrupted\n\n"
               "Examples:\n"
               "  dh exec script.py\n"
               "  dh exec script.py --host remote.example.com\n"
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
        help="JVM arguments for embedded server (default: %(default)s)",
    )
    exec_parser.add_argument(
        "--verbose",
        "-v",
        action="store_true",
        help="Show startup/connection messages",
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
    _add_version_flag(exec_parser)
    _add_connection_args(exec_parser)

    # serve subcommand (long-running application mode)
    serve_parser = subparsers.add_parser(
        "serve",
        help="Run script and keep server alive (dashboards/services)",
        description="Run a script and keep the Deephaven server running.\n\n"
                    "Use for:\n"
                    "  - Dashboards and visualizations\n"
                    "  - Long-running data pipelines\n"
                    "  - Services that need persistent server\n\n"
                    "Opens browser automatically. Server runs until Ctrl+C.",
        epilog="Examples:\n"
               "  dh serve dashboard.py\n"
               "  dh serve dashboard.py --port 8080\n"
               "  dh serve dashboard.py --iframe my_widget\n"
               "  dh serve dashboard.py --no-browser",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    serve_parser.add_argument(
        "script",
        help="Python script to execute",
    )
    serve_parser.add_argument(
        "--port",
        type=int,
        default=10000,
        help="Server port (default: %(default)s)",
    )
    serve_parser.add_argument(
        "--jvm-args",
        nargs="*",
        default=["-Xmx4g"],
        help="JVM arguments (default: %(default)s). Example: -Xmx8g",
    )
    serve_parser.add_argument(
        "--verbose",
        "-v",
        action="store_true",
        help="Show startup messages (default: quiet)",
    )
    serve_parser.add_argument(
        "--no-browser",
        action="store_true",
        help="Don't open browser automatically",
    )
    serve_parser.add_argument(
        "--iframe",
        metavar="WIDGET",
        help="Open browser to iframe URL for the given widget name",
    )
    _add_version_flag(serve_parser)

    # --- Tool commands (unchanged) ---

    # list subcommand
    subparsers.add_parser(
        "list",
        help="List running Deephaven servers",
        description="Discover and list all running Deephaven servers on this machine.\n\n"
                    "Finds servers started via dh-cli, Docker, or standalone Java.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )

    # kill subcommand
    kill_parser = subparsers.add_parser(
        "kill",
        help="Stop a running Deephaven server",
        description="Stop a Deephaven server by port number.\n\n"
                    "Works with dh-cli processes (SIGTERM) and Docker containers (docker stop).",
        epilog="Examples:\n"
               "  dh kill 10000\n"
               "  dh kill 8080",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    kill_parser.add_argument(
        "port",
        type=int,
        help="Port of the server to stop",
    )

    # lint subcommand
    lint_parser = subparsers.add_parser(
        "lint",
        help="Run ruff check on the project",
        description="Run ruff check on the current directory (or a specific file/folder).",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    lint_parser.add_argument(
        "path",
        nargs="?",
        default=".",
        help="File or directory to lint (default: current directory)",
    )
    lint_parser.add_argument(
        "--fix",
        action="store_true",
        help="Automatically fix lint issues",
    )
    lint_parser.add_argument(
        "extra",
        nargs=argparse.REMAINDER,
        help="Extra args passed to ruff (after --)",
    )

    # format subcommand
    format_parser = subparsers.add_parser(
        "format",
        help="Run ruff format on the project",
        description="Run ruff format on the current directory (or a specific file/folder).",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    format_parser.add_argument(
        "path",
        nargs="?",
        default=".",
        help="File or directory to format (default: current directory)",
    )
    format_parser.add_argument(
        "--check",
        action="store_true",
        help="Check formatting without making changes",
    )
    format_parser.add_argument(
        "extra",
        nargs=argparse.REMAINDER,
        help="Extra args passed to ruff (after --)",
    )

    # typecheck subcommand
    typecheck_parser = subparsers.add_parser(
        "typecheck",
        help="Run ty check on the project",
        description="Run ty check on the current directory (or a specific file/folder).",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    typecheck_parser.add_argument(
        "path",
        nargs="?",
        default=".",
        help="File or directory to check (default: current directory)",
    )
    typecheck_parser.add_argument(
        "extra",
        nargs=argparse.REMAINDER,
        help="Extra args passed to ty (after --)",
    )

    args = parser.parse_args()

    if args.command is None:
        if sys.stdin.isatty():
            return run_management_tui()
        parser.print_help()
        return EXIT_SUCCESS

    # --- Manager commands dispatch (no Deephaven activation needed) ---

    if args.command == "install":
        return run_install(args.install_version)
    elif args.command == "uninstall":
        return run_uninstall(args.uninstall_version)
    elif args.command == "use":
        return run_use(args.use_version, local=args.local)
    elif args.command == "versions":
        return run_versions(remote=args.remote)
    elif args.command == "java":
        return run_java_status()
    elif args.command == "java-install":
        return run_java_install()
    elif args.command == "doctor":
        return run_doctor()
    elif args.command == "config":
        return run_config(set_pair=args.set)

    # --- Runtime commands dispatch (need activated version) ---

    if args.command in _RUNTIME_COMMANDS:
        exit_code = _resolve_and_activate(args)
        if exit_code is not None:
            return exit_code

    if args.command == "repl":
        return run_repl(
            port=args.port,
            jvm_args=args.jvm_args,
            verbose=args.verbose,
            vi_mode=args.vi,
            host=args.host,
            client_kwargs=_get_client_kwargs(args) if args.host else None,
        )
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
            host=args.host,
            client_kwargs=_get_client_kwargs(args) if args.host else None,
        )
    elif args.command == "serve":
        return run_serve(args.script, args.port, args.jvm_args, args.verbose, args.no_browser, args.iframe)

    # --- Tool commands dispatch (unchanged) ---

    elif args.command == "list":
        return run_list()
    elif args.command == "kill":
        return run_kill(args.port)
    elif args.command == "lint":
        return run_lint(path=args.path, fix=args.fix, extra=args.extra)
    elif args.command == "format":
        return run_format(path=args.path, check=args.check, extra=args.extra)
    elif args.command == "typecheck":
        return run_typecheck(path=args.path, extra=args.extra)

    return EXIT_SUCCESS


# ---------------------------------------------------------------------------
# Manager command handlers
# ---------------------------------------------------------------------------

def run_management_tui() -> int:
    """Launch the interactive management TUI."""
    from deephaven_cli.tui.app import run_management_tui as _run_tui

    result = _run_tui()
    if result == "launch-repl":
        # Activate the Deephaven version before launching the REPL
        from deephaven_cli.manager.activate import activate_version
        from deephaven_cli.manager.config import resolve_version

        version = resolve_version()
        if version is None:
            print("Error: No Deephaven version installed.", file=sys.stderr)
            print("\n  dh install latest    Install the latest version", file=sys.stderr)
            return EXIT_CONNECTION_ERROR
        try:
            activate_version(version)
        except RuntimeError as e:
            print(f"Error: {e}", file=sys.stderr)
            return EXIT_CONNECTION_ERROR
        return run_repl(port=10000, jvm_args=["-Xmx4g"])
    elif result == "launch-serve":
        print("Use: dh serve <script.py>", file=sys.stderr)
        return EXIT_SUCCESS
    elif result == "launch-exec":
        print("Use: dh exec <script.py> or dh exec -c '<code>'", file=sys.stderr)
        return EXIT_SUCCESS
    return EXIT_SUCCESS


def run_install(version: str) -> int:
    """Install a Deephaven version."""
    from deephaven_cli.manager.pypi import fetch_latest_version, is_valid_version
    from deephaven_cli.manager.versions import install_version, is_version_installed

    # Resolve "latest" to an actual version
    if version == "latest":
        try:
            version = fetch_latest_version()
            print(f"Latest version: {version}")
        except Exception as e:
            print(f"Error: Could not determine latest version: {e}", file=sys.stderr)
            return EXIT_CONNECTION_ERROR

    if is_version_installed(version):
        print(f"Version {version} is already installed.")
        return EXIT_SUCCESS

    # Validate version exists on PyPI
    try:
        if not is_valid_version(version):
            print(f"Error: Version {version} not found on PyPI.", file=sys.stderr)
            return EXIT_SCRIPT_ERROR
    except Exception as e:
        print(f"Warning: Could not validate version on PyPI: {e}", file=sys.stderr)

    from rich.console import Console

    console = Console(stderr=True)
    console.print(f"Installing Deephaven {version}...")

    def _on_progress(msg: str) -> None:
        console.print(f"  {msg}")

    success = install_version(version, on_progress=_on_progress)
    if success:
        console.print(f"[green]Deephaven {version} installed successfully.[/green]")
        # Set as default if it's the first version
        from deephaven_cli.manager.config import get_default_version, set_default_version
        if get_default_version() is None:
            set_default_version(version)
            print(f"Set {version} as the default version.")
        return EXIT_SUCCESS
    else:
        print(f"Error: Failed to install Deephaven {version}.", file=sys.stderr)
        return EXIT_SCRIPT_ERROR


def run_uninstall(version: str) -> int:
    """Remove an installed Deephaven version."""
    from deephaven_cli.manager.versions import uninstall_version

    if uninstall_version(version):
        print(f"Deephaven {version} uninstalled.")
        # Clear default if this was the default version
        from deephaven_cli.manager.config import (
            get_default_version,
            get_latest_installed_version,
            load_config,
            save_config,
            set_default_version,
        )
        if get_default_version() == version:
            latest = get_latest_installed_version()
            if latest:
                set_default_version(latest)
                print(f"Default version changed to {latest}.")
            else:
                config = load_config()
                config.pop("default_version", None)
                save_config(config)
                print("No versions remaining. Default version cleared.")
        return EXIT_SUCCESS
    else:
        print(f"Error: Version {version} is not installed.", file=sys.stderr)
        return EXIT_SCRIPT_ERROR


def run_use(version: str, local: bool = False) -> int:
    """Set the default Deephaven version."""
    from deephaven_cli.manager.versions import is_version_installed

    if not is_version_installed(version):
        print(f"Error: Version {version} is not installed.", file=sys.stderr)
        print(f"Install it first: dh install {version}", file=sys.stderr)
        return EXIT_SCRIPT_ERROR

    if local:
        from pathlib import Path

        from deephaven_cli.manager.config import write_dhrc
        write_dhrc(Path.cwd(), version)
        print(f"Set local version to {version} (.dhrc written).")
    else:
        from deephaven_cli.manager.config import set_default_version
        set_default_version(version)
        print(f"Set global default version to {version}.")
    return EXIT_SUCCESS


def run_versions(remote: bool = False) -> int:
    """List installed Deephaven versions."""
    from deephaven_cli.manager.versions import list_installed_versions

    installed = list_installed_versions()
    if not installed:
        print("No Deephaven versions installed.")
        print("Install one with: dh install")
        return EXIT_SUCCESS

    print("Installed versions:")
    for info in installed:
        marker = " (default)" if info["is_default"] else ""
        print(f"  {info['version']}{marker}  [installed {info['installed_date']}]")

    if remote:
        print()
        try:
            from deephaven_cli.manager.pypi import fetch_available_versions
            available = fetch_available_versions()
            installed_set = {v["version"] for v in installed}
            remote_only = [v for v in available if v not in installed_set]
            if remote_only:
                print(f"Available from PyPI ({len(remote_only)} more):")
                for v in remote_only[:20]:
                    print(f"  {v}")
                if len(remote_only) > 20:
                    print(f"  ... and {len(remote_only) - 20} more")
            else:
                print("All available versions are installed.")
        except Exception as e:
            print(f"Error fetching remote versions: {e}", file=sys.stderr)

    return EXIT_SUCCESS


def run_java_status() -> int:
    """Show Java status."""
    from deephaven_cli.manager.java import detect_java

    info = detect_java()
    if info is None:
        print("No compatible Java found (requires Java >= 17).")
        print("\nInstall Java with: dh java install")
        return EXIT_SCRIPT_ERROR

    print(f"Java {info['version']}")
    print(f"  Path: {info['path']}")
    print(f"  Home: {info['home']}")
    print(f"  Source: {info['source']}")
    return EXIT_SUCCESS


def run_java_install() -> int:
    """Download and install Eclipse Temurin JDK."""
    from deephaven_cli.manager.java import detect_java, install_java

    existing = detect_java()
    if existing:
        print(f"Java {existing['version']} already available ({existing['source']}).")
        print("Proceeding with download anyway...")

    try:
        jdk_home = install_java()
        print(f"Java installed to: {jdk_home}")
        return EXIT_SUCCESS
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        return EXIT_SCRIPT_ERROR


def run_doctor() -> int:
    """Check environment health."""
    from deephaven_cli.manager.config import get_default_version, get_installed_versions
    from deephaven_cli.manager.java import detect_java

    print("Deephaven CLI Doctor\n")
    all_ok = True

    # Check uv
    uv_path = shutil.which("uv")
    if uv_path:
        print(f"  [ok] uv: {uv_path}")
    else:
        print("  [!!] uv: not found (required for installing versions)")
        all_ok = False

    # Check Java
    java_info = detect_java()
    if java_info:
        print(f"  [ok] Java {java_info['version']} ({java_info['source']})")
    else:
        print("  [!!] Java: not found (requires >= 17, run 'dh java install')")
        all_ok = False

    # Check installed versions
    versions = get_installed_versions()
    if versions:
        print(f"  [ok] Installed versions: {len(versions)}")
        default = get_default_version()
        if default:
            print(f"  [ok] Default version: {default}")
        else:
            print("  [!!] No default version set (run 'dh use VERSION')")
            all_ok = False
    else:
        print("  [!!] No Deephaven versions installed (run 'dh install')")
        all_ok = False

    print()
    if all_ok:
        print("Everything looks good!")
    else:
        print("Some issues found. See above for details.")
    return EXIT_SUCCESS if all_ok else EXIT_SCRIPT_ERROR


def run_config(set_pair: list[str] | None = None) -> int:
    """Show or edit configuration."""
    from deephaven_cli.manager.config import _config_path, load_config, save_config

    if set_pair:
        key, value = set_pair
        config = load_config()
        config[key] = value
        save_config(config)
        print(f"Set {key} = {value}")
        return EXIT_SUCCESS

    config = load_config()
    path = _config_path()
    print(f"Config: {path}\n")
    if not config:
        print("(empty -- no settings configured)")
        return EXIT_SUCCESS
    for key, value in config.items():
        print(f"  {key} = {value}")
    return EXIT_SUCCESS


# ---------------------------------------------------------------------------
# Animation scripts for runtime commands
# ---------------------------------------------------------------------------

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

_CONNECTING_ANIMATION_SCRIPT = '''
import sys
import time
frame = 0
while True:
    num_dots = (frame % 3) + 1
    sys.stdout.write(f"\\rConnecting{'.' * num_dots:<3}")
    sys.stdout.flush()
    frame += 1
    time.sleep(0.25)
'''


# ---------------------------------------------------------------------------
# Runtime command handlers
# ---------------------------------------------------------------------------

def _run_repl_interactive(client, port: int, vi_mode: bool, host: str | None = None) -> None:
    """Launch the Textual TUI REPL (interactive TTY mode)."""
    from deephaven_cli.repl.app import DeephavenREPLApp

    app = DeephavenREPLApp(client, port=port, vi_mode=vi_mode, host=host)
    app.run()
    if app.return_code == 1:
        print("\nServer disconnected.")


def _run_repl_fallback(client, port: int, vi_mode: bool, host: str | None = None) -> None:
    """Launch the plain-text console REPL (piped / non-TTY mode)."""
    from deephaven_cli.repl.console import DeephavenConsole

    console = DeephavenConsole(client, port=port, vi_mode=vi_mode)
    console.interact()


def run_repl(
    port: int,
    jvm_args: list[str],
    verbose: bool = False,
    vi_mode: bool = False,
    host: str | None = None,
    client_kwargs: dict | None = None,
) -> int:
    """Run the interactive REPL."""
    import subprocess

    from deephaven_cli.client import DeephavenClient

    use_tui = sys.stdin.isatty()
    run_console = _run_repl_interactive if use_tui else _run_repl_fallback

    animation_proc = None

    # Remote mode: connect directly to existing server
    if host:
        if verbose:
            print(f"Connecting to {host}:{port}...")
        elif not use_tui:
            animation_proc = subprocess.Popen(
                [sys.executable, "-c", _CONNECTING_ANIMATION_SCRIPT],
                stdout=sys.stdout,
                stderr=subprocess.DEVNULL,
            )

        try:
            with DeephavenClient(**client_kwargs) as client:
                if animation_proc:
                    animation_proc.terminate()
                    animation_proc.wait(timeout=1.0)
                    sys.stdout.write("\r" + " " * 25 + "\r")
                    sys.stdout.flush()

                if verbose and not use_tui:
                    print(f"Connected to {host}:{port}\n")
                    print("Deephaven REPL (remote)")
                    print("Type 'exit()' or press Ctrl+D to quit.\n")

                run_console(client, port=port, vi_mode=vi_mode, host=host)

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
            print(f"Error: Failed to connect to {host}:{port}: {e}", file=sys.stderr)
            return EXIT_CONNECTION_ERROR

        return EXIT_SUCCESS

    # Embedded mode: start server, then connect
    from deephaven_cli.server import DeephavenServer

    if verbose:
        print(f"Starting Deephaven server on port {port}...")
        print("(this may take a moment for JVM initialization)")
    elif not use_tui:
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
                if animation_proc:
                    animation_proc.terminate()
                    animation_proc.wait(timeout=1.0)
                    sys.stdout.write("\r" + " " * 25 + "\r")
                    sys.stdout.flush()

                if verbose and not use_tui:
                    print("Connected!\n")
                    print("Deephaven REPL")
                    print("Type 'exit()' or press Ctrl+D to quit.\n")

                run_console(client, port=actual_port, vi_mode=vi_mode, host=host)

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
    host: str | None = None,
    client_kwargs: dict | None = None,
) -> int:
    """Execute a script in batch mode (agent-friendly)."""
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

    # Resolve caller context so scripts can find adjacent files
    caller_cwd = os.getcwd()
    abs_script_path = (
        os.path.abspath(script_path)
        if script_path and script_path != "-"
        else None
    )

    def _execute_with_client(client: DeephavenClient) -> int:
        """Execute script with the given client."""
        if verbose:
            print("Executing script...", file=sys.stderr)

        executor = CodeExecutor(client)
        result = executor.execute(
            script_content,
            script_path=abs_script_path,
            cwd=caller_cwd,
        )

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
            hint = _suggest_backtick_hint(script_content, result.error)
            if hint:
                print(hint, file=sys.stderr)
            return EXIT_SCRIPT_ERROR

        return EXIT_SUCCESS

    try:
        # Remote mode: connect directly to existing server
        if host:
            if verbose:
                print(f"Connecting to {host}:{port}...", file=sys.stderr)

            with DeephavenClient(**client_kwargs) as client:
                if verbose:
                    print(f"Connected to {host}:{port}", file=sys.stderr)
                return _execute_with_client(client)

        # Embedded mode: start server, then connect
        from deephaven_cli.server import DeephavenServer

        if verbose:
            print(f"Starting Deephaven server on port {port}...", file=sys.stderr)

        with DeephavenServer(port=port, jvm_args=jvm_args, quiet=not verbose) as server:
            actual_port = server.actual_port
            if verbose:
                print(f"Server on port {actual_port}. Connecting client...", file=sys.stderr)

            with DeephavenClient(port=actual_port) as client:
                return _execute_with_client(client)

    except KeyboardInterrupt:
        print("\nInterrupted.", file=sys.stderr)
        return EXIT_INTERRUPTED
    except Exception as e:
        if host:
            print(f"Error: Failed to connect to {host}:{port}: {e}", file=sys.stderr)
        else:
            print(f"Error: {e}", file=sys.stderr)
        return EXIT_CONNECTION_ERROR
    finally:
        if timer:
            timer.cancel()


# ---------------------------------------------------------------------------
# Tool command handlers (unchanged)
# ---------------------------------------------------------------------------

def run_list() -> int:
    """List all running Deephaven servers."""
    from deephaven_cli.discovery import discover_servers, format_server_list

    servers = discover_servers()
    print(format_server_list(servers))
    return EXIT_SUCCESS


def run_kill(port: int) -> int:
    """Kill a Deephaven server by port."""
    from deephaven_cli.discovery import kill_server

    success, message = kill_server(port)
    if success:
        print(message)
        return EXIT_SUCCESS
    else:
        print(message, file=sys.stderr)
        return EXIT_CONNECTION_ERROR


def _strip_separator(extra: list[str] | None) -> list[str]:
    """Strip leading '--' from extra args."""
    return [a for a in (extra or []) if a != "--"]


def run_lint(path: str = ".", fix: bool = False, extra: list[str] | None = None) -> int:
    """Run ruff check on the project."""
    cmd = [sys.executable, "-m", "ruff", "check", path]
    if fix:
        cmd.append("--fix")
    cmd.extend(_strip_separator(extra))
    try:
        return _subprocess.run(cmd).returncode
    except FileNotFoundError:
        print("Error: ruff not found. Install with: uv tool install -e '.[dev]'", file=sys.stderr)
        return EXIT_SCRIPT_ERROR


def run_format(path: str = ".", check: bool = False, extra: list[str] | None = None) -> int:
    """Run ruff format on the project."""
    cmd = [sys.executable, "-m", "ruff", "format", path]
    if check:
        cmd.append("--check")
    cmd.extend(_strip_separator(extra))
    try:
        return _subprocess.run(cmd).returncode
    except FileNotFoundError:
        print("Error: ruff not found. Install with: uv tool install -e '.[dev]'", file=sys.stderr)
        return EXIT_SCRIPT_ERROR


def run_typecheck(path: str = ".", extra: list[str] | None = None) -> int:
    """Run ty check on the project."""
    cmd = [sys.executable, "-m", "ty", "check", path]
    cmd.extend(_strip_separator(extra))
    try:
        return _subprocess.run(cmd).returncode
    except FileNotFoundError:
        print("Error: ty not found. Install with: uv tool install -e '.[dev]'", file=sys.stderr)
        return EXIT_SCRIPT_ERROR


def run_serve(
    script_path: str, port: int, jvm_args: list[str],
    verbose: bool = False, no_browser: bool = False, iframe: str | None = None,
) -> int:
    """Run a script and keep the server alive until interrupted."""
    import subprocess

    from deephaven_cli.client import DeephavenClient
    from deephaven_cli.server import DeephavenServer

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

    animation_proc = None

    if verbose:
        print(f"Starting Deephaven server on port {port}...", flush=True)
    else:
        animation_proc = subprocess.Popen(
            [sys.executable, "-c", _ANIMATION_SCRIPT],
            stdout=sys.stdout,
            stderr=subprocess.DEVNULL,
        )

    try:
        with DeephavenServer(port=port, jvm_args=jvm_args, quiet=not verbose) as server:
            actual_port = server.actual_port
            if verbose:
                print(f"Server started on port {actual_port}", flush=True)

            with DeephavenClient(port=actual_port) as client:
                if animation_proc:
                    animation_proc.terminate()
                    animation_proc.wait(timeout=1.0)
                    sys.stdout.write("\r" + " " * 25 + "\r")
                    sys.stdout.flush()

                if verbose:
                    print(f"Running {script_path}...", flush=True)

                # Run the script directly (no output capture wrapper)
                try:
                    client.run_script(script_content)
                except Exception as e:
                    print(f"Script error: {e}", file=sys.stderr, flush=True)
                    return EXIT_SCRIPT_ERROR

                url = f"http://localhost:{actual_port}"
                if iframe:
                    url = f"{url}/iframe/widget/?name={iframe}"
                print(f"Server running at {url}", flush=True)
                print("Press Ctrl+C to stop.", flush=True)

                if not no_browser:
                    _open_browser(url)

                # Keep alive until interrupted
                try:
                    while True:
                        time.sleep(1)
                except KeyboardInterrupt:
                    print("\nShutting down...", flush=True)

    except KeyboardInterrupt:
        if animation_proc:
            animation_proc.terminate()
            sys.stdout.write("\r" + " " * 25 + "\r")
            sys.stdout.flush()
        print("\nInterrupted.", flush=True)
        return EXIT_INTERRUPTED
    except Exception as e:
        if animation_proc:
            animation_proc.terminate()
            sys.stdout.write("\r" + " " * 25 + "\r")
            sys.stdout.flush()
        print(f"Error: {e}", file=sys.stderr, flush=True)
        return EXIT_CONNECTION_ERROR

    return EXIT_SUCCESS


if __name__ == "__main__":
    sys.exit(main())
