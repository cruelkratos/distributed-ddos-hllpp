package detector

// MLAnomalyDetector is deprecated. Use ZScoreDetector (zscore.go) instead.
// Kept for API compatibility; delegates to ZScoreDetector internally.
type MLAnomalyDetector struct {
	inner *ZScoreDetector
}

// NewMLAnomalyDetector returns a detector backed by ZScoreDetector with default settings.
func NewMLAnomalyDetector() *MLAnomalyDetector {
	return &MLAnomalyDetector{inner: NewZScoreDetector(20, 3.0)}
}

// IsAttack delegates to the underlying ZScoreDetector.
func (m *MLAnomalyDetector) IsAttack(f WindowFeatures) bool {
	return m.inner.IsAttack(f)
}

// Name returns the detector name for metrics and AttackEvent.Reason.
func (m *MLAnomalyDetector) Name() string {
	return "ml_anomaly"
}
