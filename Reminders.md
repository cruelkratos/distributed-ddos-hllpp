# **BTP Development Checklist**

## **HLL++ Core Implementation**
- [ ] Define struct for **dense mode registers** (6-bit values in byte array)
- [ ] Define **sparse mode storage** (map or sorted list of bucket → value pairs)
- [ ] Implement **hash → (bucket, rho)** extraction logic
- [ ] Implement **bias correction**
- [ ] Add **sparse ↔ dense switching** based on threshold
- [ ] Implement core methods:
  - [ ] `Insert()`
  - [ ] `Estimate()`
  - [ ] `Merge()` (for distributed mode)

---

## **Thread Safety**
- [ ] Wrap updates with `sync.Mutex` or `sync.RWMutex`
- [ ] OR use worker goroutine with channels for serialized writes
- [ ] Add **concurrency tests** to ensure no race conditions

---

## **Multi-Threaded Server**
- [ ] Choose API type: **REST** or **gRPC**
- [ ] Implement endpoints:
  - [ ] `/insert`
  - [ ] `/estimate`
  - [ ] `/merge`
- [ ] Handle **concurrent requests** safely

---

## **Fault Tolerance**
- [ ] Implement **periodic snapshot** of registers to disk
- [ ] On startup, **restore** last saved state
- [ ] (Distributed) Implement **peer sync** for recovery

---

## **Dockerization**
- [ ] Create **Dockerfile** for Go API service
- [ ] Optional: Create `docker-compose.yml` for:
  - Go HLL++ server
  - Python client/analytics
  - Optional Redis/NATS for distributed comms

---

## **Kubernetes Deployment**
- [ ] Create **Deployment YAML** for multiple HLL nodes
- [ ] Add **Service** for load balancing between nodes
- [ ] Implement **aggregator node** to merge sketches
- [ ] Optional: Add **Prometheus** metrics scraping
