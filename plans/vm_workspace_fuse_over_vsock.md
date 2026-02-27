# VM Workspace Access: LD_PRELOAD Transparent File Proxy

## Context

When running `dhg exec --vm`, user code inside the Firecracker microVM has no access to host files. Scripts referencing local data (e.g., `read_csv("./tests/sample_data.csv")`) fail with `NoSuchFileException`. This plan implements a transparent, truly lazy file access mechanism using an LD_PRELOAD shared library that intercepts libc file operations and proxies them to a host file server over vsock.

**No kernel changes required** — the Firecracker CI kernel (`CONFIG_FUSE_FS is not set`) works as-is.

**Baseline benchmarks:**
- `basic_script.py` (no files): ~10.6s, 27ms restore — **must not regress**
- `basic_import.py` (reads CSV): fails — **must work after this change**

## Architecture

```
Host (dhg process)                    Guest VM (snapshot-restored)
==================                    ============================

StartFileServer()                     LD_PRELOAD=/opt/libworkspace.so
  Listen on {vsockPath}_10001          (loaded into all processes at boot)
  Serve files from host CWD            (dormant until CWD=/workspace)

RestoreFromSnapshot()                 All processes resume
                                       Runner: os.chdir('/workspace')

ExecuteViaVsock(code)                 Code: read_csv("./tests/sample.csv")
                                        ↓ resolves to /workspace/tests/sample.csv
                                        ↓ glibc openat() intercepted
                                        ↓ libworkspace: is_workspace_path? YES
                                        ↓ lazy vsock connect to host CID=2:10001
                                        ↓ stat → 0.2ms, read → 0.3ms
                                        ↓ write to /tmp/.wscache/tests/sample.csv
                                        ↓ open cached file, return fd
                                      Subsequent access: local cache, 0ms
```

**Zero-impact guarantee:** The interceptor only activates for `/workspace/*` paths. `basic_script.py` never touches `/workspace`, so the overhead is one `strncmp` per libc file call (~0 ns).

## Component 1: Host-Side File Server

**New file:** `src/internal/vm/fileserver_linux.go` (~200 lines)

A Go goroutine serving file operations from host CWD over Firecracker's guest→host vsock mechanism. Firecracker forwards guest connections to `CID=2:port` to a Unix socket at `{vsockPath}_{port}` on the host.

```go
func StartFileServer(vsockPath string, rootDir string) (io.Closer, error) {
    listenPath := vsockPath + "_10001"   // Firecracker guest→host convention
    listener, err := net.Listen("unix", listenPath)
    // ...accept connections, handle binary protocol requests
}
```

### Binary Wire Protocol

All messages are length-prefixed: `[4-byte big-endian length][payload]`

**Requests (guest → host):**
```
STAT:    [op=1][2-byte path_len][path_bytes]
READ:    [op=2][2-byte path_len][path_bytes][8-byte offset][4-byte length]
READDIR: [op=3][2-byte path_len][path_bytes]
```

**Responses (host → guest):**
```
STAT OK:    [status=0][4-byte mode][8-byte size][8-byte mtime][1-byte is_dir]
READ OK:    [status=0][4-byte bytes_read][raw file bytes]  ← no base64!
READDIR OK: [status=0][2-byte count][{2-byte name_len, name, 1-byte is_dir}...]
ERROR:      [status=1 (ENOENT) or 2 (EIO)]
```

Binary protocol eliminates JSON parsing and base64 encoding in C — raw bytes flow over the wire.

**Path validation:** `filepath.Clean` + prefix check ensures no directory traversal outside `rootDir`.

## Component 2: Guest-Side LD_PRELOAD Library

**New file:** `src/internal/vm/libworkspace.c` (~700 lines, embedded via `//go:embed`)

A C shared library that intercepts three glibc functions:
- `openat` — all `open()` calls route through this on modern Linux
- `fstatat` — all `stat()/lstat()` calls route through this
- `faccessat` — all `access()` calls route through this

### How It Works

1. **Path check** (`strncmp` on 11 bytes): If path doesn't start with `/workspace/`, call real libc function immediately. **This is the only overhead for non-workspace paths.**

2. **Cache check** (`stat("/tmp/.wscache/<relpath>")`): If cached, operate on cache file.

3. **Remote fetch** (vsock to host): If not cached, connect to host file server, download file in 1MB chunks, write to cache atomically (`mkstemp` + `rename`).

4. **Return cached fd**: Call real `openat` on the cache file.

### Key Design Details

**Re-entrancy prevention:** Thread-local `__thread int in_hook` flag. Set before any internal file I/O (cache writes), cleared after. When set, all intercepted calls pass through to real libc.

**Thread safety:** Single persistent vsock connection protected by `pthread_mutex_t`. Lazy-connect on first workspace access. Reconnect on error.

**Natural dormancy during boot:** At VM boot time, CWD is `/`. No process accesses `/workspace/*` paths. The library is loaded but never triggers. After snapshot restore, the runner does `os.chdir('/workspace')`, and user code's relative paths resolve to `/workspace/*`.

**Snapshot lifecycle:** At snapshot time, `vsock_fd = -1` (no host server exists during prepare). After restore, first workspace access triggers lazy connect to the host file server that was started before restore.

**Connection per approach:** Persistent connection (not per-request). Avoids ~0.2ms connect overhead per operation. Mutex serialization is fine — workspace file access is infrequent.

### Interceptor Pseudocode

