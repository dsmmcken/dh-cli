# CLAUDE.md

This file provides guidance to Claude Code when working in the `render/` subdirectory of dh-cli.

## What This Is

Node.js library for rendering Deephaven UI widgets in jsdom with React 18. Used by `dh render` to provide headless widget rendering, accessibility snapshots, and DOM interaction (click, fill, select) without a browser.

## Integration with dh-cli

See `MOVE-TO-DH-CLI.md` at this directory root for the full Go integration plan. Key points:

- `bin/oneshot.mjs` is the primary entry point invoked by the Go `dh render` command
- Go handles server lifecycle; this code handles rendering and interaction
- JS files are embedded in the Go binary via `//go:embed` and extracted to `~/.dh/render/` at runtime
- Requires Node.js 20+

## Directory Layout

- `src/` — Core library (ESM, `type: "module"`)
- `src/cli/` — CLI session, snapshot builder, daemon (daemon kept for standalone dev use)
- `bin/oneshot.mjs` — One-shot entry point for Go integration
- `bin/dh-render.mjs` — Interactive daemon CLI (standalone dev use)
- `bin/dh-diagnose.mjs` — One-shot diagnostic tool (standalone dev use)
- `tests/` — Full test suite (unit, integration, CLI, snapshots)
- `examples/` — Usage examples

## Commands

```bash
# Install dependencies:
npm install

# Unit and CLI tests (no server needed):
npm test

# Snapshot tests (auto-discovers Python files, starts servers per test):
npm run test:snapshots
npm run test:snapshots:update

# Integration tests:
npm run test:integration

# All tests:
npm run test:all
```

## Key Source Files

- `src/index.mjs` — Public API: `renderWidget()`, `createTestClient()`, `TestClient`, `RenderResult`
- `src/cli/session.mjs` — `DaemonSession` with all interaction methods (click, fill, select, snapshot, tables)
- `src/cli/snapshot.mjs` — Accessibility tree builder with @ref targeting
- `src/JsApiLoader.mjs` — Downloads and loads JSAPI in jsdom
- `src/WidgetClient.mjs` — Widget connection + JSON-RPC protocol

## Important Patterns

### Callable ID Instability
Callable IDs change after every server-side re-render. Never cache them across re-renders.

### Widget Type Auto-Detection
Widget types are auto-detected via `subscribeToFieldUpdates()`. The `--type` flag is optional.

### jsdom Global Installation
`TestClient` temporarily installs jsdom globals on `globalThis` and restores originals on `close()`.

### Widget Auto-Discovery
Widgets are auto-discovered from the server via `subscribeToFieldUpdates()`.
Priority: `deephaven.ui.Dashboard` > `deephaven.ui.Element`. Multiple widgets are rendered together.
The `--widget` flag is optional — only needed to target a specific variable.

### postinstall Hook
`npm install` runs `node src/patch-bundle.mjs` which patches `@deephaven/js-plugin-ui` for jsdom compatibility.
