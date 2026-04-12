#!/usr/bin/env bash
# run_iot_demo.sh — End-to-end IoT DDoS detection demo
#
# Demonstrates the tiered IoT architecture:
#   Arduino Uno (p=4, 12B)  →  Raspberry Pi 3 (p=14, 12KB)  →  Aggregator
#
# Prerequisites:
#   - Aggregator and Agent binaries built (make build / make build-arm)
#   - Arduino flashed with arduino/micro_hll/micro_hll.ino
#   - Python3 with pyserial installed (pip install pyserial)
#
# Usage:
#   # Full demo (Arduino + Pi agent + aggregator on laptop)
#   ./demo/run_iot_demo.sh --full
#
#   # Arduino-only demo (no Pi, no network)
#   ./demo/run_iot_demo.sh --arduino-only
#
#   # Simulated demo (no hardware at all)
#   ./demo/run_iot_demo.sh --simulated
#
#   # Benchmark precision comparison
#   ./demo/run_iot_demo.sh --benchmark

set -euo pipefail
cd "$(dirname "$0")/.."

SERIAL_PORT="${SERIAL_PORT:-/dev/ttyACM0}"
AGENT_ADDR="${AGENT_ADDR:-localhost:50052}"
AGGREGATOR_ADDR="${AGGREGATOR_ADDR:-localhost:50051}"
PIDS=()

cleanup() {
    echo ""
    echo "═══ Cleaning up... ═══"
    for pid in "${PIDS[@]}"; do
        kill "$pid" 2>/dev/null || true
    done
    wait 2>/dev/null || true
    echo "Done."
}
trap cleanup EXIT

banner() {
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo "  $1"
    echo "═══════════════════════════════════════════════════════════════"
    echo ""
}

# ─── Mode selection ───────────────────────────────────────────────────

MODE="${1:---simulated}"

case "$MODE" in

# ─── Full demo: Aggregator + Agent + Arduino bridge ──────────────────
--full)
    banner "Full IoT Demo: Arduino → Pi Agent → Aggregator"

    echo "Starting aggregator..."
    ./bin/aggregator &
    PIDS+=($!)
    sleep 2

    echo "Starting agent (simulation mode, listening on $AGENT_ADDR)..."
    SIM_GRPC_PORT="${AGENT_ADDR##*:}" ./bin/agent &
    PIDS+=($!)
    sleep 2

    echo "Starting serial bridge (Arduino $SERIAL_PORT → Agent $AGENT_ADDR)..."
    go run ./cmd/serial-bridge --serial "$SERIAL_PORT" --agent "$AGENT_ADDR" &
    PIDS+=($!)
    sleep 2

    banner "Phase 1: Normal Traffic (30s)"
    echo "Sending 50 IPs/sec from 1000-IP pool via iot-sim..."
    timeout 30 ./bin/iot-sim --agent "$AGENT_ADDR" --profile normal --normal-rate 50 --normal-pool 1000 || true

    banner "Phase 2: DDoS Attack Traffic (30s)"
    echo "Sending 5000 IPs/sec (unique random IPs) via iot-sim..."
    timeout 30 ./bin/iot-sim --agent "$AGENT_ADDR" --profile attack --attack-rate 5000 || true

    banner "Phase 3: Recovery (15s)"
    echo "Returning to normal traffic..."
    timeout 15 ./bin/iot-sim --agent "$AGENT_ADDR" --profile normal --normal-rate 50 --normal-pool 1000 || true

    banner "Demo Complete"
    echo "Check aggregator logs for detection events."
    echo "If Grafana is running (port 3000), check the DDoS dashboard."
    ;;

# ─── Arduino-only: test micro-HLL without network ───────────────────
--arduino-only)
    banner "Arduino-Only Demo: Micro-HLL at p=4 (12 bytes)"

    echo "Testing Arduino micro-HLL via serial..."
    echo "Serial port: $SERIAL_PORT"
    echo ""

    python3 tools/arduino_bridge/bridge.py \
        --mode arduino-only \
        --serial "$SERIAL_PORT" \
        --counts "10,50,100,500,1000"
    ;;

# ─── Simulated: no hardware, prove the concept ──────────────────────
--simulated)
    banner "Simulated IoT Demo (No Hardware Required)"

    echo "1. Testing Arduino micro-HLL simulation..."
    echo ""
    python3 tools/arduino_bridge/bridge.py \
        --mode arduino-only \
        --serial fake \
        --counts "10,50,100,500,1000"

    echo ""
    echo "2. Running multi-precision benchmark..."
    echo ""
    go run ./cmd/benchmark-precision/ --out precision_benchmark.csv

    echo ""
    echo "Results saved to precision_benchmark.csv"
    ;;

# ─── Benchmark: precision comparison for paper ───────────────────────
--benchmark)
    banner "Multi-Precision Benchmark (p=4 to p=14)"

    go run ./cmd/benchmark-precision/ --out precision_benchmark.csv

    echo ""
    echo "Results saved to precision_benchmark.csv"

    if command -v python3 &>/dev/null; then
        echo ""
        echo "Arduino comparison (simulated):"
        python3 tools/arduino_bridge/bridge.py \
            --mode arduino-only \
            --serial fake \
            --counts "10,100,1000,5000"
    fi
    ;;

*)
    echo "Usage: $0 {--full|--arduino-only|--simulated|--benchmark}"
    echo ""
    echo "  --full          Full demo: Arduino + Pi agent + aggregator"
    echo "  --arduino-only  Arduino micro-HLL test only (needs Arduino)"
    echo "  --simulated     No hardware needed — proves the concept"
    echo "  --benchmark     Multi-precision comparison for paper"
    exit 1
    ;;

esac
