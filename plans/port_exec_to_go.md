# Plan: Port `dh exec` to Go `dhg exec`

## Context

The Python `dh exec` command is a batch-mode execution tool for running Python scripts on Deephaven servers. It's used for automation and AI agent workflows. The Go `dhg` CLI currently handles only management tasks (install, config, doctor, etc). This plan adds `dhg exec` with full feature parity.

## Architecture

Go cannot start a Deephaven server directly (requires Python + JVM). The approach:

- **Go handles**: CLI parsing, version resolution, Java detection, process lifecycle, timeout, signals, exit codes, verbose Go-side info, JSON output envelope
- **Python handles**: Server start, client connect, code execution, table previews, structured JSON output (via an embedded runner script)

```
dhg exec -c "print('hello')"
  |
  Go: parse flags → resolve version → find venv python → detect Java
  |   (--verbose prints: "Resolved version 0.35.1", "Using Java at ...", "Venv: ...")
  |
  Go: ensure pydeephaven is installed in venv (auto-install if missing)
  |
  Go: read user code (from -c / file / stdin)
  |
  Go: spawn <venv-python> -c "<embedded runner>" --port 10000 ...
  |   └── user code piped via stdin
  |
  |   Python runner (via -c):
  |     reads user code from stdin
  |     parses CLI args for port, host, jvm-args, etc.
  |     embedded mode: starts Server, connects Session
  |     remote mode: connects Session to host:port
  |     executes code with output capture (eval/exec + pickle/base64)
  |     normal mode: prints stdout/stderr, table previews to stdout/stderr
  |     --output-json mode: prints single JSON blob to stdout
  |     exits with appropriate code
  |
  Go (normal): forwards stdout/stderr directly, exits with child's exit code
  Go (--json): reads runner's JSON stdout, augments with Go-side info, emits final JSON
```

**No temp files.** Runner script via `python -c`, user code via stdin, config via CLI args.

## Protocol: Go → Python

```
<venv-python> -c "<runner script>" \
  --mode embedded \
  --port 10000 \
  --jvm-args "-Xmx4g" \          # single quoted string
  --show-tables \
  --show-table-meta \
  --script-path /abs/path/to/script.py \
  --cwd /caller/working/dir \
  --output-json \                  # only when Go's --json is set
  --host remote.example.com \      # remote mode only
  --auth-type Anonymous \           # remote mode only
  --auth-token "..." \              # remote mode only
  --tls \                           # remote mode only
  --tls-ca-cert /path \             # remote mode only
  --tls-client-cert /path \         # remote mode only
  --tls-client-key /path            # remote mode only
```

The runner reads user code from stdin (`sys.stdin.read()`), parses flags with `argparse`.

## Runner Output Modes

**Normal mode** (no `--output-json`): Human-readable output.
- stdout → user's captured stdout, expression result, table previews
- stderr → user's captured stderr, errors
- Go forwards both directly to the terminal

**JSON mode** (`--output-json`): Runner prints a single JSON blob to stdout:
```json
{
  "exit_code": 0,
  "stdout": "hello\n",
  "stderr": "",
  "result_repr": null,
  "error": null,
  "tables": [
    {
      "name": "t",
      "row_count": 100,
      "is_refreshing": false,
      "columns": [{"name": "X", "type": "int64"}],
      "preview": "   X\n   0\n   1\n..."
    }
  ]
}
```

Go reads this, augments with Go-side info (version, java_home, elapsed_seconds), and emits the final JSON.

## Files to Create

### 1. `go_src/internal/exec/exec.go` — Core orchestration

Types:
- `ExecConfig` — all CLI flags + resolved state

Key exported functions:
- `Run(cfg *ExecConfig) (exitCode int, jsonResult map[string]any, err error)` — main entry
- `FindVenvPython(dhgHome, version string) (string, error)` — locate venv python binary
- `EnsurePydeephaven(pythonBin, version string) error` — auto-install pydeephaven if missing
- `RunnerScript() string` — return embedded runner content (for testing)

The runner script is embedded via `//go:embed runner.py`.

