# Port `dh serve` to Go `dh serve`

## Context

The Python CLI's `dh serve` command starts an embedded Deephaven server, runs a user script (e.g. a dashboard), opens the browser, and keeps the server alive until Ctrl+C. This needs to be ported to the Go `dh` CLI following the same architecture as `dh exec`: Go handles CLI/lifecycle, the embedded Python runner handles Deephaven interaction.

## Architecture

Same pattern as exec: Go orchestrates, `runner.py` does the Deephaven work.

```
dh serve dashboard.py --port 8080
  │
  ├─ Go: resolve version, find python, detect java
  ├─ Go: start python -c <runner.py> --mode serve --port 8080
  │       (pipe script content via stdin)
  │
  ├─ Runner: start embedded server, run script, print sentinel
  │   stdout → "__DH_READY__:http://localhost:8080"
  │   stdout → "Server running at http://localhost:8080"
  │   stdout → "Press Ctrl+C to stop."
  │   (blocks in keep-alive loop)
  │
  ├─ Go: reads stdout, detects sentinel → opens browser
  ├─ Go: forwards remaining stdout/stderr to terminal
  ├─ Go: on SIGINT → forwards to runner process group
  │
  └─ Runner: catches KeyboardInterrupt → "Shutting down..." → exit 0
```

Key difference from exec: serve runs the script directly via `session.run_script(code)` (no output-capture wrapper) and keeps the server alive.

## Files to Create

### 1. `go_src/internal/cmd/serve.go` — Cobra command

Flags:
- `SCRIPT` — positional, required (script file path)
- `--port` — int, default 10000
- `--jvm-args` — string, default "-Xmx4g"
- `--no-browser` — bool, don't open browser
- `--iframe WIDGET` — string, widget name for iframe URL
- `--version` — string, Deephaven version

`runServe()` flow:
1. Read script file (`os.ReadFile`)
2. Resolve version via `config.ResolveVersion()`
3. Find venv python via `dhexec.FindVenvPython()`
4. Ensure pydeephaven via `dhexec.EnsurePydeephaven()`
5. Detect Java via `java.Detect()`
6. Print "Starting Deephaven..." (unless `--quiet`)
7. Build runner args: `--mode serve --port N [--jvm-args=X] [--iframe W] --script-path P --cwd D`
8. Start subprocess: `python -c <runner.py> <args>` with script on stdin
9. Pipe stdout through a `bufio.Scanner`:
   - On `__DH_READY__:<url>`: open browser (if `!--no-browser`), don't forward this line
   - All other lines: forward to user's stdout
10. Handle signals: first SIGINT → forward `SIGINT` to child; second → `SIGKILL`
11. `process.Wait()` → exit with child's exit code

### 2. `go_behaviour_tests/testdata/scripts/serve.txtar` — Behaviour tests

Same mock-based approach as exec tests:
- Help text and flag verification
- Input validation (no script, nonexistent script)
- Mock-based argument passing verification
- Ready sentinel and browser-open flow
- `--no-browser`, `--iframe`, `--port`, `--jvm-args` flags

## Files to Modify

### 3. `go_src/internal/exec/runner.py` — Add serve mode

Add `--mode serve` choice to argparse and `--iframe` arg.

New `run_serve(args, code)` function:
1. Check port availability, fall back to port 0 if busy
2. Suppress JVM/server output (same fd-level redirect as `run_embedded`)
3. `Server(port=port, jvm_args=...)` → `server.start()`
4. Restore output
5. Connect via `Session(host="localhost", port=actual_port)`
6. `session.run_script(code)` — direct execution, no wrapper
7. Build URL (with `--iframe` support)
8. Print `__DH_READY__:<url>` sentinel (consumed by Go)
9. Print `Server running at <url>` and `Press Ctrl+C to stop.`
10. Keep-alive loop (`while True: time.sleep(1)`)
11. On `KeyboardInterrupt`: print "Shutting down...", close session, exit 0

### 4. `go_src/internal/cmd/root.go` — Register serve command

Add `addServeCommand(cmd)` to `NewRootCmd()`.

## What Serve Does NOT Have (vs exec)

- No `-c` flag (always a script file)
- No stdin mode (`-`)
- No `--json` output mode
- No `--show-tables` / `--show-table-meta`
- No `--host` / remote mode (embedded only)
- No `--timeout`

## Signal Handling

```
User presses Ctrl+C (1st time):
  Go catches SIGINT → sends SIGINT to child process group
  Runner catches KeyboardInterrupt → prints "Shutting down..." → exits 0

User presses Ctrl+C (2nd time):
  Go catches SIGINT → sends SIGKILL to child process group (force kill)
```

## Verification

1. `go build ./cmd/dh` — compiles
2. `cd go_unit_tests && go test ./...` — unit tests pass
3. `cd go_behaviour_tests && go test -v -run TestBehaviour/serve` — behaviour tests pass
4. Manual smoke test: `dh serve dashboard.py` with a real installed version
