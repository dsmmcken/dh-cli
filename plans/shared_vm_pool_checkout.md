# Plan: Share VM Pool Between `exec --vm` and `serve --vm`

## Context

`exec --vm` uses a pool daemon for ~20ms fast-path execution, while `serve --vm` always cold-restores (~700ms). Since the pool daemon already maintains pre-warmed VMs, `serve --vm` should be able to "check out" a warm VM from the pool to eliminate its startup latency.

The approach: add a `"checkout"` RPC to the pool that transfers VM ownership to the caller without destroying it. The serve command tries checkout first, falls back to cold restore — the same pattern exec already uses.

## Files to Modify

| File | Change |
|------|--------|
| `src/internal/vm/pool_protocol.go` | Add `CheckoutInfo` struct, add `Checkout` field to `PoolResponse` |
| `src/internal/vm/pool_linux.go` | Add `handleCheckout` method + dispatch case |
| `src/internal/vm/pool_client.go` | Add `PoolCheckout()` and `DestroyCheckedOutVM()` |
| `src/internal/cmd/serve_vm_linux.go` | Try pool checkout before cold restore |

## Step 1: Protocol Types (`pool_protocol.go`)

Add `CheckoutInfo` struct with fields needed for cross-process VM ownership:
- `InstanceID`, `PID`, `VsockPath`, `SnapVsockPath`, `InstanceDir`, `Version`

Add `Checkout *CheckoutInfo` field to `PoolResponse` (for `"checkout_result"` type).

## Step 2: Pool Handler (`pool_linux.go`)

Add `"checkout"` case to `handleConnection` switch (line 253).

New `handleCheckout` method:
1. Touch `lastReq` (idle timeout tracking)
2. Non-blocking dequeue from `p.ready` (fail fast if empty, same as `handleExec`)
3. Close `pvm.uffdCloser` (UFFD is already done before VM enters ready queue)
4. Build `CheckoutInfo` from `pvm.info` fields + `p.paths.InstanceDir(pvm.instanceID)`
5. Send `PoolResponse{Type: "checkout_result", Checkout: &info}`
6. Do NOT call `destroyPoolVM` — ownership transfers to caller
7. Backfill triggers naturally (ready channel has one fewer VM)

The `*firecracker.Machine` handle becomes unreferenced and is GC'd. The actual Firecracker process continues running independently — the caller owns it by PID.

## Step 3: Client Functions (`pool_client.go`)

**`PoolCheckout()`**: Sends `PoolRequest{Type: "checkout"}` via `poolRPC`, returns `*CheckoutInfo`.

**`DestroyCheckedOutVM(info *CheckoutInfo)`**: Cross-process cleanup:
1. `syscall.Kill(info.PID, syscall.SIGKILL)` — safe because VM is ephemeral with read-only disk
2. Poll `Signal(0)` for up to 500ms to confirm process death (can't use `Wait()` — not our child process)
3. `os.RemoveAll(info.InstanceDir)`

This matches the existing `DestroyInstance` semantics (StopVMM + RemoveAll) but works cross-process.

## Step 4: Serve Integration (`serve_vm_linux.go`)

Restructure `runServeVM` to try pool checkout first:

```
1. Read script, resolve version (unchanged)
2. NEW: If DH_VM_POOL != "0", try vm.PoolCheckout()
   - On success: verify version matches (if explicit), build InstanceInfo from CheckoutInfo
   - On version mismatch: DestroyCheckedOutVM, fall through
   - On failure: fall through to cold restore
3. If no checkout: existing cold restore path (prereqs, snapshot check, RestoreFromSnapshot)
4. Both paths produce info with VsockPath + SnapVsockPath
5. File server, HTTP proxy, script exec, browser, signal wait (unchanged)
6. Cleanup via defer: DestroyCheckedOutVM (pool path) or DestroyInstance (cold path)
```

Unlike exec, serve does NOT auto-start a pool daemon on miss — serve's cold restore latency (~700ms) is acceptable for an interactive session.

## File Server Contention Note

The file server binds at `{snapVsockPath}_10001` (per-snapshot-version, not per-instance). A long-lived serve session holds this socket. Concurrent exec pool VMs log a warning but still execute correctly — file server is only needed for guest→host workspace file access. This is the same as today's behavior with concurrent pool execs.

## Verification

1. **`make vet`** — all source passes vet
2. **`make test`** — unit + behaviour tests pass (existing tests unaffected since pool is not available in test environments)
3. **Build succeeds**: `CGO_ENABLED=0 make build`
4. **Manual inspection**: Read through the diff to verify:
   - Checkout path produces identical `info` shape as cold restore path
   - Cleanup is correct for both paths (defer ordering)
   - Version mismatch destroys the checked-out VM before falling back
   - No leaked VMs on any error path
