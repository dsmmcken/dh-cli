# Plan: Rename `dh app` to `dh serve` + URL printing + browser auto-open

## Changes

### 1. Rename subcommand `app` → `serve` in `src/deephaven_cli/cli.py`

**Parser definition (~lines 263-299):**
- Rename `"app"` to `"serve"` in `subparsers.add_parser()`
- Update help text and examples to reference `dh serve`
- Add `--no-browser` flag (default: browser opens automatically)

**Command routing (~line 329-330):**
- Change `args.command == "app"` to `args.command == "serve"`
- Pass `args.no_browser` to the function

### 2. Update `run_app()` → `run_serve()` in `src/deephaven_cli/cli.py` (~lines 623-676)

- Rename function to `run_serve()`
- Add `no_browser: bool = False` parameter
- After server starts and script executes, **always** print the full clickable URL:
  ```
  Server running at http://localhost:{actual_port}
  ```
- Unless `--no-browser`, open the URL via `webbrowser.open()` (stdlib, no new deps)

### 3. Update `README.md`

- Replace `dh app` references with `dh serve`

## Files to modify

- `src/deephaven_cli/cli.py`
- `README.md`

## Verification

1. `dh serve dashboard.py` — prints URL, opens browser
2. `dh serve dashboard.py --no-browser` — prints URL, no browser
3. `dh serve dashboard.py -v` — verbose output + URL + browser
