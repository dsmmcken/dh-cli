# Plan: Allow `dh serve` without a script argument

## Problem

Running `dh serve` without any arguments fails because the command requires exactly
one positional argument (the script file). Users should be able to run `dh serve`
to simply start a bare Deephaven server — no script, no extra connection work — just
start the server, open the browser, and keep it alive.

## Current Behavior

- `serve.go:51` — `Args: cobra.ExactArgs(1)` enforces a script argument
- `runner.py:376-464` — `run_serve()` always connects via pydeephaven Session and
  calls `session.run_script(code)`, even when there's nothing to run
- The connection retry loop (lines 414-428) adds up to 5 seconds of latency that
  is unnecessary when no script needs to be executed

## Desired Behavior

- `dh serve` (no args) starts the Deephaven server, opens the browser, and blocks
  until Ctrl+C
- `dh serve script.py` works as before (starts server, runs script, opens browser,
  blocks)
- When there is no script, the runner should skip the pydeephaven Session connection
  entirely — no retry loop, no `run_script()` call

## Changes Required

### 1. `src/internal/cmd/serve.go`

- Change `Args: cobra.ExactArgs(1)` → `Args: cobra.MaximumNArgs(1)`
- Guard the script-reading block: only read the file if `len(args) > 0`
- When no script is provided, pass empty string to stdin (or a no-op marker)
- Update `Use:` line from `"serve SCRIPT"` to `"serve [SCRIPT]"` and adjust help text

### 2. `src/internal/exec/runner.py` — `run_serve()`

- When `code` is empty/whitespace, skip the entire pydeephaven connection + script
  execution block (lines 414-438)
- Go straight to printing the `__DH_READY__` sentinel and entering the keep-alive
  loop
- This eliminates the pydeephaven import, 10-retry connection loop, and
  `run_script()` call for the no-script case

### 3. `main()` in `runner.py`

- Currently lines 524-535 exit early with code 0 for empty input in non-serve mode.
  For serve mode, empty code should proceed into `run_serve()` so the server starts.
  Adjust the early-exit guard to only apply when `args.mode != "serve"`.

## Flow (no-script case)

```
dh serve
  → Go: no script file, passes empty stdin to runner
  → Python: run_serve() starts JVM server
  → Python: code is empty → skip Session connect + run_script
  → Python: print __DH_READY__:<url>
  → Go: detects sentinel, opens browser
  → Python: keep-alive loop until Ctrl+C
```

## Testing

- Add a behaviour test: `dh serve` (no args) should start successfully and emit the
  ready sentinel
- Existing `dh serve script.py` tests should continue to pass
