package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// UniqueIPsGauge is the current window distinct IP count (HLL estimate).
	UniqueIPsGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ddos_unique_ips_current_window",
		Help: "Current window distinct IP count (HLL estimate).",
	})
	// AttackStatusGauge is 1 if attack detected, 0 otherwise.
	AttackStatusGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ddos_attack_status",
		Help: "1 if attack detected (e.g. current count > threshold), 0 otherwise.",
	})
	// MemoryUsageGauge is approximate HLL sketch memory in bytes.
	MemoryUsageGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ddos_memory_usage_bytes",
		Help: "Approximate HLL sketch memory usage in bytes.",
	})
)

func init() {
	prometheus.MustRegister(UniqueIPsGauge, AttackStatusGauge, MemoryUsageGauge)
}

// UpdateWindowMetrics updates the Prometheus gauges from current/previous count,
// attack status, and memory bytes. Call from the agent's check loop.
func UpdateWindowMetrics(uniqueIPs uint64, attack bool, memoryBytes uint64) {
	UniqueIPsGauge.Set(float64(uniqueIPs))
	if attack {
		AttackStatusGauge.Set(1)
	} else {
		AttackStatusGauge.Set(0)
	}
	MemoryUsageGauge.Set(float64(memoryBytes))
}

// Handler returns the HTTP handler for /metrics.
func Handler() http.Handler {
	return promhttp.Handler()
}

// ListenAndServe starts the metrics HTTP server on addr (e.g. ":9090").
// It blocks until the server fails; run in a goroutine.
func ListenAndServe(addr string) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", Handler())
	return http.ListenAndServe(addr, mux)
}
