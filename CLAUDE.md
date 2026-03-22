# Project Instructions for Claude Code

## Project Structure

- **`src/`** — Main Go CLI source (module: `github.com/dsmmcken/dh-cli/src`)
- **`unit_tests/`** — Unit tests (uses `replace ../src` directive)
- **`behaviour_tests/`** — Black-box CLI/TUI tests (testscript .txtar files)
- **`plans/`** — Design docs / implementation plans

## Build & Install

### Standard (local machine with `uvx` available)

```bash
make install-local
```

This builds a wheel for the current platform and installs it via `uv tool install` to `~/.local/bin/dh`.

### Sandbox / CI fallback (when Python `os.getcwd()` fails)

If the project lives on a virtiofs or FUSE mount, Python's `os.getcwd()` will fail, breaking `uv`, `go-to-wheel`, and `make install-local`. Use direct Go build instead:

```bash
CGO_ENABLED=0 make build && cp dh ~/.local/bin/dh
```

**How to tell:** If you see `Current directory does not exist` from uv/go-to-wheel, or `FileNotFoundError` from Python's `os.getcwd()`, use the fallback build.

Do **not** use `sudo cp`, `make install`, or `go install` directly.

### Running `dh exec` in the sandbox

The sandbox has no Java and no `dh install`, so the only way to run Deephaven code is via the VM snapshot path. Build the binary first, then use `DH_HOME` to point at the persisted workspace artifacts.

```bash
# Build (needed each sandbox session — binary is ephemeral)
CGO_ENABLED=0 make build && cp dh ~/.local/bin/dh

# Run code (auto-detects latest snapshot, no --version needed)
DH_HOME=/workspace/.dh dh exec --vm -c 'print("hello world")'
DH_HOME=/workspace/.dh dh exec --vm script.py
echo 'print("hi")' | DH_HOME=/workspace/.dh dh exec --vm -
```

If no snapshot exists yet, build one first (requires Docker):

```bash
DH_HOME=/workspace/.dh dh vm prepare -v    # ~2-5 min, artifacts persist in /workspace/.dh/
```

### Fakeroot workaround for `dh vm prepare`

`dh vm prepare` uses fakeroot to build the ext4 rootfs with correct (root) file ownership. In this sandbox, fakeroot-tcp hangs indefinitely. Workaround: build the rootfs manually by extracting the Docker image and using `mke2fs -d` directly (files will be owned by UID 1000 instead of root, but init runs as root in the VM so mounts still work):

```bash
# 1. Docker build (vm prepare does this, or reuse cached image)
#    The image name is dh-vm-<version>, e.g. dh-vm-41.3

# 2. Export and extract
docker rm -f dh-vm-export-tmp 2>/dev/null
docker create --name dh-vm-export-tmp dh-vm-41.3
docker export -o /tmp/rootfs.tar dh-vm-export-tmp
docker rm -f dh-vm-export-tmp

mkdir -p /tmp/dh-rootfs && tar xf /tmp/rootfs.tar -C /tmp/dh-rootfs

# 3. Fix merged-usr symlinks (Docker export breaks them)
for name in lib lib64 bin sbin; do
    top="/tmp/dh-rootfs/$name"
    usr="/tmp/dh-rootfs/usr/$name"
    if [ -d "$top" ] && [ ! -L "$top" ]; then
        mkdir -p "$usr"
        cp -a --no-clobber "$top/." "$usr/" 2>/dev/null || true
        rm -rf "$top" && ln -s "usr/$name" "$top"
    fi
done
rm -f /tmp/dh-rootfs/sbin/init && ln -s /sbin/init.sh /tmp/dh-rootfs/sbin/init

# 4. Create ext4 and write sources hash
ROOTFS=/workspace/.dh/vm/rootfs/deephaven-41.3.ext4
mkdir -p /workspace/.dh/vm/rootfs
mke2fs -t ext4 -d /tmp/dh-rootfs -F -b 4096 "$ROOTFS" 3G

# 5. Get the sources hash from the Go binary (so vm prepare won't rebuild)
cd src && cat > internal/vm/h_test.go <<'EOF'
package vm
import ("fmt";"testing")
func TestH(t *testing.T) { fmt.Printf("H=%s\n", rootfsSourcesHash()) }
EOF
H=$(CGO_ENABLED=0 go test -run TestH -v ./internal/vm/ 2>&1 | grep -o 'H=[a-f0-9]*' | cut -d= -f2)
rm internal/vm/h_test.go
echo -n "$H" > "${ROOTFS}.srchash"

# 6. Now vm prepare will skip rootfs build and go straight to boot+snapshot
DH_HOME=/workspace/.dh dh vm prepare -v
```

