# Evaluation: DDoS Detection and Memory Comparison (B.Tech Report)

This document describes how to run the detection and memory benchmarks so you can reproduce results for your project report.

## Prerequisites

- Go 1.24+
- Build from the project root (where `config.json` and `go.mod` are):

```bash
cd /path/to/BTP
```

Ensure `config.json` contains at least:

```json
{"precision": 14, "hashAlgorithm": "xxhash"}
```

## 1. DDoS Detection Effectiveness

The detection evaluation feeds synthetic traffic (normal and attack windows) into the same pipeline as the agent (WindowManager + ThresholdDetector) and computes recall, false positives, and time-to-detect.

### Build

```bash
go build -o eval-detection.exe ./cmd/eval-detection
```

### Run (default scenario)

- **Window**: 2s  
- **Threshold**: 5000 distinct IPs per window  
- **Normal**: 300 distinct IPs per window  
- **Attack**: 15000 distinct IPs per window  
- **Total windows**: 12 (windows 4–7 are attack)

```bash
./eval-detection.exe
```

This runs in real time (about 24 seconds for 12 × 2s windows). Results are printed to stdout.

### Options

| Flag | Default | Description |
|------|---------|-------------|
| `-window` | 2s | Window duration |
| `-threshold` | 5000 | Attack threshold (distinct IPs) |
| `-normal` | 300 | Distinct IPs per window (normal) |
| `-attack` | 15000 | Distinct IPs per window (attack) |
| `-windows` | 12 | Total number of windows |
| `-attack-from` | 4 | First attack window index (0-based) |
| `-attack-count` | 4 | Number of consecutive attack windows |
| `-seed` | 42 | RNG seed for reproducibility |
| `-out` | (none) | If set, write results CSV here |

### Example: threshold sweep (for report table)

Run with different thresholds and record recall vs false positives:

```bash
./eval-detection.exe -threshold=3000 -out=eval_t3k.csv
./eval-detection.exe -threshold=5000 -out=eval_t5k.csv
./eval-detection.exe -threshold=10000 -out=eval_t10k.csv
```

Then use the CSV files or stdout to build a “threshold vs recall vs FP rate” table.

### Interpreting results

- **Recall**: TP / (TP + FN) — fraction of attack windows in which we raised an alert.  
- **False positives**: Alerts during normal windows.  
- **Time to detect**: Time from the start of the first attack window to the first `AttackDetected` event.

---

## 2. Memory Comparison: HLL++ vs Exact Counting

The memory benchmark compares approximate memory of the HLL++ pipeline (two sketches, constant size) with a `map[string]struct{}` exact-counting baseline (grows with distinct IPs).

### Build

```bash
go build -o benchmark-memory.exe ./cmd/benchmark-memory
```

### Run

```bash
./benchmark-memory.exe -out=memory_benchmark.csv
```

This tests distinct-IP counts 1k, 10k, 100k, 1M. For each count it:

1. Builds a WindowManager (two HLL++ sketches), inserts that many distinct IPs, and records `ApproxMemoryBytes()` and the cardinality estimate.  
2. Measures heap delta for a `map[string]struct{}` with the same number of distinct IPs.

Output is printed to stdout and written to the CSV.

### Options

| Flag | Default | Description |
|------|---------|-------------|
| `-out` | memory_benchmark.csv | Output CSV path |
| `-window` | 10s | Window duration (for WindowManager) |
| `-seed` | 1 | RNG seed |

### CSV columns

- `distinct_ips`: Number of distinct IPs.  
- `hll_memory_bytes`: Approximate HLL memory (two sketches, theoretical dense size at p=14).  
- `exact_memory_bytes`: Heap delta for the exact-counting map.  
- `hll_estimate`: Cardinality estimate from the sketch.

### Using the results in your report

- Plot **Memory (KB) vs distinct IPs**: one series for HLL++ (flat, ~40 KB for two sketches), one for exact counting (growing).  
- State that your system uses constant, small memory (~40 KB) independent of traffic scale, compared to O(n) for exact counting.

---

## 3. Summary sentence for the report

You can use a line like:

> Our system detects synthetic DDoS with X% recall and Y false positives over Z normal windows, with constant memory of ~40 KB compared to O(n) for exact counting.

Fill X, Y, Z from the detection eval and memory benchmark runs above.
