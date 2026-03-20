# Plan: Add `--vm` Flag to `dh render` (Renderer Inside VM)

## Context

`dh exec --vm` and `dh serve --vm` use Firecracker snapshot-restore for near-instant Deephaven execution. `dh render` currently requires Java + Python venv on the host to start a server, and Node.js + npm to run the renderer. Adding `--vm` eliminates all host dependencies by running both the Deephaven server AND the Node.js renderer inside the VM. The host just sends a vsock request and prints the output.

**Architecture:**
```
Host: dh render script.py --vm
  → restore VM from snapshot
  → vsock: {code, render: true, widget, actions}
  → VM runner: run_script(code) → spawn node oneshot.mjs → capture output
  → vsock response: {render_output: "...snapshot tree..."}
  → print output
```

No Node.js, npm, Java, or HTTP proxy needed on the host.

## Files to Change

### 1. `src/internal/vm/rootfs_linux.go` — Add Node.js + render runtime to rootfs

**Dockerfile changes:**
- Install Node.js 20 via NodeSource apt repo (adds ~100MB)
- Copy render runtime dir into `/opt/render/`
- Run `npm install --omit=dev --legacy-peer-deps --loglevel error` inside Docker build
- `postinstall` hook (`node src/patch-bundle.mjs`) runs automatically during npm install

**Build function changes:**
- Import `render` package to access `render.EmbeddedFS`
- In `buildRootfsDocker`, extract embedded render files to `tmpDir/render/`
- Dockerfile copies them into the image

**Rootfs size:** Currently 2GB ext4. Node.js + render node_modules adds ~300-400MB. Increase ext4 size to 3GB.

### 2. `src/internal/vm/vm_runner.py` — Add render request handling

Add `handle_render_request(session, request)` function:

1. Run user's Python script via `session.run_script(wrapper)` (same as exec)
2. If script errors, return immediately with error
3. Spawn Node.js: `node --no-warnings --import /opt/render/src/css-loader.mjs /opt/render/bin/oneshot.mjs --url http://localhost:10000 --widget <name> --timeout <ms> --rows <n> [actions...]`
4. Capture stdout (snapshot text) and stderr
5. Return in response as `render_output`

Dispatch in `handle_request`: check `request.get("render", False)` and route to `handle_render_request`.

### 3. `src/internal/vm/machine_linux.go` — Extend vsock types

**VsockRequest** — add fields:
```go
Render        bool     `json:"render,omitempty"`
Widget        string   `json:"widget,omitempty"`
Actions       []string `json:"actions,omitempty"`
RenderTimeout int      `json:"render_timeout,omitempty"`
MaxRows       int      `json:"max_rows,omitempty"`
RenderJSON    bool     `json:"render_json,omitempty"`
```

**VsockResponse** — add field:
```go
RenderOutput  string   `json:"render_output,omitempty"`
```

These fields are ignored by existing exec/serve paths (omitempty + vm_runner.py ignores unknown keys).

### 4. `src/internal/cmd/render.go` — Add flags and VM dispatch

- Add `renderVMFlag bool` and `renderPoolFlag bool` vars
- Register `--vm` and `--pool` flags on both render and diagnose commands
- Validation: `--vm` + `--url` mutually exclusive, `--pool` requires `--vm`, `--vm` + `--jvm-args` (if Changed) mutually exclusive
- When `renderVMFlag`:
  - Widget detection still runs on host (just regex, no deps)
  - Skip `render.CheckNode()`, `render.EnsureRuntime()`, server startup, Node.js invocation
  - Call `runRenderVM(cmd, args, diagnose)` and return

### 5. `src/internal/cmd/render_vm_linux.go` — New file (Linux VM implementation)

`//go:build linux`

**`runRenderVM(cmd *cobra.Command, args []string, diagnose bool) error`:**

1. Read script, detect widget name
2. Resolve version (flag → env → config → latest snapshot)
3. Pool checkout or cold restore (same pattern as `serve_vm_linux.go`)
4. Start file server (`vm.StartFileServer` on `SnapVsockPath`)
5. Build `VsockRequest{Code: script, Render: true, Widget: name, Actions: actionArgs, RenderTimeout: timeout, MaxRows: rows}`
   - If `diagnose`, set `Actions: ["diagnose"]`
6. `vm.ExecuteViaVsock(info.VsockPath, vm.VsockPort, req)`
7. Print `resp.RenderOutput` to stdout
8. Print `resp.Stderr` to stderr (if any)
9. Cleanup via defers

### 6. `src/internal/cmd/render_vm_other.go` — New file (platform stub)

`//go:build !linux` — prints error and exits, same pattern as `serve_vm_other.go`.

### 7. `unit_tests/render_test.go` — Add tests

- `--vm` and `--pool` flags visible in help output
- `--vm` + `--url` mutual exclusion error
- `--pool` requires `--vm` error

## Key Design Decisions

- **Widget detection on host:** Still runs `render.DetectWidgetName()` on the host. It's pure regex on the script file — no deps needed. Avoids sending widget detection logic into the VM.
- **Render runtime in rootfs, not snapshot memory:** npm deps are installed during Docker build (rootfs), not during snapshot warmup. They persist on disk, not in memory. This keeps snapshot_mem small.
- **No snapshot warmup for Node.js:** V8 warms up fast enough on its own. The JVM warmup (20 iterations) is the one that matters. Adding Node.js warmup would bloat snapshot memory for marginal benefit.
- **Pool support via PoolCheckout:** Same as serve — get a dedicated VM, keep it alive for the render request, destroy after.

## Shared code (no duplication)

- `listSnapshotVersions` — already in `serve_vm_linux.go`, same package + build tag, reusable
- `vm.RestoreFromSnapshot`, `vm.ExecuteViaVsock`, `vm.StartFileServer`, etc. — all existing

## Verification

```bash
# Build (includes render runtime in embedded FS)
CGO_ENABLED=0 make build && make test

# Flag tests
./dh render --help                       # shows --vm, --pool
./dh render script.py --vm --url X       # error: mutually exclusive
./dh render script.py --pool             # error: requires --vm

# End-to-end (requires dh vm prepare with new rootfs that includes Node.js)
dh vm prepare -v                         # rebuilds rootfs with Node.js + render runtime
dh render test_button.py --vm
dh render test_iris_dashboard.py --vm
dh render test_button.py --vm click "Primary"
dh render diagnose test_button.py --vm
```

## Implementation Order

1. `render_vm_other.go` — stub (satisfies compilation everywhere)
2. `machine_linux.go` — extend VsockRequest/VsockResponse types
3. `vm_runner.py` — add render request handler
4. `rootfs_linux.go` — add Node.js + render runtime to Dockerfile, increase ext4 size
5. `render.go` — add flags, validation, VM dispatch
6. `render_vm_linux.go` — VM implementation
7. `render_test.go` — flag and validation tests
8. Build, test, verify with `dh vm prepare` + `dh render --vm`
