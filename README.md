# Distributed DDoS Detection with HyperLogLog++

![Go](https://img.shields.io/badge/Go-00ADD8?logo=Go&logoColor=white&style=for-the-badge)
![Python](https://img.shields.io/badge/Python-3776AB?style=for-the-badge&logo=python&logoColor=white)

[![CI](https://github.com/cruelkratos/distributed-ddos-hllpp/actions/workflows/ci.yml/badge.svg)](https://github.com/cruelkratos/distributed-ddos-hllpp/actions/workflows/ci.yml)
[![Go build](https://github.com/cruelkratos/distributed-ddos-hllpp/actions/workflows/go_build_test_pipeline.yml/badge.svg)](https://github.com/cruelkratos/distributed-ddos-hllpp/actions/workflows/go_build_test_pipeline.yml)

## Overview

A production-ready Go system for real-time distributed DDoS detection.
Each host runs a lightweight **agent** that captures packets, inserts source IPs
into a time-windowed HLL++ sketch, and ships the sketch over gRPC to a central
**aggregator**. The aggregator merges sketches from all agents, maintains a global
unique-IP estimate, and runs a pluggable detector to fire alerts.

The cardinality estimation core is a custom HyperLogLog++ implementation based on
the Google research paper *"HyperLogLog in Practice"* (Heule et al., 2013).

---

## Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│  Kubernetes Cluster                                              │
│                                                                  │
│  ┌──────────────┐   ┌──────────────┐   ┌──────────────┐        │
│  │  Agent       │   │  Agent       │   │  Agent       │        │
│  │  (DaemonSet) │   │  (DaemonSet) │   │  (DaemonSet) │        │
│  │  pcap→HLL++  │   │  pcap→HLL++  │   │  pcap→HLL++  │        │
│  └──────┬───────┘   └──────┬───────┘   └──────┬───────┘        │
│         │  gRPC MergeSketch│                  │                 │
│         └──────────────────▼──────────────────┘                 │
│                    ┌──────────────┐                              │
│                    │  Aggregator  │                              │
│                    │  (Deployment)│                              │
│                    │  Merge+Detect│                              │
│                    │  :50051 gRPC │                              │
│                    │  :2112 Prom  │                              │
│                    └──────────────┘                              │
└──────────────────────────────────────────────────────────────────┘
```

---

## Key Features

- **HLL++ core** — sparse→dense auto-transition, 6-bit packed registers, bias correction via k-NN interpolation; full `Merge()` support for distributed aggregation.
- **Thread-safe windowed counting** — `WindowManager` rotates HLL sketches on a configurable interval; exports serializable snapshots for shipping.
- **gRPC sketch shipping** — agents serialize and push current sketches to the aggregator via `MergeSketch`; aggregator exposes `GetEstimate`, `GetSketch`, `Reset`, and `Health` RPCs.
- **Pluggable detectors** — swap detection strategy at runtime:
  - `threshold` — fires when unique-IP count exceeds a fixed value.
  - `zscore` — statistical Z-score over a rolling history window.
  - `ewma` — Exponentially Weighted Moving Average; alert when count exceeds `baseline × (1 + deviationFactor)`.
- **Prometheus metrics** — aggregator exposes `/metrics` on `:2112`.
- **Kubernetes-ready** — DaemonSet for agents, Deployment for aggregator, manifests in `k8s/`.
- **Docker images** — `Dockerfile.agent` (alpine + libpcap) and `Dockerfile.aggregator` (scratch, CGO-free).
- **CI pipeline** — GitHub Actions: race-detector tests, binary builds, multi-detector eval comparison, Docker image builds.

---

## Project Structure

```
.
├── cmd/
│   ├── agent/            # Agent binary (pcap capture → windowed HLL → detector → gRPC)
│   ├── aggregator/       # Aggregator binary (gRPC server, global HLL, detection loop)
│   ├── eval-detection/   # Evaluation harness — simulate traffic, compare detectors
│   └── benchmark-memory/ # Memory benchmark for HLL++ register storage
├── ddos/
│   ├── capture/          # Packet capture (gopacket/pcap)
│   ├── detector/         # Detector interface + ThresholdDetector, ZScoreDetector, EWMADetector
│   ├── eval/             # Synthetic traffic evaluation (Scenario, Result, WriteCSV)
│   ├── metrics/          # Prometheus counter/gauge helpers
│   └── window/           # WindowManager — rotating HLL++ windows + attack events
├── types/
│   ├── hll/              # Core HLL++ (sparse, dense, bias correction, merge, export)
│   ├── register/         # 6-bit packed register array + hashing
│   └── sparse/           # Sparse list representation
├── tools/
│   └── push_sketch/      # Integration test tool — push synthetic sketches to aggregator
├── k8s/
│   ├── daemonset-agent.yaml
│   └── deployment-aggregator.yaml
├── .github/workflows/
│   ├── ci.yml                    # Main CI: race tests, build, eval, docker
│   └── go_build_test_pipeline.yml
├── Dockerfile.agent
├── Dockerfile.aggregator
├── server.proto          # gRPC service definition
└── bias/                 # Bias correction data (JSON) for HLL++ accuracy
```

---

## Getting Started

### Prerequisites

- Go 1.24+
- `libpcap-dev` (for the agent: `apt-get install libpcap-dev` or `brew install libpcap`)

### Build all binaries

```bash
# Agent (requires CGO for pcap)
CGO_ENABLED=1 go build -o bin/agent ./cmd/agent

# Aggregator (pure Go, no CGO needed)
CGO_ENABLED=0 go build -o bin/aggregator ./cmd/aggregator

# Eval harness
go build -o bin/eval-detection ./cmd/eval-detection
```

### Run locally

**Start the aggregator:**
```bash
./bin/aggregator -listen :50051 -metrics :2112 -detector zscore
```

**Start an agent** (requires root/CAP_NET_RAW for pcap):
```bash
sudo ./bin/agent -iface eth0 -window 10s -detector zscore -aggregator localhost:50051
```

**Push synthetic sketches for integration testing:**
```bash
go run ./tools/push_sketch -addr localhost:50051 -ips 8000
```

### Run the detector comparison evaluation

```bash
# Side-by-side table: threshold vs zscore vs ewma
go run ./cmd/eval-detection --detector compare

# Single detector with custom params
go run ./cmd/eval-detection --detector ewma --ewma-alpha 0.15 --ewma-deviation 1.5

# Save results to CSV
go run ./cmd/eval-detection --detector zscore --out results.csv
```

### Run tests

```bash
# All tests
go test ./...

# With race detector
go test -race -timeout 120s ./...

# Detector unit tests only
go test -v ./ddos/detector/
```

---

## Detectors

| Detector | Flag | When to use |
|---|---|---|
| Threshold | `threshold` | Known baseline; simple fixed limit |
| Z-Score | `zscore` | Stationary traffic with occasional spikes |
| EWMA | `ewma` | Slowly varying baseline; rewards gradual adaptation |

**EWMA tuning flags:** `--ewma-alpha` (0<α≤1), `--ewma-deviation` (e.g. `2.0` = alert when count is 2× above baseline), `--ewma-warmup` (windows before alerts fire).

**Z-Score tuning flags:** `--zs-history` (rolling window length), `--zs-threshold` (sigma).

---

## Docker

```bash
docker build -f Dockerfile.agent -t ddos/agent:latest .
docker build -f Dockerfile.aggregator -t ddos/aggregator:latest .
```

---

## Kubernetes

```bash
kubectl apply -f k8s/deployment-aggregator.yaml
kubectl apply -f k8s/daemonset-agent.yaml
```

Set `AGGREGATOR_ADDR` in the DaemonSet env to point agents at the aggregator service (e.g. `aggregator-svc:50051`).

---

## HLL++ Algorithm

Based on: Heule, S., Nunkesser, M., & Hall, A. (2013). *HyperLogLog in Practice: Algorithmic Engineering of a State of The Art Cardinality Estimation Algorithm*.

Key implementation details:
- Precision `p=14` (16 384 registers, ~0.8% relative error)
- Sparse mode (`p'=25`) for low-cardinality sets with automatic dense transition
- Bias correction using k-NN interpolation (k=6) over pre-computed JSON data
- 64-bit xxhash for collision resistance at high cardinalities

---

[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
