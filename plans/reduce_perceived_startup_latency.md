# Plan: Reduce Perceived REPL Startup Latency

## Goal
Display an animated "Connecting..." message immediately on startup, then replace it with the REPL once the connection is ready.

## Current vs Proposed Behavior

| Current | Proposed |
|---------|----------|
| Blank terminal for 15+ seconds | "Connecting..." appears immediately |
| No feedback during startup | Animated dots show progress |
| Sudden REPL appearance | Smooth transition to REPL |

---

## Architecture

```
User runs `dh repl`
        │
        ▼
┌─────────────────────────────────┐
│ Terminal shows:                 │
│                                 │
│   Connecting...                 │  ← Animated dots (., .., ...)
│                                 │
│ Background: Server starting     │
└─────────────────────────────────┘
        │
        │ Connection ready
        │ Clear line, start REPL
        ▼
┌─────────────────────────────────┐
│ >>> _                           │  ← Normal REPL
│ ┌─────────────────────────────┐ │
│ │ ✓ Connected | Tables: 5    │ │  ← Toolbar
│ └─────────────────────────────┘ │
└─────────────────────────────────┘
```

---

## Implementation

### Files to Modify

#### 1. `/src/deephaven_cli/repl/console.py`

Add a simple animated "Connecting..." display that runs while the server starts in the background:

```python
import sys
import time
import threading

def _show_connecting_animation(stop_event: threading.Event) -> None:
    """Display animated 'Connecting...' until stop_event is set."""
    dots = 0
    while not stop_event.is_set():
        # Clear line and print with animated dots
        sys.stdout.write(f"\rConnecting{'.' * (dots % 4):<3}")
        sys.stdout.flush()
        dots += 1
        stop_event.wait(0.3)
    # Clear the connecting message
    sys.stdout.write("\r" + " " * 20 + "\r")
    sys.stdout.flush()
```

#### 2. Update `interact()` method in `DeephavenConsole`

```python
def interact(self) -> None:
    """Run the interactive REPL loop."""
    # Start connecting animation
    stop_animation = threading.Event()
    animation_thread = threading.Thread(
        target=_show_connecting_animation,
        args=(stop_animation,),
        daemon=True
    )
    animation_thread.start()

    try:
        # Do the actual connection (blocking)
        self._connect_to_server()
    finally:
        # Stop animation
        stop_animation.set()
        animation_thread.join(timeout=1.0)

    # Now start normal REPL
    self._run_repl_loop()
```

---

## Agent-Testable Plan

### Prerequisites
- Working `dh repl` command that currently has a startup delay

### Test Steps

1. **Verify immediate feedback**
   ```bash
   # Run the REPL and observe output
   timeout 5 dh repl 2>&1 | head -1
   # Expected: Should contain "Connecting" within first second
   ```

2. **Verify animation works**
   ```bash
   # Run for a few seconds to see dots animate
   timeout 3 dh repl 2>&1
   # Expected: Output should show "Connecting" with varying dots
   ```

3. **Verify transition to REPL**
   ```bash
   # Start REPL and wait for prompt
   echo "exit()" | dh repl 2>&1
   # Expected: Should show ">>>" prompt after connecting
   # Expected: "Connecting..." should NOT appear in final output (cleared)
   ```

4. **Verify Ctrl+C during connecting**
   ```bash
   # Send SIGINT shortly after starting
   timeout 2 dh repl 2>&1 || true
   # Expected: Should exit cleanly without traceback
   ```

### Success Criteria
- [ ] "Connecting..." appears within 100ms of running `dh repl`
- [ ] Dots animate (cycle through ., .., ...)
- [ ] Animation is cleared when connection completes
- [ ] REPL prompt appears and works normally after connection
- [ ] Ctrl+C during connecting exits cleanly

---

## Implementation Order

1. Add `_show_connecting_animation()` function to `console.py`
2. Refactor `interact()` to start animation before connecting
3. Ensure animation stops cleanly on connection or interrupt
4. Test manually
5. Run agent tests

---

## Summary

Simple approach: print animated "Connecting..." text while the server starts in a background operation, then clear it and show the normal REPL. No complex prompt_toolkit Application needed - just basic terminal output with `\r` to overwrite the line.
