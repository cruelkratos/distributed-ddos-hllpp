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

	// --- Extended telemetry gauges ---

	PacketCountGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ddos_packet_count",
		Help: "Total packets in the current window.",
	})
	ByteVolumeGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ddos_byte_volume",
		Help: "Total bytes in the current window.",
	})
	LodaScoreGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ddos_loda_score",
		Help: "LODA anomaly score (higher = more anomalous).",
	})
	HSTScoreGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ddos_hst_score",
		Help: "Half-Space Trees anomaly score (higher = more anomalous).",
	})
	EnsembleScoreGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ddos_ensemble_score",
		Help: "Weighted ensemble anomaly score in [0,1].",
	})
	AnomalyStateGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ddos_anomaly_state",
		Help: "Current anomaly state: 0=NORMAL, 1=UNDER_ATTACK, 2=RECOVERY.",
	})
	DropsGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ddos_drops_total",
		Help: "Cumulative number of packets dropped by rate limiter.",
	})
	NSGLockdownGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ddos_nsg_lockdown_active",
		Help: "1 if Azure NSG lockdown is active, 0 otherwise.",
	})
)

func init() {
	prometheus.MustRegister(
		UniqueIPsGauge, AttackStatusGauge, MemoryUsageGauge,
		PacketCountGauge, ByteVolumeGauge,
		LodaScoreGauge, HSTScoreGauge, EnsembleScoreGauge,
		AnomalyStateGauge, DropsGauge,
		NSGLockdownGauge,
	)
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

// UpdateExtendedMetrics updates the ML-telemetry Prometheus gauges.
func UpdateExtendedMetrics(packets, bytes uint64, lodaScore, hstScore, ensembleScore float64, state int, drops uint64) {
	PacketCountGauge.Set(float64(packets))
	ByteVolumeGauge.Set(float64(bytes))
	LodaScoreGauge.Set(lodaScore)
	HSTScoreGauge.Set(hstScore)
	EnsembleScoreGauge.Set(ensembleScore)
	AnomalyStateGauge.Set(float64(state))
	DropsGauge.Set(float64(drops))
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
