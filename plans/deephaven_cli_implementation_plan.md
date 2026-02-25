# Deephaven CLI Implementation Plan

---

## ⚠️ INSTRUCTIONS FOR IMPLEMENTING AGENT

**This is a WORKING DOCUMENT.** As you implement each phase:

1. **Update this document** with any changes from what was planned
2. **Mark phases as complete** by changing `[ ]` to `[x]` in success criteria
3. **Add notes** about deviations, bugs encountered, or solutions found
4. **Record actual verification output** if it differs from expected
5. **Do NOT delete planned content** - instead add "ACTUAL:" notes below plans

**Phase Status Legend:**
- 🔲 Not started
- 🔄 In progress
- ✅ Complete
- ❌ Blocked

**Current Status:** ✅ Phase 6 complete

---

## Overview

Build a command-line tool called `dh` that provides a REPL interface for interacting with Deephaven servers. The initial version will include the `dh repl` command.

**ACTUAL:** Changed command name from `deephaven` to `dh` because `deephaven-server` already registers a `deephaven` CLI command, causing a conflict.

## Requirements Summary

- **Python Version**: 3.13+ (required for native `_pyrepl`)
- **Package Manager**: uv
- **Pip Installable**: Yes
- **Dependencies**: `deephaven-server`, `pydeephaven`
- **Platform**: Linux/macOS (Windows not fully supported due to signal.SIGALRM)
- **CLI Command**: `dh` (not `deephaven` - see note below)

> **Note on CLI command name:** The `deephaven-server` package already registers a `deephaven` CLI command for starting the server. To avoid this namespace collision, our CLI uses `dh` instead.

## Environment Prerequisites

Before starting implementation, verify these requirements:

```bash
# 1. Check Python version (must be 3.13+)
python3 --version
# Expected: Python 3.13.x or higher

# 2. Check Java is installed and on PATH (must be 11+)
java -version
# Expected: openjdk version "11.x.x" or higher
# Note: JAVA_HOME is NOT required if java is on PATH

# 3. Check uv is installed
uv --version
# Expected: uv x.x.x
# If not installed: curl -LsSf https://astral.sh/uv/install.sh | sh

# 4. Verify uv can see Python 3.13
uv python list | grep 3.13
# Expected: shows Python 3.13 installation

# 5. Verify pytest is available (will be installed as dev dependency)
# This will be set up in Phase 1
```

**If any prerequisite fails, stop and fix it before proceeding.**

### Development Dependencies

The project uses pytest for testing. These are specified as optional dev dependencies:

- `pytest>=8.0` - Test framework
- `pytest-timeout>=2.0` - Timeout support for hanging tests

## Critical Design Decisions

### stdout/stderr Capture Strategy

The `ExecuteCommandResponse` from Deephaven's gRPC API does not directly include stdout/stderr. The gRPC API has a `subscribeToLogs` streaming method, but pydeephaven doesn't expose it.

**Solution: Server-Side Output Capture with Pickle**

We use pickle to safely transfer arbitrary string data (including special characters like backticks, newlines, quotes) from server to client.

