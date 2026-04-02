# DDoS Detection System — Architecture & Operations Guide

## Table of Contents

1. [System Overview](#system-overview)
2. [Architecture](#architecture)
3. [Detection Pipeline](#detection-pipeline)
4. [Anomaly Detectors](#anomaly-detectors)
5. [State Machine](#state-machine)
6. [Mitigation Engine](#mitigation-engine)
7. [Cross-Node Correlation](#cross-node-correlation)
8. [Monitoring & Dashboards](#monitoring--dashboards)
9. [Project Structure](#project-structure)
10. [Building](#building)
11. [Running Tests](#running-tests)
12. [Docker Compose Demo](#docker-compose-demo)
13. [Kubernetes (kind) Demo](#kubernetes-kind-demo)
14. [Configuration Reference](#configuration-reference)
15. [Memory Budget](#memory-budget)

---

## System Overview

This system detects and mitigates volumetric DDoS attacks across a distributed network of IoT nodes. Each node runs a lightweight **agent** that captures traffic, extracts features, and scores anomalies using an **ensemble** of four detectors: LODA, HST, Z-Score, and EWMA. A central **aggregator** collects telemetry from all nodes via gRPC, performs cross-node correlation, and issues global defense commands when a coordinated attack is detected.

Key capabilities:
- **Per-node ML detection** with LODA (Lightweight Online Detector of Anomalies) + HST (Streaming Half-Space Trees)
- **Hysteresis-based state machine** (NORMAL → UNDER_ATTACK → RECOVERY → NORMAL) to prevent flapping
- **Software-level mitigation** via token-bucket rate limiting + Count-Min Sketch per-IP tracking
- **Cross-node correlation** at the aggregator (majority-vote triggers global defense)
- **HLL++ cardinality sketches** for unique IP counting in ~12 KB per sketch
- **Full observability** with Prometheus metrics + Grafana dashboards
- **~36 KB per-node memory** for all detection and sketch structures

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Aggregator                               │
│  ┌──────────────┐  ┌────────────────┐  ┌─────────────────────┐ │
│  │ gRPC Server   │  │ Correlation    │  │ Defense Command     │ │
│  │ MergeSketch() │→ │ Loop (10s)     │→ │ GetDefenseCommand() │ │
│  │ GetDefense()  │  │ ≥50% attack →  │  │                     │ │
│  │ InjectIP()    │  │ global defense │  │ Prometheus :9091     │ │
│  └──────────────┘  └────────────────┘  └─────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
         ▲  gRPC (MergeSketch + telemetry)       │  gRPC (defense cmd)
         │                                       ▼
┌────────┴──────────────────────────────────────────────────────┐
│                     Agent (per-node)                           │
│  ┌─────────┐  ┌──────────────┐  ┌────────────┐  ┌──────────┐ │
│  │ Capture  │→ │ HLL++ Window │→ │ Ensemble   │→ │ State    │ │
│  │ pcap /   │  │ 10s rotation │  │ Detector   │  │ Machine  │ │
│  │ sim-mode │  │ TrafficStats │  │ LODA+HST+  │  │ N/A/R    │ │
│  └─────────┘  └──────────────┘  │ ZScore+EWMA│  └────┬─────┘ │
│                                  └────────────┘       │       │
│  ┌─────────────────────────────────────────────────────┘       │
│  │  ┌───────────────────┐   ┌───────────┐                     │
│  └→ │ Mitigation Ctrl   │ → │ Rate      │   Prometheus :9090  │
│     │ active on ATTACK  │   │ Limiter   │                     │
│     │ or RECOVERY       │   │ TBucket + │                     │
│     └───────────────────┘   │ CMS       │                     │
│                              └───────────┘                     │
└────────────────────────────────────────────────────────────────┘
```

---

## Detection Pipeline

Each agent processes traffic in **10-second windows**:

1. **Capture**: Packets arrive via libpcap (production) or gRPC `InjectIP` (simulation mode). Source IPs are added to an HLL++ sketch (p=14, m=16384 registers ≈ 12.3 KB).

2. **Feature Extraction**: At window rotation, 8 features are extracted:
   | Index | Feature | Description |
   |-------|---------|-------------|
   | 0 | `uniqueIPs` | HLL++ estimate of current window |
   | 1 | `prevUniqueIPs` | HLL++ estimate of previous window |
   | 2 | `ipsRatio` | current / previous (spike indicator) |
   | 3 | `packetCount` | Total packets in window |
   | 4 | `byteVolume` | Total bytes in window |
   | 5 | `bytesPerPacket` | Average packet size |
   | 6 | `ewmaResidual` | Deviation from EWMA baseline |
   | 7 | `zScore` | Statistical z-score of IP count |

3. **Ensemble Scoring**: All 4 detectors produce scores, combined via weighted sigmoid normalization to [0, 1].

4. **State Transition**: The ensemble score feeds the hysteresis state machine.

5. **Mitigation**: Rate limiter activates/deactivates based on state.

6. **Telemetry Shipping**: HLL sketch + all scores + state sent to aggregator via gRPC.

---

## Anomaly Detectors

### LODA (Lightweight Online Detector of Anomalies)
- **File**: `ddos/detector/loda.go`
- **Method**: 40 random sparse projections × 16-bin histograms
- **Scoring**: Mean negative log-likelihood across projections
- **Online learning**: Welford normalization tracks running mean/variance
- **Memory**: `40 × (16 × 2B + 8 × 4B) = ~2.6 KB`
- **Warmup**: 30 samples before scoring starts

### HST (Streaming Half-Space Trees)
- **File**: `ddos/detector/hst.go`
- **Method**: 5 random axis-aligned binary trees, depth 5
- **Scoring**: High refMass/mass ratio at leaf nodes → anomalous
- **Window swap**: Every 200 samples, mass → refMass, mass = 0
- **Memory**: `5 × 63 nodes × 12B/node ≈ 1.4 KB`

### Z-Score
- **File**: `ddos/detector/zscore.go`
- **Method**: Sliding window of 20 past cardinalities; alerts when z > 3σ
- **Memory**: `20 × 8B = 160 bytes`

### EWMA
- **File**: `ddos/detector/ewma.go`
- **Method**: Exponentially weighted moving average baseline; alerts when current > baseline × 3
- **Memory**: ~50 bytes

### Ensemble Combination
- **File**: `ddos/detector/ensemble.go`
- **Weights**: LODA 0.4, HST 0.3, ZScore 0.2, EWMA 0.1
- **Normalization**: Each raw score → sigmoid → [0, 1]
- **Combined score**: Weighted sum in [0, 1]
- **Default threshold**: 0.6

---

## State Machine

**File**: `ddos/detector/statemachine.go`

Three states with hysteresis-based transitions to prevent flapping:

```
              3 consecutive         first clean
              attack windows        window
  NORMAL ──────────────────► UNDER_ATTACK ──────────► RECOVERY
    ▲                              ▲                     │
    │    5 consecutive             │    attack relapse    │
    │    clean windows             └─────────────────────┘
    └──────────────────────── RECOVERY
```

- **NORMAL → UNDER_ATTACK**: Requires `attackConfirm` (default: 3) consecutive windows with ensemble score > threshold
- **UNDER_ATTACK → RECOVERY**: First window with score below threshold
- **RECOVERY → NORMAL**: Requires `recoveryConfirm` (default: 5) consecutive clean windows
- **RECOVERY → UNDER_ATTACK**: Immediate relapse on any anomalous window

---

## Mitigation Engine

### Rate Limiter (`ddos/mitigation/ratelimiter.go`)
- **Global token bucket**: Configurable RPS with 2× burst capacity
- **Per-IP tracking**: Count-Min Sketch (4 rows × 512 cols × uint16 = 4 KB)
- **Periodic CMS decay**: Halves all counters every 5s to prevent saturation

### Controller (`ddos/mitigation/controller.go`)
- Activates rate limiter when state is `UNDER_ATTACK` or `RECOVERY`
- Resets CMS and restores token bucket when returning to `NORMAL`
- `ShouldAllow(ip)` is the single entry point for packet gating

---

## Cross-Node Correlation

The aggregator (`cmd/aggregator/main.go`) performs distributed attack detection:

1. **Telemetry Collection**: Each agent ships HLL sketch + scores + state every 10s via `MergeSketch` gRPC
2. **Per-Node Tracking**: Maintains map of `nodeId → {lodaScore, hstScore, ensembleScore, anomalyState, lastSeen}`
3. **Correlation Loop** (every window duration):
   - Counts nodes in `UNDER_ATTACK` state
   - If ≥ 50% of active nodes are under attack → global defense active
4. **Defense Command**: Agents poll `GetDefenseCommand` every 5s; if defense active, agent forces state to `UNDER_ATTACK`
5. **Stale Cleanup**: Nodes not seen for 2× window duration are removed

Prometheus metrics exported:
- `ddos_nodes_total`: Number of active nodes
- `ddos_nodes_under_attack`: Nodes currently in UNDER_ATTACK state
- `ddos_global_defense_active`: 1 when global defense is triggered

---

## Monitoring & Dashboards

### Prometheus Metrics

**Agent metrics** (port 9090):
| Metric | Type | Description |
|--------|------|-------------|
| `ddos_unique_ips` | Gauge | HLL++ cardinality estimate |
| `ddos_packet_count` | Gauge | Packets in current window |
| `ddos_byte_volume` | Gauge | Bytes in current window |
| `ddos_loda_score` | Gauge | Raw LODA anomaly score |
| `ddos_hst_score` | Gauge | Raw HST anomaly score |
| `ddos_ensemble_score` | Gauge | Combined ensemble score [0,1] |
| `ddos_anomaly_state` | Gauge | 0=NORMAL, 1=UNDER_ATTACK, 2=RECOVERY |
| `ddos_drops_total` | Gauge | Cumulative packets dropped by rate limiter |

**Aggregator metrics** (port 9091):
| Metric | Type | Description |
|--------|------|-------------|
| `ddos_nodes_total` | Gauge | Active nodes |
| `ddos_nodes_under_attack` | Gauge | Nodes in UNDER_ATTACK |
| `ddos_global_defense_active` | Gauge | Global defense flag |

### Grafana Dashboard
Pre-provisioned dashboard with 8 panels:
1. **Unique IPs (HLL Estimate)** — time series per node
2. **Anomaly Scores (LODA / HST / Ensemble)** — color-coded time series
3. **Anomaly State Timeline** — state-timeline with value mappings (green/yellow/red)
4. **Packet Drop Rate** — time series of `ddos_drops_total`
5. **Global Defense Status** — stat panel (ACTIVE/INACTIVE)
6. **Nodes Under Attack** — gauge
7. **Packet Count / Byte Volume** — time series
8. **Total Registered Nodes** — stat panel

---

## Project Structure

```
BTP/
├── cmd/
│   ├── agent/main.go              # Per-node agent with ensemble detection
│   ├── aggregator/main.go         # Central correlation + defense
│   └── iot-sim/main.go            # Traffic simulator for demos
├── ddos/
│   ├── capture/
│   │   ├── interface.go           # Network interface listing
│   │   ├── pcap.go                # libpcap packet capture
│   │   └── stats.go               # Atomic traffic counters
│   ├── detector/
│   │   ├── detector.go            # Interfaces + FeatureVector
│   │   ├── loda.go                # LODA detector
│   │   ├── hst.go                 # HST detector
│   │   ├── ensemble.go            # Weighted ensemble
│   │   ├── statemachine.go        # 3-state FSM with hysteresis
│   │   ├── zscore.go              # Z-score detector
│   │   ├── ewma.go                # EWMA detector
│   │   └── threshold.go           # Simple threshold detector
│   ├── metrics/metrics.go         # Prometheus metric definitions
│   ├── mitigation/
│   │   ├── ratelimiter.go         # Token bucket + CMS
│   │   └── controller.go          # Links state machine → limiter
│   └── window/window.go           # HLL++ window rotation
├── server/
│   ├── server.pb.go               # Generated protobuf messages
│   └── server_grpc.pb.go          # Generated gRPC service stubs
├── types/hll/                     # HLL++ implementation
├── k8s/
│   ├── daemonset-agent.yaml       # Agent DaemonSet (production)
│   ├── deployment-aggregator.yaml # Aggregator Deployment + Service
│   ├── iot-simulation/            # IoT demo manifests
│   │   ├── deployment-iot-agent.yaml
│   │   └── job-attacker.yaml
│   └── monitoring/                # Prometheus + Grafana K8s manifests
│       ├── prometheus.yaml
│       └── grafana.yaml
├── monitoring/
│   ├── prometheus/prometheus.yml  # Docker Compose Prometheus config
│   └── grafana/
│       ├── provisioning/
│       │   ├── datasources/prometheus.yaml
│       │   └── dashboards/default.yaml
│       └── dashboards/ddos_dashboard.json
├── demo/
│   ├── kind-setup.sh              # Create kind cluster + load images
│   ├── run_demo.sh                # Deploy full K8s demo
│   └── run_docker_demo.sh         # Deploy via docker-compose
├── Dockerfile.agent               # Agent image (with libpcap)
├── Dockerfile.aggregator          # Aggregator image (scratch)
├── Dockerfile.iot-sim             # IoT simulator image
├── docker-compose.yml             # Full demo stack (9 services)
├── Makefile                       # Build, test, demo targets
└── server.proto                   # gRPC service definition
```

---

## Building

### Prerequisites
- Go 1.24+
- libpcap-dev (for agent with live capture; not needed for sim-mode)
- protoc + protoc-gen-go + protoc-gen-go-grpc (only if modifying proto)
- Docker + Docker Compose (for demo)
- kind (for K8s demo)

### Build All Binaries

```bash
make build
```

This produces:
- `bin/agent` — detection agent
- `bin/aggregator` — central aggregator
- `bin/iot-sim` — traffic simulator

### Regenerate Protobuf (if server.proto changes)

```bash
make proto
```

---

## Running Tests

### All Tests

```bash
make test
```

### With Coverage Report

```bash
make test-cover
```

### Test Packages Individually

```bash
# Detector tests (LODA, HST, ensemble, state machine, Z-score, EWMA)
go test -v ./ddos/detector/...

# Mitigation tests (rate limiter, controller)
go test -v ./ddos/mitigation/...
```

### Test Summary

| Test Suite | Tests | Coverage Area |
|------------|-------|---------------|
| `detector_extended_test.go` | 24 | LODA, HST, ensemble, state machine, feature extraction, sigmoid |
| `ewma_test.go` | 6 | EWMA warmup, spike detection, adaptation |
| `zscore_test.go` | 6 | Z-score baseline, spike, history |
| `mitigation_test.go` | 11 | Rate limiter, CMS, controller lifecycle |

---

## Docker Compose Demo

The fastest way to see the system in action. Runs 9 services locally without K8s.

### Start

```bash
make docker-demo
# or
docker-compose up -d --build
```

### Services

| Service | Port | Description |
|---------|------|-------------|
| `aggregator` | 50051 (gRPC), 9091 (metrics) | Central aggregator |
| `iot-node-1` | 9001 (metrics) | Agent — normal traffic |
| `iot-node-2` | 9002 (metrics) | Agent — attack traffic |
| `iot-node-3` | 9003 (metrics) | Agent — mixed (attack at t=30s) |
| `iot-sim-1` | — | Simulator → node-1 (normal) |
| `iot-sim-2` | — | Simulator → node-2 (attack) |
| `iot-sim-3` | — | Simulator → node-3 (mixed) |
| `prometheus` | 9090 | Metrics scraping |
| `grafana` | 3000 | Dashboards (admin/admin) |

### Access

- **Grafana**: http://localhost:3000 (login: admin / admin)
  - Pre-configured dashboard: "DDoS Detection - IoT Network"
- **Prometheus**: http://localhost:9090
- **Agent metrics**: http://localhost:9001/metrics, :9002, :9003
- **Aggregator metrics**: http://localhost:9091/metrics

### Observe the Attack

1. Open Grafana dashboard
2. Watch the **Anomaly Scores** panel — node-2 (attack) will show elevated LODA/HST/Ensemble scores within ~30s
3. Node-3 starts normal, then attacks at t=30s — watch the spike
4. **Anomaly State Timeline** shows NORMAL (green) → UNDER_ATTACK (red) → RECOVERY (yellow)
5. **Global Defense Status** turns ACTIVE when ≥2 of 3 nodes are under attack
6. **Packet Drop Rate** shows the rate limiter dropping traffic

### Stop

```bash
docker-compose down -v
```

---

## Kubernetes (kind) Demo

### Setup

```bash
# Create kind cluster, build images, and load them
make kind-setup

# Deploy the full stack
make kind-demo
```

### What Gets Deployed

1. **ddos-aggregator**: Deployment + Service (ClusterIP, ports 50051 + 9091)
2. **iot-agent**: Deployment (3 replicas) running agents in sim-mode
3. **iot-attacker**: Job running the traffic simulator with mixed profile
4. **prometheus**: Deployment + Service (NodePort 30090)
5. **grafana**: Deployment + Service (NodePort 30030)

### Access

```bash
# Grafana
open http://localhost:30030   # admin/admin

# Prometheus
open http://localhost:30090

# Logs
kubectl logs -f deployment/ddos-aggregator
kubectl logs -f deployment/iot-agent
kubectl logs -f job/iot-attacker
```

### Cleanup

```bash
make kind-clean
```

---

## Configuration Reference

### Agent Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-iface` | `eth0` | Network interface for pcap capture |
| `-window` | `10s` | Window duration |
| `-threshold` | `5000` | Unique IP threshold (for threshold detector) |
| `-detector` | `ensemble` | Detector: threshold, zscore, ewma, ensemble |
| `-ensemble-threshold` | `0.6` | Ensemble score threshold for attack [0-1] |
| `-metrics` | `:9090` | Prometheus metrics address |
| `-aggregator` | ` ` | Aggregator gRPC address |
| `-ship-interval` | `10s` | How often to ship sketches |
| `-node-id` | hostname | Unique node identifier |
| `-sim-mode` | `false` | Enable simulation mode (no pcap) |
| `-sim-grpc` | `:50052` | gRPC listen address for InjectIP |
| `-global-rps` | `1000` | Rate limiter: global requests/sec |
| `-per-ip-limit` | `50` | Rate limiter: per-IP requests per decay window |

### Aggregator Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-grpc` | `:50051` | gRPC listen address |
| `-metrics` | `:9091` | Prometheus metrics address |
| `-detector` | `zscore` | Detector for local checks |
| `-window` | `10s` | Correlation loop period |

### IoT Simulator Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-target` | `localhost:50052` | Agent gRPC address |
| `-profile` | `normal` | Traffic profile: normal, attack, mixed |
| `-duration` | `60s` | Simulation duration |
| `-attack-at` | `30s` | When attack starts (mixed profile) |

---

## Memory Budget

Per-node memory consumption of detection structures:

| Component | Memory | Notes |
|-----------|--------|-------|
| HLL++ sketch (current) | 12.3 KB | p=14, m=16384 uint8 registers |
| HLL++ sketch (previous) | 12.3 KB | Kept for ratio calculation |
| LODA detector | 2.6 KB | 40 proj × (16 uint16 + 8 float32) |
| HST detector | 1.4 KB | 5 trees × 63 nodes × 12B |
| Z-Score history | 0.16 KB | 20 × float64 |
| EWMA state | ~0.05 KB | Single float64 + params |
| Rate limiter CMS | 4.0 KB | 4 × 512 × uint16 |
| Token bucket | ~0.05 KB | Float64s |
| TrafficStats | ~0.02 KB | Two atomic.Uint64 |
| State machine | ~0.05 KB | State + counters |
| **Total** | **~33 KB** | Well within 36 KB target |
