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

    def execute(self, code: str) -> ExecutionResult:
        """Execute code and return captured output."""
        # Parse code to find assigned variable names
        assigned_names = get_assigned_names(code)

        # Get tables before execution
        tables_before = set(self.client.tables)

        # Build and execute the wrapper script (captures output + creates result table)
        wrapper = self._build_wrapper(code)

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

    def _build_wrapper(self, code: str) -> str:
        """Build the wrapper script that captures output and creates result table."""
        code_repr = repr(code)

        # This script:
        # 1. Captures stdout/stderr
        # 2. Executes user code (trying eval first for expressions)
        # 3. Pickles results and base64 encodes (safe for Deephaven string column)
        # 4. Creates result table with the encoded data
        return textwrap.dedent(f'''
            import io as __dh_io
            import sys as __dh_sys
            import pickle as __dh_pickle
            import base64 as __dh_base64

            __dh_stdout_buf = __dh_io.StringIO()
            __dh_stderr_buf = __dh_io.StringIO()
            __dh_orig_stdout = __dh_sys.stdout
            __dh_orig_stderr = __dh_sys.stderr
            __dh_sys.stdout = __dh_stdout_buf
            __dh_sys.stderr = __dh_stderr_buf
            __dh_result = None
            __dh_error = None

            try:
                try:
                    __dh_result = eval({code_repr})
                except SyntaxError:
                    exec({code_repr})
            except Exception as __dh_e:
                import traceback as __dh_tb
                __dh_error = __dh_tb.format_exc()
            finally:
                __dh_sys.stdout = __dh_orig_stdout
                __dh_sys.stderr = __dh_orig_stderr

            # Package results and encode safely
            __dh_results_dict = {{
                "stdout": __dh_stdout_buf.getvalue(),
                "stderr": __dh_stderr_buf.getvalue(),
                "result_repr": repr(__dh_result) if __dh_result is not None else None,
                "error": __dh_error,
            }}
            __dh_pickled = __dh_base64.b64encode(__dh_pickle.dumps(__dh_results_dict)).decode("ascii")

            # Create result table with encoded data
            from deephaven import empty_table
            __dh_result_table = empty_table(1).update([f"data = `{{__dh_pickled}}`"])

            # Clean up wrapper variables (except result table)
            del __dh_io, __dh_sys, __dh_pickle, __dh_base64
            del __dh_stdout_buf, __dh_stderr_buf, __dh_orig_stdout, __dh_orig_stderr
            del __dh_result, __dh_error, __dh_results_dict, __dh_pickled
        ''').strip()

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

    def get_table_preview(
        self,
        table_name: str,
        rows: int = 10,
        show_meta: bool = True,
    ) -> tuple[str, TableMeta | None]:
        """Get a string preview of a table.

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
                lines.append(preview_df.to_string())

            return "\n".join(lines), meta

        except Exception as e:
            return f"(error previewing table: {e})", None
