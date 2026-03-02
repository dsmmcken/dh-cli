# Plan: Add `--vm` Mode to `dh serve`

## Context

`dh serve` runs a user script against an embedded Deephaven server and keeps
the server alive for interactive use (dashboards, visualizations). It requires
Java on the host and takes several seconds to start.

`dh exec --vm` restores a Firecracker microVM from a snapshot that already
contains a running Deephaven server + warm Python session. Execution is fast
(~20ms from pool, ~700ms cold restore) and needs no host-side Java or Python.

Adding `--vm` to `dh serve` would let users run persistent dashboards through
the VM path — no Java install, fast startup, sandboxed execution. The challenge
is that the VM has **zero TCP networking** (only vsock), so the Deephaven web UI
running inside the VM is not directly reachable from the host browser. We solve
this with a bidirectional byte-level TCP↔vsock proxy.

## Architecture

```
Browser (host)
    │  HTTP / WebSocket / gRPC
    │  TCP localhost:<--port>
    ▼
┌─────────────────────┐
│  Go TCP→vsock proxy  │   host process (serve.go)
│  net.Listen("tcp")   │
└──────────┬──────────┘
           │  connectVsock(path, 10002)
           │  "CONNECT 10002\n" → "OK ...\n"
           ▼
┌─────────────────────┐
│  Python vsock→TCP    │   inside VM (vm_runner.py)
│  AF_VSOCK:10002      │   already running in snapshot
└──────────┬──────────┘
           │  TCP 127.0.0.1:10000
           ▼
┌─────────────────────┐
│  Deephaven server    │   inside VM, already running in snapshot
│  localhost:10000     │
└─────────────────────┘
```

Pure layer-4 proxy — no HTTP parsing. WebSockets, gRPC, and static assets all
work transparently. Each browser connection gets its own vsock stream.

## Changes

### 1. Add constant: `src/internal/vm/vm.go`

Add `HTTPProxyPort = 10002` alongside existing `FileServerPort = 10001`.

### 2. VM-side vsock→TCP proxy: `src/internal/vm/vm_runner.py`

Add a proxy thread started in `main()` before the snapshot is captured.
It will already be running when a VM is restored.

New functions:
- `_bridge(src, dst)` — copy bytes between two sockets until EOF/error
- `_handle_proxy_conn(vsock_conn, dh_port)` — accept one vsock connection,
  open TCP to `127.0.0.1:<dh_port>`, bridge bidirectionally in two threads
- `serve_http_proxy(dh_port)` — listen on `AF_VSOCK` port 10002, accept loop
  spawning `_handle_proxy_conn` per connection as daemon threads

In `main()`, start `serve_http_proxy(10000)` in a daemon thread before the
existing `serve_forever(session)` call. Since this runs before snapshot
capture, the listener is already active on restore — zero additional latency.

### 3. Host-side TCP→vsock proxy: `src/internal/vm/proxy_linux.go` (new file)

Follows the same structural pattern as `fileserver_linux.go`:

```go
type httpProxy struct {
    listener  net.Listener
    vsockPath string
    done      chan struct{}
    wg        sync.WaitGroup
}
```

- `StartHTTPProxy(listenAddr, vsockPath string) (*httpProxy, error)` —
  `net.Listen("tcp", listenAddr)`, start accept loop goroutine
- `acceptLoop()` — accept TCP connections, spawn `handleConn` goroutines,
  check `done` channel on errors (same pattern as fileserver)
- `handleConn(tcpConn)` — call `connectVsock(vsockPath, HTTPProxyPort)`,
  then two goroutines doing `io.Copy` in each direction; when either
  direction finishes, close both connections
- `Close()` — close done channel, close listener, `wg.Wait()`

### 4. Serve command: `src/internal/cmd/serve.go`

**New flag:** `serveVMFlag bool` registered as `--vm`.

**Updated help text:** Add `dh serve dashboard.py --vm` example.

**Branch in `runServe()`:** If `serveVMFlag`, call new `runServeVM()` and return.