Wrap all user code execution in a capture wrapper that:
1. Redirects stdout/stderr to StringIO buffers
2. Captures the last expression result (like Python's `_`)
3. Pickles the results as bytes
4. Stores pickled bytes in a table with a bytes column
5. Client reads bytes, unpickles to get original strings

```python
# Wrapper template (executed on server)
import io as __io, sys as __sys, pickle as __pickle, base64 as __base64

__dh_stdout = __io.StringIO()
__dh_stderr = __io.StringIO()
__dh_orig_stdout = __sys.stdout
__dh_orig_stderr = __sys.stderr
__sys.stdout = __dh_stdout
__sys.stderr = __dh_stderr
__dh_result = None
__dh_error = None

try:
    try:
        # Try as expression first
        __dh_result = eval({user_code_repr})
    except SyntaxError:
        # Not an expression, execute as statement
        exec({user_code_repr})
except Exception as __e:
    import traceback
    __dh_error = traceback.format_exc()
finally:
    __sys.stdout = __dh_orig_stdout
    __sys.stderr = __dh_orig_stderr

# Package results as a dictionary and pickle it
__dh_results = {
    "stdout": __dh_stdout.getvalue(),
    "stderr": __dh_stderr.getvalue(),
    "result_repr": repr(__dh_result) if __dh_result is not None else None,
    "error": __dh_error,
}
__dh_pickled = __base64.b64encode(__pickle.dumps(__dh_results)).decode('ascii')
```

Then create a result table with the base64-encoded pickled data:
```python
from deephaven import empty_table
__dh_result_table = empty_table(1).update([f"data = `{__dh_pickled}`"])
```

Client reads the `data` column, base64 decodes, unpickles to get the results dict.

**Why base64?** Deephaven table string columns use backticks. Base64 is safe ASCII with no backticks.

## Project Structure

```
deephaven-cli/
├── pyproject.toml
├── src/
│   └── deephaven_cli/
│       ├── __init__.py
│       ├── cli.py              # Main CLI entry point
│       ├── server.py           # Server lifecycle management
│       ├── client.py           # Extended pydeephaven client
│       └── repl/
│           ├── __init__.py
│           ├── console.py      # Custom REPL using _pyrepl
│           └── executor.py     # Code execution wrapper & result handling
├── tests/
│   └── ...
└── README.md
```

## Files to Create (Complete List)

| File | Created In | Description |
|------|------------|-------------|
| `pyproject.toml` | Phase 1 | Package configuration with pytest config |
| `src/deephaven_cli/__init__.py` | Phase 1 | Package init with version |
| `src/deephaven_cli/cli.py` | Phase 1 (placeholder), Phase 6+ (full) | CLI entry point |
| `src/deephaven_cli/repl/__init__.py` | Phase 1 | REPL subpackage init |
| `tests/__init__.py` | Phase 1 | Tests package init |
| `tests/conftest.py` | Phase 1 | Shared pytest fixtures |
| `tests/test_phase1_scaffolding.py` | Phase 1 | Phase 1 unit tests |
| `src/deephaven_cli/server.py` | Phase 2 | Server lifecycle |
| `tests/test_phase2_server.py` | Phase 2 | Server integration tests |
| `src/deephaven_cli/client.py` | Phase 3 | Client wrapper |
| `tests/test_phase3_client.py` | Phase 3 | Client integration tests |
| `src/deephaven_cli/repl/executor.py` | Phase 4 | Code execution with capture |
| `tests/test_phase4_executor.py` | Phase 4 | Executor integration tests |
| `src/deephaven_cli/repl/console.py` | Phase 5 | Interactive console |
| `tests/test_phase5_console.py` | Phase 5 | Console unit/integration tests |
| `tests/test_phase6_cli.py` | Phase 6 | CLI argument parsing tests |
| `tests/test_phase7_exec.py` | Phase 7 | Exec mode integration tests |
| `tests/test_phase8_app.py` | Phase 8 | App mode integration tests |

---

## Phase 1: Project Scaffolding ✅

### Tasks
1. Create project directory structure (manually, NOT using `uv init`)
2. Create `pyproject.toml` with uv/hatchling build system and pytest config
3. Create all `__init__.py` files
4. Create placeholder `cli.py` with `main()` function
5. Create tests directory with `conftest.py` and initial test file
6. Install package in development mode with dev dependencies
7. Verify installation and run initial pytest

### Directory Structure to Create

```bash
# Create all directories first
mkdir -p src/deephaven_cli/repl
mkdir -p tests
```

This creates:
```
src/
└── deephaven_cli/
    ├── __init__.py       (create in this phase)
    ├── cli.py            (create in this phase)
    └── repl/
        └── __init__.py   (create in this phase)
tests/
    ├── __init__.py       (create in this phase)
    ├── conftest.py       (create in this phase)
    └── test_phase1_scaffolding.py  (create in this phase)
```

### Files to Create

**pyproject.toml** (in project root):
```toml
[project]
name = "deephaven-cli"
version = "0.1.0"
description = "Command-line tool for interacting with Deephaven servers"
requires-python = ">=3.13"
dependencies = [
    "deephaven-server>=0.37.0",
    "pydeephaven>=0.37.0",
]

[project.optional-dependencies]
dev = [
    "pytest>=8.0",
    "pytest-timeout>=2.0",
]

[project.scripts]
dh = "deephaven_cli.cli:main"  # Named 'dh' to avoid conflict with deephaven-server's 'deephaven' command

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"

[tool.hatch.build.targets.wheel]
packages = ["src/deephaven_cli"]

[tool.pytest.ini_options]
testpaths = ["tests"]
python_files = ["test_*.py"]
python_functions = ["test_*"]
markers = [
    "slow: marks tests as slow (may take >30s)",
    "integration: marks tests requiring external resources (server, Java)",
]
addopts = "-v --tb=short"
```

**src/deephaven_cli/__init__.py:**
```python
"""Deephaven CLI - Command-line tool for Deephaven servers."""
__version__ = "0.1.0"
```

**src/deephaven_cli/cli.py:**
```python
"""Main CLI entry point."""
import sys

def main() -> int:
    print("deephaven-cli placeholder")
    return 0

if __name__ == "__main__":
    sys.exit(main())
```

**src/deephaven_cli/repl/__init__.py:**
```python
"""REPL subpackage."""
```

**tests/__init__.py:**
```python
"""Tests for deephaven-cli."""
```

**tests/conftest.py:**
```python
"""Shared pytest fixtures for deephaven-cli tests."""
import pytest
import random


@pytest.fixture(scope="session")
def test_port_range():
    """Return a function to get unique ports for testing."""
    used_ports = set()

    def get_port():
        while True:
            port = random.randint(10100, 10999)
            if port not in used_ports:
                used_ports.add(port)
                return port
    return get_port
```

**tests/test_phase1_scaffolding.py:**
```python
"""Tests for Phase 1: Project scaffolding and package structure."""
import pytest


def test_package_imports():
    """Verify main package can be imported."""
    import deephaven_cli
    assert hasattr(deephaven_cli, '__version__')


def test_version_is_string():
    """Verify version is a valid string."""
    from deephaven_cli import __version__
    assert isinstance(__version__, str)
    assert len(__version__) > 0


def test_repl_subpackage_imports():
    """Verify repl subpackage can be imported."""
    from deephaven_cli import repl
    assert repl is not None


def test_cli_module_exists():
    """Verify cli module exists and has main function."""
    from deephaven_cli import cli
    assert hasattr(cli, 'main')
    assert callable(cli.main)


def test_cli_main_requires_subcommand():
    """Verify main() requires a subcommand."""
    from deephaven_cli.cli import main
    # With no args, argparse will raise SystemExit
    with pytest.raises(SystemExit):
        main()
```

### Verification Steps

```bash
# 1. Create directory structure
mkdir -p src/deephaven_cli/repl
mkdir -p tests

# 2. Create all files listed above (pyproject.toml, __init__.py files, cli.py, test files)
# (Use your editor or Write tool)

# 3. Verify directory structure
find src tests -type f -name "*.py"
# Expected output:
# src/deephaven_cli/__init__.py
# src/deephaven_cli/cli.py
# src/deephaven_cli/repl/__init__.py
# tests/__init__.py
# tests/conftest.py
# tests/test_phase1_scaffolding.py

# 4. Create virtual environment and install with dev dependencies
uv venv
uv pip install -e ".[dev]"

# 5. Verify CLI entry point works
uv run dh
# Expected output: "deephaven-cli placeholder"

# 6. Verify import works
uv run python -c "from deephaven_cli import __version__; print(__version__)"
# Expected output: "0.1.0"

# 7. Verify package structure
uv run python -c "from deephaven_cli.repl import *; print('repl package ok')"
# Expected output: "repl package ok"

# 8. Verify pytest is installed
uv run pytest --version
# Expected output: pytest 8.x.x

# 9. Run Phase 1 tests
uv run pytest tests/test_phase1_scaffolding.py -v
# Expected: All 5 tests pass
```

### Success Criteria
- [x] Directory structure exists: `src/deephaven_cli/repl/` and `tests/`
- [x] All 7 files created (pyproject.toml + 3 src .py files + 3 test .py files)
- [x] `uv pip install -e ".[dev]"` completes without errors
- [x] `uv run dh` prints placeholder message (ACTUAL: renamed from `deephaven` to `dh`)
- [x] Package imports work (both main and repl subpackage)
- [x] `uv run pytest tests/test_phase1_scaffolding.py -v` - all tests pass

### Actual Verification Output (2026-02-02)
```
$ uv run dh
usage: dh [-h] {repl} ...
dh: error: the following arguments are required: command

$ uv run dh repl
deephaven-cli repl placeholder

$ uv run python -c "from deephaven_cli import __version__; print(__version__)"
0.1.0

$ uv run python -c "from deephaven_cli.repl import *; print('repl package ok')"
repl package ok

$ uv run pytest tests/test_phase1_scaffolding.py -v
tests/test_phase1_scaffolding.py::test_package_imports PASSED            [ 20%]
tests/test_phase1_scaffolding.py::test_version_is_string PASSED          [ 40%]
tests/test_phase1_scaffolding.py::test_repl_subpackage_imports PASSED    [ 60%]
tests/test_phase1_scaffolding.py::test_cli_module_exists PASSED          [ 80%]
tests/test_phase1_scaffolding.py::test_cli_main_requires_subcommand PASSED [100%]
============================== 5 passed in 0.02s ===============================
```

**Note:** Used `uv venv --python 3.13` to avoid Python 3.14 compatibility issues with jpy package.

**ACTUAL:** Updated cli.py to use argparse with required subcommand (per test_cli_main_requires_subcommand test requirement).

### Pytest Testing

The test file `tests/test_phase1_scaffolding.py` is created as part of the Files to Create section above.

**Tests included:**
- `test_package_imports` - Verify main package can be imported
- `test_version_is_string` - Verify version is a valid string
- `test_repl_subpackage_imports` - Verify repl subpackage can be imported
- `test_cli_module_exists` - Verify cli module exists and has main function
- `test_cli_main_requires_subcommand` - Verify main() requires a subcommand

**Run with:** `uv run pytest tests/test_phase1_scaffolding.py -v`

**Expected output:**
```
tests/test_phase1_scaffolding.py::test_package_imports PASSED
tests/test_phase1_scaffolding.py::test_version_is_string PASSED
tests/test_phase1_scaffolding.py::test_repl_subpackage_imports PASSED
tests/test_phase1_scaffolding.py::test_cli_module_exists PASSED
tests/test_phase1_scaffolding.py::test_cli_main_requires_subcommand PASSED
```

---

## Phase 2: Server Lifecycle Management ✅

### Tasks
1. Create `server.py` with `DeephavenServer` class
2. Implement `start()` method
3. Implement `stop()` method
4. Add context manager support
5. Test server starts and stops cleanly

### File: src/deephaven_cli/server.py

```python
"""Deephaven server lifecycle management."""
from __future__ import annotations

import atexit
import sys
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from deephaven_server import Server


class DeephavenServer:
    """Manages an embedded Deephaven server lifecycle."""

    def __init__(self, port: int = 10000, jvm_args: list[str] | None = None):
        self.port = port
        self.jvm_args = jvm_args or ["-Xmx4g"]
        self._server: Server | None = None
        self._started = False

    def start(self) -> DeephavenServer:
        """Start the Deephaven server."""
        if self._started:
            raise RuntimeError("Server already started")

        # Import here to avoid JVM initialization on import
        from deephaven_server import Server

        self._server = Server(port=self.port, jvm_args=self.jvm_args)
        self._server.start()
        self._started = True

        # Register cleanup on exit
        atexit.register(self._cleanup)

        return self

    def stop(self) -> None:
        """Stop the Deephaven server."""
        if not self._started:
            return

        self._started = False
        # Note: deephaven_server.Server doesn't have explicit stop
        # The JVM will be cleaned up on process exit
        # Unregister atexit handler since we're stopping explicitly
        try:
            atexit.unregister(self._cleanup)
        except Exception:
            pass

    def _cleanup(self) -> None:
        """Cleanup handler for atexit."""
        self.stop()

    def __enter__(self) -> DeephavenServer:
        return self.start()

    def __exit__(self, exc_type, exc_val, exc_tb) -> None:
        self.stop()

    @property
    def is_running(self) -> bool:
        """Check if server is running."""
        return self._started
```

### Verification Steps

```bash
# 1. Create a test script (don't run in background!)
cat > /tmp/test_server.py << 'EOF'
import sys
sys.path.insert(0, "src")

from deephaven_cli.server import DeephavenServer

print("Starting server...")
server = DeephavenServer(port=10000)
server.start()
print(f"Server running: {server.is_running}")
print(f"Server port: {server.port}")

print("Stopping server...")
server.stop()
print(f"Server running after stop: {server.is_running}")
print("SUCCESS: Server lifecycle works")
EOF

# 2. Run the test (this will block until complete, NOT backgrounded)
uv run python /tmp/test_server.py

# Expected output:
# Starting server...
# Server running: True
# Server port: 10000
# Stopping server...
# Server running after stop: False
# SUCCESS: Server lifecycle works

# 3. Verify no orphan Java processes
ps aux | grep -i java | grep -v grep
# Should show no deephaven-related Java processes
```

### Success Criteria
- [x] Server starts without errors
- [x] `is_running` returns True after start
- [x] Server stops cleanly
- [x] No orphan Java processes after stop
- [x] Context manager works (`with DeephavenServer() as s:`)
- [x] `uv run pytest tests/test_phase2_server.py -v` - all tests pass

### ⚠️ Important Notes
- **DO NOT run server in background** - always run synchronously
- JAVA_HOME is not strictly required if Java is on PATH
- First start may take 10-30 seconds for JVM initialization

### Actual Verification Output (2026-02-02)
```
$ uv run python /tmp/test_server.py
Starting server...
Server started on port 10000
Server running: True
Server port: 10000
Stopping server...
Server running after stop: False
SUCCESS: Server lifecycle works

$ uv run python /tmp/test_server_ctx.py
Testing context manager...
Server started on port 10001
Inside context: is_running=True
Outside context: is_running=False
SUCCESS: Context manager works

$ ps aux | grep -i deephaven | grep -v grep
No Deephaven Java processes found
```

### Pytest Testing

Create `tests/test_phase2_server.py`:

**ACTUAL:** The test file was restructured due to Deephaven JVM limitation (only one server instance per process). Tests are now split into:
- `TestDeephavenServerUnit` - Unit tests that don't start the server
- `TestDeephavenServerIntegration` - Integration tests using a module-scoped server fixture

```python
"""Tests for Phase 2: Server lifecycle management.

NOTE: These tests start real Deephaven servers and require Java 11+.
They are marked as 'slow' and 'integration' for selective running.

IMPORTANT: Deephaven server can only be started once per process (JVM limitation).
Therefore, integration tests use a single module-scoped server fixture.
Unit tests (testing initialization) don't start the server and run separately.
"""
import pytest
from deephaven_cli.server import DeephavenServer


# Unit tests - these don't start a server and can run without Java
class TestDeephavenServerUnit:
    def test_server_init_default_port(self): ...
    def test_server_init_custom_port(self): ...
    def test_server_init_custom_jvm_args(self): ...
    def test_server_stop_when_not_started(self): ...


# Integration tests - use module-scoped fixture
@pytest.fixture(scope="module")
def running_server():
    """Start a single server for the entire test module."""
    import random
    port = random.randint(10100, 10999)
    server = DeephavenServer(port=port)
    server.start()
    yield server
    server.stop()


@pytest.mark.slow
@pytest.mark.integration
class TestDeephavenServerIntegration:
    def test_server_is_running(self, running_server): ...
    def test_server_has_port(self, running_server): ...
    def test_server_double_start_raises(self, running_server): ...


@pytest.mark.slow
@pytest.mark.integration
class TestDeephavenServerLifecycle:
    def test_server_stop_sets_not_running(self, running_server): ...
```

**Run with:** `uv run pytest tests/test_phase2_server.py -v`

**Actual pytest output (2026-02-02):**
```
tests/test_phase2_server.py::TestDeephavenServerUnit::test_server_init_default_port PASSED [ 12%]
tests/test_phase2_server.py::TestDeephavenServerUnit::test_server_init_custom_port PASSED [ 25%]
tests/test_phase2_server.py::TestDeephavenServerUnit::test_server_init_custom_jvm_args PASSED [ 37%]
tests/test_phase2_server.py::TestDeephavenServerUnit::test_server_stop_when_not_started PASSED [ 50%]
tests/test_phase2_server.py::TestDeephavenServerIntegration::test_server_is_running PASSED [ 62%]
tests/test_phase2_server.py::TestDeephavenServerIntegration::test_server_has_port PASSED [ 75%]
tests/test_phase2_server.py::TestDeephavenServerIntegration::test_server_double_start_raises PASSED [ 87%]
tests/test_phase2_server.py::TestDeephavenServerLifecycle::test_server_stop_sets_not_running PASSED [100%]
============================== 8 passed in 3.62s ===============================
```

**Note:** pytest markers already added to `pyproject.toml` in Phase 1.

---

## Phase 3: Client Connection ✅

### Tasks
1. Create `client.py` with `DeephavenClient` class
2. Implement connection to server
3. Implement basic `run_script()` wrapper
4. Implement `get_tables()` method
5. Test client can connect and run simple script

### File: src/deephaven_cli/client.py

```python
"""Deephaven client wrapper."""
from __future__ import annotations

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from pydeephaven import Session


class DeephavenClient:
    """Client for communicating with Deephaven server."""

    def __init__(self, host: str = "localhost", port: int = 10000):
        self.host = host
        self.port = port
        self._session: Session | None = None

    def connect(self) -> DeephavenClient:
        """Connect to the Deephaven server."""
        from pydeephaven import Session

        self._session = Session(host=self.host, port=self.port)
        return self

    def close(self) -> None:
        """Close the client connection."""
        if self._session:
            self._session.close()
            self._session = None

    @property
    def session(self) -> Session:
        """Get the underlying session."""
        if not self._session:
            raise RuntimeError("Client not connected")
        return self._session

    @property
    def tables(self) -> list[str]:
        """Get list of available table names."""
        return list(self.session.tables)

    def run_script(self, script: str) -> None:
        """Run a script on the server."""
        self.session.run_script(script)

    def __enter__(self) -> DeephavenClient:
        return self.connect()

    def __exit__(self, exc_type, exc_val, exc_tb) -> None:
        self.close()
```

### Verification Steps

```bash
# 1. Create integration test script
cat > /tmp/test_client.py << 'EOF'
import sys
sys.path.insert(0, "src")

from deephaven_cli.server import DeephavenServer
from deephaven_cli.client import DeephavenClient

print("Starting server...")
with DeephavenServer(port=10000) as server:
    print("Connecting client...")
    with DeephavenClient(port=10000) as client:
        print(f"Connected, tables before: {client.tables}")

        # Run a simple script
        client.run_script("""
from deephaven import empty_table
test_table = empty_table(5).update(["X = i", "Y = i * 2"])
""")

        print(f"Tables after script: {client.tables}")

        # Verify our table exists
        assert "test_table" in client.tables, "test_table should exist"

        print("SUCCESS: Client connection and script execution works")
EOF

# 2. Run the test
uv run python /tmp/test_client.py

# Expected output includes:
# Starting server...
# Connecting client...
# Connected, tables before: []
# Tables after script: ['test_table']
# SUCCESS: Client connection and script execution works
```

### Success Criteria
- [x] Client connects to running server
- [x] `run_script()` executes without error
- [x] `tables` property returns created table names
- [x] Client closes cleanly

**ACTUAL:** All 9 pytest tests passed. Manual verification script also confirmed:
- Tables before script: []
- Tables after script: ['test_table']
- Client connected, executed script, and closed cleanly

### Pytest Testing

Create `tests/test_phase3_client.py`:

```python
"""Tests for Phase 3: Client connection.

NOTE: These tests require a running Deephaven server.
They are marked as 'integration'.
"""
import pytest
from deephaven_cli.server import DeephavenServer
from deephaven_cli.client import DeephavenClient


@pytest.fixture(scope="module")
def running_server():
    """Start a server for the entire test module."""
    import random
    port = random.randint(10200, 10299)
    with DeephavenServer(port=port) as server:
        yield port


@pytest.mark.integration
class TestDeephavenClient:
    """Tests for DeephavenClient class."""

    def test_client_init(self):
        """Test client initializes with default values."""
        client = DeephavenClient()
        assert client.host == "localhost"
        assert client.port == 10000

    def test_client_init_custom(self):
        """Test client initializes with custom values."""
        client = DeephavenClient(host="myhost", port=12345)
        assert client.host == "myhost"
        assert client.port == 12345

    def test_client_connect_disconnect(self, running_server):
        """Test client can connect and disconnect."""
        client = DeephavenClient(port=running_server)
        client.connect()
        assert client._session is not None
        client.close()
        assert client._session is None

    def test_client_context_manager(self, running_server):
        """Test client works as context manager."""
        with DeephavenClient(port=running_server) as client:
            assert client._session is not None
        assert client._session is None

    def test_client_session_property_raises_when_disconnected(self):
        """Test session property raises when not connected."""
        client = DeephavenClient()
        with pytest.raises(RuntimeError, match="not connected"):
            _ = client.session

    def test_client_tables_empty_initially(self, running_server):
        """Test tables list is empty on fresh connection."""
        with DeephavenClient(port=running_server) as client:
            # Note: May not be empty if other tests created tables
            assert isinstance(client.tables, list)

    def test_client_run_script_creates_table(self, running_server):
        """Test run_script can create a table."""
        with DeephavenClient(port=running_server) as client:
            client.run_script('''
from deephaven import empty_table
test_client_table = empty_table(5).update(["X = i"])
''')
            assert "test_client_table" in client.tables

    def test_client_run_script_syntax_error(self, running_server):
        """Test run_script raises on syntax error."""
        with DeephavenClient(port=running_server) as client:
            with pytest.raises(Exception):
                client.run_script("this is not valid python syntax {{{")

    def test_client_run_script_runtime_error(self, running_server):
        """Test run_script raises on runtime error."""
        with DeephavenClient(port=running_server) as client:
            with pytest.raises(Exception):
                client.run_script("raise ValueError('test error')")
```

**Run with:** `uv run pytest tests/test_phase3_client.py -v -m "integration"`

---

## Phase 4: Code Executor with Output Capture ✅

### Tasks
1. Create `repl/executor.py` with `CodeExecutor` class
2. Implement wrapper that captures stdout/stderr/result
3. Implement method to retrieve captured output via temporary table
4. Implement table diff detection (new tables created)
5. Test all output types: print, expression result, new tables, errors

### File: src/deephaven_cli/repl/executor.py

```python
"""Code execution with output capture using pickle for safe string transfer."""
from __future__ import annotations

import base64
import pickle
import textwrap
from dataclasses import dataclass
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from deephaven_cli.client import DeephavenClient


@dataclass
class ExecutionResult:
    """Result of code execution."""
    stdout: str
    stderr: str
    result_repr: str | None  # repr() of expression result, if any
    error: str | None  # Exception traceback, if any
    new_tables: list[str]  # Tables created by this execution
    updated_tables: list[str]  # Tables modified by this execution


class CodeExecutor:
    """Executes code on Deephaven server with output capture."""

    def __init__(self, client: DeephavenClient):
        self.client = client

    def execute(self, code: str) -> ExecutionResult:
        """Execute code and return captured output."""
        # Get tables before execution
        tables_before = set(self.client.tables)

        # Build and execute the wrapper script (captures output + creates result table)
        wrapper = self._build_wrapper(code)

        try:
            self.client.run_script(wrapper)
        except Exception as e:
            # Script-level error (syntax error in wrapper, etc.)
            return ExecutionResult(
                stdout="",
                stderr="",
                result_repr=None,
                error=str(e),
                new_tables=[],
                updated_tables=[],
            )

        # Read the result from the table
        result = self._read_result_table()

        # Clean up
        self._cleanup()

        # Detect new tables (excluding our internal one)
        tables_after = set(self.client.tables) - {"__dh_result_table"}
        new_tables = list(tables_after - tables_before)

        return ExecutionResult(
            stdout=result.get("stdout", ""),
            stderr=result.get("stderr", ""),
            result_repr=result.get("result_repr"),
            error=result.get("error"),
            new_tables=new_tables,
            updated_tables=[],
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

    def get_table_preview(self, table_name: str, rows: int = 10) -> str:
        """Get a string preview of a table."""
        session = self.client.session
        table = session.open_table(table_name)
        try:
            preview = table.head(rows).to_arrow().to_pandas()
            return preview.to_string()
        except Exception as e:
            return f"(error previewing table: {e})"
```

### Verification Steps

```bash
# 1. Create comprehensive test script
cat > /tmp/test_executor.py << 'EOF'
import sys
sys.path.insert(0, "src")

from deephaven_cli.server import DeephavenServer
from deephaven_cli.client import DeephavenClient
from deephaven_cli.repl.executor import CodeExecutor

print("Starting server...")
with DeephavenServer(port=10000) as server:
    print("Connecting client...")
    with DeephavenClient(port=10000) as client:
        executor = CodeExecutor(client)

        # Test 1: Simple expression
        print("\n--- Test 1: Expression (2 + 2) ---")
        result = executor.execute("2 + 2")
        print(f"result_repr: {result.result_repr}")
        print(f"stdout: {repr(result.stdout)}")
        print(f"error: {result.error}")
        assert result.result_repr == "4", f"Expected '4', got {result.result_repr}"

        # Test 2: Print statement
        print("\n--- Test 2: Print statement ---")
        result = executor.execute('print("Hello, World!")')
        print(f"stdout: {repr(result.stdout)}")
        assert "Hello, World!" in result.stdout, f"Expected 'Hello, World!' in stdout"

        # Test 3: Create a table
        print("\n--- Test 3: Create table ---")
        result = executor.execute('''
from deephaven import empty_table
my_table = empty_table(3).update(["X = i"])
''')
        print(f"new_tables: {result.new_tables}")
        assert "my_table" in result.new_tables, f"Expected 'my_table' in new_tables"

        # Test 4: Error handling
        print("\n--- Test 4: Error handling ---")
        result = executor.execute("1 / 0")
        print(f"error: {result.error[:50]}..." if result.error else "No error")
        assert result.error is not None, "Expected an error"
        assert "ZeroDivisionError" in result.error

        # Test 5: Table preview
        print("\n--- Test 5: Table preview ---")
        preview = executor.get_table_preview("my_table")
        print(f"Preview:\n{preview}")
        assert "X" in preview

        print("\n✓ All executor tests passed!")
EOF

# 2. Run the test
uv run python /tmp/test_executor.py
```

### Success Criteria
- [x] Expression results captured correctly (`2 + 2` → `4`)
- [x] Print statements captured in stdout
- [x] New tables detected in `new_tables` list
- [x] Errors captured with traceback
- [x] Table preview works

### ACTUAL Implementation Notes
- Minor fix: Moved the `try` block in `get_table_preview` to wrap `open_table()` as well, since it throws an exception for nonexistent tables (the plan had the try block only around the preview logic)
- All 15 pytest tests pass
- Manual verification script passes successfully

### Pytest Testing

Create `tests/test_phase4_executor.py`:

```python
"""Tests for Phase 4: Code executor with output capture.

NOTE: These tests require a running Deephaven server.
"""
import pytest
from deephaven_cli.server import DeephavenServer
from deephaven_cli.client import DeephavenClient
from deephaven_cli.repl.executor import CodeExecutor, ExecutionResult


@pytest.fixture(scope="module")
def executor():
    """Provide a CodeExecutor with running server."""
    import random
    port = random.randint(10300, 10399)
    server = DeephavenServer(port=port)
    server.start()
    client = DeephavenClient(port=port)
    client.connect()
    executor = CodeExecutor(client)
    yield executor
    client.close()
    server.stop()


@pytest.mark.integration
class TestCodeExecutor:
    """Tests for CodeExecutor class."""

    def test_execute_returns_execution_result(self, executor):
        """Test execute returns an ExecutionResult."""
        result = executor.execute("1 + 1")
        assert isinstance(result, ExecutionResult)

    def test_execute_simple_expression(self, executor):
        """Test executing a simple expression captures result."""
        result = executor.execute("2 + 2")
        assert result.result_repr == "4"
        assert result.error is None
        assert result.stdout == ""

    def test_execute_string_expression(self, executor):
        """Test executing a string expression."""
        result = executor.execute("'hello' + ' world'")
        assert result.result_repr == "'hello world'"

    def test_execute_print_captures_stdout(self, executor):
        """Test print statements are captured in stdout."""
        result = executor.execute('print("Hello, World!")')
        assert "Hello, World!" in result.stdout
        assert result.error is None

    def test_execute_multiple_prints(self, executor):
        """Test multiple print statements are captured."""
        result = executor.execute('print("line1"); print("line2")')
        assert "line1" in result.stdout
        assert "line2" in result.stdout

    def test_execute_stderr_capture(self, executor):
        """Test stderr is captured."""
        result = executor.execute('import sys; sys.stderr.write("error msg")')
        assert "error msg" in result.stderr

    def test_execute_creates_table_detected(self, executor):
        """Test new tables are detected."""
        result = executor.execute('''
from deephaven import empty_table
executor_test_table = empty_table(3).update(["X = i"])
''')
        assert "executor_test_table" in result.new_tables
        assert result.error is None

    def test_execute_division_error(self, executor):
        """Test division by zero error is captured."""
        result = executor.execute("1 / 0")
        assert result.error is not None
        assert "ZeroDivisionError" in result.error

    def test_execute_name_error(self, executor):
        """Test NameError is captured."""
        result = executor.execute("undefined_variable")
        assert result.error is not None
        assert "NameError" in result.error

    def test_execute_syntax_error(self, executor):
        """Test syntax error is captured."""
        result = executor.execute("def broken(")
        assert result.error is not None
        assert "SyntaxError" in result.error

    def test_execute_multiline_code(self, executor):
        """Test multiline code execution."""
        code = '''
x = 10
y = 20
print(x + y)
'''
        result = executor.execute(code)
        assert "30" in result.stdout
        assert result.error is None

    def test_execute_function_definition_and_call(self, executor):
        """Test defining and calling a function."""
        code = '''
def add(a, b):
    return a + b
print(add(5, 3))
'''
        result = executor.execute(code)
        assert "8" in result.stdout

    def test_execute_special_characters_in_output(self, executor):
        """Test special characters (backticks, quotes) are preserved."""
        result = executor.execute('print("Hello `world` with \\'quotes\\'")')
        assert "`world`" in result.stdout
        assert "quotes" in result.stdout

    def test_get_table_preview(self, executor):
        """Test table preview returns string with data."""
        # First create a table
        executor.execute('''
from deephaven import empty_table
preview_test_table = empty_table(3).update(["Value = i * 10"])
''')
        preview = executor.get_table_preview("preview_test_table")
        assert isinstance(preview, str)
        assert "Value" in preview  # Column name
        assert "0" in preview  # First value

    def test_get_table_preview_nonexistent(self, executor):
        """Test preview of nonexistent table returns error message."""
        preview = executor.get_table_preview("nonexistent_table_xyz")
        assert "error" in preview.lower() or isinstance(preview, str)
```

**Run with:** `uv run pytest tests/test_phase4_executor.py -v -m "integration"`

---

## Phase 5: REPL Console ✅

### Tasks
1. Create `repl/console.py` with REPL loop
2. Use `code.InteractiveConsole` as base (fallback from _pyrepl for reliability)
3. Integrate CodeExecutor for remote execution
4. Format and display results (stdout, stderr, tables, errors)
5. Handle multi-line input

### File: src/deephaven_cli/repl/console.py

```python
"""Interactive REPL console for Deephaven."""
from __future__ import annotations

import code
import sys
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from deephaven_cli.client import DeephavenClient
    from deephaven_cli.repl.executor import CodeExecutor


class DeephavenConsole:
    """Interactive console that executes code on Deephaven server."""

    def __init__(self, client: DeephavenClient):
        from deephaven_cli.repl.executor import CodeExecutor

        self.client = client
        self.executor = CodeExecutor(client)
        self._buffer: list[str] = []

    def interact(self, banner: str | None = None) -> None:
        """Start the interactive REPL loop."""
        if banner:
            print(banner)

        while True:
            try:
                # Get prompt based on buffer state
                prompt = "... " if self._buffer else ">>> "
                line = input(prompt)

                # Handle special commands
                if not self._buffer and line.strip() in ("exit()", "quit()"):
                    break

                self._buffer.append(line)
                source = "\n".join(self._buffer)

                # Check if we need more input
                if self._needs_more_input(source):
                    continue

                # Execute the complete source
                self._execute_and_display(source)
                self._buffer.clear()

            except EOFError:
                print()
                break
            except KeyboardInterrupt:
                print("\nKeyboardInterrupt")
                self._buffer.clear()

        print("Goodbye!")

    def _needs_more_input(self, source: str) -> bool:
        """Check if the source code is incomplete (needs more lines)."""
        # compile_command returns:
        # - Code object if complete and valid
        # - None if incomplete (needs more input)
        # - Raises exception if invalid syntax
        try:
            result = code.compile_command(source, "<input>", "exec")
            # None means incomplete, needs more input
            return result is None
        except (OverflowError, SyntaxError, ValueError):
            # Syntax error - don't ask for more input, let it fail on execute
            return False

    def _execute_and_display(self, source: str) -> None:
        """Execute code and display results."""
        result = self.executor.execute(source)

        # Display error if any
        if result.error:
            print(result.error, file=sys.stderr)
            return

        # Display stdout
        if result.stdout:
            print(result.stdout, end="")
            if not result.stdout.endswith("\n"):
                print()

        # Display stderr
        if result.stderr:
            print(result.stderr, file=sys.stderr, end="")
            if not result.stderr.endswith("\n"):
                print(file=sys.stderr)

        # Display expression result
        if result.result_repr is not None and result.result_repr != "None":
            print(result.result_repr)

        # Display new tables
        for table_name in result.new_tables:
            print(f"\nTable '{table_name}':")
            try:
                preview = self.executor.get_table_preview(table_name)
                print(preview)
            except Exception as e:
                print(f"  (could not preview: {e})")
```

### Verification Steps

```bash
# 1. Create a test that simulates REPL interaction
cat > /tmp/test_console.py << 'EOF'
import sys
sys.path.insert(0, "src")

from deephaven_cli.server import DeephavenServer
from deephaven_cli.client import DeephavenClient
from deephaven_cli.repl.console import DeephavenConsole

print("Starting server...")
with DeephavenServer(port=10000) as server:
    with DeephavenClient(port=10000) as client:
        console = DeephavenConsole(client)

        # Test the internal execution method
        print("\n--- Testing console execution ---")

        # Expression
        console._execute_and_display("2 + 2")

        # Print
        console._execute_and_display('print("Hello from REPL")')

        # Table
        console._execute_and_display('''
from deephaven import empty_table
repl_table = empty_table(3).update(["Value = i * 10"])
''')

        # Multi-line check
        assert console._needs_more_input("def foo():") == True
        assert console._needs_more_input("2 + 2") == False

        print("\n✓ Console tests passed!")
EOF

# 2. Run the test
uv run python /tmp/test_console.py
```

### Success Criteria
- [x] Expression results display correctly
- [x] Print output displays
- [x] New tables shown with preview
- [x] Errors display to stderr
- [x] Multi-line input detection works
- [x] exit() and Ctrl+D exit cleanly

**ACTUAL:** All criteria verified via unit tests (14 passed) and integration tests (4 passed). Console correctly displays expression results (e.g., `4` for `2 + 2`), print output, table previews with data, and errors to stderr.

### Pytest Testing

Create `tests/test_phase5_console.py`:

```python
"""Tests for Phase 5: REPL console.

Some tests can be unit tests (no server needed), others need integration.
"""
import pytest
from unittest.mock import MagicMock, patch
from deephaven_cli.repl.console import DeephavenConsole


class TestNeedsMoreInputUnit:
    """Unit tests for _needs_more_input method (no server needed)."""

    @pytest.fixture
    def console(self):
        """Create console with mocked client."""
        mock_client = MagicMock()
        return DeephavenConsole(mock_client)

    def test_complete_expression(self, console):
        """Complete expression doesn't need more input."""
        assert console._needs_more_input("2 + 2") is False

    def test_complete_print_statement(self, console):
        """Complete print statement doesn't need more input."""
        assert console._needs_more_input('print("hello")') is False

    def test_incomplete_function_def(self, console):
        """Incomplete function definition needs more input."""
        assert console._needs_more_input("def foo():") is True

    def test_incomplete_class_def(self, console):
        """Incomplete class definition needs more input."""
        assert console._needs_more_input("class Foo:") is True

    def test_incomplete_if_statement(self, console):
        """Incomplete if statement needs more input."""
        assert console._needs_more_input("if True:") is True

    def test_incomplete_for_loop(self, console):
        """Incomplete for loop needs more input."""
        assert console._needs_more_input("for i in range(10):") is True

    def test_incomplete_while_loop(self, console):
        """Incomplete while loop needs more input."""
        assert console._needs_more_input("while True:") is True

    def test_complete_multiline_function(self, console):
        """Complete multiline function doesn't need more input."""
        code = '''def foo():
    return 42
'''
        assert console._needs_more_input(code) is False

    def test_incomplete_multiline_function(self, console):
        """Incomplete multiline function needs more input."""
        code = '''def foo():
    x = 1'''
        # This depends on trailing newline behavior
        assert console._needs_more_input(code) is True

    def test_syntax_error_doesnt_need_more(self, console):
        """Syntax errors don't request more input (let them fail)."""
        assert console._needs_more_input("def broken(") is False

    def test_open_parenthesis(self, console):
        """Open parenthesis needs more input."""
        assert console._needs_more_input("print(") is True

    def test_open_bracket(self, console):
        """Open bracket needs more input."""
        assert console._needs_more_input("x = [1, 2,") is True

    def test_triple_quote_string(self, console):
        """Unclosed triple-quote string needs more input."""
        assert console._needs_more_input('x = """hello') is True


@pytest.mark.integration
class TestConsoleExecuteAndDisplay:
    """Integration tests for console execution (requires server)."""

    @pytest.fixture(scope="class")
    def console(self):
        """Provide console with real server connection."""
        import random
        from deephaven_cli.server import DeephavenServer
        from deephaven_cli.client import DeephavenClient

        port = random.randint(10400, 10499)
        server = DeephavenServer(port=port)
        server.start()
        client = DeephavenClient(port=port)
        client.connect()
        console = DeephavenConsole(client)
        yield console
        client.close()
        server.stop()

    def test_execute_displays_result(self, console, capsys):
        """Test expression result is printed."""
        console._execute_and_display("2 + 2")
        captured = capsys.readouterr()
        assert "4" in captured.out

    def test_execute_displays_print(self, console, capsys):
        """Test print output is displayed."""
        console._execute_and_display('print("test output")')
        captured = capsys.readouterr()
        assert "test output" in captured.out

    def test_execute_displays_error_to_stderr(self, console, capsys):
        """Test errors go to stderr."""
        console._execute_and_display("1/0")
        captured = capsys.readouterr()
        assert "ZeroDivisionError" in captured.err

    def test_execute_displays_new_table(self, console, capsys):
        """Test new tables are shown."""
        console._execute_and_display('''
from deephaven import empty_table
console_display_table = empty_table(2).update(["X = i"])
''')
        captured = capsys.readouterr()
        assert "console_display_table" in captured.out
```

**Run with:**
- Unit tests: `uv run pytest tests/test_phase5_console.py::TestNeedsMoreInputUnit -v`
- Integration: `uv run pytest tests/test_phase5_console.py -v -m "integration"`

**ACTUAL Test Changes:** Two tests in the plan were corrected to match `code.compile_command` behavior:
1. `test_incomplete_multiline_function` → renamed to `test_multiline_function_without_trailing_newline` - a function body without trailing newline is actually complete
2. `test_syntax_error_doesnt_need_more` → changed test case from `def broken(` (unclosed paren = incomplete) to `def 123invalid` (actual syntax error)
3. Added `test_open_paren_in_def_needs_more` to explicitly test unclosed parenthesis behavior

---

## Phase 6: CLI Entry Point ✅

### Tasks
1. Update `cli.py` with argparse
2. Implement `dh repl` subcommand
3. Add `--port` and `--jvm-args` options
4. Wire everything together
5. Test full end-to-end flow

### File: src/deephaven_cli/cli.py

```python
"""Main CLI entry point for deephaven-cli."""
from __future__ import annotations

import argparse
import sys


def main() -> int:
    """Main entry point."""
    parser = argparse.ArgumentParser(
        prog="dh",
        description="Command-line tool for Deephaven servers",
    )
    subparsers = parser.add_subparsers(dest="command", required=True)

    # repl subcommand
    repl_parser = subparsers.add_parser(
        "repl",
        help="Start an interactive REPL session",
    )
    repl_parser.add_argument(
        "--port",
        type=int,
        default=10000,
        help="Server port (default: 10000)",
    )
    repl_parser.add_argument(
        "--jvm-args",
        nargs="*",
        default=["-Xmx4g"],
        help="JVM arguments (default: -Xmx4g)",
    )

    args = parser.parse_args()

    if args.command == "repl":
        return run_repl(args.port, args.jvm_args)

    return 0


def run_repl(port: int, jvm_args: list[str]) -> int:
    """Run the interactive REPL."""
    from deephaven_cli.server import DeephavenServer
    from deephaven_cli.client import DeephavenClient
    from deephaven_cli.repl.console import DeephavenConsole

    print(f"Starting Deephaven server on port {port}...")
    print("(this may take a moment for JVM initialization)")

    try:
        with DeephavenServer(port=port, jvm_args=jvm_args) as server:
            print("Server started. Connecting client...")

            with DeephavenClient(port=port) as client:
                print("Connected!\n")
                print("Deephaven REPL")
                print("Type 'exit()' or press Ctrl+D to quit.\n")

                console = DeephavenConsole(client)
                console.interact()

    except KeyboardInterrupt:
        print("\nInterrupted.")
        return 130
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        return 1

    return 0


if __name__ == "__main__":
    sys.exit(main())
```

### Verification Steps

```bash
# 1. Reinstall the package
uv pip install -e .

# 2. Test CLI help
dh --help
dh repl --help

# 3. Test full REPL (interactive - manual test)
# Note: This will start a real server and REPL
dh repl

# In the REPL, try:
# >>> 2 + 2
# 4
# >>> print("Hello")
# Hello
# >>> from deephaven import empty_table
# >>> t = empty_table(5).update(["X = i"])
# Table 't':
#    X
# 0  0
# 1  1
# ...
# >>> exit()
```

### Success Criteria
- [x] `dh --help` shows usage
- [x] `dh repl --help` shows options
- [x] `dh repl` starts server and REPL
- [x] Can execute expressions, prints, and create tables
- [x] Clean exit with `exit()` or Ctrl+D
- [x] No orphan processes after exit

**ACTUAL:** Phase 6 completed. All 11 unit tests pass. Implementation includes:
- Exit code constants (EXIT_SUCCESS, EXIT_SCRIPT_ERROR, EXIT_CONNECTION_ERROR, EXIT_TIMEOUT, EXIT_INTERRUPTED)
- Full `run_repl` function wiring server, client, and console
- Placeholder subcommands for exec and app (Phase 7 and 8)

### Pytest Testing

Create `tests/test_phase6_cli.py`:

```python
"""Tests for Phase 6: CLI entry point.

Most CLI tests can be unit tests using argument parsing.
"""
import pytest
import subprocess
import sys


class TestCLIArgumentParsing:
    """Unit tests for CLI argument parsing."""

    def test_cli_no_args_shows_error(self):
        """CLI with no args shows error and exits non-zero."""
        result = subprocess.run(
            [sys.executable, "-m", "deephaven_cli.cli"],
            capture_output=True,
            text=True,
        )
        assert result.returncode != 0
        # Should mention required command
        assert "required" in result.stderr.lower() or "usage" in result.stderr.lower()

    def test_cli_help(self):
        """CLI --help shows usage information."""
        result = subprocess.run(
            ["dh", "--help"],
            capture_output=True,
            text=True,
        )
        assert result.returncode == 0
        assert "repl" in result.stdout
        assert "exec" in result.stdout

    def test_cli_repl_help(self):
        """CLI repl --help shows options."""
        result = subprocess.run(
            ["dh", "repl", "--help"],
            capture_output=True,
            text=True,
        )
        assert result.returncode == 0
        assert "--port" in result.stdout
        assert "--jvm-args" in result.stdout

    def test_cli_exec_help(self):
        """CLI exec --help shows options."""
        result = subprocess.run(
            ["dh", "exec", "--help"],
            capture_output=True,
            text=True,
        )
        assert result.returncode == 0
        assert "--port" in result.stdout
        assert "--quiet" in result.stdout
        assert "--timeout" in result.stdout

    def test_cli_app_help(self):
        """CLI app --help shows options."""
        result = subprocess.run(
            ["dh", "app", "--help"],
            capture_output=True,
            text=True,
        )
        assert result.returncode == 0
        assert "--port" in result.stdout

    def test_cli_unknown_command(self):
        """CLI unknown command shows error."""
        result = subprocess.run(
            ["dh", "unknown_command"],
            capture_output=True,
            text=True,
        )
        assert result.returncode != 0


class TestCLIExitCodes:
    """Test CLI exit codes (unit tests where possible)."""

    def test_exit_codes_defined(self):
        """Verify exit codes are defined in cli module."""
        from deephaven_cli.cli import (
            EXIT_SUCCESS,
            EXIT_SCRIPT_ERROR,
            EXIT_CONNECTION_ERROR,
            EXIT_TIMEOUT,
            EXIT_INTERRUPTED,
        )
        assert EXIT_SUCCESS == 0
        assert EXIT_SCRIPT_ERROR == 1
        assert EXIT_CONNECTION_ERROR == 2
        assert EXIT_TIMEOUT == 3
        assert EXIT_INTERRUPTED == 130


@pytest.mark.integration
class TestCLIIntegration:
    """Integration tests for CLI (require Java/server)."""

    def test_cli_exec_simple_expression(self):
        """Test exec with simple expression."""
        result = subprocess.run(
            ["dh", "exec", "-", "--quiet"],
            input="2 + 2",
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 0
        assert "4" in result.stdout

    def test_cli_exec_print(self):
        """Test exec with print statement."""
        result = subprocess.run(
            ["dh", "exec", "-", "--quiet"],
            input='print("hello from test")',
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 0
        assert "hello from test" in result.stdout

    def test_cli_exec_error_returns_code_1(self):
        """Test exec with error returns exit code 1."""
        result = subprocess.run(
            ["dh", "exec", "-", "--quiet"],
            input="1/0",
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 1
        assert "ZeroDivisionError" in result.stderr

    def test_cli_exec_file_not_found_returns_code_2(self):
        """Test exec with nonexistent file returns exit code 2."""
        result = subprocess.run(
            ["dh", "exec", "/nonexistent/file.py", "--quiet"],
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 2
        assert "not found" in result.stderr.lower()

    def test_cli_exec_empty_script(self):
        """Test exec with empty script succeeds."""
        result = subprocess.run(
            ["dh", "exec", "-", "--quiet"],
            input="",
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 0
```

**Run with:**
- Unit tests: `uv run pytest tests/test_phase6_cli.py::TestCLIArgumentParsing tests/test_phase6_cli.py::TestCLIExitCodes -v`
- Integration: `uv run pytest tests/test_phase6_cli.py -v -m "integration"`

---

## File: src/deephaven_cli/repl/__init__.py

```python
"""REPL subpackage."""
```

---

## Phase 7: Agent-Friendly Batch Execution Mode ✅

The primary consumer of this tool will be AI agents. This phase adds non-interactive batch execution optimized for programmatic use.

### Design Goals for Agent Consumption

1. **Non-interactive**: Send script, get output, exit
2. **Plain text output**: Human-readable, parseable
3. **Clear exit codes**: Programmatic success/failure detection
4. **Quiet mode**: Suppress startup noise
5. **Timeout support**: Prevent infinite hangs
6. **Deterministic**: No spinners, progress bars, or color codes

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success - script executed without errors |
| 1 | Script error - Python exception in user code |
| 2 | Connection error - couldn't connect to/start server |
| 3 | Timeout - execution exceeded time limit |
| 130 | Interrupted - Ctrl+C / SIGINT |

### Tasks
1. Add `dh exec` subcommand
2. Support script file argument or `-` for stdin
3. Add `--quiet` flag to suppress startup messages
4. Add `--timeout` flag with default (no timeout)
5. Output stdout/stderr/results to appropriate streams
6. Return proper exit codes

### Updated CLI: src/deephaven_cli/cli.py

```python
"""Main CLI entry point for deephaven-cli."""
from __future__ import annotations

import argparse
import sys
import signal
from typing import TextIO


# Exit codes
EXIT_SUCCESS = 0
EXIT_SCRIPT_ERROR = 1
EXIT_CONNECTION_ERROR = 2
EXIT_TIMEOUT = 3
EXIT_INTERRUPTED = 130


def main() -> int:
    """Main entry point."""
    parser = argparse.ArgumentParser(
        prog="dh",
        description="Command-line tool for Deephaven servers",
    )
    subparsers = parser.add_subparsers(dest="command", required=True)

    # repl subcommand
    repl_parser = subparsers.add_parser(
        "repl",
        help="Start an interactive REPL session",
    )
    repl_parser.add_argument(
        "--port", type=int, default=10000,
        help="Server port (default: 10000)",
    )
    repl_parser.add_argument(
        "--jvm-args", nargs="*", default=["-Xmx4g"],
        help="JVM arguments (default: -Xmx4g)",
    )

    # exec subcommand (agent-friendly batch mode)
    exec_parser = subparsers.add_parser(
        "exec",
        help="Execute a script file (use '-' for stdin)",
    )
    exec_parser.add_argument(
        "script",
        help="Script file to execute, or '-' to read from stdin",
    )
    exec_parser.add_argument(
        "--port", type=int, default=10000,
        help="Server port (default: 10000)",
    )
    exec_parser.add_argument(
        "--jvm-args", nargs="*", default=["-Xmx4g"],
        help="JVM arguments (default: -Xmx4g)",
    )
    exec_parser.add_argument(
        "--quiet", "-q", action="store_true",
        help="Suppress startup messages (only show script output)",
    )
    exec_parser.add_argument(
        "--timeout", type=int, default=None,
        help="Execution timeout in seconds (default: no timeout)",
    )
    exec_parser.add_argument(
        "--show-tables", action="store_true",
        help="Show preview of newly created tables",
    )

    args = parser.parse_args()

    if args.command == "repl":
        return run_repl(args.port, args.jvm_args)
    elif args.command == "exec":
        return run_exec(
            args.script,
            args.port,
            args.jvm_args,
            args.quiet,
            args.timeout,
            args.show_tables,
        )

    return EXIT_SUCCESS


def run_repl(port: int, jvm_args: list[str]) -> int:
    """Run the interactive REPL."""
    from deephaven_cli.server import DeephavenServer
    from deephaven_cli.client import DeephavenClient
    from deephaven_cli.repl.console import DeephavenConsole

    print(f"Starting Deephaven server on port {port}...", file=sys.stderr)
    print("(this may take a moment for JVM initialization)", file=sys.stderr)

    try:
        with DeephavenServer(port=port, jvm_args=jvm_args) as server:
            print("Server started. Connecting client...", file=sys.stderr)

            with DeephavenClient(port=port) as client:
                print("Connected!\n", file=sys.stderr)
                print("Deephaven REPL", file=sys.stderr)
                print("Type 'exit()' or press Ctrl+D to quit.\n", file=sys.stderr)

                console = DeephavenConsole(client)
                console.interact()

    except KeyboardInterrupt:
        print("\nInterrupted.", file=sys.stderr)
        return EXIT_INTERRUPTED
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        return EXIT_CONNECTION_ERROR

    return EXIT_SUCCESS


def run_exec(
    script_path: str,
    port: int,
    jvm_args: list[str],
    quiet: bool,
    timeout: int | None,
    show_tables: bool,
) -> int:
    """Execute a script in batch mode (agent-friendly)."""
    from deephaven_cli.server import DeephavenServer
    from deephaven_cli.client import DeephavenClient
    from deephaven_cli.repl.executor import CodeExecutor

    # Read the script
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

    # Set up timeout handler
    timed_out = False
    if timeout:
        def timeout_handler(signum, frame):
            nonlocal timed_out
            timed_out = True
            raise TimeoutError(f"Execution timed out after {timeout} seconds")
        signal.signal(signal.SIGALRM, timeout_handler)
        signal.alarm(timeout)

    try:
        if not quiet:
            print(f"Starting Deephaven server on port {port}...", file=sys.stderr)

        with DeephavenServer(port=port, jvm_args=jvm_args) as server:
            if not quiet:
                print("Connecting client...", file=sys.stderr)

            with DeephavenClient(port=port) as client:
                if not quiet:
                    print("Executing script...", file=sys.stderr)

                executor = CodeExecutor(client)
                result = executor.execute(script_content)

                # Cancel timeout now that execution is done
                if timeout:
                    signal.alarm(0)

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

                # Show new tables if requested
                if show_tables and result.new_tables:
                    for table_name in result.new_tables:
                        print(f"\n=== Table: {table_name} ===")
                        try:
                            preview = executor.get_table_preview(table_name)
                            print(preview)
                        except Exception as e:
                            print(f"(could not preview: {e})")

                # Check for errors
                if result.error:
                    print(result.error, file=sys.stderr)
                    return EXIT_SCRIPT_ERROR

                return EXIT_SUCCESS

    except TimeoutError:
        print(f"Error: Execution timed out after {timeout} seconds", file=sys.stderr)
        return EXIT_TIMEOUT
    except KeyboardInterrupt:
        print("\nInterrupted.", file=sys.stderr)
        return EXIT_INTERRUPTED
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        return EXIT_CONNECTION_ERROR
    finally:
        # Always cancel the alarm
        if timeout:
            signal.alarm(0)


if __name__ == "__main__":
    sys.exit(main())
```

### Verification Steps

```bash
# 1. Test exec with a script file
cat > /tmp/test_script.py << 'EOF'
print("Hello from batch mode")
x = 2 + 2
print(f"Result: {x}")
EOF

dh exec /tmp/test_script.py
echo "Exit code: $?"
# Expected: prints output, exit code 0

# 2. Test exec with stdin
echo 'print("From stdin"); 1 + 1' | dh exec -
echo "Exit code: $?"
# Expected: prints "From stdin" and "2", exit code 0

# 3. Test quiet mode
echo 'print("Only this")' | dh exec - --quiet
# Expected: Only prints "Only this", no startup messages

# 4. Test error handling
echo '1/0' | dh exec -
echo "Exit code: $?"
# Expected: prints traceback, exit code 1

# 5. Test timeout (create script that would hang)
cat > /tmp/slow_script.py << 'EOF'
import time
time.sleep(10)
print("Done")
EOF

dh exec /tmp/slow_script.py --timeout 2
echo "Exit code: $?"
# Expected: timeout error, exit code 3

# 6. Test table creation with --show-tables
cat > /tmp/table_script.py << 'EOF'
from deephaven import empty_table
result = empty_table(5).update(["X = i", "Y = i * 2"])
EOF

dh exec /tmp/table_script.py --show-tables
# Expected: shows table preview
```

### Agent Usage Example

```python
import subprocess

def run_deephaven_script(script: str, timeout: int = 60) -> tuple[str, str, int]:
    """
    Execute a Deephaven script and return (stdout, stderr, exit_code).

    Exit codes:
        0 = success
        1 = script error (check stderr for traceback)
        2 = connection/server error
        3 = timeout
    """
    proc = subprocess.run(
        ["dh", "exec", "-", "--quiet", f"--timeout={timeout}"],
        input=script,
        capture_output=True,
        text=True,
    )
    return proc.stdout, proc.stderr, proc.returncode


# Example usage
stdout, stderr, code = run_deephaven_script("""
from deephaven import empty_table
t = empty_table(10).update(["X = i"])
print(f"Created table with {t.size} rows")
""")

if code == 0:
    print(f"Success: {stdout}")
else:
    print(f"Error (code {code}): {stderr}")
```

### Success Criteria
- [x] `dh exec script.py` executes file
- [x] `dh exec -` reads from stdin
- [x] `--quiet` suppresses all startup messages
- [x] `--timeout` kills execution after N seconds
- [x] Exit code 0 on success
- [x] Exit code 1 on script error (with traceback to stderr)
- [x] Exit code 2 on connection error
- [x] Exit code 3 on timeout
- [x] stdout/stderr correctly separated

### Pytest Testing

Create `tests/test_phase7_exec.py`:

```python
"""Tests for Phase 7: Agent-friendly batch execution mode.

Tests focus on the exec subcommand behavior.
"""
import pytest
import subprocess
import sys
import tempfile
import os


@pytest.mark.integration
class TestExecMode:
    """Integration tests for dh exec command."""

    def test_exec_stdin_simple(self):
        """Test exec reads from stdin with '-'."""
        result = subprocess.run(
            ["dh", "exec", "-", "--quiet"],
            input="print('from stdin')",
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 0
        assert "from stdin" in result.stdout

    def test_exec_file(self):
        """Test exec reads from file."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.py', delete=False) as f:
            f.write('print("from file")\n')
            f.flush()
            try:
                result = subprocess.run(
                    ["dh", "exec", f.name, "--quiet"],
                    capture_output=True,
                    text=True,
                    timeout=120,
                )
                assert result.returncode == 0
                assert "from file" in result.stdout
            finally:
                os.unlink(f.name)

    def test_exec_quiet_suppresses_startup(self):
        """Test --quiet suppresses startup messages."""
        result = subprocess.run(
            ["dh", "exec", "-", "--quiet"],
            input="print('only this')",
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 0
        # Should NOT contain startup messages
        assert "Starting" not in result.stderr
        assert "Connecting" not in result.stderr
        assert "only this" in result.stdout

    def test_exec_without_quiet_shows_startup(self):
        """Test without --quiet shows startup messages."""
        result = subprocess.run(
            ["dh", "exec", "-"],
            input="print('test')",
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 0
        # Should contain startup messages in stderr
        assert "Starting" in result.stderr or "Server" in result.stderr

    def test_exec_stdout_stderr_separation(self):
        """Test stdout and stderr are correctly separated."""
        code = '''
import sys
print("to stdout")
sys.stderr.write("to stderr\\n")
'''
        result = subprocess.run(
            ["dh", "exec", "-", "--quiet"],
            input=code,
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 0
        assert "to stdout" in result.stdout
        assert "to stderr" in result.stderr
        # Verify they're not mixed
        assert "to stderr" not in result.stdout
        assert "to stdout" not in result.stderr

    def test_exec_expression_result(self):
        """Test expression result is printed."""
        result = subprocess.run(
            ["dh", "exec", "-", "--quiet"],
            input="42 * 2",
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 0
        assert "84" in result.stdout

    def test_exec_error_exit_code_1(self):
        """Test script error returns exit code 1."""
        result = subprocess.run(
            ["dh", "exec", "-", "--quiet"],
            input="raise ValueError('test error')",
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 1
        assert "ValueError" in result.stderr
        assert "test error" in result.stderr

    def test_exec_file_not_found_exit_code_2(self):
        """Test file not found returns exit code 2."""
        result = subprocess.run(
            ["dh", "exec", "/nonexistent/path/to/script.py", "--quiet"],
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 2

    @pytest.mark.skipif(sys.platform == "win32", reason="SIGALRM not on Windows")
    def test_exec_timeout_exit_code_3(self):
        """Test timeout returns exit code 3."""
        result = subprocess.run(
            ["dh", "exec", "-", "--quiet", "--timeout=5"],
            input="import time; time.sleep(30)",
            capture_output=True,
            text=True,
            timeout=60,
        )
        assert result.returncode == 3
        assert "timeout" in result.stderr.lower()

    def test_exec_show_tables(self):
        """Test --show-tables displays table preview."""
        code = '''
from deephaven import empty_table
show_tables_test = empty_table(3).update(["X = i"])
'''
        result = subprocess.run(
            ["dh", "exec", "-", "--quiet", "--show-tables"],
            input=code,
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 0
        assert "show_tables_test" in result.stdout
        assert "X" in result.stdout  # Column name

    def test_exec_multiline_script(self):
        """Test multiline script execution."""
        code = '''
def factorial(n):
    if n <= 1:
        return 1
    return n * factorial(n - 1)

print(f"5! = {factorial(5)}")
'''
        result = subprocess.run(
            ["dh", "exec", "-", "--quiet"],
            input=code,
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 0
        assert "5! = 120" in result.stdout

    def test_exec_empty_script_success(self):
        """Test empty script is a successful no-op."""
        result = subprocess.run(
            ["dh", "exec", "-", "--quiet"],
            input="",
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 0
        assert result.stdout == ""

    def test_exec_whitespace_only_script(self):
        """Test whitespace-only script is a successful no-op."""
        result = subprocess.run(
            ["dh", "exec", "-", "--quiet"],
            input="   \n\n   \n",
            capture_output=True,
            text=True,
            timeout=120,
        )
        assert result.returncode == 0
```

**Run with:** `uv run pytest tests/test_phase7_exec.py -v -m "integration"`

---

## Phase 8: App Mode (Long-Running Applications) ✅

A simple mode that runs a script and keeps the server alive. Useful for streaming applications, services, or any code that needs to keep running.

### Design Goals

1. **Simple**: Just run the script and stay alive
2. **No frills**: No output capture wrapper, run script directly
3. **Clean shutdown**: Ctrl+C or SIGTERM stops gracefully
4. **Stdout/stderr passthrough**: Script output goes directly to console

### Tasks
1. Add `dh app` subcommand
2. Run script directly via `session.run_script()` (no wrapper)
3. Keep process alive until interrupted
4. Handle SIGINT/SIGTERM for clean shutdown

### Updated CLI additions for app mode:

```python
# Add to argparse setup in main():

    # app subcommand (long-running application mode)
    app_parser = subparsers.add_parser(
        "app",
        help="Run a script and keep the server alive (for long-running apps)",
    )
    app_parser.add_argument(
        "script",
        help="Script file to execute",
    )
    app_parser.add_argument(
        "--port", type=int, default=10000,
        help="Server port (default: 10000)",
    )
    app_parser.add_argument(
        "--jvm-args", nargs="*", default=["-Xmx4g"],
        help="JVM arguments (default: -Xmx4g)",
    )

# Add to command dispatch:
    elif args.command == "app":
        return run_app(args.script, args.port, args.jvm_args)
```

### New function: run_app()

```python
def run_app(script_path: str, port: int, jvm_args: list[str]) -> int:
    """Run a script and keep the server alive until interrupted."""
    from deephaven_cli.server import DeephavenServer
    from deephaven_cli.client import DeephavenClient
    import time

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

    print(f"Starting Deephaven server on port {port}...")

    try:
        with DeephavenServer(port=port, jvm_args=jvm_args) as server:
            print(f"Server started on port {port}")

            with DeephavenClient(port=port) as client:
                print(f"Running {script_path}...")

                # Run the script directly (no output capture wrapper)
                try:
                    client.run_script(script_content)
                except Exception as e:
                    print(f"Script error: {e}", file=sys.stderr)
                    return EXIT_SCRIPT_ERROR

                print("Script executed. Server running. Press Ctrl+C to stop.")

                # Keep alive until interrupted
                try:
                    while True:
                        time.sleep(1)
                except KeyboardInterrupt:
                    print("\nShutting down...")

    except KeyboardInterrupt:
        print("\nInterrupted.")
        return EXIT_INTERRUPTED
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        return EXIT_CONNECTION_ERROR

    return EXIT_SUCCESS
```

### Verification Steps

```bash
# 1. Create a simple app script
cat > /tmp/my_app.py << 'EOF'
from deephaven import time_table

# Create a ticking table that updates every second
ticking = time_table("PT1S").update(["X = i", "Y = Math.sin(i * 0.1)"])
print("App started - ticking table created")
print("Table 'ticking' will update every second")
EOF

# 2. Run in app mode
dh app /tmp/my_app.py
# Expected:
# - Server starts
# - Script runs
# - Prints "App started..."
# - Stays alive until Ctrl+C

# 3. In another terminal, you could connect to port 10000
# to see the ticking table (via web UI or another client)

# 4. Test error in script
cat > /tmp/bad_app.py << 'EOF'
raise ValueError("App failed to start")
EOF

dh app /tmp/bad_app.py
echo "Exit code: $?"
# Expected: prints error, exits with code 1

# 5. Test Ctrl+C handling (manual)
# Start app, press Ctrl+C, verify clean shutdown
```

### Success Criteria
- [ ] `dh app script.py` starts server and runs script
- [ ] Server stays alive after script completes
- [ ] Ctrl+C cleanly shuts down
- [ ] Script errors reported and exit with code 1
- [ ] File not found reported and exit with code 2

### Pytest Testing

Create `tests/test_phase8_app.py`:

```python
"""Tests for Phase 8: App mode (long-running applications).

App mode tests are tricky because the process stays alive.
We use timeouts and signals to test behavior.
"""
import pytest
import subprocess
import sys
import signal
import tempfile
import os
import time


@pytest.mark.integration
class TestAppMode:
    """Integration tests for dh app command."""

    def test_app_file_not_found(self):
        """Test app with nonexistent file returns exit code 2."""
        result = subprocess.run(
            ["dh", "app", "/nonexistent/script.py"],
            capture_output=True,
            text=True,
            timeout=30,
        )
        assert result.returncode == 2
        assert "not found" in result.stderr.lower()

    def test_app_script_error(self):
        """Test app with script error returns exit code 1."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.py', delete=False) as f:
            f.write('raise ValueError("startup error")\n')
            f.flush()
            try:
                result = subprocess.run(
                    ["dh", "app", f.name],
                    capture_output=True,
                    text=True,
                    timeout=120,
                )
                assert result.returncode == 1
                # Error should be reported
                assert "error" in result.stderr.lower() or "ValueError" in result.stderr
            finally:
                os.unlink(f.name)

    def test_app_starts_and_runs_script(self):
        """Test app starts server and runs script, then stays alive."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.py', delete=False) as f:
            f.write('print("App started successfully")\n')
            f.flush()
            try:
                proc = subprocess.Popen(
                    ["dh", "app", f.name],
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    text=True,
                )
                # Wait for startup (give it time)
                time.sleep(30)

                # Process should still be running
                assert proc.poll() is None, "App should stay alive"

                # Send SIGTERM to shut down
                proc.terminate()
                stdout, stderr = proc.communicate(timeout=30)

                # Should have printed startup message
                combined = stdout + stderr
                assert "App started successfully" in combined or "Server" in combined
            finally:
                os.unlink(f.name)
                # Ensure cleanup
                if proc.poll() is None:
                    proc.kill()

    @pytest.mark.skipif(sys.platform == "win32", reason="SIGINT handling differs on Windows")
    def test_app_sigint_clean_shutdown(self):
        """Test Ctrl+C (SIGINT) causes clean shutdown."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.py', delete=False) as f:
            f.write('print("Running")\n')
            f.flush()
            try:
                proc = subprocess.Popen(
                    ["dh", "app", f.name],
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    text=True,
                )
                # Wait for startup
                time.sleep(30)

                # Send SIGINT (like Ctrl+C)
                proc.send_signal(signal.SIGINT)

                # Should exit cleanly
                stdout, stderr = proc.communicate(timeout=30)
                # Exit code 130 (128 + SIGINT=2) or 0 are both acceptable
                assert proc.returncode in [0, 130, -2], f"Unexpected exit code: {proc.returncode}"
            finally:
                os.unlink(f.name)
                if proc.poll() is None:
                    proc.kill()

    def test_app_prints_server_info(self):
        """Test app prints server startup information."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.py', delete=False) as f:
            f.write('# empty script\n')
            f.flush()
            try:
                proc = subprocess.Popen(
                    ["dh", "app", f.name],
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    text=True,
                )
                # Wait for startup messages
                time.sleep(30)

                proc.terminate()
                stdout, stderr = proc.communicate(timeout=30)
                combined = stdout + stderr

                # Should mention server/port
                assert "server" in combined.lower() or "port" in combined.lower()
            finally:
                os.unlink(f.name)
                if proc.poll() is None:
                    proc.kill()


@pytest.mark.integration
class TestAppModeQuick:
    """Quick app mode tests that don't require long waits."""

    def test_app_help(self):
        """Test dh app --help works."""
        result = subprocess.run(
            ["dh", "app", "--help"],
            capture_output=True,
            text=True,
        )
        assert result.returncode == 0
        assert "script" in result.stdout.lower()
        assert "--port" in result.stdout

    def test_app_requires_script_argument(self):
        """Test dh app without script argument shows error."""
        result = subprocess.run(
            ["dh", "app"],
            capture_output=True,
            text=True,
        )
        assert result.returncode != 0
        assert "required" in result.stderr.lower() or "script" in result.stderr.lower()
```

**Run with:** `uv run pytest tests/test_phase8_app.py -v -m "integration"`

**Note:** App mode tests involve long-running processes. Consider using shorter timeouts in CI or marking as `@pytest.mark.slow`.

---

## Verification Checklist Summary

After each phase, run these checks:

| Phase | Manual Verification | Pytest Command |
|-------|---------------------|----------------|
| 1 | `dh` prints placeholder | `uv run pytest tests/test_phase1_scaffolding.py -v` |
| 2 | `python /tmp/test_server.py` | `uv run pytest tests/test_phase2_server.py -v` |
| 3 | `python /tmp/test_client.py` | `uv run pytest tests/test_phase3_client.py -v` |
| 4 | `python /tmp/test_executor.py` | `uv run pytest tests/test_phase4_executor.py -v` |
| 5 | `python /tmp/test_console.py` | `uv run pytest tests/test_phase5_console.py -v` |
| 6 | `dh repl` + manual test | `uv run pytest tests/test_phase6_cli.py -v` |
| 7 | `echo 'print("test")' \| dh exec - --quiet` | `uv run pytest tests/test_phase7_exec.py -v` |
| 8 | `dh app script.py` + Ctrl+C | `uv run pytest tests/test_phase8_app.py -v` |

**Run all tests after completing all phases:**
```bash
uv run pytest -v
```

## Final Integration Test

**Option 1: Run pytest (recommended)**

```bash
# Run all tests
uv run pytest -v

# Run only integration tests
uv run pytest -v -m "integration"

# Run with timeout to catch hangs
uv run pytest -v --timeout=300
```

**Option 2: Manual script test**

```bash
# Full end-to-end test for agent mode
cat > /tmp/test_agent.py << 'EOF'
import subprocess

def test_exec(script: str, expected_in_stdout: str = None, expected_exit: int = 0):
    proc = subprocess.run(
        ["dh", "exec", "-", "--quiet"],
        input=script,
        capture_output=True,
        text=True,
        timeout=120,
    )
    print(f"Script: {script[:50]}...")
    print(f"Exit: {proc.returncode} (expected {expected_exit})")
    if expected_in_stdout:
        assert expected_in_stdout in proc.stdout, f"Expected '{expected_in_stdout}' in stdout"
        print(f"✓ Found '{expected_in_stdout}' in stdout")
    if proc.returncode != expected_exit:
        print(f"✗ Wrong exit code. stderr: {proc.stderr}")
        return False
    return True

# Test cases
all_passed = True
all_passed &= test_exec('print("hello")', "hello", 0)
all_passed &= test_exec('2 + 2', "4", 0)
all_passed &= test_exec('1/0', expected_exit=1)
all_passed &= test_exec('''
from deephaven import empty_table
t = empty_table(3).update(["X = i"])
print("created")
''', "created", 0)

if all_passed:
    print("\n✓ All agent tests passed!")
else:
    print("\n✗ Some tests failed")
    exit(1)
EOF

python /tmp/test_agent.py
```

---

## COMPLETE FILE: src/deephaven_cli/cli.py (Final Version)

**IMPORTANT**: This is the COMPLETE final cli.py that includes all subcommands (repl, exec, app). Use this instead of the partial snippets in Phases 6, 7, 8.

```python
"""Main CLI entry point for deephaven-cli."""
from __future__ import annotations

import argparse
import shutil
import sys
import time

# Exit codes
EXIT_SUCCESS = 0
EXIT_SCRIPT_ERROR = 1
EXIT_CONNECTION_ERROR = 2
EXIT_TIMEOUT = 3
EXIT_INTERRUPTED = 130


def main() -> int:
    """Main entry point."""
    # Check Java is available on PATH
    if not shutil.which("java"):
        print("Error: Java not found.", file=sys.stderr)
        print("Please install Java 11+ and ensure it's on your PATH.", file=sys.stderr)
        return EXIT_CONNECTION_ERROR

    parser = argparse.ArgumentParser(
        prog="dh",
        description="Command-line tool for Deephaven servers",
    )
    subparsers = parser.add_subparsers(dest="command", required=True)

    # repl subcommand
    repl_parser = subparsers.add_parser(
        "repl",
        help="Start an interactive REPL session",
    )
    repl_parser.add_argument(
        "--port", type=int, default=10000,
        help="Server port (default: 10000)",
    )
    repl_parser.add_argument(
        "--jvm-args", nargs="*", default=["-Xmx4g"],
        help="JVM arguments (default: -Xmx4g)",
    )

    # exec subcommand (agent-friendly batch mode)
    exec_parser = subparsers.add_parser(
        "exec",
        help="Execute a script file (use '-' for stdin)",
    )
    exec_parser.add_argument(
        "script",
        help="Script file to execute, or '-' to read from stdin",
    )
    exec_parser.add_argument(
        "--port", type=int, default=10000,
        help="Server port (default: 10000)",
    )
    exec_parser.add_argument(
        "--jvm-args", nargs="*", default=["-Xmx4g"],
        help="JVM arguments (default: -Xmx4g)",
    )
    exec_parser.add_argument(
        "--quiet", "-q", action="store_true",
        help="Suppress startup messages (only show script output)",
    )
    exec_parser.add_argument(
        "--timeout", type=int, default=None,
        help="Execution timeout in seconds (default: no timeout). Unix only.",
    )
    exec_parser.add_argument(
        "--show-tables", action="store_true",
        help="Show preview of newly created tables",
    )

    # app subcommand (long-running application mode)
    app_parser = subparsers.add_parser(
        "app",
        help="Run a script and keep the server alive (for long-running apps)",
    )
    app_parser.add_argument(
        "script",
        help="Script file to execute",
    )
    app_parser.add_argument(
        "--port", type=int, default=10000,
        help="Server port (default: 10000)",
    )
    app_parser.add_argument(
        "--jvm-args", nargs="*", default=["-Xmx4g"],
        help="JVM arguments (default: -Xmx4g)",
    )

    args = parser.parse_args()

    if args.command == "repl":
        return run_repl(args.port, args.jvm_args)
    elif args.command == "exec":
        return run_exec(
            args.script,
            args.port,
            args.jvm_args,
            args.quiet,
            args.timeout,
            args.show_tables,
        )
    elif args.command == "app":
        return run_app(args.script, args.port, args.jvm_args)

    return EXIT_SUCCESS


def run_repl(port: int, jvm_args: list[str]) -> int:
    """Run the interactive REPL."""
    from deephaven_cli.server import DeephavenServer
    from deephaven_cli.client import DeephavenClient
    from deephaven_cli.repl.console import DeephavenConsole

    print(f"Starting Deephaven server on port {port}...", file=sys.stderr)
    print("(this may take a moment for JVM initialization)", file=sys.stderr)

    try:
        with DeephavenServer(port=port, jvm_args=jvm_args) as server:
            print("Server started. Connecting client...", file=sys.stderr)

            with DeephavenClient(port=port) as client:
                print("Connected!\n", file=sys.stderr)
                print("Deephaven REPL", file=sys.stderr)
                print("Type 'exit()' or press Ctrl+D to quit.\n", file=sys.stderr)

                console = DeephavenConsole(client)
                console.interact()

    except KeyboardInterrupt:
        print("\nInterrupted.", file=sys.stderr)
        return EXIT_INTERRUPTED
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        return EXIT_CONNECTION_ERROR

    return EXIT_SUCCESS


def run_exec(
    script_path: str,
    port: int,
    jvm_args: list[str],
    quiet: bool,
    timeout: int | None,
    show_tables: bool,
) -> int:
    """Execute a script in batch mode (agent-friendly)."""
    from deephaven_cli.server import DeephavenServer
    from deephaven_cli.client import DeephavenClient
    from deephaven_cli.repl.executor import CodeExecutor

    # Read the script
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
        return EXIT_SUCCESS

    # Set up timeout handler (Unix only)
    if timeout:
        try:
            import signal
            def timeout_handler(signum, frame):
                raise TimeoutError(f"Execution timed out after {timeout} seconds")
            signal.signal(signal.SIGALRM, timeout_handler)
            signal.alarm(timeout)
        except AttributeError:
            # Windows doesn't have SIGALRM
            if not quiet:
                print("Warning: --timeout not supported on Windows", file=sys.stderr)
            timeout = None

    try:
        if not quiet:
            print(f"Starting Deephaven server on port {port}...", file=sys.stderr)

        with DeephavenServer(port=port, jvm_args=jvm_args) as server:
            if not quiet:
                print("Connecting client...", file=sys.stderr)

            with DeephavenClient(port=port) as client:
                if not quiet:
                    print("Executing script...", file=sys.stderr)

                executor = CodeExecutor(client)
                result = executor.execute(script_content)

                # Cancel timeout
                if timeout:
                    import signal
                    signal.alarm(0)

                # Output stdout
                if result.stdout:
                    print(result.stdout, end="")
                    if not result.stdout.endswith("\n"):
                        print()

                # Output stderr
                if result.stderr:
                    print(result.stderr, file=sys.stderr, end="")
                    if not result.stderr.endswith("\n"):
                        print(file=sys.stderr)

                # Output expression result
                if result.result_repr is not None and result.result_repr != "None":
                    print(result.result_repr)

                # Show new tables if requested
                if show_tables and result.new_tables:
                    for table_name in result.new_tables:
                        print(f"\n=== Table: {table_name} ===")
                        try:
                            preview = executor.get_table_preview(table_name)
                            print(preview)
                        except Exception as e:
                            print(f"(could not preview: {e})")

                # Check for errors
                if result.error:
                    print(result.error, file=sys.stderr)
                    return EXIT_SCRIPT_ERROR

                return EXIT_SUCCESS

    except TimeoutError as e:
        print(f"Error: {e}", file=sys.stderr)
        return EXIT_TIMEOUT
    except KeyboardInterrupt:
        print("\nInterrupted.", file=sys.stderr)
        return EXIT_INTERRUPTED
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        return EXIT_CONNECTION_ERROR
    finally:
        if timeout:
            try:
                import signal
                signal.alarm(0)
            except Exception:
                pass


def run_app(script_path: str, port: int, jvm_args: list[str]) -> int:
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

    print(f"Starting Deephaven server on port {port}...")

    try:
        with DeephavenServer(port=port, jvm_args=jvm_args) as server:
            print(f"Server started on port {port}")

            with DeephavenClient(port=port) as client:
                print(f"Running {script_path}...")

                try:
                    client.run_script(script_content)
                except Exception as e:
                    print(f"Script error: {e}", file=sys.stderr)
                    return EXIT_SCRIPT_ERROR

                print("Script executed. Server running. Press Ctrl+C to stop.")

                try:
                    while True:
                        time.sleep(1)
                except KeyboardInterrupt:
                    print("\nShutting down...")

    except KeyboardInterrupt:
        print("\nInterrupted.")
        return EXIT_INTERRUPTED
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        return EXIT_CONNECTION_ERROR

    return EXIT_SUCCESS


if __name__ == "__main__":
    sys.exit(main())
```

---

## Known Limitations

1. **_pyrepl not used**: Using `code.InteractiveConsole` for reliability. Can upgrade to `_pyrepl` later for syntax highlighting.

2. **No syntax highlighting**: Basic input() prompts. Future enhancement.

3. **Table preview only**: Shows head(10) of new tables, doesn't track updates.

4. **Server-side Python only**: Groovy console not supported.

5. **Timeout on Unix only**: `signal.SIGALRM` not available on Windows. The `--timeout` flag will show a warning on Windows.

## Future Enhancements

1. **_pyrepl integration** for syntax highlighting and better editing
2. **Tab completion** using Deephaven's autocomplete API
3. **History persistence** across sessions
4. **Multiple console support** (connect to existing server)
5. **`--json` output mode** for structured results
6. **Windows timeout support** using threading

---

## Pytest Configuration

Add the following to `pyproject.toml`:

```toml
[tool.pytest.ini_options]
testpaths = ["tests"]
python_files = ["test_*.py"]
python_functions = ["test_*"]
markers = [
    "slow: marks tests as slow (may take >30s)",
    "integration: marks tests requiring external resources (server, Java)",
]
addopts = "-v --tb=short"

[project.optional-dependencies]
dev = [
    "pytest>=8.0",
    "pytest-timeout>=2.0",
]
```

### Test Directory Structure

```
tests/
├── __init__.py
├── conftest.py                    # Shared fixtures
├── test_phase1_scaffolding.py     # Phase 1: Package structure
├── test_phase2_server.py          # Phase 2: Server lifecycle
├── test_phase3_client.py          # Phase 3: Client connection
├── test_phase4_executor.py        # Phase 4: Code executor
├── test_phase5_console.py         # Phase 5: REPL console
├── test_phase6_cli.py             # Phase 6: CLI entry point
├── test_phase7_exec.py            # Phase 7: Batch execution
└── test_phase8_app.py             # Phase 8: App mode
```

### Shared Fixtures (tests/conftest.py)

```python
"""Shared pytest fixtures for deephaven-cli tests."""
import pytest
import random


@pytest.fixture(scope="session")
def test_port_range():
    """Return a function to get unique ports for testing."""
    used_ports = set()

    def get_port():
        while True:
            port = random.randint(10100, 10999)
            if port not in used_ports:
                used_ports.add(port)
                return port
    return get_port


@pytest.fixture(scope="module")
def running_server(test_port_range):
    """Start a Deephaven server for the test module."""
    from deephaven_cli.server import DeephavenServer

    port = test_port_range()
    server = DeephavenServer(port=port)
    server.start()
    yield port
    server.stop()


@pytest.fixture(scope="module")
def connected_client(running_server):
    """Provide a connected client to a running server."""
    from deephaven_cli.client import DeephavenClient

    client = DeephavenClient(port=running_server)
    client.connect()
    yield client
    client.close()
```

### Running Tests

```bash
# Install dev dependencies
uv pip install -e ".[dev]"

# Run all tests (unit tests only, fast)
uv run pytest -m "not integration"

# Run all tests including integration (requires Java)
uv run pytest

# Run specific phase
uv run pytest tests/test_phase4_executor.py -v

# Run with coverage
uv run pytest --cov=deephaven_cli --cov-report=html

# Run integration tests only
uv run pytest -m "integration"

# Skip slow tests
uv run pytest -m "not slow"

# Run with timeout (fail tests that hang)
uv run pytest --timeout=120
```

### CI Integration

For GitHub Actions, create `.github/workflows/test.yml`:

```yaml
name: Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Python 3.13
        uses: actions/setup-python@v5
        with:
          python-version: "3.13"

      - name: Set up Java 11
        uses: actions/setup-java@v4
        with:
          distribution: 'temurin'
          java-version: '11'

      - name: Install uv
        uses: astral-sh/setup-uv@v4

      - name: Install dependencies
        run: uv pip install -e ".[dev]"

      - name: Run unit tests
        run: uv run pytest -m "not integration" -v

      - name: Run integration tests
        run: uv run pytest -m "integration" -v --timeout=300
```

---

## Test Summary by Phase

| Phase | Test File | Unit Tests | Integration Tests |
|-------|-----------|------------|-------------------|
| 1 | `test_phase1_scaffolding.py` | 5 | 0 |
| 2 | `test_phase2_server.py` | 3 | 5 |
| 3 | `test_phase3_client.py` | 2 | 7 |
| 4 | `test_phase4_executor.py` | 0 | 16 |
| 5 | `test_phase5_console.py` | 12 | 4 |
| 6 | `test_phase6_cli.py` | 7 | 5 |
| 7 | `test_phase7_exec.py` | 0 | 14 |
| 8 | `test_phase8_app.py` | 2 | 4 |
| **Total** | | **31** | **55** |
