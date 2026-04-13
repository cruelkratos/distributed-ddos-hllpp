#!/usr/bin/env bash
# run_experiment.sh — Run a full detection evaluation experiment.
# Usage: ./scripts/run_experiment.sh [--detector ensemble] [--windows 20] [--out-dir results/exp1]
set -euo pipefail

DETECTOR="${1:-ensemble}"
WINDOWS="${2:-20}"
OUT_DIR="${3:-results/$(date +%Y%m%d_%H%M%S)}"
SEED=42
NORMAL=300
ATTACK=15000
ATTACK_FROM=8
ATTACK_COUNT=6
WINDOW_DUR="2s"

mkdir -p "$OUT_DIR"

echo "=== Running experiment ==="
echo "  Detector:    $DETECTOR"
echo "  Windows:     $WINDOWS"
echo "  Output:      $OUT_DIR"

# Run detection eval with experiment logger.
go run ./cmd/eval-detection/ \
  --detector "$DETECTOR" \
  --window "$WINDOW_DUR" \
  --windows "$WINDOWS" \
  --normal "$NORMAL" \
  --attack "$ATTACK" \
  --attack-from "$ATTACK_FROM" \
  --attack-count "$ATTACK_COUNT" \
  --seed "$SEED" \
  --out "$OUT_DIR/eval_results.csv" \
  --experiment-dir "$OUT_DIR/experiment" \
  --experiment-id "exp_${DETECTOR}_$(date +%Y%m%d_%H%M%S)" \
  2>&1 | tee "$OUT_DIR/eval.log"

# Run memory/resource benchmark.
go run ./cmd/benchmark-system/ \
  --nodes 1 \
  --window "$WINDOW_DUR" \
  --windows "$WINDOWS" \
  --normal "$NORMAL" \
  --attack "$ATTACK" \
  --attack-from "$ATTACK_FROM" \
  --attack-count "$ATTACK_COUNT" \
  --detector "$DETECTOR" \
  --out "$OUT_DIR/benchmark_resources.csv" \
  2>&1 | tee "$OUT_DIR/benchmark.log"

echo "=== Experiment complete: $OUT_DIR ==="
ls -la "$OUT_DIR/"
