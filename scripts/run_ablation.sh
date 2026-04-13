#!/usr/bin/env bash
# run_ablation.sh — Run ablation study across all detectors and output comparison table.
# Usage: ./scripts/run_ablation.sh [--out-dir results/ablation]
set -euo pipefail

OUT_DIR="${1:-results/ablation_$(date +%Y%m%d_%H%M%S)}"
mkdir -p "$OUT_DIR"

echo "=== Running ablation study ==="

# Run the built-in ablation mode — compares all detectors.
go run ./cmd/eval-detection/ \
  --detector ablation \
  --window 2s \
  --windows 20 \
  --normal 300 \
  --attack 15000 \
  --attack-from 8 \
  --attack-count 6 \
  --seed 42 \
  --experiment-dir "$OUT_DIR" \
  2>&1 | tee "$OUT_DIR/ablation.log"

# Run individual experiments for each detector to get detailed CSV output.
for det in threshold zscore ewma loda hst ensemble; do
  echo "--- Detailed eval: $det ---"
  det_dir="$OUT_DIR/$det"
  mkdir -p "$det_dir"

  go run ./cmd/eval-detection/ \
    --detector "$det" \
    --window 2s \
    --windows 20 \
    --normal 300 \
    --attack 15000 \
    --attack-from 8 \
    --attack-count 6 \
    --seed 42 \
    --out "$det_dir/eval.csv" \
    --experiment-dir "$det_dir/experiment" \
    2>&1 | tee "$det_dir/eval.log"
done

echo "=== Ablation study complete: $OUT_DIR ==="
