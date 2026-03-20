# Moving render/ into dh-cli

Instructions for Claude Code on the dh-cli side after the user copies this project into `dh-cli/render/`.

## Context

This directory was previously a standalone Node.js project called `dh-render-test` (repo: dh-ui-ascii). It has been moved into dh-cli as a subdirectory. The original repo no longer exists.

The purpose is to add `dh render` and `dh render diagnose` commands to the Go CLI. The Go commands handle server lifecycle and shell out to Node.js to run the rendering.

## What this directory contains

- `src/` — Node.js ESM library for rendering Deephaven UI widgets in jsdom with React 18
- `bin/oneshot.mjs` — **Primary entry point for Go integration**. One-shot script: connect, render, perform actions, output, exit. No daemon.
- `bin/dh-render.mjs` — Interactive daemon-based CLI (kept for dev/standalone use)
- `bin/dh-diagnose.mjs` — One-shot diagnostic tool (kept for dev/standalone use)
- `tests/` — Full test suite (unit, integration, CLI, snapshots). Needs `npm install` and a DH server for integration/snapshot tests. Unit tests run standalone.
- `examples/` — Usage examples
- `package.json` — Node dependencies. Run `npm install` in this directory.

## Architecture overview

1. `src/index.mjs` exports `createTestClient(url)` which loads JSAPI, connects to a DH server, and renders widgets into jsdom
2. `src/cli/session.mjs` has `DaemonSession` with all interaction methods (click, fill, select, snapshot, tables, etc.)
3. `src/cli/snapshot.mjs` builds accessibility tree snapshots with @ref targeting
4. `bin/oneshot.mjs` wires these together: parse args → create client → render widget → execute action pipeline → print results → exit

## Go integration tasks

### 1. Create the embed package

```
src/internal/render/
  embed.go        — //go:embed embedded/*
  setup.go        — extract to ~/.dh/render/, run npm install, version check
  embedded/       — populated by `make render-prepare`
```

**embed.go:**
```go
package render

import "embed"

//go:embed embedded
var EmbeddedFS embed.FS
```

**setup.go** should:
- Check `~/.dh/render/.version` against a hash of the embedded files
- If missing or mismatched: extract embedded/ to ~/.dh/render/, run `npm install --production`
- Verify `node --version` >= 20
- Return the path to `~/.dh/render/`

### 2. Create render command

New file: `src/internal/cmd/render.go`

Register in `root.go` → `addRenderCommands(cmd)`

```
dh render <script.py> [actions...] [flags]
dh render diagnose <script.py> [flags]
```

**Actions** (parsed left-to-right from args after the script path):
- `snapshot` — print accessibility tree
- `click <target>` — click by text or @ref, then auto-snapshot
- `fill <target> <value>` — fill text field, then auto-snapshot
- `select <target> <value>` — select picker option, then auto-snapshot
- `tables` — list exported tables
- `table <id>` — fetch table data
- `html` — dump rendered HTML
- `wait <ms>` — pause for async effects
- `diagnose` — full diagnostic report (only valid as subcommand: `dh render diagnose`)

If no actions given, default to `snapshot`.

**Flags:**
- `--url` — skip server start, connect to existing server
- `--widget` — override widget name (default: auto-detect from Python filename)
- `--port` — server port (default: 0 = auto)
- `--timeout` — render timeout ms (default: 15000)
- `--rows` — max table rows (default: 10)

**Flow:**
```
1. ensureRenderRuntime()  → extract JS if needed, verify node
2. Parse script path + actions from args
3. If --url not provided:
   a. Detect widget name from script (regex: /^(\w+_widget)\s*=/ on file contents)
   b. Start DH server: reuse serve logic with --port 0 --no-browser
   c. Wait for __DH_READY__: signal, capture URL
4. Build command: node ~/.dh/render/bin/oneshot.mjs --url <url> --widget <name> <actions...>
5. Exec node process, pipe stdout/stderr to terminal
6. On exit: kill server if we started it
```

### 3. Update Makefile

Add targets:
```makefile
RENDER_SRC = render
RENDER_EMBED = src/internal/render/embedded

render-prepare:
	rm -rf $(RENDER_EMBED)
	mkdir -p $(RENDER_EMBED)
	cp $(RENDER_SRC)/package.json $(RENDER_EMBED)/
	cp $(RENDER_SRC)/package-lock.json $(RENDER_EMBED)/ 2>/dev/null || true
	cp -r $(RENDER_SRC)/src $(RENDER_EMBED)/
	cp -r $(RENDER_SRC)/bin $(RENDER_EMBED)/

build: render-prepare
	cd $(SRC_DIR) && CGO_ENABLED=0 go build ...
```

### 4. Wire into dh doctor

Add a Node.js check to `dh doctor`:
```
Node.js: v22.1.0 ✓  (required for dh render)
```

Or if missing:
```
Node.js: not found ✗  (install Node.js 20+ for dh render)
```

### 5. Update .gitignore

Add to dh-cli root .gitignore:
```
render/node_modules/
src/internal/render/embedded/
```

## Testing after move

```bash
# Unit tests (no server needed):
cd render && npm install && npm test

# Full test suite (needs DH server):
cd render && npm run test:all
```

## UX examples

```bash
# Default: start server, render, snapshot, stop server
$ dh render test_button.py
Starting server... ready on :54321
Rendering button_widget...

@e1 [button] "Primary"
@e2 [button] "Secondary"
@e3 [button] "Danger" (disabled)

# Click then see result
$ dh render test_button.py click "Primary"
Starting server... ready on :54321
Rendering button_widget...
Clicked "Primary"

@e1 [button] "Primary" (pressed)
@e2 [button] "Secondary"
@e3 [button] "Danger" (disabled)

# Multiple actions
$ dh render test_form.py fill "Name" "Alice" fill "Email" "a@b.com" click "Submit"

# Against running server
$ dh render test_button.py --url http://localhost:10000 snapshot

# Diagnose
$ dh render diagnose test_button.py

# Tables
$ dh render test_table.py tables --rows 20
```

## Key gotchas

1. **Callable ID instability** — Server-side re-renders change callable IDs. The oneshot script handles this internally; Go doesn't need to worry about it.
2. **JSAPI download** — First render against a new server version downloads ~4MB of JSAPI to /tmp/dh-render-jsapi/. This is cached.
3. **CSS loader hooks** — The Node process needs `--import ./src/css-loader.mjs` flag to handle CSS imports from @deephaven packages. The oneshot script handles this.
4. **Widget name detection** — Python files follow convention: `button_widget = ui.button(...)`. Regex: `/^(\w+_widget)\s*=/m`. If the script doesn't match, user must pass `--widget`.
5. **postinstall hook** — `package.json` has a postinstall script (`node src/patch-bundle.mjs`) that patches @deephaven/js-plugin-ui. This runs automatically during `npm install`.
