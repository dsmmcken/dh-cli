#!/bin/bash
# Benchmark dh render --vm with the iris dashboard script.
# Runs cold and pool renders, verifies full snapshot output, prints summary.
#
# Usage:
#   ./scripts/bench-render.sh              # 1 cold + 3 pool (default)
#   ./scripts/bench-render.sh --runs 5     # 1 cold + 5 pool
#   DH_HOME=/workspace/.dh ./scripts/bench-render.sh

set -euo pipefail

SCRIPT="src/render/tests/scripts/complex/test_iris_dashboard.py"
RUNS=3
DH_HOME="${DH_HOME:-$HOME/.dh}"
export DH_HOME

while [[ $# -gt 0 ]]; do
  case "$1" in
    --runs) RUNS="$2"; shift 2 ;;
    --script) SCRIPT="$2"; shift 2 ;;
    *) echo "Usage: $0 [--runs N] [--script PATH]"; exit 1 ;;
  esac
done

# Resolve script path relative to repo root
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
if [[ ! "$SCRIPT" = /* ]]; then
  SCRIPT="$REPO_ROOT/$SCRIPT"
fi

if [[ ! -f "$SCRIPT" ]]; then
  echo "Script not found: $SCRIPT" >&2
  exit 1
fi

# --- Snapshot validation ---
# Key lines that must appear in the iris dashboard output.
# Adjust if using a different script.
EXPECTED_PATTERNS=(
  '\[dashboard\]'
  '\[UITable\].*150 rows'
  '\[Figure\] scatter'
  '\[picker\].*Current Species'
  '\[illustrated_message\]'
)

validate_output() {
  local output="$1"
  local label="$2"
  for pat in "${EXPECTED_PATTERNS[@]}"; do
    if ! echo "$output" | grep -qE "$pat"; then
      echo "FAIL: $label missing expected pattern: $pat" >&2
      echo "$output" >&2
      return 1
    fi
  done
  return 0
}

# Extract a timing value from verbose output.
# Matches "[timing] key ...: 1234ms" where key is a prefix (e.g. "session.open").
extract_timing() {
  echo "$1" | grep -oP "\[timing\] ${2}[^:]*: \K[0-9]+" || echo "-"
}

# --- Setup ---
echo "Benchmark: $(basename "$SCRIPT")"
echo "DH_HOME:   $DH_HOME"
echo "Runs:      1 cold + $RUNS pool"
echo ""

# Kill stale state
pkill -9 firecracker 2>/dev/null || true
dh vm pool stop 2>/dev/null || true
sleep 1
rm -rf "$DH_HOME/vm/run/"* 2>/dev/null || true

# --- Cold render ---
echo "=== Cold render ==="
START=$(date +%s%N)
OUTPUT=$(DH_VM_POOL=0 dh render "$SCRIPT" --vm -v 2>&1) || true
END=$(date +%s%N)
WALL=$(( (END - START) / 1000000 ))

if validate_output "$OUTPUT" "cold"; then
  STATUS="OK"
else
  STATUS="FAIL"
fi

COLD_WALL=$WALL
COLD_TOTAL=$(extract_timing "$OUTPUT" "total render")
COLD_SCRIPT=$(extract_timing "$OUTPUT" "script execution")
COLD_NODE=$(extract_timing "$OUTPUT" "node.js renderer")
COLD_OPEN=$(extract_timing "$OUTPUT" "session.open")
COLD_RENDER=$(extract_timing "$OUTPUT" "session.render")

printf "  wall=%dms total=%sms script=%sms node=%sms open=%sms render=%sms [%s]\n" \
  "$COLD_WALL" "$COLD_TOTAL" "$COLD_SCRIPT" "$COLD_NODE" "$COLD_OPEN" "$COLD_RENDER" "$STATUS"

# --- Start pool ---
pkill -9 firecracker 2>/dev/null || true
sleep 1
rm -rf "$DH_HOME/vm/run/"* 2>/dev/null || true

# Auto-detect snapshot version
SNAP_VERSION=$(ls -1 "$DH_HOME/vm/snapshots/" 2>/dev/null | head -1)
if [[ -z "$SNAP_VERSION" ]]; then
  echo "No snapshots found in $DH_HOME/vm/snapshots/" >&2
  exit 1
fi

dh vm pool start -n 1 --idle-timeout 10m --version "$SNAP_VERSION" -v > /tmp/bench-pool.log 2>&1 &
POOL_PID=$!

# Wait for pool
for i in $(seq 1 60); do
  if dh vm pool status 2>/dev/null | grep -q "Ready VMs:    1"; then break; fi
  sleep 1
done

if ! dh vm pool status 2>/dev/null | grep -q "Ready VMs:"; then
  echo "Pool failed to start. Log:" >&2
  cat /tmp/bench-pool.log >&2
  exit 1
fi

# --- Pool renders ---
declare -a POOL_WALLS POOL_TOTALS POOL_SCRIPTS POOL_NODES POOL_OPENS POOL_RENDERS

for run in $(seq 1 "$RUNS"); do
  # Wait for backfill (skip on first run — VM is already ready)
  if [[ $run -gt 1 ]]; then
    for i in $(seq 1 60); do
      if dh vm pool status 2>/dev/null | grep -q "Ready VMs:    1"; then break; fi
      sleep 1
    done
  fi

  echo "=== Pool render #$run ==="
  START=$(date +%s%N)
  OUTPUT=$(dh render "$SCRIPT" --vm -v 2>&1) || true
  END=$(date +%s%N)
  WALL=$(( (END - START) / 1000000 ))

  if validate_output "$OUTPUT" "pool #$run"; then
    STATUS="OK"
  else
    STATUS="FAIL"
  fi

  T_TOTAL=$(extract_timing "$OUTPUT" "total render")
  T_SCRIPT=$(extract_timing "$OUTPUT" "script execution")
  T_NODE=$(extract_timing "$OUTPUT" "node.js renderer")
  T_OPEN=$(extract_timing "$OUTPUT" "session.open")
  T_RENDER=$(extract_timing "$OUTPUT" "session.render")

  POOL_WALLS+=("$WALL")
  POOL_TOTALS+=("$T_TOTAL")
  POOL_SCRIPTS+=("$T_SCRIPT")
  POOL_NODES+=("$T_NODE")
  POOL_OPENS+=("$T_OPEN")
  POOL_RENDERS+=("$T_RENDER")

  printf "  wall=%dms total=%sms script=%sms node=%sms open=%sms render=%sms [%s]\n" \
    "$WALL" "$T_TOTAL" "$T_SCRIPT" "$T_NODE" "$T_OPEN" "$T_RENDER" "$STATUS"
done

# --- Stop pool ---
dh vm pool stop 2>/dev/null || true
wait "$POOL_PID" 2>/dev/null || true

# --- Summary table ---
echo ""
TITLE="Render Benchmark: $(basename "$SCRIPT")"
echo "┌─────────────────────────────────────────────────────────────────────────────┐"
printf "│  %-73s  │\n" "$TITLE"
echo "├────────────┬─────────┬─────────┬─────────┬─────────┬─────────┬─────────────┤"
echo "│            │  wall   │  total  │ script  │  node   │  open   │   render    │"
echo "├────────────┼─────────┼─────────┼─────────┼─────────┼─────────┼─────────────┤"

fmt_row() {
  local label="$1" wall="$2" total="$3" script="$4" node="$5" open="$6" render="$7"
  printf "│ %-10s │ %5sms │ %5sms │ %5sms │ %5sms │ %5sms │ %7sms   │\n" \
    "$label" "$wall" "$total" "$script" "$node" "$open" "$render"
}

fmt_row "cold" "$COLD_WALL" "$COLD_TOTAL" "$COLD_SCRIPT" "$COLD_NODE" "$COLD_OPEN" "$COLD_RENDER"

for i in $(seq 0 $((RUNS - 1))); do
  fmt_row "pool #$((i+1))" "${POOL_WALLS[$i]}" "${POOL_TOTALS[$i]}" "${POOL_SCRIPTS[$i]}" \
    "${POOL_NODES[$i]}" "${POOL_OPENS[$i]}" "${POOL_RENDERS[$i]}"
done

# Compute pool averages
avg() {
  local -n arr=$1; local sum=0
  for v in "${arr[@]}"; do [[ "$v" == "-" ]] && { echo "-"; return; }; sum=$((sum + v)); done
  echo $((sum / ${#arr[@]}))
}

AVG_WALL=$(avg POOL_WALLS)
AVG_TOTAL=$(avg POOL_TOTALS)
AVG_SCRIPT=$(avg POOL_SCRIPTS)
AVG_NODE=$(avg POOL_NODES)
AVG_OPEN=$(avg POOL_OPENS)
AVG_RENDER=$(avg POOL_RENDERS)

echo "├────────────┼─────────┼─────────┼─────────┼─────────┼─────────┼─────────────┤"
fmt_row "pool avg" "$AVG_WALL" "$AVG_TOTAL" "$AVG_SCRIPT" "$AVG_NODE" "$AVG_OPEN" "$AVG_RENDER"
echo "└────────────┴─────────┴─────────┴─────────┴─────────┴─────────┴─────────────┘"

# Speedup
if [[ "$COLD_WALL" -gt 0 && "$AVG_WALL" -gt 0 ]]; then
  SAVED=$((COLD_WALL - AVG_WALL))
  PCT=$((SAVED * 100 / COLD_WALL))
  echo ""
  echo "Pool avg saves ${SAVED}ms (${PCT}%) vs cold render."
fi
