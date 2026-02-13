# Fix: REPL spams numbers when server is killed externally

## Context

When a server is killed from the manage servers screen (or externally), the REPL doesn't detect the disconnection. pydeephaven's background keep-alive daemon thread keeps retrying token refreshes, producing gRPC errors that corrupt the Textual TUI display — showing as repeating numbers that can't be stopped.

The root cause: `Session._keep_alive()` runs on a daemon `threading.Timer`, checking `self.is_connected` at the top. When the server dies, it retries with backoff, logging warnings and eventually raising `DHError` in the daemon thread. The gRPC C-core library also writes error output directly to stderr, bypassing Python. The Textual app has no mechanism to detect server death.

## Changes

### 1. `src/deephaven_cli/client.py` — Add `force_disconnect()` + harden `close()`

- Add `force_disconnect()`: sets `session.is_connected = False` (stops keep-alive timer from rescheduling), cancels pending timer, closes gRPC channel/flight client without sending any RPCs, sets `_session = None`
- Add `is_connected` property for easy checking
- Harden `close()` to catch exceptions when server is already dead, falling back to force-cleanup

### 2. `src/deephaven_cli/repl/app.py` — Add health check + graceful exit

- Add `_disconnected` flag and `_health_timer` to `__init__`
- Start `set_interval(3.0, _schedule_health_check)` in `on_mount()`
- `_schedule_health_check()`: spawns worker thread with `exclusive=True, group="health_check"`
- `_do_health_check()`: tries `config_service.get_configuration_constants()`, calls `_handle_disconnection` on failure
- `_handle_disconnection()`: stops health timer, calls `client.force_disconnect()` immediately, shows notification, exits after 2s delay
- Add `_disconnected` guards to `_execute_code()` and `_show_variable()`

### 3. No changes needed to `cli.py`

`DeephavenClient.close()` already checks `if self._session:`, so after `force_disconnect()` sets it to `None`, the context manager `__exit__` is a safe no-op.

## Verification

1. Start `dh repl`, kill server from another terminal → should see "Server disconnected" notification, clean exit within ~5s, no garbled output
2. Normal `exit()` and Ctrl+C still work when server is alive
3. Kill server mid-command-execution → no crash, clean disconnect
