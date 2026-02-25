# Plan: `dh serve --iframe WIDGET` flag

## Overview

Add `--iframe WIDGET` option to `dh serve` that opens the browser to the iframe URL for a specific widget instead of the default server URL.

Deephaven iframe URL format: `http://localhost:{port}/iframe/widget/?name={widget_name}`

## Example usage

```bash
# Opens http://localhost:10000/iframe/widget/?name=sin_chart
dh serve dashboard.py --iframe sin_chart

# Combined with other flags
dh serve dashboard.py --port 8080 --iframe my_table
dh serve dashboard.py --iframe my_widget --no-browser  # prints URL but doesn't open
```

## Changes

### `src/deephaven_cli/cli.py`

1. **Add `--iframe` argument** to `serve_parser`:
   ```python
   serve_parser.add_argument(
       "--iframe",
       metavar="WIDGET",
       help="Open browser to iframe URL for the given widget name",
   )
   ```

2. **Pass to `run_serve()`**: Add `iframe: str | None = None` parameter.

3. **In `run_serve()`** (~line 760): Change URL construction:
   ```python
   url = f"http://localhost:{actual_port}"
   if iframe:
       url = f"{url}/iframe/widget/?name={iframe}"
   ```
   The printed URL and browser URL both use the same `url` variable, so both update automatically.

4. **Update routing** (~line 390): Pass `args.iframe` to `run_serve()`.

5. **Update epilog examples** to show `--iframe`.

### `README.md`

Add `--iframe` to serve mode docs.

## Files to modify

- `src/deephaven_cli/cli.py` — add flag, pass through, construct URL
- `README.md` — document the flag

## Verification

1. `dh serve tests/basic_script.py --iframe t` — should open `http://localhost:{port}/iframe/widget/?name=t`
2. `dh serve tests/basic_script.py` — should still open `http://localhost:{port}` (no change)
3. `dh serve tests/basic_script.py --iframe t --no-browser` — should print iframe URL but not open browser
