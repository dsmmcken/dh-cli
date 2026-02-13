# Expose caller's CWD and `__file__` to `dh exec` scripts

## Context

When `dh exec tests/basic_import.py` is run, the script content is read locally and sent as a string to the Deephaven server via gRPC. On the server, `__file__` is undefined and the CWD is whatever the server process inherited — so `Path(__file__).parent / "data.csv"` and `open("data.csv")` both fail. The user expects local file paths to just work when running from a folder.

## Approach

Inject `os.chdir(caller_cwd)` and `__file__ = script_path` into the executor wrapper, so the server's Python environment matches the caller's filesystem context. CWD is saved before and restored after each execution to avoid side effects.

## Files to change

### 1. `src/deephaven_cli/repl/executor.py`

**`execute()` (line 87)** — Add optional params:
```python
def execute(self, code: str, *, script_path: str | None = None, cwd: str | None = None) -> ExecutionResult:
```
Pass them through to `_build_wrapper()`.

**`_build_wrapper()` (line 137)** — Add same params, inject into wrapper:
```python
def _build_wrapper(self, code: str, script_path: str | None = None, cwd: str | None = None) -> str:
```

At the top of the wrapper (before stdout capture), add:
```python
import os as __dh_os
__dh_orig_cwd = __dh_os.getcwd()
```

If `cwd` is provided:
```python
__dh_os.chdir('{cwd}')
```

If `script_path` is provided, inject into the user code's namespace:
```python
__file__ = '{script_path}'
```

In the `finally` block, restore CWD:
```python
__dh_os.chdir(__dh_orig_cwd)
```

In the cleanup `del` line, add `__dh_os, __dh_orig_cwd`.

### 2. `src/deephaven_cli/cli.py`

**`run_exec()` (line 1178)** — Pass CWD and script path to executor:
```python
import os
abs_script_path = os.path.abspath(script_path) if script_path and script_path != "-" else None
caller_cwd = os.getcwd()
result = executor.execute(script_content, script_path=abs_script_path, cwd=caller_cwd)
```

For `-c` code: `script_path=None`, `cwd=os.getcwd()` (CWD still useful for relative paths in inline code).

### 3. No changes to REPL

The REPL calls `executor.execute(code)` without these params — it keeps working as before (params are optional with `None` defaults, so no CWD/file injection happens for interactive sessions).

## Verification

Run the existing test:
```bash
dh exec tests/basic_import.py
```
Should print all 9 rows from `sample_data.csv` — `Path(__file__).parent / "sample_data.csv"` resolves correctly.

Also test:
```bash
cd tests && dh exec -c "import csv; print(list(csv.DictReader(open('sample_data.csv'))))"
```
Should work because CWD is `tests/`.
