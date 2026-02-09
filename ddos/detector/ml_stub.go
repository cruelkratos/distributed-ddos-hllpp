package detector

// MLAnomalyDetector is a stub for a future ML-based anomaly detector.
// Replace this implementation with a real model (ONNX, TensorFlow Lite, or external API)
// without changing the pipeline.
type MLAnomalyDetector struct{}

// NewMLAnomalyDetector returns a stub detector that never signals attack.
// Use this to wire the pipeline; replace with real ML logic later.
func NewMLAnomalyDetector() *MLAnomalyDetector {
	return &MLAnomalyDetector{}
}

// IsAttack always returns false in the stub. Future: load model, compute features, return score.
func (m *MLAnomalyDetector) IsAttack(f WindowFeatures) bool {
	_ = f // use f when implementing real ML
	return false
}

// Name returns the detector name for metrics and AttackEvent.Reason.
func (m *MLAnomalyDetector) Name() string {
	return "ml_anomaly"
}