```c
int openat(int dirfd, const char *pathname, int flags, ...) {
    if (in_hook) return real_openat(dirfd, pathname, flags, mode);

    char resolved[PATH_MAX];
    resolve_path(dirfd, pathname, resolved);     // handle CWD, ./, ../

    const char *rel;
    if (!is_workspace_path(resolved, &rel))      // strncmp "/workspace/"
        return real_openat(dirfd, pathname, flags, mode);  // fast path

    if (flags & (O_WRONLY | O_RDWR))             // read-only workspace
        { errno = EROFS; return -1; }

    in_hook = 1;
    int rc = ensure_cached_file(rel);            // fetch via vsock if needed
    in_hook = 0;

    if (rc < 0) { errno = ENOENT; return -1; }

    char cache_path[PATH_MAX];
    cache_path_for(rel, cache_path);
    in_hook = 1;
    int fd = real_openat(AT_FDCWD, cache_path, flags & ~O_CREAT, 0);
    in_hook = 0;
    return fd;
}
```

### Caching Strategy

- **Cache location:** `/tmp/.wscache/<relative_path>`
- **Write-once:** No invalidation. VM is ephemeral (one exec, then destroyed).
- **Atomic writes:** `mkstemp` → write → `rename` (prevents partial reads on race).
- **Stat optimization:** If file is already cached, `fstatat` runs on the local cache file (~0μs). Otherwise, one vsock round-trip (~200μs).

## Component 3: Rootfs Changes

**Modified file:** `src/internal/vm/rootfs_linux.go`

### Dockerfile Additions

```dockerfile
# Build libworkspace.so (LD_PRELOAD library for transparent workspace access)
RUN apt-get update && apt-get install -y --no-install-recommends gcc libc6-dev
COPY libworkspace.c /tmp/libworkspace.c
RUN gcc -shared -fPIC -O2 -o /opt/libworkspace.so /tmp/libworkspace.c -ldl -lpthread \
    && strip /opt/libworkspace.so \
    && rm /tmp/libworkspace.c \
    && apt-get purge -y gcc libc6-dev && apt-get autoremove -y
```

### Init Script Additions

```bash
# Transparent workspace file access
export LD_PRELOAD=/opt/libworkspace.so
mkdir -p /workspace
```

Added **before** all process starts, so the JVM, Python, and runner daemon all inherit LD_PRELOAD. The library is naturally dormant during boot (no `/workspace/*` access occurs).

## Component 4: Integration

### `src/internal/exec/exec_vm_linux.go`

Start file server before VM restore:
```go
cwd, _ := os.Getwd()
vsockPath := filepath.Join(vmPaths.SnapshotDirForVersion(version), "vsock.sock")
fileServer, err := vm.StartFileServer(vsockPath, cwd)
if err != nil && cfg.Verbose {
    fmt.Fprintf(cfg.Stderr, "Warning: file server: %v\n", err)
}
if fileServer != nil {
    defer fileServer.Close()
}
```

### `src/internal/vm/vm_runner.py`

In `handle_request()`, before building wrapper:
```python
os.chdir('/workspace')  # relative paths → /workspace/* → intercepted by libworkspace
```

### `src/internal/vm/vm.go`

Add constant: `FileServerPort = 10001`

## Files Summary

| File | Change | Lines |
|------|--------|-------|
| `src/internal/vm/libworkspace.c` | **New.** LD_PRELOAD C library | ~700 |
| `src/internal/vm/fileserver_linux.go` | **New.** Host-side binary file server | ~200 |
| `src/internal/vm/rootfs_linux.go` | Embed .c, Dockerfile gcc step, init LD_PRELOAD | ~30 changed |
| `src/internal/vm/vm_runner.py` | Add `os.chdir('/workspace')` | ~2 changed |
| `src/internal/vm/vm.go` | Add `FileServerPort` constant | ~1 changed |
| `src/internal/exec/exec_vm_linux.go` | Start file server before restore | ~15 added |
| `src/internal/vm/machine_linux.go` | (reference only — vsock patterns) | unchanged |

## Performance

| Scenario | Impact |
|----------|--------|
| `basic_script.py` (no workspace access) | **0ms** — `strncmp` per libc call, never matches |
| `basic_import.py` (193-byte CSV) | **~0.7ms** — connect 0.2ms + stat 0.2ms + read 0.3ms, then cached |
| 1MB data file | **~2ms** — 1 stat + 1 read chunk |
| 100MB data file | **~50ms** — 1 stat + ~100 read chunks (1MB each) |
| Repeat access to same file | **0ms** — served from `/tmp/.wscache/` |

## Verification

```bash
# 1. Requires re-preparing the snapshot (new rootfs with libworkspace.so)
DHG_HOME=/workspace/.dhg dhg vm prepare -v

# 2. Build dhg
CGO_ENABLED=0 make build && cp dhg ~/.local/bin/dhg

# 3. Regression — must not regress
time DHG_HOME=/workspace/.dhg dhg exec --vm -v ./tests/basic_script.py

# 4. Feature — must work (currently fails)
time DHG_HOME=/workspace/.dhg dhg exec --vm -v ./tests/basic_import.py

# 5. Unit tests
make test
```

## Risks

| Risk | Mitigation |
|------|-----------|
| JVM bypasses glibc for file I/O | Doesn't happen on OpenJDK 17 — all file I/O goes through glibc |
| Re-entrancy bug causes hang | `__thread in_hook` TLS guard; tested with simple C test harness |
| vsock timeout during fetch | `SO_RCVTIMEO`/`SO_SNDTIMEO` = 5s; returns ENOENT on timeout |
| Cache fills /tmp | VM is ephemeral; /tmp has ~2GB space; typical workspace < 100MB |
| Old snapshot without libworkspace.so | File server starts but nobody connects — zero impact |
| New binary without file server + new snapshot | libworkspace.so fails to connect, returns ENOENT — same as today |