Logic in `Run()`:
1. Validate inputs (exactly one of -c / script / stdin)
2. Read code from source (file, stdin, or -c) into a string
3. Resolve version via `config.ResolveVersion()`
4. Find venv python: `~/.dhg/versions/<ver>/.venv/bin/python`
5. Auto-install `pydeephaven` if missing from venv (matching server version)
6. If embedded mode: detect Java via `java.Detect()`, set JAVA_HOME
7. If `--verbose`: print Go-side details to stderr (resolved version, Java path, venv path)
8. Build args list for the runner (--mode, --port, --output-json if --json, etc.)
9. Spawn: `exec.CommandContext(ctx, pythonBin, "-c", runnerScript, runnerArgs...)`
10. Pipe user code to child's stdin
11. Handle timeout via `context.WithTimeout` → kills process group on deadline
12. Handle SIGINT → forward to child process
13. Normal: forward stdout/stderr directly. JSON: capture runner's JSON stdout, augment, emit
14. Return child's exit code

**`EnsurePydeephaven`** logic:
- Run `<venv-python> -c "import pydeephaven"` to check if installed
- If import fails, run `uv pip install --python <venv-python> pydeephaven==<version>`
- Print "Installing pydeephaven..." to stderr (unless --quiet)

### 2. `go_src/internal/exec/exec_unix.go` / `exec_windows.go` — Platform-specific process group

- `processGroupAttr() *syscall.SysProcAttr` — Setpgid on unix, CREATE_NEW_PROCESS_GROUP on windows
- `killProcessGroup(pid int) error` — `syscall.Kill(-pid, SIGKILL)` on unix, `taskkill /F /T` on windows

Follows existing pattern in `discovery/kill_unix.go` and `discovery/kill_windows.go`.

### 3. `go_src/internal/exec/runner.py` — Python runner (real file, ~300 lines)

A real `.py` file in the Go project tree — editable, lintable, testable. Embedded into the Go binary at compile time via `//go:embed runner.py` in exec.go. Self-contained — only imports from `deephaven_server`, `pydeephaven`, and stdlib (no dependency on `deephaven_cli`).

**Entry point**: parses args with `argparse`, reads user code from stdin, dispatches to embedded or remote mode.

**Execution flow** (ported from `src/deephaven_cli/repl/executor.py` and `src/deephaven_cli/cli.py:run_exec()`):

1. **Parse args & read code**: `argparse` for flags, `sys.stdin.read()` for code
2. **Connect**: Start embedded server or connect to remote host
3. **Execute code**: Build wrapper script that captures stdout/stderr via StringIO, tries eval() then falls back to exec(), catches exceptions, pickles results dict, base64-encodes into a Deephaven table (`__dh_result_table`)
4. **Read results**: Client reads `__dh_result_table`, base64-decodes, unpickles → gets stdout, stderr, result_repr, error
5. **Detect assigned tables**: AST-parse the user's code to find assigned variable names (`get_assigned_names`), check which are now tables on the server
6. **Normal output mode** (no `--output-json`):
   - Print stdout to sys.stdout, stderr to sys.stderr, expression result to sys.stdout
   - If `--show-tables`: for each assigned table, fetch preview and print:
     ```
     === Table: t (100 rows, static) ===
     Columns: X (int64), Y (string)

      X  Y
      0  a
      1  b
     ```
   - Column types/row count shown unless `--no-show-table-meta`
7. **JSON output mode** (`--output-json`):
   - Collect stdout, stderr, result_repr, error, and table metadata into a dict
   - Table metadata includes: name, row_count, is_refreshing, columns (name+type), preview string
   - Print single JSON blob to stdout via `json.dumps()`
8. **Clean up**: Delete `__dh_result_table` from server, close session

**Key functions** (ported from):
- `build_wrapper(code, script_path, cwd)` — from executor.py lines 143-211
- `get_assigned_names(code)` — from executor.py lines 15-47
- `read_result_table(session)` — from executor.py lines 213-227
- `get_table_preview(session, name, show_meta)` — from executor.py lines 277-331
- `run_embedded(args, code)` — from cli.py lines 1248-1260
- `run_remote(args, code)` — from cli.py lines 1239-1246

