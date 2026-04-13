#!/usr/bin/env bash
# run_scalability.sh — Run scalability experiments: vary N nodes, measure resource usage.
# Usage: ./scripts/run_scalability.sh [--out-dir results/scalability]
set -euo pipefail

OUT_DIR="${1:-results/scalability_$(date +%Y%m%d_%H%M%S)}"
mkdir -p "$OUT_DIR"

echo "=== Running scalability experiment ==="

NODE_COUNTS="1 2 5 10 20 50"

for n in $NODE_COUNTS; do
  echo "--- Nodes: $n ---"
  go run ./cmd/benchmark-system/ \
    --nodes "$n" \
    --window 2s \
    --windows 20 \
    --normal 300 \
    --attack 15000 \
    --attack-from 8 \
    --attack-count 6 \
    --detector ensemble \
    --out "$OUT_DIR/bench_nodes_${n}.csv" \
    2>&1 | tee "$OUT_DIR/nodes_${n}.log"
done

echo "=== Scalability experiment complete: $OUT_DIR ==="

# Combine all CSVs into one for easy plotting.
head -1 "$OUT_DIR/bench_nodes_1.csv" > "$OUT_DIR/combined_scalability.csv"
for n in $NODE_COUNTS; do
  tail -n +2 "$OUT_DIR/bench_nodes_${n}.csv" >> "$OUT_DIR/combined_scalability.csv"
done
echo "Combined CSV: $OUT_DIR/combined_scalability.csv"
