#!/usr/bin/env bash
# live_demo_capture.sh — Run full live demo against ESP32 and capture all data.
# Captures: aggregator defense state, timing, detection latency.
set -euo pipefail

OUT_DIR="${1:-results/live_demo}"
ESP32_IP="${2:-10.201.115.205}"
AGGREGATOR="http://localhost:9091"
RATE=50
NORMAL_DUR=20
ATTACK_DUR=40
RECOVERY_DUR=30
TOTAL_DUR=$((NORMAL_DUR + ATTACK_DUR + RECOVERY_DUR))

mkdir -p "$OUT_DIR"

echo "=== Live ESP32 Demo Capture ==="
echo "ESP32: $ESP32_IP:50052"
echo "Aggregator: $AGGREGATOR"
echo "Phases: normal=${NORMAL_DUR}s attack=${ATTACK_DUR}s recovery=${RECOVERY_DUR}s"
echo "Output: $OUT_DIR"
echo ""

# 1. Record start time.
START_TS=$(date +%s)
echo "$START_TS" > "$OUT_DIR/start_timestamp.txt"

# 2. Start defense state poller in background.
echo "timestamp,elapsed_sec,activated,global_score,reason" > "$OUT_DIR/defense_timeline.csv"
(
  while true; do
    NOW=$(date +%s)
    ELAPSED=$((NOW - START_TS))
    RESP=$(curl -s "$AGGREGATOR/api/defense" 2>/dev/null || echo '{"activated":false,"global_score":0,"reason":""}')
    ACTIVATED=$(echo "$RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); print('true' if d.get('activated') else 'false')" 2>/dev/null || echo "unknown")
    SCORE=$(echo "$RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('global_score',0))" 2>/dev/null || echo "0")
    REASON=$(echo "$RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('reason',''))" 2>/dev/null || echo "")
    echo "$NOW,$ELAPSED,$ACTIVATED,$SCORE,$REASON" >> "$OUT_DIR/defense_timeline.csv"
    
    # Also record phase.
    if [ "$ELAPSED" -lt "$NORMAL_DUR" ]; then
      PHASE="normal"
    elif [ "$ELAPSED" -lt "$((NORMAL_DUR + ATTACK_DUR))" ]; then
      PHASE="attack"
    else
      PHASE="recovery"
    fi
    
    echo "[${ELAPSED}s] phase=$PHASE activated=$ACTIVATED score=$SCORE"
    
    if [ "$ELAPSED" -gt "$((TOTAL_DUR + 10))" ]; then
      break
    fi
    sleep 2
  done
) &
POLLER_PID=$!

# 3. Start Prometheus metrics scraper in background (if available).
echo "timestamp,metric,value" > "$OUT_DIR/prometheus_samples.csv"
(
  while true; do
    NOW=$(date +%s)
    ELAPSED=$((NOW - START_TS))
    METRICS=$(curl -s "$AGGREGATOR/metrics" 2>/dev/null || echo "")
    
    # Extract key metrics.
    for m in ddos_unique_ips_current_window ddos_ensemble_score ddos_anomaly_state ddos_nodes_total ddos_nodes_under_attack ddos_global_defense_active ddos_memory_rss_bytes ddos_memory_heap_bytes ddos_cpu_percent ddos_goroutines ddos_loda_score ddos_hst_score; do
      VAL=$(echo "$METRICS" | grep "^${m}" | grep -v '#' | head -1 | awk '{print $2}' 2>/dev/null || echo "")
      if [ -n "$VAL" ]; then
        echo "$NOW,$m,$VAL" >> "$OUT_DIR/prometheus_samples.csv"
      fi
    done
    
    if [ "$ELAPSED" -gt "$((TOTAL_DUR + 10))" ]; then
      break
    fi
    sleep 2
  done
) &
SCRAPER_PID=$!

echo ""
echo "=== Starting UDP flood against ESP32 ==="
echo "  go run ./cmd/udp-flood -target ${ESP32_IP}:50052 -rate $RATE -normal ${NORMAL_DUR}s -attack ${ATTACK_DUR}s -recovery ${RECOVERY_DUR}s"
echo ""

# 4. Run the UDP flood.
go run ./cmd/udp-flood \
  -target "${ESP32_IP}:50052" \
  -rate "$RATE" \
  -normal "${NORMAL_DUR}s" \
  -attack "${ATTACK_DUR}s" \
  -recovery "${RECOVERY_DUR}s" \
  2>&1 | tee "$OUT_DIR/udp_flood.log"

# 5. Wait for background processes and give recovery time.
echo ""
echo "Waiting for recovery polling..."
sleep 15

# 6. Kill background processes.
kill "$POLLER_PID" 2>/dev/null || true
kill "$SCRAPER_PID" 2>/dev/null || true
wait "$POLLER_PID" 2>/dev/null || true
wait "$SCRAPER_PID" 2>/dev/null || true

# 7. Final defense state.
echo ""
echo "=== Final State ==="
curl -s "$AGGREGATOR/api/defense" | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'activated={d[\"activated\"]} score={d[\"global_score\"]:.3f} reason={d[\"reason\"]}')" 2>/dev/null || true

# 8. Count data points.
echo ""
echo "=== Data Summary ==="
echo "Defense timeline points: $(wc -l < "$OUT_DIR/defense_timeline.csv")"
echo "Prometheus samples: $(wc -l < "$OUT_DIR/prometheus_samples.csv")"
echo "Output directory: $OUT_DIR"
ls -la "$OUT_DIR/"
