# Phase 5: Packaging and Distribution

**Depends on:** All previous phases (working binary)
**This is the final phase.**

## Goal

Package the `dh` Go binary as a Python wheel via `go-to-wheel` so it can be installed with `uv tool install`.

## Deliverables

### Build script

A `Makefile` or shell script in `go_src/` that:

1. Builds the binary for the current platform:
   ```bash
   CGO_ENABLED=0 go build -ldflags="-X main.version=$(VERSION)" -o dhg ./cmd/dhg
   ```

2. Cross-compiles for all platforms (for release):
   ```bash
   CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ...
   CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ...
   CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build ...
   CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ...
   CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ...
   CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build ...
   ```

### go-to-wheel packaging

```bash
pip install go-to-wheel
go-to-wheel go_src/ \
  --name dh-cli \
  --set-version-var main.version \
  --version 0.1.0 \
  --readme README.md \
  --entry-point dhg
```

This produces platform-specific wheels in `dist/`:
```
dist/
  dh_cli-0.1.0-py3-none-manylinux_2_17_x86_64.whl
  dh_cli-0.1.0-py3-none-manylinux_2_17_aarch64.whl
  dh_cli-0.1.0-py3-none-macosx_11_0_x86_64.whl
  dh_cli-0.1.0-py3-none-macosx_11_0_arm64.whl
  dh_cli-0.1.0-py3-none-win_amd64.whl
  dh_cli-0.1.0-py3-none-win_arm64.whl
```

### Installation test

```bash
# Install from local wheel
uv tool install dist/dh_cli-0.1.0-*.whl

# Verify
dh --version
dh doctor --json
dh java --json
```

## Files to create

```
go_src/
  Makefile                 # build, build-all, package, clean targets
```

## Verification

```bash
cd go_src
make build                 # local binary
make package               # go-to-wheel → dist/
uv tool install dist/dh_cli-*.whl
which dhg                  # on PATH
dh --version              # works
dh doctor                 # works
uv tool uninstall dh-cli  # clean up
```
