#!/bin/bash
# Multi-node demo: 3 devices (ESP32 + Pi + Laptop) with attack phases.
# Captures aggregator timeline and per-node data.

set -e
BASEDIR="$(cd "$(dirname "$0")/.." && pwd)"
RESULTS="$BASEDIR/results/multi_node"
mkdir -p "$RESULTS"

ESP32_IP="10.120.149.205"
PI_IP="10.120.149.221"
LAPTOP_IP="127.0.0.1"
AGGREGATOR="http://localhost:9091"

NORMAL_DUR=60
ATTACK_DUR=45
RECOVERY_DUR=60

echo "=== Multi-Node DDoS Detection Demo ==="
echo "Nodes: ESP32 ($ESP32_IP), Pi ($PI_IP), Laptop ($LAPTOP_IP)"
echo "Phases: Normal ${NORMAL_DUR}s → Attack ${ATTACK_DUR}s → Recovery ${RECOVERY_DUR}s"
echo ""

# Timeline capture: poll aggregator every 2 seconds
echo "timestamp,phase,global_score,defense_activated,reason" > "$RESULTS/defense_timeline.csv"
capture_timeline() {
    local phase="$1"
    while true; do
        resp=$(curl -s "$AGGREGATOR/api/defense" 2>/dev/null || echo '{}')
        activated=$(echo "$resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('activated',''))" 2>/dev/null)
        score=$(echo "$resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('global_score',''))" 2>/dev/null)
        reason=$(echo "$resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('reason',''))" 2>/dev/null)
        echo "$(date -u +%Y-%m-%dT%H:%M:%SZ),$phase,$score,$activated,$reason" >> "$RESULTS/defense_timeline.csv"
        sleep 2
    done
}

# Build UDP flood tool
echo "Building UDP flood tool..."
cd "$BASEDIR"
go build -o /tmp/udp-flood ./cmd/udp-flood/

# Reset aggregator state so previous demo doesn't contaminate baselines.
echo "Resetting aggregator state..."
curl -s -X POST http://localhost:9091/api/reset | python3 -m json.tool

# Warm-up: send normal-rate traffic for 60s so detectors learn the baseline.
# Without this, the z-score sees 0→300 as a spike and false-triggers.
echo "Warming up detectors with normal traffic (60s)..."
/tmp/udp-flood -target "$ESP32_IP:50052" -rate 30 -attack-rate 30 -normal "60s" -attack 0s -recovery 0s &
WU_ESP=$!
/tmp/udp-flood -target "$PI_IP:50052" -rate 30 -attack-rate 30 -normal "60s" -attack 0s -recovery 0s &
WU_PI=$!
/tmp/udp-flood -target "$LAPTOP_IP:50053" -rate 30 -attack-rate 30 -normal "60s" -attack 0s -recovery 0s &
WU_LAPTOP=$!
wait $WU_ESP $WU_PI $WU_LAPTOP 2>/dev/null
echo "Warm-up done. Detectors have a stable baseline."

echo ""
echo "--- Phase 1: NORMAL (${NORMAL_DUR}s) ---"
capture_timeline "normal" &
TIMELINE_PID=$!

# Normal traffic to all 3 (low rate)
/tmp/udp-flood -target "$ESP32_IP:50052" -rate 30 -attack-rate 30 -normal "${NORMAL_DUR}s" -attack 0s -recovery 0s &
FLOOD_ESP32=$!
/tmp/udp-flood -target "$PI_IP:50052" -rate 30 -attack-rate 30 -normal "${NORMAL_DUR}s" -attack 0s -recovery 0s &
FLOOD_PI=$!
/tmp/udp-flood -target "$LAPTOP_IP:50053" -rate 30 -attack-rate 30 -normal "${NORMAL_DUR}s" -attack 0s -recovery 0s &
FLOOD_LAPTOP=$!

wait $FLOOD_ESP32 $FLOOD_PI $FLOOD_LAPTOP 2>/dev/null
echo "Normal phase done."

# Switch timeline phase
kill $TIMELINE_PID 2>/dev/null
echo ""
echo "--- Phase 2: ATTACK (${ATTACK_DUR}s) ---"
capture_timeline "attack" &
TIMELINE_PID=$!

# Attack traffic to all 3 (high rate)
/tmp/udp-flood -target "$ESP32_IP:50052" -rate 5000 -attack-rate 5000 -normal "${ATTACK_DUR}s" -attack 0s -recovery 0s &
FLOOD_ESP32=$!
/tmp/udp-flood -target "$PI_IP:50052" -rate 5000 -attack-rate 5000 -normal "${ATTACK_DUR}s" -attack 0s -recovery 0s &
FLOOD_PI=$!
/tmp/udp-flood -target "$LAPTOP_IP:50053" -rate 5000 -attack-rate 5000 -normal "${ATTACK_DUR}s" -attack 0s -recovery 0s &
FLOOD_LAPTOP=$!

wait $FLOOD_ESP32 $FLOOD_PI $FLOOD_LAPTOP 2>/dev/null
echo "Attack phase done."

# Switch timeline phase
kill $TIMELINE_PID 2>/dev/null
echo ""
echo "--- Phase 3: RECOVERY (${RECOVERY_DUR}s) ---"
capture_timeline "recovery" &
TIMELINE_PID=$!

# Recovery traffic (low rate again)
/tmp/udp-flood -target "$ESP32_IP:50052" -rate 30 -attack-rate 30 -normal "${RECOVERY_DUR}s" -attack 0s -recovery 0s &
FLOOD_ESP32=$!
/tmp/udp-flood -target "$PI_IP:50052" -rate 30 -attack-rate 30 -normal "${RECOVERY_DUR}s" -attack 0s -recovery 0s &
FLOOD_PI=$!
/tmp/udp-flood -target "$LAPTOP_IP:50053" -rate 30 -attack-rate 30 -normal "${RECOVERY_DUR}s" -attack 0s -recovery 0s &
FLOOD_LAPTOP=$!

wait $FLOOD_ESP32 $FLOOD_PI $FLOOD_LAPTOP 2>/dev/null
echo "Recovery phase done."

kill $TIMELINE_PID 2>/dev/null

echo ""
echo "=== Demo Complete ==="
echo "Results: $RESULTS/defense_timeline.csv"
ROWS=$(wc -l < "$RESULTS/defense_timeline.csv")
echo "Timeline rows: $ROWS"
echo ""
echo "Defense activation events:"
grep -i "true" "$RESULTS/defense_timeline.csv" | head -5 || echo "  (none)"