**Exit codes**: 0=success, 1=script error, 2=connection error, 130=interrupted.
(Timeout is handled by Go killing the process, not by the runner.)

### 4. `go_src/internal/cmd/exec.go` — Cobra command

Flags (matching Python `dh exec`):
- Positional: `SCRIPT` (file path or `-` for stdin)
- `-c CODE`: inline Python code
- `--port` (default: 10000)
- `--jvm-args` (default: `"-Xmx4g"`, single quoted string)
- `--timeout SECONDS`, `--no-show-tables`, `--no-table-meta`
- `--version VERSION` (Deephaven version)
- `--host`, `--auth-type`, `--auth-token`, `--tls`, `--tls-ca-cert`, `--tls-client-cert`, `--tls-client-key`
- Global: `--json/-j`, `--verbose/-v`, `--quiet/-q`

Uses `os.Exit(exitCode)` directly (exec exit codes must be the process exit code).

### 5. `go_unit_tests/exec_test.go` — Unit tests

- Config validation (both -c and script → error, neither → error, each alone → ok)
- FindVenvPython (creates fake venv dir structure)
- Mode detection (no --host → embedded, with --host → remote)
- Runner script embedded content verification
- Exec command help text verification (has -c, --timeout, --host, --json)

## Files to Modify

### 1. `go_src/internal/cmd/root.go` — Wire exec command

Add `addExecCommand(cmd)` in `NewRootCmd()` (line ~32, before `return cmd`).

### 2. `go_src/internal/versions/install.go` — Add pydeephaven to install

The Python `dh install` already installs `pydeephaven=={version}` (see `manager/versions.py:89-90`). The Go install is missing this. Change line 57:

```go
// Before:
pipArgs := []string{"pip", "install", "--python", pythonBin, fmt.Sprintf("deephaven-server==%s", version)}

// After:
pipArgs := []string{"pip", "install", "--python", pythonBin,
    fmt.Sprintf("deephaven-server==%s", version),
    fmt.Sprintf("pydeephaven==%s", version),
}
```

### 3. `go_src/cmd/dhg/main.go` — `-c` shorthand

Before `cmd.Execute()`, rewrite `dhg -c "code"` → `dhg exec -c "code"` (matching Python cli.py line 201-202).

## Implementation Order

1. Modify `versions/install.go` — add pydeephaven to pip install
2. Create `go_src/internal/exec/runner.py` — port Python execution logic (both output modes)
3. Create `go_src/internal/exec/exec.go` — types, validation, FindVenvPython, EnsurePydeephaven, embed runner, Run()
4. Create `exec_unix.go` / `exec_windows.go` — process group helpers
5. Create `go_src/internal/cmd/exec.go` — Cobra command with all flags
6. Modify `root.go` — wire addExecCommand
7. Modify `main.go` — `-c` shorthand
8. Create `go_unit_tests/exec_test.go` — unit tests
9. Run `make test` to verify no regressions

## Verification

1. `cd go_src && make test` — all existing + new unit tests pass
2. `dhg exec -c "print('hello')"` — prints "hello", exits 0
3. `dhg -c "print('shorthand')"` — shorthand works
4. `echo "print('stdin')" | dhg exec -` — stdin works
5. `dhg exec script.py` — file execution works
6. `dhg exec -c "from deephaven import empty_table; t = empty_table(5)"` — shows table preview
7. `dhg exec -c "import time; time.sleep(100)" --timeout 2` — exits 3 after 2s
8. `dhg exec -c "print('json')" --json` — outputs structured JSON with tables, exit_code, etc.
9. `dhg exec -c "1/0"` — exits 1 with traceback on stderr
10. `dhg exec -c "print('remote')" --host localhost --port 10000` — remote mode
11. `dhg exec -c "print('hello')" -v` — verbose shows "Resolved version ...", "Using Java at ...", etc.
12. Auto-install: exec on a venv without pydeephaven installs it automatically
