# Plan: Add VM Management to Interactive TUI

## Context

The `dh vm` CLI offers snapshot preparation, status checking, artifact cleanup, and pool daemon management — but none of it is accessible from the interactive TUI (`dh` with no args / `dh setup`). Users must drop to the CLI for all VM operations. This plan adds a "VM management" screen to the TUI, following existing patterns.

## Design

### Single Screen Approach

One new screen (`vmmanage.go`) that mirrors `dh vm status` — showing prerequisites, snapshots, and pool status in sections — plus action keys for prepare, clean, start/stop pool. This matches the existing screen complexity (cf. `servers.go`, `versions.go`) and avoids over-engineering with sub-screens.

A second file (`vmprepare.go`) handles the long-running prepare operation, following the `installprogress.go` pattern.

### Screen Layout

```
  VM Management

  Prerequisites
    ✓ KVM accessible
    ✓ Docker available
    ✓ Firecracker binary

  Snapshots
  > 0.36.3: ready
    0.36.2: incomplete

  Pool daemon: running (pid=12345)
    Version: 0.36.3  Ready: 1/1  Idle: 42s

  ↑/k up  ↓/j down  p prepare  d delete  s start pool  x stop pool  ? more  esc back
```

- **Cursor navigation** operates on the snapshot list only
- **Pool section** is informational (no cursor); actions via keys

### Async Data Loading

- `Init()` fires `tea.Batch(loadVMStatus(), pollVMTick())`
- `loadVMStatus()` runs in a goroutine: calls `vm.CheckPrerequisites()`, reads snapshot dir, calls `vm.PoolProbe()` + `vm.PoolCommand(status)` if running
- `pollVMTick()` ticks every 5s for periodic refresh
- All results arrive via a single `VMStatusLoadedMsg`

### Actions

| Key | Action | Implementation |
|-----|--------|----------------|
| `p` | Prepare snapshot | Push `VMPrepareScreen` (version = resolved default) |
| `d` | Delete selected snapshot | `os.RemoveAll(snapshotDir)` + `os.Remove(rootfs)` inline, then refresh |
| `s` | Start pool daemon | `exec.Command(exePath, "vm", "pool", "start", "--background")` in goroutine, then refresh |
| `x` | Stop pool daemon | `vm.PoolCommand(&vm.PoolRequest{Type: "stop"})` inline, then refresh |
| `+` | Scale pool +1 | `vm.PoolCommand(&vm.PoolRequest{Type: "scale", TargetSize: current+1})` |
| `-` | Scale pool -1 | `vm.PoolCommand(&vm.PoolRequest{Type: "scale", TargetSize: max(1, current-1)})` |

### VMPrepareScreen (`vmprepare.go`)

Follows `installprogress.go` pattern:
- Shows step-by-step progress with a progress bar
- Runs the prepare steps in a goroutine, sending `vmPrepareStepMsg{step, message}` updates
- Steps: "Ensuring Firecracker binary" → "Ensuring kernel" → "Checking prerequisites" → "Building rootfs" → "Creating snapshot"
- On success: shows completion message, auto-pops on keypress
- On error: shows error, waits for keypress

### Non-Linux Handling

`vm.CheckPrerequisites()` returns "not supported" errors on non-Linux. The screen shows these and disables action keys (prepare, pool start).

## Files to Create/Modify

### New files
- `src/internal/tui/screens/vmmanage.go` — VM management screen
- `src/internal/tui/screens/vmprepare.go` — VM prepare progress screen

### Modified files
- `src/internal/tui/screens/mainmenu.go` — Add "VM management" menu item (index 5) after "Configuration"
- `unit_tests/tui_test.go` — Add tests for VM screen navigation and key handling

### Existing code to reuse
- `vm.NewVMPaths(dhHome)` — canonical paths (`src/internal/vm/vm.go:67`)
- `vm.CheckPrerequisites(paths)` — prerequisite checks (`src/internal/vm/prereqs_linux.go:40`)
- `vm.FormatPrereqErrors(errs)` — format errors (`src/internal/vm/prereqs_linux.go:137`)
- `vm.CheckSnapshot(paths, version)` — validate snapshot (`src/internal/vm/prereqs_linux.go:153`)
- `vm.PoolProbe()` — check if pool running (`src/internal/vm/pool_client.go:27`)
- `vm.PoolCommand(req)` — pool RPC (`src/internal/vm/pool_client.go:83`)
- `vm.EnsureFirecracker/EnsureKernel/EnsureRootfs/BootAndSnapshot` — prepare steps
- `config.ResolveVersion()` — resolve default version (`src/internal/config/`)
- `pushScreen()/popScreen()` — navigation helpers (`src/internal/tui/screens/helpers.go`)
- `colorPrimary/colorDim/colorSuccess/colorError` — color palette (`src/internal/tui/screens/colors.go`)

## Verification

1. `CGO_ENABLED=0 make build` — compiles cleanly
2. `CGO_ENABLED=0 make vet` — no vet errors
3. `CGO_ENABLED=0 make test` — all tests pass
4. Main menu shows "VM management" as 6th item
5. VM screen loads and shows prerequisites + empty snapshot list (no VMs in sandbox)
6. Pool section shows "not running" (no pool in sandbox)
7. Action keys are wired correctly (prepare pushes progress screen, etc.)
