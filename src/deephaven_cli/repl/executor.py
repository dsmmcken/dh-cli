"""Code execution with output capture using pickle for safe string transfer."""
from __future__ import annotations

import ast
import base64
import pickle
import textwrap
from dataclasses import dataclass
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from deephaven_cli.client import DeephavenClient


def get_assigned_names(code: str) -> set[str]:
    """Extract variable names being assigned in the code.

    Handles:
    - Simple assignments: t = ...
    - Tuple unpacking: a, b = ...
    - Annotated assignments: t: Table = ...
    - Augmented assignments: t += ...
    - Walrus operator: (t := ...)
    """
    try:
        tree = ast.parse(code)
    except SyntaxError:
        return set()

    names: set[str] = set()

    for node in ast.walk(tree):
        if isinstance(node, ast.Assign):
            # Handle: t = ... or a, b = ...
            for target in node.targets:
                names.update(_extract_names_from_target(target))
        elif isinstance(node, ast.AnnAssign) and node.target:
            # Handle: t: Table = ...
            names.update(_extract_names_from_target(node.target))
        elif isinstance(node, ast.AugAssign):
            # Handle: t += ...
            names.update(_extract_names_from_target(node.target))
        elif isinstance(node, ast.NamedExpr):
            # Handle: (t := ...)
            names.add(node.target.id)

    return names


def _extract_names_from_target(target: ast.expr) -> set[str]:
    """Extract variable names from an assignment target."""
    names: set[str] = set()
    if isinstance(target, ast.Name):
        names.add(target.id)
    elif isinstance(target, (ast.Tuple, ast.List)):
        for elt in target.elts:
            names.update(_extract_names_from_target(elt))
    # Ignore attribute assignments (obj.attr = ...) and subscripts (obj[key] = ...)
    return names


@dataclass
class TableMeta:
    """Metadata about a table."""
    row_count: int
    is_refreshing: bool
    columns: list[tuple[str, str]]  # (name, type) pairs


@dataclass
class ExecutionResult:
    """Result of code execution."""
    stdout: str
    stderr: str
    result_repr: str | None  # repr() of expression result, if any
    error: str | None  # Exception traceback, if any
    new_tables: list[str]  # Tables created by this execution
    assigned_tables: list[str]  # Tables assigned in this execution (new or reassigned)


class CodeExecutor:
    """Executes code on Deephaven server with output capture."""

    def __init__(self, client: DeephavenClient):
        self.client = client

    def execute(
        self,
        code: str,
        *,
        script_path: str | None = None,
        cwd: str | None = None,
    ) -> ExecutionResult:
        """Execute code and return captured output."""
        # Parse code to find assigned variable names
        assigned_names = get_assigned_names(code)

        # Get tables before execution
        tables_before = set(self.client.tables)

        # Build and execute the wrapper script (captures output + creates result table)
        wrapper = self._build_wrapper(code, script_path=script_path, cwd=cwd)

        try:
            self.client.run_script(wrapper)
        except KeyboardInterrupt:
            # Let this propagate for proper handling by CLI
            raise
        except Exception as e:
            # Script-level error (syntax error in wrapper, etc.)
            return ExecutionResult(
                stdout="",
                stderr="",
                result_repr=None,
                error=str(e),
                new_tables=[],
                assigned_tables=[],
            )

        # Read the result from the table
        result = self._read_result_table()

        # Clean up
        self._cleanup()

        # Detect new tables (excluding our internal one)
        tables_after = set(self.client.tables) - {"__dh_result_table"}
        new_tables = list(tables_after - tables_before)

        # Find assigned variables that are now tables on the server
        # This catches both new assignments and reassignments
        assigned_tables = [name for name in assigned_names if name in tables_after]

        return ExecutionResult(
            stdout=result.get("stdout", ""),
            stderr=result.get("stderr", ""),
            result_repr=result.get("result_repr"),
            error=result.get("error"),
            new_tables=new_tables,
            assigned_tables=assigned_tables,
        )

    def _build_wrapper(
        self,
        code: str,
        script_path: str | None = None,
        cwd: str | None = None,
    ) -> str:
        """Build the wrapper script that captures output and creates result table."""
        code_repr = repr(code)

        lines: list[str] = []

        # Optional: set CWD and __file__ so scripts can find adjacent files
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

    def _read_result_table(self) -> dict:
        """Read and decode the pickled results from the table."""
        session = self.client.session
        table = session.open_table("__dh_result_table")
        try:
            arrow_table = table.to_arrow()
            df = arrow_table.to_pandas()
            if len(df) > 0:
                encoded_data = df.iloc[0]["data"]
                # Decode base64 and unpickle
                pickled_bytes = base64.b64decode(encoded_data.encode("ascii"))
                return pickle.loads(pickled_bytes)
        except Exception as e:
            return {"error": f"Failed to read results: {e}"}
        return {}

    def _cleanup(self) -> None:
        """Clean up the result table from server namespace."""
        cleanup_script = """
try:
    del __dh_result_table
except NameError:
    pass
"""
        try:
            self.client.run_script(cleanup_script)
        except Exception:
            pass

    def get_table_arrow(
        self,
        table_name: str,
        max_rows: int = 10_000,
    ) -> tuple | None:
        """Get Arrow data and metadata for a table.

        Args:
            table_name: Name of the table on the server.
            max_rows: Maximum rows to transfer (default: 10,000).

        Returns:
            Tuple of (arrow_table, TableMeta) or None on error.
        """
        session = self.client.session
        try:
            import pyarrow as pa

            table = session.open_table(table_name)
            is_refreshing = table.is_refreshing

            # Fetch data — use slice for large tables to avoid transferring everything
            arrow_full = table.to_arrow()
            total_rows = arrow_full.num_rows
            schema = arrow_full.schema
            columns = [(field.name, str(field.type)) for field in schema]
            meta = TableMeta(total_rows, is_refreshing, columns)

            if total_rows <= max_rows:
                return arrow_full, meta
            else:
                return arrow_full.slice(0, max_rows), meta
        except Exception:
            return None

    def get_table_preview(
        self,
        table_name: str,
        rows: int = 10,
        show_meta: bool = True,
    ) -> tuple[str, TableMeta | None]:
        """Get a string preview of a table (used by fallback console).

        Args:
            table_name: Name of the table to preview
            rows: Number of rows to show (default: 10)
            show_meta: Include column types in output (default: True)

        Returns:
            Tuple of (preview_string, TableMeta or None on error)
        """
        session = self.client.session
        try:
            table = session.open_table(table_name)
            arrow_table = table.to_arrow()

            # Get metadata
            total_rows = arrow_table.num_rows
            is_refreshing = table.is_refreshing
            schema = arrow_table.schema
            columns = [(field.name, str(field.type)) for field in schema]

            meta = TableMeta(total_rows, is_refreshing, columns)

            # Build output
            lines = []

            if show_meta:
                # Format column types
                col_info = ", ".join(f"{name} ({typ})" for name, typ in columns)
                if len(f"Columns: {col_info}") > 80:
                    # Use row format for many columns
                    lines.append("Columns:")
                    for name, typ in columns:
                        lines.append(f"  {name} ({typ})")
                else:
                    lines.append(f"Columns: {col_info}")
                lines.append("")

            # Data preview
            if total_rows == 0:
                lines.append("(empty table)")
            else:
                preview_df = arrow_table.slice(0, rows).to_pandas()
                lines.append(preview_df.to_string(index=False))

            return "\n".join(lines), meta

        except Exception as e:
            return f"(error previewing table: {e})", None