**New function `runServeVM(cmd, args)`:**

1. Read script file (`os.ReadFile`)
2. Resolve version — quick path only (flag → env → latest snapshot),
   no venv/Java needed
3. Prereqs + snapshot check (parallel, same as `exec_vm_linux.go`)
4. `vm.RestoreFromSnapshot()` with defer destroy
5. `vm.StartFileServer(info.VsockPath, cwd)` for host file access
6. `vm.StartHTTPProxy(fmt.Sprintf("127.0.0.1:%d", servePortFlag), info.VsockPath)`
7. Execute user script: `vm.ExecuteViaVsock(info.VsockPath, vm.VsockPort, &vm.VsockRequest{Code: script})`
   - If script fails (exit_code != 0): print stderr/error, destroy VM, exit
8. Construct URL:
   - Base: `http://localhost:<port>`
   - With `--iframe`: `http://localhost:<port>/iframe/widget/?name=<name>`
9. Print "Server running at <url>" + "Press Ctrl+C to stop."
10. If `!--no-browser`: `screens.OpenBrowser(url)`
11. Signal handling: wait for SIGINT/SIGTERM, then cleanup via defers
12. `--jvm-args` is silently ignored (JVM in VM is pre-configured)

### 5. Build tag gating: `src/internal/cmd/serve_vm_linux.go` + `serve_vm_stub.go`

The VM restore and proxy code is Linux-only. Following the existing pattern
in `exec_vm_linux.go`:

- `serve_vm_linux.go` (`//go:build linux`): contains `runServeVM()`
- `serve_vm_stub.go` (`//go:build !linux`): returns
  `"--vm requires Linux with KVM"` error

The `--vm` flag itself stays in `serve.go` (cross-platform), and `runServe()`
calls the platform-gated function.

### 6. Snapshot rebuild required

Because `vm_runner.py` changes (adding the HTTP proxy thread), existing
snapshots won't have the proxy listener. Users must run `dh vm prepare`
again after updating. This is acceptable since `vm_runner.py` is baked
into the rootfs at snapshot creation time.

Add a note to the `--vm` flag help text: "Requires snapshot prepared with
this version or later."

## Flag Behavior Summary

| Flag | `dh serve` (local) | `dh serve --vm` |
|------|-------------------|-----------------|
| `--port N` | DH server binds to port N | Host proxy listens on port N |
| `--no-browser` | Skip browser open | Skip browser open |
| `--iframe NAME` | URL includes `/iframe/widget/?name=NAME` | Same |
| `--version V` | Resolve DH version V | Use snapshot for version V |
| `--jvm-args` | Passed to JVM | Ignored (VM JVM pre-configured) |

## Files Changed

| File | Change |
|------|--------|
| `src/internal/vm/vm.go` | Add `HTTPProxyPort` constant |
| `src/internal/vm/vm_runner.py` | Add HTTP proxy thread (~40 lines) |
| `src/internal/vm/proxy_linux.go` | **New** — host-side TCP→vsock proxy (~80 lines) |
| `src/internal/cmd/serve.go` | Add `--vm` flag, branch to platform function |
| `src/internal/cmd/serve_vm_linux.go` | **New** — `runServeVM()` implementation (~120 lines) |
| `src/internal/cmd/serve_vm_stub.go` | **New** — stub for non-Linux (~10 lines) |

## Verification

1. Rebuild rootfs + snapshot: `dh vm prepare -v` (picks up new vm_runner.py)
2. Run: `dh serve --vm dashboard.py --port 8080`
   - Confirm VM restores, script executes, proxy starts
   - Confirm browser opens to `http://localhost:8080`
   - Confirm web UI loads, WebSocket connects, tables render
3. Run: `dh serve --vm dashboard.py --no-browser --iframe my_widget`
   - Confirm browser does NOT open
   - Confirm printed URL includes `/iframe/widget/?name=my_widget`
4. Ctrl+C: confirm VM destroyed, proxy stopped, clean exit
5. `make vet` passes
6. Behaviour tests pass: `make test`
