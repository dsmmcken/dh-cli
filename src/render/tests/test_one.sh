#!/bin/bash
# Test a single component script against dh-render.
# Usage: ./test_one.sh tests/components/test_button.py button_widget
#
# Starts a DH server with the script, runs dh-render, reports result.
set -euo pipefail

SCRIPT="$1"
WIDGET="$2"
PORT="${3:-10000}"
DH_RENDER="$(cd "$(dirname "$0")/../dh-render-test" && pwd)/bin/dh-render.mjs"

echo "=== Testing: $SCRIPT ($WIDGET) ==="

# Kill anything on the port
fuser -k ${PORT}/tcp 2>/dev/null || true
sleep 1

# Start server
cd /workspace
dh serve "$SCRIPT" --port $PORT --no-browser &>/tmp/dh-test-server.log &
SERVER_PID=$!

# Wait for server
for i in $(seq 1 30); do
    if curl -s -o /dev/null -w "%{http_code}" "http://localhost:${PORT}/jsapi/dh-core.js" 2>/dev/null | grep -q 200; then
        echo "  Server ready (${i}s)"
        break
    fi
    sleep 1
    if [ $i -eq 30 ]; then
        echo "  FAIL: Server didn't start"
        # Check if it picked a different port
        ACTUAL_PORT=$(grep -oP 'http://localhost:\K[0-9]+' /tmp/dh-test-server.log 2>/dev/null | tail -1)
        if [ -n "$ACTUAL_PORT" ] && [ "$ACTUAL_PORT" != "$PORT" ]; then
            echo "  Server used port $ACTUAL_PORT instead of $PORT"
            PORT=$ACTUAL_PORT
        else
            kill $SERVER_PID 2>/dev/null
            exit 1
        fi
    fi
done

# Clean daemon state
kill $(ps aux | grep daemon.mjs | grep -v grep | awk '{print $2}') 2>/dev/null || true
rm -f /tmp/dh-render-*.sock /tmp/dh-render-state.json

# Open connection
cd /workspace/dh-render-test
if ! node "$DH_RENDER" open "http://localhost:${PORT}" 2>&1; then
    echo "  FAIL: Could not open connection"
    kill $SERVER_PID 2>/dev/null
    exit 1
fi

# Render
OUTPUT=$(node "$DH_RENDER" render "$WIDGET" 2>&1) || {
    echo "  FAIL: Render failed"
    echo "  $OUTPUT"
    node "$DH_RENDER" close 2>/dev/null || true
    kill $SERVER_PID 2>/dev/null
    exit 1
}
echo "  Rendered OK"
echo "$OUTPUT" | head -20

# Snapshot
SNAP=$(node "$DH_RENDER" snapshot 2>&1)
echo "  Snapshot:"
echo "$SNAP" | sed 's/^/    /'

# Close
node "$DH_RENDER" close 2>/dev/null || true
kill $SERVER_PID 2>/dev/null || true
echo "  PASS"
