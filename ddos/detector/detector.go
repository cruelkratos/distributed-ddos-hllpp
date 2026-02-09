package detector

// WindowFeatures holds inputs for detection (threshold or ML).
type WindowFeatures struct {
	CurrentWindowCount  uint64
	PreviousWindowCount uint64
	WindowDurationSec   float64
	// Optional: add more later for ML (e.g. packet rate, entropy, historical baseline).
}

// Detector decides if the current window indicates a DDoS attack.
// Implementations: ThresholdDetector (Phase 2), MLAnomalyDetector (future).
type Detector interface {
	IsAttack(f WindowFeatures) bool
	Name() string // e.g. "threshold", "ml_anomaly" — for AttackEvent.Reason and metrics
}
