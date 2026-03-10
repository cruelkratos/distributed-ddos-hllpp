package detector

// ThresholdDetector implements Detector with a simple count > threshold rule.
type ThresholdDetector struct {
	Threshold uint64
}

// NewThresholdDetector returns a detector that signals attack when
// CurrentWindowCount exceeds the given threshold.
func NewThresholdDetector(threshold uint64) *ThresholdDetector {
	return &ThresholdDetector{Threshold: threshold}
}

// IsAttack returns true when current window distinct IP count exceeds threshold.
func (t *ThresholdDetector) IsAttack(f WindowFeatures) bool {
	return f.CurrentWindowCount > t.Threshold
}

// Name returns the detector name for metrics and AttackEvent.Reason.
func (t *ThresholdDetector) Name() string {
	return "threshold"
}
