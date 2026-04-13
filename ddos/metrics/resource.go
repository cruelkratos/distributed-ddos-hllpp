package metrics

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Resource metrics for benchmarking — RSS, heap, stack, CPU, goroutines, network.
var (
	MemoryRSSGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ddos_memory_rss_bytes",
		Help: "Resident set size from /proc/self/status VmRSS.",
	})
	MemoryHeapGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ddos_memory_heap_bytes",
		Help: "Go heap allocation (runtime.ReadMemStats HeapAlloc).",
	})
	MemoryStackGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ddos_memory_stack_bytes",
		Help: "Go stack in-use (runtime.ReadMemStats StackInuse).",
	})
	CPUPercentGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ddos_cpu_percent",
		Help: "Approximate CPU usage percent from /proc/self/stat.",
	})
	GoroutinesGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ddos_goroutines",
		Help: "Number of active goroutines.",
	})
	GRPCBytesSentCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ddos_grpc_bytes_sent_total",
		Help: "Total bytes sent via gRPC sketch shipping.",
	})
	GRPCBytesReceivedCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ddos_grpc_bytes_received_total",
		Help: "Total bytes received via gRPC.",
	})
	MergeLatencyHistogram = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "ddos_merge_latency_seconds",
		Help:    "Latency of MergeSketch operations.",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 12), // 1ms to 2s
	})
	DetectionLatencyHistogram = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "ddos_detection_latency_seconds",
		Help:    "Time from window close to detection decision.",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 12),
	})

	// ESP32 metrics (set by aggregator when parsing HTTP merge payloads).
	ESP32FreeHeapGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ddos_esp32_free_heap_bytes",
		Help: "ESP32 free heap bytes reported by node.",
	}, []string{"node"})
	ESP32ShipLatencyGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ddos_esp32_ship_latency_ms",
		Help: "ESP32 HTTP POST latency in milliseconds.",
	}, []string{"node"})
	ESP32LoopTimeGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ddos_esp32_loop_time_us",
		Help: "ESP32 main loop execution time in microseconds.",
	}, []string{"node"})
)

func init() {
	prometheus.MustRegister(
		MemoryRSSGauge, MemoryHeapGauge, MemoryStackGauge,
		CPUPercentGauge, GoroutinesGauge,
		GRPCBytesSentCounter, GRPCBytesReceivedCounter,
		MergeLatencyHistogram, DetectionLatencyHistogram,
		ESP32FreeHeapGauge, ESP32ShipLatencyGauge, ESP32LoopTimeGauge,
	)
}

// ResourceCollector periodically samples process resource usage.
type ResourceCollector struct {
	interval time.Duration
	stop     chan struct{}
	wg       sync.WaitGroup

	// CPU tracking state.
	lastCPUTime  uint64
	lastWallTime time.Time
}

// NewResourceCollector creates a collector that samples every interval.
func NewResourceCollector(interval time.Duration) *ResourceCollector {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &ResourceCollector{
		interval: interval,
		stop:     make(chan struct{}),
	}
}

// Start begins periodic resource collection in a background goroutine.
func (rc *ResourceCollector) Start() {
	rc.wg.Add(1)
	go rc.loop()
}

// Stop terminates the collection loop.
func (rc *ResourceCollector) Stop() {
	close(rc.stop)
	rc.wg.Wait()
}

func (rc *ResourceCollector) loop() {
	defer rc.wg.Done()
	rc.lastWallTime = time.Now()
	rc.lastCPUTime = readProcCPUTicks()
	ticker := time.NewTicker(rc.interval)
	defer ticker.Stop()
	for {
		select {
		case <-rc.stop:
			return
		case <-ticker.C:
			rc.collect()
		}
	}
}

func (rc *ResourceCollector) collect() {
	// Go runtime stats.
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	MemoryHeapGauge.Set(float64(ms.HeapAlloc))
	MemoryStackGauge.Set(float64(ms.StackInuse))
	GoroutinesGauge.Set(float64(runtime.NumGoroutine()))

	// RSS from /proc/self/status.
	rss := readVmRSS()
	if rss > 0 {
		MemoryRSSGauge.Set(float64(rss))
	}

	// CPU usage approximation.
	now := time.Now()
	cpuTicks := readProcCPUTicks()
	elapsed := now.Sub(rc.lastWallTime).Seconds()
	if elapsed > 0 && cpuTicks > rc.lastCPUTime {
		ticksPerSec := float64(100) // Linux default CLK_TCK
		cpuSec := float64(cpuTicks-rc.lastCPUTime) / ticksPerSec
		CPUPercentGauge.Set((cpuSec / elapsed) * 100.0)
	}
	rc.lastCPUTime = cpuTicks
	rc.lastWallTime = now
}

// readVmRSS reads VmRSS from /proc/self/status in bytes.
func readVmRSS() uint64 {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "VmRSS:") {
			var kb uint64
			fmt.Sscanf(strings.TrimPrefix(line, "VmRSS:"), "%d", &kb)
			return kb * 1024
		}
	}
	return 0
}

// readProcCPUTicks reads user+system CPU ticks from /proc/self/stat.
func readProcCPUTicks() uint64 {
	data, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return 0
	}
	// Fields: pid (comm) state ppid ... utime stime (fields 14 and 15, 1-indexed)
	// Find the closing ')' of comm field.
	s := string(data)
	idx := strings.LastIndex(s, ")")
	if idx < 0 {
		return 0
	}
	fields := strings.Fields(s[idx+2:])
	if len(fields) < 13 {
		return 0
	}
	var utime, stime uint64
	fmt.Sscanf(fields[11], "%d", &utime)
	fmt.Sscanf(fields[12], "%d", &stime)
	return utime + stime
}

// ResourceSnapshot captures a point-in-time resource measurement for CSV export.
type ResourceSnapshot struct {
	Timestamp   time.Time
	RSSBytes    uint64
	HeapBytes   uint64
	StackBytes  uint64
	CPUPercent  float64
	Goroutines  int
	GRPCBytesTx uint64
	GRPCBytesRx uint64
}

// TakeResourceSnapshot returns the current resource state.
func TakeResourceSnapshot() ResourceSnapshot {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ResourceSnapshot{
		Timestamp:  time.Now(),
		RSSBytes:   readVmRSS(),
		HeapBytes:  ms.HeapAlloc,
		StackBytes: ms.StackInuse,
		Goroutines: runtime.NumGoroutine(),
	}
}
