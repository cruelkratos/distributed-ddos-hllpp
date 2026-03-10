# **BTP Development Checklist**

## **HLL++ Core Implementation**
- [x] Define struct for **dense mode registers** (6-bit values in byte array)
- [x] Define **sparse mode storage** (map or sorted list of bucket → value pairs)
- [x] Implement **hash → (bucket, rho)** extraction logic
- [x] Implement **bias correction**
- [x] Add **sparse ↔ dense switching** based on threshold
- [x] Implement core methods:
  - [x] `Insert()`
  - [x] `Estimate()`
  - [x] `Merge()` (for distributed mode)

---

## **Thread Safety**
- [x] Wrap updates with `sync.Mutex` or `sync.RWMutex`
- [x] Concurrency-safe window rotation with fine-grained per-bucket locks
- [x] Fixed reentrant-lock and lock-ordering bugs in `MergeSets` and `hllSet.Merge`

---

## **Multi-Threaded Server / API**
- [x] gRPC server (`cmd/aggregator`) with `MergeSketch`, `GetEstimate`, `GetSketch`, `Reset`, `Health`
- [x] Handle **concurrent requests** safely (global RWMutex on aggregated HLL set)

---

## **Distributed Agent Pipeline**
- [x] `cmd/agent` — pcap capture → `WindowManager` → sketch export → gRPC `MergeSketch`
- [x] Configurable aggregator address, ship interval, window duration, detector type
- [x] Standalone mode (no aggregator) still runs local detection

---

## **Detection**
- [x] `ThresholdDetector` — fixed count threshold
- [x] `ZScoreDetector` — rolling Z-score anomaly detection with warmup
- [x] `EWMADetector` — exponentially weighted moving average baseline
- [x] `MLAnomalyDetector` (backward-compat wrapper → delegates to ZScoreDetector)
- [x] Pluggable `Detector` interface wired into `WindowManager` and aggregator

---

## **Evaluation Harness**
- [x] `ddos/eval` — synthetic traffic scenario with configurable attack windows
- [x] `cmd/eval-detection` — pluggable `--detector` flag (threshold, zscore, ewma, compare)
- [x] CSV export of results

---

## **Fault Tolerance / Pcap**
- [x] pcap timeout treated as non-fatal (capture continues on timeout)
- [ ] Periodic snapshot of registers to disk (optional, not yet implemented)
- [ ] On startup, restore last saved state (optional)

---

## **Dockerization**
- [x] `Dockerfile.agent` — multi-stage build, alpine runtime with libpcap
- [x] `Dockerfile.aggregator` — multi-stage CGO-free build, scratch runtime

---

## **Kubernetes Deployment**
- [x] `k8s/daemonset-agent.yaml` — DaemonSet with aggregator env, detector flag
- [x] `k8s/deployment-aggregator.yaml` — Deployment with gRPC/metrics ports
- [ ] Service YAML for aggregator load-balancing (optional enhancement)

---

## **CI / Quality**
- [x] `.github/workflows/ci.yml` — race-detector tests, build artifacts, eval comparison, docker builds
- [x] `go test -race ./...` run locally — no races detected
- [x] `go vet ./...` — clean

---

## **Tools**
- [x] `tools/push_sketch` — synthetic sketch pusher for integration testing