### UFFD for fast VM snapshot restore

VM snapshot restore is **10-30x faster** with UFFD (userfaultfd) enabled. Without it, every memory page triggers a synchronous disk read on first access. The sandbox needs both the sysctl AND seccomp permission:

- **sysctl** (host-level): `sudo sysctl -w vm.unprivileged_userfaultfd=1`
- **seccomp** (container-level): the `userfaultfd` syscall must be in the allowlist

Check with: `dh doctor | grep UFFD`

## Go Toolchain Setup

Current Go version: **1.26.0**

If the sandbox doesn't have the right Go version, install via `golang.org/dl`:

```bash
go install golang.org/dl/go1.26.0@latest
~/go/bin/go1.26.0 download
```

Then persist in `/etc/sandbox-persistent.sh`:

```bash
export PATH="$HOME/go/bin:$HOME/sdk/go1.26.0/bin:$PATH"
export GOTOOLCHAIN=go1.26.0
```

Note: `CGO_ENABLED=0` is required — the sandbox has no gcc.

## Running Tests

```bash
make test    # unit + behaviour tests
make vet     # vet all source
```

## Render Benchmarking

To benchmark render performance after changing `vm_runner.py`, render JS files, or `warmup.mjs`:

1. **Build**: `CGO_ENABLED=0 make build && cp dh ~/.local/bin/dh`
2. **Rebuild snapshot** (required — these files are baked into the rootfs via `//go:embed`):
   - `vm_runner.py`, `warmup.mjs`, `JsApiLoader.mjs`, `index.mjs` etc. are all embedded in the Go binary and copied into the Docker image during `vm prepare`
   - The rootfs sources hash (`rootfsSourcesHash()`) determines whether `vm prepare` rebuilds the rootfs. It includes `vm_runner.py` but NOT render JS files. To force rebuild after JS-only changes: `rm /workspace/.dh/vm/rootfs/*.srchash`
   - Delete old snapshot too: `rm -rf /workspace/.dh/vm/snapshots/<version>`
   - Then use the fakeroot workaround (see above) to rebuild rootfs + `DH_HOME=/workspace/.dh dh vm prepare -v` for the snapshot
3. **Benchmark**: `DH_HOME=/workspace/.dh ./scripts/bench-render.sh --runs 3`
   - Runs 1 cold + N pool renders, validates snapshot output, prints summary table
   - Cold renders use `DH_VM_POOL=0`; pool renders auto-start a pool of 1

**Baseline (2026-03-22)**: cold=10021ms, pool avg=4510ms (script~2936ms, render~1127ms)

## Plan File Location

When creating plans in plan mode, always save them to the `plans/` directory in this project.

**IMPORTANT:** Do NOT use the default three-random-words naming convention. Instead, always name plan files with a clear, descriptive name that reflects the plan's purpose (e.g., `plans/add_user_authentication.md`, `plans/refactor_database_layer.md`, `plans/fix_video_encoding_bug.md`).

## Committing Plans

When committing implementation work, include any relevant plan files from `plans/` in the commit. Plans serve as documentation of the design decisions and should be versioned alongside the code they describe.
